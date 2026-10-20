package compiler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

// Draft explicitly requests the deterministic floor. It never calls Model and
// never invents a semantic mode for ambiguous pictures. Structured cards support
// exact identity/pose and unique-target replacement decisions without heuristics.
func (c *Compiler) Draft(ctx context.Context, b Brief) (result Result, err error) {
	defer func() { completeAccounting(&result) }()
	return c.draft(ctx, b, nil)
}

func (c *Compiler) draft(ctx context.Context, brief Brief, known *Structure) (Result, error) {
	r := Result{Version: Version, Status: "failed", Brief: brief, UsageComplete: true}
	if c == nil {
		return r, invalid("compiler", "Initialize the compiler with New.")
	}
	if ctx.Err() != nil {
		return r, failure(contextir.CodeCanceled, "draft", "context", "Drafting was canceled.", ctx.Err())
	}
	intake, err := c.ValidateIntake(ctx, brief)
	r.Brief = intake.Brief
	r.Decisions = intake.Decisions
	if err != nil {
		return r, err
	}
	b, target, director := intake.Brief, intake.Target, intake.Director
	r.Director = director
	if b.Seed != nil || b.Effort != "" {
		r.Decisions = append(r.Decisions, decision("execution", "explicit_draft_operation", ExecutionOptions{Seed: b.Seed, Effort: b.Effort}, "The explicitly requested deterministic draft makes no model calls, so sampling and reasoning choices do not apply to this artifact."))
	}
	observed, err := c.collectEvidence(ctx, b)
	evidence := observed.Evidence
	r.Evidence = evidence
	r.Calls = append(r.Calls, observed.Calls...)
	r.ObservationCards = observed.ObservationCards
	r.UnmeteredAssetIDs = observed.UnmeteredAssetIDs
	if len(observed.UnmeteredAssetIDs) > 0 {
		r.UsageComplete = false
	}
	if err != nil {
		return r, err
	}
	b = mergeObservedCards(b, evidence)
	b.Evidence = evidence
	r.Brief = b
	if err := validateExplicitReplacementInputs(b); err != nil {
		return r, err
	}
	st := Structure{}
	if known != nil && len(known.Shots) > 0 {
		st = *known
	} else {
		mode, rule, err := InferMode(b)
		if err != nil {
			return r, err
		}
		if mode == "" {
			return r, invalid("mode", "Deterministic drafting needs explicit mode or explicit image roles; semantic image routing requires Compile.")
		}
		st.Mode = mode
		st.Style = b.Style
		if st.Style == "" {
			for _, card := range b.Cards {
				if card.Style != "" {
					st.Style = card.Style
					break
				}
			}
		}
		if st.Style == "" {
			st.Style = "Live-action, cinematic"
		}
		r.Decisions = append(r.Decisions, decision("mode", rule, mode, "Deterministic mode follows only typed attachments and caller roles."))
		for i, a := range b.Assets {
			role := a.Role
			if role == "" {
				switch a.Kind {
				case contextir.MediaImage:
					switch mode {
					case contextir.ModeFirstFrame:
						role = RoleFirstFrame
					case contextir.ModeLastFrame:
						role = RoleLastFrame
					case contextir.ModeFirstLastFrame:
						if i == 0 {
							role = RoleFirstFrame
						} else {
							role = RoleLastFrame
						}
					default:
						role = RoleSubject
					}
				case contextir.MediaVideo:
					role = RoleEditSource
				case contextir.MediaAudio:
					role = RoleBGM
				}
				r.Decisions = append(r.Decisions, decision("assets."+a.ID+".role", "explicit_draft_default", role, "The deterministic draft uses a documented kind/mode default; it does not claim semantic role inference."))
			}
			st.Assets = append(st.Assets, StructuralAsset{AssetID: a.ID, Role: role, Replaces: a.Replaces})
		}
		allocated, allocErr := deterministicAllocation(b, st, target, c.options.MaxAutoShots)
		if allocErr != nil {
			return r, allocErr
		}
		rotation := DraftCameras(scopeFor(b, st).Magnitude)
		for i, s := range allocated {
			purpose := "develop"
			if i == 0 {
				purpose = "establish"
			}
			st.Shots = append(st.Shots, ProposedShot{Start: contextir.Time(s.StartMS), Purpose: purpose, Framing: "medium", Action: b.Intent, WhatChanges: fmt.Sprintf("Requested scene segment %d of %d", i+1, len(allocated)), Camera: rotation[i%len(rotation)]})
		}

		st.Dialogue = append([]Dialogue(nil), b.Dialogue...)
		st.OnscreenText = append([]string(nil), b.OnscreenText...)
		st.Requested.Speech = len(b.Dialogue) > 0
		st.Requested.OnscreenText = len(b.OnscreenText) > 0
		st.Forbidden.Music = b.Silent
		if !scopeFor(b, st).Music {
			st.MusicConstraint = contextir.ConstraintNoAddedMusic
		}
	}
	st, ds, err := validateStructure(b, target, st, st.Mode, c.options.MaxAutoShots)
	r.Decisions = append(r.Decisions, ds...)
	if err != nil {
		return r, err
	}
	r.Structure = st
	r.Scope = scopeFor(b, st)
	req, err := requestFor(b, target, st, r.Scope)
	r.Request = req
	if err != nil {
		return r, err
	}
	preflightFindings, preflightErr := c.preflight(b, st, req)
	r.Findings = append(r.Findings, preflightFindings...)
	if preflightErr != nil {
		return r, preflightErr
	}
	choices, loraFindings, loraErr := c.resolveLoRAs(b, st.Mode, target)
	r.Findings = append(r.Findings, loraFindings...)
	if loraErr != nil {
		return r, loraErr
	}
	req.LoRAs = choices
	r.Request = req
	d, err := draftDocument(b, req, st, evidence)
	if err != nil {
		return r, err
	}
	if err = contextir.Validate(d); err != nil {
		return r, err
	}
	artifact, err := h3.Compile(d, c.options.Profile)
	if err != nil {
		return r, err
	}
	assessedFindings, assessErr := c.assess(b, st, r.Scope, d)
	r.Findings = append(r.Findings, assessedFindings...)
	err = assessErr
	if err != nil {
		return r, err
	}
	r.Document = d
	r.Artifact = artifact
	r.Budget, err = estimateBudget(artifact, b)
	if err != nil {
		return r, err
	}
	r.Status = "draft"
	r.Decisions = append(r.Decisions, decision("status", "explicit_operation", "draft", "This deterministic floor renders supplied intent and evidence; no model interpreted the request and no generation quality is implied."))
	return r, nil
}

