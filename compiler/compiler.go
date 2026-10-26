package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/h3check"
	"github.com/ly95/h3-context-ir-go/lora"
	"github.com/ly95/h3-context-ir-go/planner"
)

func New(model Model, perceiver Perceiver, options Options) (*Compiler, error) {
	if options.Registry != nil {
		wire, err := json.Marshal(options.Registry)
		if err != nil {
			return nil, invalid("options.registry", "Registry must have serializable finite metadata.")
		}
		var registry lora.Registry
		if err = json.Unmarshal(wire, &registry); err != nil {
			return nil, invalid("options.registry", "Registry metadata cannot be snapshotted.")
		}
		registry.OpenFile = options.Registry.OpenFile
		options.Registry = &registry
	}
	if reflect.DeepEqual(options.Profile, h3.Profile{}) {
		options.Profile = h3.OfficialProfile()
	} else if options.Profile.Name == "" {
		return nil, invalid("options.profile", "A custom profile must have an explicit identity.")
	}
	if options.Profile.Format != h3.FormatVersion || len(options.Profile.Modes) == 0 || options.Profile.FPS.Denominator > math.MaxInt64/24 || options.Profile.FPS.Numerator != 24*options.Profile.FPS.Denominator || options.Profile.FPS.Denominator <= 0 {
		return nil, invalid("options.profile", "Choose a supported H3 format, mode set, and 24 fps profile.")
	}
	if options.Profile.MinFrames < 0 || options.Profile.MaxFrames < 0 || options.Profile.CanvasMultiple < 0 || (options.Profile.MaxFrames > 0 && options.Profile.MinFrames > options.Profile.MaxFrames) {
		return nil, invalid("options.profile", "Invalid profile runtime limits.")
	}
	for _, mode := range options.Profile.Modes {
		switch mode {
		case contextir.ModeText, contextir.ModeFirstFrame, contextir.ModeLastFrame, contextir.ModeFirstLastFrame, contextir.ModeReference:
		default:
			return nil, invalid("options.profile.modes", "Profile declares an unsupported H3 mode.")
		}
	}
	if limits := options.Profile.Limits; limits.Pictures < 0 || limits.Videos < 0 || limits.VideoAudio < 0 || limits.StandaloneAudio < 0 {
		return nil, invalid("options.profile.limits", "Runtime attachment limits must be nonnegative.")
	}
	options.Profile.Modes = append([]contextir.Mode(nil), options.Profile.Modes...)
	if options.Profile.Grid != nil {
		grid := *options.Profile.Grid
		options.Profile.Grid = &grid
	}

	if options.ScopeConflictPolicy == "" {
		options.ScopeConflictPolicy = ScopeConflictReject
	}
	if options.ScopeConflictPolicy != ScopeConflictReject && options.ScopeConflictPolicy != ScopeConflictSuppliedWins {
		return nil, invalid("options.scope_conflict_policy", "Choose reject or supplied_content_wins explicitly.")
	}
	if options.ClarificationPolicy == "" {
		options.ClarificationPolicy = ClarificationRequire
	}
	if options.ClarificationPolicy != ClarificationRequire && options.ClarificationPolicy != ClarificationContinue {
		return nil, invalid("options.clarification_policy", "Choose require or continue_with_recorded_choice explicitly.")
	}
	if options.FramePolicy == "" {
		options.FramePolicy = FramePolicyExact
	}
	if options.FramePolicy != FramePolicyExact && options.FramePolicy != FramePolicySnapUp {
		return nil, invalid("options.frame_policy", "Choose exact or snap_up.")
	}
	if options.FailurePolicy == "" {
		options.FailurePolicy = FailurePolicyFail
	}
	if options.FailurePolicy != FailurePolicyFail && options.FailurePolicy != FailurePolicyDraft {
		return nil, invalid("options.failure_policy", "Choose fail or deterministic_draft.")
	}
	if options.MaxAutoShots == 0 {
		options.MaxAutoShots = 4
	}
	if options.MaxAutoShots < 1 || options.MaxAutoShots > MaxPinnedShots {
		return nil, invalid("options.max_auto_shots", "Maximum automatic shots must be between 1 and 10.")
	}
	if options.MaxStructureRetries < 0 || options.MaxStructureRetries > 3 {
		return nil, invalid("options.max_structure_retries", "Structural retries must be between 0 and 3.")
	}
	if options.Planner.MaxRetries < 0 || options.Planner.MaxRetries > 3 {
		return nil, invalid("options.planner.max_retries", "Planner retries must be between 0 and 3.")
	}
	if protocol := options.Planner.RepairProtocol; protocol != "" && protocol != planner.RepairValidatedDocument && protocol != planner.RepairDiagnosticsOnly {
		return nil, invalid("options.planner.repair_protocol", "Choose validated_document/v1 or diagnostics_only/v1 for model repair requests.")
	}
	if g := options.Profile.Grid; g != nil && (g.Modulus <= 0 || g.Remainder < 0 || g.Remainder >= g.Modulus) {
		return nil, invalid("options.profile.frame_grid", "Invalid profile frame grid.")
	}
	return &Compiler{model: model, perceiver: perceiver, options: options}, nil
}

func (c *Compiler) Compile(ctx context.Context, brief Brief) (result Result, err error) {
	defer func() { completeAccounting(&result) }()
	if c.needsExecution(brief) {
		next, err := c.executionFor(ctx, brief)
		if err != nil {
			return Result{Version: Version, Status: "failed", Brief: brief}, err
		}
		return next.Compile(ctx, brief)
	}
	r, err := c.compile(ctx, brief)
	if err == nil || c == nil || c.options.FailurePolicy != FailurePolicyDraft || ctx.Err() != nil {
		return r, err
	}
	if !contextir.HasCode(err, contextir.CodeModel) && !contextir.HasCode(err, contextir.CodeModelOutput) {
		return r, err
	}
	drafted, draftErr := c.draft(ctx, r.Brief, &r.Structure)
	if draftErr != nil {
		return r, err
	}
	drafted.Calls = append(r.Calls, drafted.Calls...)
	drafted.CompositionReviews = append(r.CompositionReviews, drafted.CompositionReviews...)
	drafted.ObservationCards = append(r.ObservationCards, drafted.ObservationCards...)
	drafted.UnmeteredAssetIDs = append(r.UnmeteredAssetIDs, drafted.UnmeteredAssetIDs...)
	drafted.UsageComplete = drafted.UsageComplete && r.UsageComplete
	drafted.ModelError = err
	var typed *contextir.Error
	if errors.As(err, &typed) {
		drafted.Diagnostics = append(drafted.Diagnostics, typed.Issues...)
	}
	drafted.Decisions = append(drafted.Decisions, decision("status", "explicit_failure_policy", "draft", "The caller selected deterministic drafting after a model failure; this is not a model-validated compile."))
	return drafted, nil
}

func (c *Compiler) prepare(ctx context.Context, brief Brief, requireEvidence bool) (Result, error) {
	r := Result{Version: Version, Status: "failed", Brief: brief, UsageComplete: true}
	if c == nil {
		return r, invalid("compiler", "Initialize the compiler with New.")
	}
	if err := ctx.Err(); err != nil {
		return r, failure(contextir.CodeCanceled, "compile", "context", "The compile operation was canceled.", err)
	}
	measured, inspectionDecisions, err := c.inspectBriefAssets(ctx, brief)
	if err != nil {
		r.Brief = measured
		r.Decisions = inspectionDecisions
		return r, err
	}
	intake, err := c.ValidateIntake(ctx, measured)
	r.Brief = intake.Brief
	r.Decisions = append(inspectionDecisions, intake.Decisions...)
	if err != nil {
		return r, err
	}
	b, target, director := intake.Brief, intake.Target, intake.Director
	r.Director = director
	r.Decisions = append(r.Decisions, executionDecision(b))
	var evidence []contextir.Evidence
	if requireEvidence || !isNil(c.perceiver) || len(b.Evidence) > 0 || len(b.Cards) > 0 {
		observed, perceptionErr := c.collectEvidence(ctx, b)
		err = perceptionErr
		evidence = observed.Evidence
		r.Evidence = evidence
		r.Calls = append(r.Calls, observed.Calls...)
		r.ObservationCards = observed.ObservationCards
		for _, extraction := range observed.ObservationCards {
			if extraction.Card.AssetID != "" {
				r.Decisions = append(r.Decisions, decision("assets."+extraction.AssetID+".observed_card", "typed_existing_facts", extraction.Sources, "This card structures existing observations with exact fact provenance; no new media inspection or caller description was substituted."))
			}
		}
		r.UnmeteredAssetIDs = append(r.UnmeteredAssetIDs, observed.UnmeteredAssetIDs...)
		if len(observed.UnmeteredAssetIDs) > 0 {
			r.UsageComplete = false
		}
		if err != nil {
			return r, err
		}
		b = mergeObservedCards(b, evidence)
		b.Evidence = evidence
		r.Brief = b
	} else if len(b.Assets) > 0 {
		r.Decisions = append(r.Decisions, decision("perception", "readonly_interpretation", "unobserved", "Propose interprets caller intent and attachment metadata without claiming media observation. Compile still requires perception before publication."))
	}

	if _, transcriptErr := transcriptSources(b); transcriptErr != nil {
		return r, transcriptErr
	}
	if err := validateExplicitReplacementInputs(b); err != nil {
		return r, err
	}
	if isNil(c.model) {
		return r, failure(contextir.CodeModel, "setup", "model", "Compile requires a model adapter; Draft is a separate explicit operation.", nil)
	}
	modeDecision, modeErr := c.DecideMode(ctx, b)
	r.Calls = append(r.Calls, modeDecision.Calls...)
	if modeErr != nil {
		return r, modeErr
	}
	modeDecision.ClarificationPolicy = c.options.ClarificationPolicy
	r.ClarificationPolicy = c.options.ClarificationPolicy
	r.ModeDecision = &modeDecision
	r.Findings = append(r.Findings, modeInterpretationFindings(modeDecision)...)
	if modeDecision.NeedsClarification && len(modeDecision.Questions) > 0 && c.options.ClarificationPolicy == ClarificationContinue {
		r.Decisions = append(r.Decisions, decision("clarification_policy", "explicit_continue_policy", modeDecision, "The caller explicitly permits publication using the recorded mode despite the unresolved geometry/question; no mode or aspect was changed."))
		r.Findings = append(r.Findings, h3check.Finding{Rule: "compiler-clarification-continued", Severity: "WARN", Msg: "The caller explicitly selected continue_with_recorded_choice; unresolved frame/reference questions and any aspect mismatch remain in ModeDecision."})
	}
	if modeDecision.NeedsClarification && len(modeDecision.Questions) > 0 && c.options.ClarificationPolicy == ClarificationRequire {
		r.Status = "needs_input"
		r.Decisions = append(r.Decisions, decision("mode", modeDecision.Rule, modeDecision, "The caller must choose the frame/reference mechanism before interpretation or publication continues."))
		return r, nil
	}
	mode := modeDecision.Mode
	r.Decisions = append(r.Decisions, decision("mode", modeDecision.Rule, modeDecision, "Mode follows the production typed routing contract; publication capabilities are checked separately."))
	roleDecision, roleErr := c.DecideRoles(ctx, b, mode)
	r.Calls = append(r.Calls, roleDecision.Calls...)
	r.Decisions = append(r.Decisions, roleDecision.Decisions...)
	if roleErr != nil {
		return r, roleErr
	}
	b.Assets = roleDecision.Assets
	r.Brief = b
	if frames := actualFramePreservations(b, target); len(frames) > 0 {
		r.Decisions = append(r.Decisions, decision("frame_preservation", "actual_frame_responsibility", frames, "Each actual frame anchor preserves the entire supplied image at its exact target time; reference-sheet cleanup does not apply to that frame."))
	}
	replacementDecision, replacementErr := c.ResolveReplacements(ctx, b, mode)
	r.Calls = append(r.Calls, replacementDecision.Calls...)
	if replacementErr != nil {
		return r, replacementErr
	}
	if len(replacementDecision.Targets) > 0 {
		r.Decisions = append(r.Decisions, decision("replacement_targets", "typed_identity_resolution", replacementDecision, "Named figures unsupported by observed identity evidence remain user-symbolic; source look and leftover identity are never guessed."))
	}
	contentScope, scopeErr := c.DecideScope(ctx, b)
	r.Calls = append(r.Calls, contentScope.Calls...)
	r.ScopeDecision = &contentScope
	if scopeErr != nil {
		return r, scopeErr
	}
	r.Decisions = append(r.Decisions, decision("content_scope", "typed_request_interpretation", contentScope, "Caller requests and prohibitions are interpreted before code applies the explicit creativity level."))
	licence, licenceErr := c.DecideLicence(ctx, b)
	r.Calls = append(r.Calls, licence.Calls...)
	if licenceErr != nil {
		return r, licenceErr
	}
	r.Licence = &licence
	r.Decisions = append(r.Decisions, decision("reference_licence", "typed_attribute_precedence", licence, "Each attribute follows explicit request or observed reference; poses and reference-sheet presentation are not subject identity."))
	st, calls, ds, err := c.structure(ctx, b, target, evidence, director, mode, replacementDecision.Targets, licence, contentScope)
	r.Calls = append(r.Calls, calls...)
	r.Decisions = append(r.Decisions, ds...)
	if err != nil {
		return r, err
	}
	r.Structure = st
	r.ReplacementMotionObligations = st.ReplacementMotionObligations
	scope := scopeFor(b, st)
	r.Scope = scope
	request, err := requestFor(b, target, st, scope)
	r.Request = request
	if err != nil {
		return r, err
	}
	preflightFindings, preflightErr := c.preflight(b, st, request)
	r.Findings = append(r.Findings, preflightFindings...)
	if preflightErr != nil {
		return r, preflightErr
	}
	choices, loraFindings, loraErr := c.resolveLoRAs(b, st.Mode, target)
	r.Findings = append(r.Findings, loraFindings...)
	if loraErr != nil {
		return r, loraErr
	}
	request.LoRAs = choices
	r.Request = request
	slots, slotErr := fixedDialogueSlots(request, st)
	if slotErr != nil {
		return r, slotErr
	}
	if len(slots) > 0 {
		r.Decisions = append(r.Decisions, decision("dialogue", "host_literal_slots", slots, "The structure stage fixes placement; the host restores exact dialogue after prose. The writer does not author the supplied words."))
	}
	r.Decisions = append(r.Decisions, decision("shots.camera", "host_template", st.Shots, "Canonical camera events are assembled from the typed structural plan after prose."))
	r.Status = "proposed"
	return r, nil
}