func draftDocument(b Brief, req contextir.Request, st Structure, evidence []contextir.Evidence) (contextir.Document, error) {
	d := contextir.Document{LoRAs: req.LoRAs, Version: contextir.Version, Mode: req.Mode, Target: req.Target, Assets: req.Assets, Evidence: evidence, Dialogue: req.Dialogue, Constraints: req.Constraints, Summary: contextir.Text(b.Intent), Soundscape: "N/A", Music: "N/A", Style: st.Style}
	for i, pin := range req.PinnedShots {
		s := contextir.Shot{ID: pin.ID, Start: pin.Start, End: pin.End, Description: contextir.Text(st.Shots[i].Action)}
		if i > 0 {
			s.Transition = "cut"
		}
		s.Events = append(s.Events, contextir.Event{Kind: contextir.EventCamera, Description: contextir.Text(cameraPhrase(st.Shots[i].Camera))})
		d.Shots = append(d.Shots, s)
	}
	if req.Mode == contextir.ModeReference {
		var subjectErr error
		d.Subjects, subjectErr = compatibilitySubjects(b, st, d.Shots)
		if subjectErr != nil {
			return d, subjectErr
		}
	}

	references, referenceErr := canonicalReferenceContract(b, st, req, d.Subjects, b.VoiceBindings)
	if referenceErr != nil {
		return d, referenceErr
	}
	d.References = references.References
	d.Speakers = references.Speakers

	for i, line := range req.Dialogue {
		hint := "the requested speaker"
		if i < len(st.Dialogue) && st.Dialogue[i].SpeakerHint != "" {
			hint = st.Dialogue[i].SpeakerHint
		}
		sid := line.SpeakerIDs[0]
		speakerExists := false
		for _, speaker := range d.Speakers {
			speakerExists = speakerExists || speaker.ID == sid
		}
		if !speakerExists {
			d.Speakers = append(d.Speakers, contextir.Speaker{ID: sid, Description: hint})
		}
	}
	for _, text := range st.OnscreenText {
		d.Shots[0].Events = append(d.Shots[0].Events, contextir.Event{Kind: contextir.EventVisibleText, VisibleText: text})
	}
	slots, slotErr := fixedDialogueSlots(req, st)
	if slotErr != nil {
		return d, slotErr
	}
	wire, encodeErr := json.Marshal(d)
	if encodeErr != nil {
		return d, encodeErr
	}
	assembled := assembleFixedEvents(wire, req, st, slots)
	if err := strictDecode(assembled, &d); err != nil {
		return d, err
	}
	return d, nil
}