func (c *Compiler) Propose(ctx context.Context, brief Brief) (result Result, err error) {
	defer func() { completeAccounting(&result) }()
	if c.needsExecution(brief) {
		next, err := c.executionFor(ctx, brief)
		if err != nil {
			return Result{Version: Version, Status: "failed", Brief: brief}, err
		}
		return next.Propose(ctx, brief)
	}
	return c.prepare(ctx, brief, false)
}

func (c *Compiler) compile(ctx context.Context, brief Brief) (Result, error) {
	r, err := c.prepare(ctx, brief, true)
	if err != nil || r.Status == "needs_input" {
		return r, err
	}
	b, st, scope, request, director, evidence := r.Brief, r.Structure, r.Scope, r.Request, r.Director, r.Evidence
	r.Status = "failed"

	identities, identityErr := canonicalSubjects(b, st, request, r.Licence)
	if identityErr != nil {
		return r, identityErr
	}
	for _, subject := range identities {
		if subject.Origin == "user_symbolic" {
			r.Decisions = append(r.Decisions, decision("subjects."+subject.ID, "user_symbolic", subject, "The caller named this removed figure; it is not claimed as an observation of the source clip."))
		}
	}
	references, referenceErr := canonicalReferenceContract(b, st, request, identities, b.VoiceBindings)
	if referenceErr != nil {
		return r, referenceErr
	}
	composeLedger := []contextir.CallUsage{}
	planAttempt := 0
	cm := controlledModel{inner: c.model, structure: st, scope: scope, director: director, licence: r.Licence, identities: identities, references: &references, frames: actualFramePreservations(b, request.Target), transcripts: transcriptInputs(b), observePlanCall: func(response contextir.ModelResponse) {
		planAttempt++
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		composeLedger = append(composeLedger, contextir.CallUsage{Stage: "plan", Attempt: planAttempt, Model: response.Model, Usage: response.Usage, Valid: valid})
	}}
	plannerOptions := c.options.Planner
	parentValidator := plannerOptions.ValidateDocument
	assessmentFindings := r.Findings
	var acceptedArtifact h3.Artifact
	plannerOptions.ValidateDocument = func(locked contextir.Request, doc contextir.Document) error {
		if parentValidator != nil {
			if err := parentValidator(locked, doc); err != nil {
				return err
			}
		}
		expected, appearanceErr := canonicalAppearanceContract(doc, identities)
		if appearanceErr != nil {
			return appearanceErr
		}
		if err := verifySubjectContract(doc, expected); err != nil {
			return err
		}
		if err := references.verify(doc); err != nil {
			return err
		}
		if err := checkDocumentControls(b, st, scope, doc); err != nil {
			return err
		}
		if err := RequiredPlacedAppearances(doc); err != nil {
			return err
		}
		artifact, findings, err := c.assess(b, st, scope, doc, r.Licence)
		assessmentFindings = append(append(r.Findings[:0:0], r.Findings...), findings...)
		if err != nil {
			return err
		}
		review, reviewErr := c.ReviewComposition(ctx, b, st, doc)
		if review.Required {
			r.CompositionReviews = append(r.CompositionReviews, review)
			composeLedger = append(composeLedger, review.Calls...)
		}
		if reviewErr != nil {
			stop := ctx.Err() != nil || contextir.HasCode(reviewErr, contextir.CodeModel) || contextir.HasCode(reviewErr, contextir.CodeCanceled)
			for _, call := range review.Calls {
				stop = stop || !call.Valid
			}
			if stop {
				return planner.StopValidation(reviewErr)
			}
			return reviewErr
		}
		acceptedArtifact = artifact
		return nil
	}
	p, err := planner.New(cm, nil, plannerOptions)
	if err != nil {
		return r, err
	}
	plan, err := p.PlanWithEvidence(ctx, request, evidence)
	r.Calls = append(r.Calls, composeLedger...)
	r.Findings = assessmentFindings
	if err != nil {
		return r, err
	}
	r.Document = plan.Document
	r.Artifact = acceptedArtifact
	r.Budget, err = estimateBudget(r.Artifact, b)
	if err != nil {
		return r, err
	}
	r.Status = "compiled"
	r.Decisions = append(r.Decisions, decision("subjects.shot_ids", "typed_actual_use_projection", r.Document.Subjects, "High-level target appearances derive only from actual RichText and speaker-dialogue bindings. Removed figures carry their incoming replacement's transfer scope; unused source observations remain in Evidence."))
	r.Decisions = append(r.Decisions, decision("creativity", "caller_or_default", scope, "Scope reflects requested elements, explicit prohibitions, and the selected four-position creativity control."))
	return r, nil
}

func (c *Compiler) collectEvidence(ctx context.Context, b Brief) (out perceptionAccounting, err error) {
	return c.collectEvidenceMode(ctx, b, true)
}

func (c *Compiler) collectEvidenceMode(ctx context.Context, b Brief, extractCards bool) (out perceptionAccounting, err error) {
	assets := make([]contextir.Asset, len(b.Assets))
	for i, a := range b.Assets {
		assets[i] = a.Asset
	}
	suppliedEvidence := append([]contextir.Evidence(nil), b.Evidence...)
	suppliedCards := map[string]*contextir.AssetCard{}
	for _, e := range b.Evidence {
		suppliedCards[e.AssetID] = e.Card
	}
	for _, card := range b.Cards {
		if supplied, exists := suppliedCards[card.AssetID]; exists && supplied != nil && sameJSON(*supplied, card) {
			continue
		}
		a, ok := assetID(b.Assets, card.AssetID)
		if !ok {
			return out, invalid("cards.asset_id", "Card must identify an input asset.")
		}
		suppliedEvidence = append(suppliedEvidence, cardEvidence(card, a))
	}
	if err := contextir.ValidateEvidence(assets, suppliedEvidence); err != nil {
		return out, err
	}
	supplied := map[string]contextir.Evidence{}
	for _, e := range suppliedEvidence {
		supplied[e.AssetID] = e
	}
	for _, a := range b.Assets {
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "perception", "context", "Perception was canceled.", ctx.Err())
		}
		e, ok := supplied[a.ID]
		if !ok {
			if isNil(c.perceiver) {
				return out, failure(contextir.CodePerception, "perception", "assets", "Each asset requires supplied observed evidence or an injected perception adapter.", nil)
			}
			var err error
			if metered, ok := c.perceiver.(contextir.MeteredPerceiver); ok {
				var actualCalls []contextir.CallUsage
				e, actualCalls, err = metered.PerceiveWithCalls(ctx, a.Asset)
				actualCalls = append([]contextir.CallUsage(nil), actualCalls...)
				for i := range actualCalls {
					if actualCalls[i].AssetID == "" {
						actualCalls[i].AssetID = a.ID
					}
					actualCalls[i].Valid = actualCalls[i].Valid && actualCalls[i].Usage.InputTokens >= 0 && actualCalls[i].Usage.OutputTokens >= 0
					if actualCalls[i].AssetID != a.ID {
						actualCalls[i].Valid = false
					}
					if actualCalls[i].Unmetered && !slices.Contains(out.UnmeteredAssetIDs, a.ID) {
						out.UnmeteredAssetIDs = append(out.UnmeteredAssetIDs, a.ID)
					}
				}
				out.Calls = append(out.Calls, actualCalls...)
				if err == nil {
					for _, call := range actualCalls {
						if call.AssetID != a.ID {
							err = failure(contextir.CodeModelOutput, "perception", "calls.asset_id", "A metered perception call must identify the asset actually inspected; the original ledger was retained.", nil)
							break
						}
						if call.Unmetered {
							if call.Valid || call.Usage != (contextir.Usage{}) {
								err = failure(contextir.CodeModelOutput, "perception", "usage", "An opaque perception call must report unknown usage explicitly, not a valid or invented amount.", nil)
								break
							}
							continue
						}
						if !call.Valid {
							err = failure(contextir.CodeModelOutput, "perception", "usage", "Metered perception returned invalid usage.", nil)
							break
						}
					}
				}
			} else {
				e, err = c.perceiver.Perceive(ctx, a.Asset)
				out.UnmeteredAssetIDs = append(out.UnmeteredAssetIDs, a.ID)
			}
			if ctx.Err() != nil {
				return out, failure(contextir.CodeCanceled, "perception", "context", "Perception was canceled.", ctx.Err())
			}
			if err != nil {
				return out, failure(contextir.CodePerception, "perception", "assets", "The perception adapter could not inspect the asset.", err)
			}
		}
		if e.AssetID != a.ID || strings.TrimSpace(e.AnalyzerVersion) == "" || len(e.Facts) == 0 {
			return out, failure(contextir.CodePerception, "perception", "evidence", "Evidence must identify the inspected asset, analyzer version, and observed facts.", nil)
		}
		if e.Coverage == nil && !c.options.Planner.AllowUnspecifiedCoverage {
			return out, failure(contextir.CodePerception, "perception", "evidence.coverage", "Declare perception coverage; legacy coverage needs explicit permission.", nil)
		}
		if e.Coverage != nil && e.Coverage.Status == contextir.CoveragePartial && !c.options.Planner.AllowPartialPerception {
			return out, failure(contextir.CodePerception, "perception", "evidence.coverage", "Partial perception needs explicit permission.", nil)
		}
		if e.Card == nil && extractCards {
			extraction, extractionErr := c.ExtractObservationCard(ctx, a.Asset, e)
			out.Calls = append(out.Calls, extraction.Calls...)
			out.ObservationCards = append(out.ObservationCards, extraction)
			if extractionErr != nil {
				out.Evidence = append(out.Evidence, e)
				return out, extractionErr
			}
			card := extraction.Card
			e.Card = &card
			e.Limitations = append(append([]string(nil), e.Limitations...), "The typed card was structured from existing facts; it is not an additional media inspection.")
		}
		out.Evidence = append(out.Evidence, e)
	}
	if err := contextir.ValidateEvidence(assets, out.Evidence); err != nil {
		return out, err
	}
	return out, nil
}

func scopeFor(b Brief, st Structure) Scope {
	s := st.Requested
	switch b.Creativity {
	case CreativityRestrained:
		s.Magnitude = "plain"
	case CreativityBalanced:
		s.Music = true
		s.Magnitude = "measured"
	case CreativityBold:
		s.Speech = true
		s.Music = true
		s.OnscreenText = true
		s.Magnitude = "assertive"
	case CreativityExtreme:
		s.Speech = true
		s.Music = true
		s.OnscreenText = true
		s.Magnitude = "maximal"
	}
	if len(st.Dialogue) > 0 {
		s.Speech = true
	}
	if len(st.OnscreenText) > 0 {
		s.OnscreenText = true
	}
	if st.Forbidden.Speech {
		s.Speech = false
	}
	if st.Forbidden.Music || b.Silent {
		s.Music = false
	}
	if st.Forbidden.OnscreenText {
		s.OnscreenText = false
	}
	if b.SilenceAll {
		s.Speech = false
		s.Music = false
	}
	return s
}

func requestFor(b Brief, target contextir.Target, st Structure, scope Scope) (contextir.Request, error) {
	req := contextir.Request{Instruction: b.Intent, Mode: st.Mode, Target: target, AllowCreativeAdditions: b.Creativity != CreativityRestrained, AllowGeneratedDialogue: scope.Speech && (b.Creativity == CreativityBold || b.Creativity == CreativityExtreme || len(st.Dialogue) == 0)}
	for _, a := range b.Assets {
		req.Assets = append(req.Assets, a.Asset)
	}
	for i, a := range st.Assets {
		bound := contextir.AssetBinding{Source: sourceFor(b.Assets[i], a.Role), Role: rootRole(a.Role), Replaces: a.Replaces, PairedVideoID: b.Assets[i].PairedVideoID}
		if a.Role == RoleFirstFrame {
			at := contextir.Time(0)
			bound.Anchor = &at
		}
		if a.Role == RoleLastFrame {
			at := target.Duration
			bound.Anchor = &at
		}
		if st.Mode == contextir.ModeReference && (a.Role == RoleFirstFrame || a.Role == RoleLastFrame) {
			bound.Role = contextir.RoleKeyframe
		}
		req.Bindings = append(req.Bindings, bound)
	}
	for _, a := range b.FrameAnchors {
		if a.Role != RoleFirstFrame && a.Role != RoleLastFrame {
			return req, invalid("frame_anchors.role", "Frame anchors require frame_anchor_first or frame_anchor_last.")
		}
		if _, ok := assetID(b.Assets, a.AssetID); !ok {
			return req, invalid("frame_anchors.asset_id", "Frame anchor must reference an input asset.")
		}
		at := a.At
		role := rootRole(a.Role)
		if st.Mode == contextir.ModeReference {
			role = contextir.RoleKeyframe
		}
		bound := contextir.AssetBinding{Source: contextir.SourceRef{AssetID: a.AssetID, Track: contextir.TrackVisual}, Role: role, Anchor: &at}
		alreadyBound := false
		for _, existing := range req.Bindings {
			alreadyBound = alreadyBound || sameJSON(existing, bound)
		}
		if !alreadyBound {
			req.Bindings = append(req.Bindings, bound)
		}
	}
	if err := bindProjectedFrameIdentities(b, st, &req); err != nil {
		return req, err
	}
	speakerByHint := map[string]string{}
	for i, line := range st.Dialogue {
		lang := line.Language
		if lang == "" {
			lang = "English"
		}
		sid := fmt.Sprintf("speaker-%d", i+1)
		if line.SpeakerHint != "" {
			key := fmt.Sprintf("%t/%s", line.Voiceover, line.SpeakerHint)
			if existing, ok := speakerByHint[key]; ok {
				sid = existing
			} else {
				speakerByHint[key] = sid
			}
		}
		req.Dialogue = append(req.Dialogue, contextir.DialogueLine{ID: fmt.Sprintf("line-%d", i+1), Kind: line.Kind, Text: line.Text, Language: lang, SpeakerIDs: []string{sid}, Voiceover: line.Voiceover, Delivery: line.Delivery})
	}
	for i, s := range b.Constraints {
		req.Constraints = append(req.Constraints, contextir.Constraint{ID: fmt.Sprintf("constraint-%d", i+1), Kind: contextir.ConstraintRequirement, Text: s})
	}
	if b.SilenceAll {
		req.Constraints = append(req.Constraints, contextir.Constraint{ID: "compiler-silence", Kind: contextir.ConstraintSilence, Text: "The target has no sound."})
	} else {
		if !scope.Music {
			kind := st.MusicConstraint
			if kind == "" {
				kind = contextir.ConstraintNoAddedMusic
			}
			if kind != contextir.ConstraintNoMusic && kind != contextir.ConstraintNoAddedMusic {
				return req, invalid("music_constraint", "Music scope must distinguish no_music from no_added_music.")
			}
			text := "Do not add a new score. Explicitly bound BGM copying remains permitted."
			if kind == contextir.ConstraintNoMusic {
				text = "The target contains no music, including copied background music."
			}
			req.Constraints = append(req.Constraints, contextir.Constraint{ID: "compiler-no-music", Kind: kind, Text: text})
		}
		if !scope.Speech {
			req.Constraints = append(req.Constraints, contextir.Constraint{ID: "compiler-no-dialogue", Kind: contextir.ConstraintNoDialogue, Text: "Do not add spoken or sung lines."})
		}
	}
	req.OnscreenText = append([]string(nil), st.OnscreenText...)
	req.AllowGeneratedVisibleText = (b.Creativity == CreativityBold || b.Creativity == CreativityExtreme) && !st.Forbidden.OnscreenText
	req.AdditionScope = &contextir.AdditionScope{Dialogue: scope.Speech, Music: scope.Music, VisibleText: scope.OnscreenText}
	for i, s := range st.Shots {
		end := target.Duration
		if i+1 < len(st.Shots) {
			end = st.Shots[i+1].Start
		}
		req.PinnedShots = append(req.PinnedShots, contextir.ShotBoundary{ID: fmt.Sprintf("shot-%d", i+1), Start: s.Start, End: end})
	}
	return req, contextir.ValidateRequest(req)
}

func assetID(as []Asset, id string) (Asset, bool) {
	for _, a := range as {
		if a.ID == id {
			return a, true
		}
	}
	return Asset{}, false
}

func invalid(path, message string) error {
	return failure(contextir.CodeInvalid, "compiler", path, message, nil)
}
func failure(code contextir.Code, stage, path, message string, cause error) error {
	return &contextir.Error{Code: code, Stage: stage, Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}, Cause: cause}
}
func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}

type controlledModel struct {
	inner           Model
	structure       Structure
	scope           Scope
	director        *Director
	licence         *LicenceDecision
	identities      []contextir.Subject
	transcripts     []map[string]string
	references      *referenceContract
	frames          []FramePreservation
	observePlanCall func(contextir.ModelResponse)
}

func (m controlledModel) Generate(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
	var input map[string]json.RawMessage
	if err := json.Unmarshal(req.Input, &input); err != nil {
		return contextir.ModelResponse{}, err
	}
	var original contextir.Request
	if err := json.Unmarshal(input["request"], &original); err != nil {
		return contextir.ModelResponse{}, err
	}
	slots, slotErr := fixedDialogueSlots(original, m.structure)
	if slotErr != nil {
		return contextir.ModelResponse{}, slotErr
	}
	writerRequest := original
	writerRequest.Dialogue = nil
	input["request"], _ = json.Marshal(writerRequest)
	writerStructure := m.structure
	writerStructure.Dialogue = nil
	writerStructure.DialoguePlacements = nil
	writerSchema, schemaErr := writerResponseSchema(req.ResponseSchema, original)
	if schemaErr != nil {
		return contextir.ModelResponse{}, schemaErr
	}
	writerCanonical := withoutSubjectShotIDs(m.identities)
	writerSchema, schemaErr = lockSubjectSchema(writerSchema, writerCanonical)
	if schemaErr != nil {
		return contextir.ModelResponse{}, schemaErr
	}
	if m.references != nil {
		writerSchema, schemaErr = m.references.writerSchema(writerSchema)
		if schemaErr != nil {
			return contextir.ModelResponse{}, schemaErr
		}
	}
	req.ResponseSchema = writerSchema
	notes := ""
	if m.director != nil {
		notes = DirectorInstruction(m.director, m.scope.Music)
	}
	prose := prosePolicy(m.structure)
	phrases := make([]string, len(m.structure.Shots))
	for i, s := range m.structure.Shots {
		phrases[i] = cameraPhrase(s.Camera)
	}
	control, _ := json.Marshal(map[string]any{"replacement_motion_obligations": m.structure.ReplacementMotionObligations, "frame_preservation": m.frames, "caller_transcripts": m.transcripts, "canonical_subjects": writerCanonical, "reference_licence": m.licence, "prose_length_policy": prose, "camera_phrases": phrases, "structure": writerStructure, "fixed_dialogue_slots": slots, "scope": m.scope, "director_notes": notes, "precedence": []string{"explicit_request", "observed_reference", "director_notes"}})
	appearances := projectionRequiredSubjectAppearances(m.identities, m.structure.SubjectProjections)
	if len(appearances) > 0 {
		var controls map[string]json.RawMessage
		_ = json.Unmarshal(control, &controls)
		controls["required_subject_appearances"], _ = json.Marshal(appearances)
		control, _ = json.Marshal(controls)
	}
	if prose.TargetWords > 0 {
		var controls map[string]json.RawMessage
		_ = json.Unmarshal(control, &controls)
		controls["main_prose_word_target"], _ = json.Marshal(prose.TargetWords)
		control, _ = json.Marshal(controls)
	}
	input["compiler_controls"] = control
	var boundaryBrief Brief
	for _, asset := range original.Assets {
		boundaryBrief.Assets = append(boundaryBrief.Assets, Asset{Asset: asset})
	}
	var boundaryEvidence []contextir.Evidence
	_ = json.Unmarshal(input["evidence"], &boundaryEvidence)
	for _, observed := range boundaryEvidence {
		if observed.Card != nil {
			boundaryBrief.Cards = append(boundaryBrief.Cards, *observed.Card)
		}
	}
	input["composition_facts"], _ = json.Marshal(compositionFacts(boundaryBrief, m.structure, m.frames))
	if m.references != nil {
		if err := m.references.projectWriterInput(input); err != nil {
			return contextir.ModelResponse{}, err
		}
	}
	if err := m.projectWriterRepair(input, original); err != nil {
		return contextir.ModelResponse{}, err
	}
	req.Input, _ = json.Marshal(input)
	req.System += "\nCompiler structural controls and reference_licence bind this composition. The attribute licence explicitly settles which request or reference governs action, medium, wardrobe, lighting, framing and palette. Caller transcripts establish only exact words; they do not establish that the model heard audio, voice/timbre/music characteristics, or complete acoustic coverage. Only reuse transcript words through the explicit reference role. The host canonical_subjects own all referenced subject IDs, definitions, sources, target attribute permissions and replacements: preserve them exactly and reference their IDs. You may create unsourced new requested/creative subjects when permitted. For content-reference identity plates, poses and sheet layout/labels/studio lights are excluded presentation, never identity. Actual frame anchors have the opposite frame-content obligation: frame_preservation requires the ENTIRE supplied image at its exact instant, including all existing panels, labels, background and positions. Do not isolate a hero panel, remove the grid or invent a cropped/prepared source. Moving a depicted figure later does not erase the whole starting frame. Existing lettering inside that source frame is inherited pixels, not a request to add new typography. Medium restyling changes the style opening, never by itself an attribute_transfer retention marker. Preserve each shot ID, start/end, semantic action, camera vocabulary and what_changes. The host inserts all canonical camera events after prose; do not return camera events or competing camera prose. Fixed dialogue slots are also host-owned: omit every fixed line from Document.dialogue and omit its dialogue events. The host restores the exact words, speaker IDs, language and continuity from those slots. You may write additional dialogue only when writerRequest.allow_generated_dialogue explicitly allows it. Create speakers matching the fixed slot speaker IDs and hints if their identity is known; never guess a new fixed utterance. Exact words, replacement relationships and permissions are mandatory. Only the director NOTES steer unresolved visual/sound choices; explicit caller and observed asset facts outrank them. Director names are withheld. Do not copy template dialogue or locations. Write visual descriptions in English while preserving every dialogue and on-screen word in its original language. Develop substantive shot descriptions and visible/audible events: physical setting, spatial relationships, light/material detail, performance and meaningful changes. This is the complete H3 scene brief, not a one-sentence outline. Avoid repeated facts, padding, unobservable emotion labels, invented media attributes, or a detached technical commentary. The requested creativity magnitude describes values, never shot count. For maximal magnitude use explicit large amplitude or fast speed where compatible with the request; quiet contrast remains permitted."
	if original.Mode != contextir.ModeReference {
		req.System += "\nThis is a base mode: every narrative RichText span must contain literal text only. Write natural complete sentences with the necessary subject identity/appearance in the prose itself. Do not return subject_id or reference_id spans: base formats would insert full definition sentences inline and break the grammar. Return subjects=[] and omit speaker subject_id links; source observations remain available separately as Evidence."
	}
	req.System += "\nDocument.style is HOST-OWNED and deliberately absent from your response schema: OMIT the style field entirely. Compose using compiler_controls.structure.style as the resolved look; host assembly inserts that exact canonical style after your strict wire response is accepted. Never author a replacement style during initial writing or repair."
	req.System += "\ncanonical_subjects describe the TARGET after accepted caller identity requirements, while raw observed evidence describes the SOURCE. Explicit target identity claims have already been resolved. Do not reassert conflicting source gender, species, age, clothes or props, and never dismiss the caller's target as a labeling mistake. Keep the canonical target identity consistent in every description, pronoun and action. For content references only, reference-sheet layout/panels/turnaround labels are excluded presentation. The actual frame anchors in frame_preservation retain those same elements as required frame pixels."
	req.System += "\nDocument.references and Speaker.voice_reference_id are HOST-OWNED: omit them entirely. canonical_references and canonical_voice_bindings specify their exact relationships for composition. Property audio is not recorded signal copying; only a BGM copy responsibility reuses its signal. Structure/storyboard references supply their named planning properties, never scene actors or setting. A placed or replacement identity must be used in a real shot/action, not merely in metadata."
	if len(m.transcripts) > 0 {
		req.System += "\n" + transcriptWriterInstruction
	}
	req.System += "\n" + compositionBoundaryFor(original.Mode)
	req.System += "\n" + proseLengthInstruction(prose)
	req.System += "\n" + replacementMotionInstruction
	req.System += "\n" + writerHygieneInstruction
	if len(slots) > 0 {
		req.System += "\n" + fixedDialoguePerformanceInstruction
	}
	if len(original.OnscreenText) > 0 {
		req.System += "\n" + visibleTextPerformanceInstruction
	}
	req.System += "\nSubject shot_ids are HOST-OWNED: omit them. Bind referenced identities and environments by subject_id in the actual shot/event descriptions where they contribute; merely listing a source definition does not place it in every shot. An actor which leaves need not remain in later shots. Unsourced new targets should also use actual structural bindings in reference mode. In base modes return no subject metadata: all identities belong in natural literal narrative, while actual source-frame observations remain Evidence."
	if len(appearances) > 0 {
		req.System += "\n" + requiredSubjectAppearanceInstruction
	}
	response, err := m.inner.Generate(ctx, req)
	if m.observePlanCall != nil {
		m.observePlanCall(response)
	}
	if err != nil {
		return response, err
	}
	decoded, decodeErr := contextir.Decode(bytes.NewReader(response.JSON))
	if decodeErr != nil {
		return response, &contextir.Error{Code: contextir.CodeModelOutput, Stage: "compose", Issues: repairDiagnostics(decodeErr, "response", "Return the complete exact Document wire shape before host assembly."), Cause: decodeErr}
	}
	var rawFields map[string]json.RawMessage
	_ = json.Unmarshal(response.JSON, &rawFields)
	if _, authoredStyle := rawFields["style"]; authoredStyle {
		return response, failure(contextir.CodeModelOutput, "compose", "style", "Omit the host-owned style field entirely; the compiler inserts its resolved style after prose.", nil)
	}
	if metadataErr := verifyWriterSubjectMetadata(decoded); metadataErr != nil {
		return response, metadataErr
	}
	if subjectErr := verifySubjectContract(decoded, writerCanonical); subjectErr != nil {
		return response, subjectErr
	}
	if literalErr := verifyLiteralBaseProse(decoded); literalErr != nil {
		return response, literalErr
	}
	if protocolErr := unexpandedWriterSlots(response.JSON, slots); protocolErr != nil {
		return response, protocolErr
	}
	if m.references != nil {
		assembled, contractErr := m.references.assemble(response.JSON)
		if contractErr != nil {
			return response, contractErr
		}
		response.JSON = assembled
	}
	response.JSON = assembleFixedEvents(response.JSON, original, m.structure, slots, m.identities)
	var assembled contextir.Document
	if err := strictDecode(response.JSON, &assembled); err != nil {
		return response, err
	}
	projected, projectionErr := projectSubjectAppearances(assembled, m.identities)
	if projectionErr != nil {
		return response, projectionErr
	}
	encoded, encodeErr := json.Marshal(projected)
	if encodeErr != nil {
		return response, failure(contextir.CodeInvalid, "compose", "subjects.shot_ids", "Host-derived subject appearances could not be encoded.", encodeErr)
	}
	if err := verifyTranscriptLiteralPlacement(projected, m.structure.TranscriptUses, m.structure.OnscreenText); err != nil {
		return response, err
	}
	response.JSON = encoded
	return response, nil
}

func checkDocumentControls(b Brief, st Structure, scope Scope, d contextir.Document) error {
	if err := verifyCompleteCopy(b, d); err != nil {
		return err
	}
	if d.Style != st.Style {
		return failure(contextir.CodeModelOutput, "compose", "style", "Preserve the structurally resolved visual style.", nil)
	}
	if len(d.Shots) != len(st.Shots) {
		return failure(contextir.CodeModelOutput, "compose", "shots", "Preserve the structural shot plan.", nil)
	}
	for i, s := range d.Shots {
		if i > 0 {
			switch s.Transition {
			case "", "cut", "the camera cuts to", "the shot cuts to", "the shot transitions to", "the shot changes to", "the shot switches to":
			default:
				return failure(contextir.CodeModelOutput, "compose", "shots.transition", "Use a transition from the closed H3 cut vocabulary.", nil)
			}
		}
		end := d.Target.Duration
		if i+1 < len(st.Shots) {
			end = st.Shots[i+1].Start
		}
		if s.ID != fmt.Sprintf("shot-%d", i+1) || s.Start != st.Shots[i].Start || s.End != end {
			return failure(contextir.CodeModelOutput, "compose", "shots", "Preserve exact structural shot IDs, starts, and ends.", nil)
		}
	}
	for i, shot := range d.Shots {
		cameras := 0
		for _, event := range shot.Events {
			if event.Kind == contextir.EventCamera {
				cameras++
				if len(event.Description) != 1 || event.Description[0].Text != cameraPhrase(st.Shots[i].Camera) {
					return failure(contextir.CodeModelOutput, "compose", "shots.events.camera", "Copy the exact structurally chosen camera phrase in one literal camera event.", nil)
				}
			}
		}
		if cameras != 1 {
			return failure(contextir.CodeModelOutput, "compose", "shots.events.camera", "Each shot requires exactly one structurally chosen camera event.", nil)
		}
	}
	return nil
}
