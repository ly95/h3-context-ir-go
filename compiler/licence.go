package compiler

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/plan"
	"github.com/ly95/contextir/planner"
)

var LicenceAttributes = []string{"action", "medium", "wardrobe", "lighting", "framing", "palette"}

type StyleDecision struct {
	ObservedPhrase  string `json:"observed_phrase"`
	Phrase          string `json:"phrase"`
	Source          string `json:"source"`
	ReferenceMedium string `json:"reference_medium"`
	RequestedMedium string `json:"requested_medium"`
	Discrepancy     bool   `json:"discrepancy"`
	Note            string `json:"note"`
}

// LicenceDecision exposes exactly who may govern each visual attribute. Pose and
// sheet presentation remain in the observed subject record, with a separate
// mechanism-owned permission rather than being promoted into identity.
type IdentityDecision struct {
	RequestBinding       string                        `json:"request_binding,omitempty"`
	DescriptorSource     string                        `json:"descriptor_source,omitempty"`
	TargetMention        string                        `json:"target_mention,omitempty"`
	Kind                 string                        `json:"kind,omitempty"`
	Claims               []IdentityClaim               `json:"claims,omitempty"`
	AdjustedAttributes   []IdentityAttributeAdjustment `json:"adjusted_attributes,omitempty"`
	ExcludedPresentation []string                      `json:"excluded_presentation,omitempty"`
	Label                string                        `json:"label"`
	Descriptor           string                        `json:"descriptor"`
	RetainedAttributes   []string                      `json:"retained_attributes"`
	RequestedAttributes  []string                      `json:"requested_attributes"`
	RequestQuote         string                        `json:"request_quote"`
	WardrobeAttributes   []string                      `json:"wardrobe_attributes"`
}

type IdentityClaim struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
	Quote     string `json:"quote"`
}

type IdentityAttributeAdjustment struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type LicenceDecision struct {
	ReferencePermissions map[string]bool     `json:"reference_permissions,omitempty"`
	Medium               *MediumDecision     `json:"medium_decision,omitempty"`
	Identity             []IdentityDecision  `json:"identity"`
	Governs              map[string]string   `json:"governs"`
	Reasons              map[string]string   `json:"reasons"`
	TransformPhrase      *string             `json:"transform_phrase"`
	TransformTarget      string              `json:"transform_target"`
	MediumTransferred    bool                `json:"medium_transferred"`
	Style                StyleDecision       `json:"style"`
	Subjects             []plan.SubjectPlan  `json:"subjects,omitempty"`
	Calls                []planner.CallUsage `json:"calls,omitempty"`
}

// DecideLicence is the production reference/request precedence boundary. It
// interprets open-ended language through a typed model contract, never verb or
// garment lists. The same decision locks the structural and prose stages.
type LicenceReference struct {
	ActualFrame bool                `json:"actual_frame,omitempty"`
	ID          string              `json:"id"`
	Kind        contextir.MediaKind `json:"kind"`
	Role        Role                `json:"role,omitempty"`
	Card        AssetCard           `json:"card"`
}

func (c *Compiler) DecideLicence(ctx context.Context, b Brief) (LicenceDecision, error) {
	return c.DecideLicenceWithReferences(ctx, b, nil)
}

// DecideLicenceWithReferences supports reference analysis independent of media
// attachment/publication. Additional readings govern licence/style only: they
// are never fabricated Asset entries, conditioning sockets or observed subjects.
func (c *Compiler) DecideLicenceWithReferences(ctx context.Context, b Brief, references []LicenceReference) (LicenceDecision, error) {
	if c.needsExecution(b) {
		next, err := c.executionFor(ctx, b)
		if err != nil {
			return LicenceDecision{}, err
		}
		return next.DecideLicenceWithReferences(ctx, b, references)
	}
	return c.decideLicence(ctx, b, references)
}

func (c *Compiler) decideLicence(ctx context.Context, brief Brief, additional []LicenceReference) (LicenceDecision, error) {
	out := LicenceDecision{Governs: map[string]string{}, Reasons: map[string]string{}}
	if c == nil {
		return out, invalid("compiler", "Initialize the compiler with New.")
	}
	b, err := normalizeBrief(brief, c.options)
	if err != nil {
		return out, err
	}
	b, err = roleScopedCompositionBrief(b)
	if err != nil {
		return out, err
	}
	pb, cards := compatibilityInputs(b, Structure{})
	manifest, err := plan.BuildManifest(pb, budget.Target{Frames: 192, FPS: 24, Canvas: [2]int{1344, 768}})
	if err != nil {
		return out, err
	}
	out.Subjects = plan.BuildSubjects(manifest, cards)
	readings := []LicenceReference{}
	for _, asset := range b.Assets {
		if card, ok := cardByID(b.Cards, asset.ID); ok {
			readings = append(readings, LicenceReference{ID: asset.ID, Kind: asset.Kind, Role: asset.Role, ActualFrame: isActualFrameAsset(b, asset.ID), Card: card})
		}
	}
	seenReadings := map[string]bool{}
	for _, reading := range readings {
		seenReadings[reading.ID] = true
	}
	for _, reading := range additional {
		if reading.ID == "" || seenReadings[reading.ID] {
			return out, invalid("reference_readings", "Independent reference readings need unique nonempty IDs, without duplicate attachments.")
		}
		if reading.Kind != contextir.MediaImage && reading.Kind != contextir.MediaVideo && reading.Kind != contextir.MediaAudio {
			return out, invalid("reference_readings.kind", "A reference reading needs an explicit supported media kind.")
		}
		if reading.Card.AssetID != "" && reading.Card.AssetID != reading.ID {
			return out, invalid("reference_readings.card.asset_id", "Reading and Card identities must agree.")
		}
		reading.Card = roleScopedCard(reading.Card, reading.Role)
		seenReadings[reading.ID] = true
		readings = append(readings, reading)
	}
	visualOwners := map[string]bool{}
	addVisualOwner := func(kind contextir.MediaKind, role Role) {
		if kind != contextir.MediaImage && kind != contextir.MediaVideo {
			return
		}
		for _, attribute := range LicenceAttributes {
			visualOwners[attribute] = visualOwners[attribute] || roleLicencesVisual(role, attribute)
		}
	}
	for _, asset := range b.Assets {
		addVisualOwner(asset.Kind, asset.Role)
	}
	for _, reading := range readings {
		addVisualOwner(reading.Kind, reading.Role)
	}
	visual := visualOwners["medium"]
	out.ReferencePermissions = visualOwners
	if isNil(c.model) {
		return out, failure(contextir.CodeModel, "licence", "model", "Reference attribute precedence needs a model adapter.", nil)
	}
	medium, mediumErr := c.decideMedium(ctx, b, readings, visual)
	out.Calls = append(out.Calls, medium.Calls...)
	if mediumErr != nil {
		return out, mediumErr
	}
	out.Medium = &medium
	object := func(props map[string]any) map[string]any {
		required := []string{}
		for _, a := range LicenceAttributes {
			if _, ok := props[a]; ok {
				required = append(required, a)
			}
		}
		if len(required) != len(props) {
			required = nil
			for key := range props {
				required = append(required, key)
			}
			sort.Strings(required)
		}
		return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": props}
	}
	governs := map[string]any{}
	reasons := map[string]any{}
	for _, a := range LicenceAttributes {
		governs[a] = map[string]any{"type": "string", "enum": []string{"request", "reference"}}
		if !visualOwners[a] {
			governs[a] = map[string]any{"const": "request"}
		}
		if a == "medium" {
			owner := "reference"
			if medium.Disposition != "preserve" {
				owner = "request"
			}
			governs[a] = map[string]any{"const": owner}
		}
		reasons[a] = map[string]any{"type": "string"}
	}
	sheets := map[string]bool{}
	for _, entry := range manifest {
		if card, ok := cards[entry.SHA256]; ok && card.IsReferenceSheet && !isActualFrameAsset(b, entry.SHA256) {
			sheets[entry.Label] = true
		}
	}
	responsibilities := map[string][]Role{}
	for _, subject := range out.Subjects {
		for _, source := range subject.Sources {
			for _, entry := range manifest {
				if entry.Label == source && !contains(responsibilities[subject.Label], Role(entry.Role)) {
					responsibilities[subject.Label] = append(responsibilities[subject.Label], Role(entry.Role))
				}
			}
		}
	}
	identity, identityCalls, identityErr := c.decideIdentityRequirements(ctx, b, out.Subjects, sheets, responsibilities)
	out.Calls = append(out.Calls, identityCalls...)
	if identityErr != nil {
		return out, identityErr
	}
	out.Identity = identity
	schema, _ := json.Marshal(object(map[string]any{"governs": object(governs), "reasons": object(reasons)}))
	const system = `Resolve reference/request licences for each visual attribute. Return only governs and reasons matching the supplied JSON contract. Interpret the complete request semantically, without keyword lists. The separate medium_decision already settles rendering-medium permission and taxonomy: the medium owner is pinned by the schema. Host code computes the style phrase, transformation phrase/target, discrepancy and note from the accepted medium decision. Do not return those host-owned fields.
With no visual reference, every visual attribute is request-governed. Otherwise the reference governs every attribute the user has not actually specified. Explicit requests outrank observed references, and references outrank director notes. A director only fills unresolved space. Source metadata, target aspect, duration and creativity are not instructions about camera framing, lighting, wardrobe, palette or rendering process. A stale Asset.Description is caller commentary; actual Card observations govern reference appearance. Independent reference_readings are real supplied readings even when this analysis attaches no conditioning media.
Action: a requested new action or performed behavior overrides a subject plate's static pose, including an explicit vocal performance or speaking action. Action authority concerns what the figure is asked to do, not only locomotion; a performance request does not need physical travel. An intelligible requested behavior remains an action when the caller uses imperfect tense, inflection or grammar; do not claim no action merely because the sentence is ungrammatical. Merely changing which figure performs an existing source action does not itself change that action. Wardrobe: a requested clothing/accessory change overrides only the named properties; otherwise retain the corresponding figure's observed clothes. Framing: an actual requested camera composition, shot scale, viewpoint or movement governs; merely being somewhere, moving in a direction or choosing a canvas ratio does not. Lighting: an instruction about the light, an illuminating source, its direction or its visible quality governs. Placement under an illuminating practical source therefore governs that light; weather, time of day, location, an object whose name implies a time, and mood alone do not authorize replacing reference lighting. Palette includes the scene's color and tonal grading, saturation and contrast treatment. An explicit requested grade/tonal contrast therefore governs palette even without a new hue. An object's incidental color does not recolor or regrade the scene.
In an edit, the edit_source clip owns the scene's observed appearance before incoming identity portraits. Incoming replacement/placement images supply identity; their studio lighting/backdrop must not replace the clip's world. For a non-frame content reference, sheet panels, labels, multiple views and posing are presentation, not the new scene or identity. For actual_frame sources, the entire supplied image is the target frame at its anchored instant: preserve those pixels, including their complete layout and source background. Never authorize sheet cleanup on an actual frame merely because it is a sheet. An observed pose is licensed only for an actual frame anchor, never as an identity attribute. Keep rendering restyling independent of retention relationship: changing a medium does not itself select attribute_transfer.
Target identities are already resolved by identity_requirements. Those are TARGET descriptors and properties after explicit caller requirements; original observed_subjects remain the SOURCE. Do not reassert a conflicting source gender/species/age/wardrobe as a request error. Neither re-author those identity decisions nor return identity fields. Resolve only the remaining attribute owners and their reasons.`
	var diagnostics []contextir.Issue
	for attempt := 1; attempt <= c.options.MaxStructureRetries+1; attempt++ {
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "licence", "context", "Reference licence interpretation was canceled.", ctx.Err())
		}
		safe := b
		safe.Director = ""
		safe.DirectorProfile = nil
		safe.Evidence = nil
		director, dirErr := resolveDirector(b)
		if dirErr != nil {
			return out, dirErr
		}
		notes := ""
		if director != nil {
			notes = director.Notes
		}
		input, _ := json.Marshal(map[string]any{"version": Version, "reference_permissions": visualOwners, "medium_decision": medium, "reference_readings": readings, "brief": safe, "reference_manifest": manifest, "observed_subjects": out.Subjects, "identity_requirements": out.Identity, "director_notes": notes, "diagnostics": diagnostics})
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "licence", System: system, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		out.Calls = append(out.Calls, planner.CallUsage{Stage: "licence", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "licence", "context", "Reference licence interpretation was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return out, failure(contextir.CodeModel, "licence", "model", "Reference licence interpretation failed.", callErr)
		}
		if !valid {
			return out, failure(contextir.CodeModelOutput, "licence", "usage", "Reference licence interpretation returned invalid usage.", callErr)
		}
		if callErr != nil {
			if repairableModelOutput(callErr) && attempt <= c.options.MaxStructureRetries {
				diagnostics = repairDiagnostics(callErr, "licence", "Return a complete valid licence object.")
				continue
			}
			return out, failure(modelFailureCode(callErr), "licence", "model", "Reference licence interpretation failed.", callErr)
		}
		var reply struct {
			Governs map[string]string `json:"governs"`
			Reasons map[string]string `json:"reasons"`
		}
		err = validateWire(response.JSON, schema)
		if err == nil {
			err = strictDecode(response.JSON, &reply)
		}
		if err == nil {
			out.Governs = reply.Governs
			out.Reasons = reply.Reasons
			out.TransformTarget = medium.Target
			out.Style = resolvedVisualStyle(b, medium, readings, reply.Governs)
			if medium.OperationQuote != "" {
				quote := medium.OperationQuote
				out.TransformPhrase = &quote
			}
			out.MediumTransferred = visual && medium.Disposition == "convert"
			return out, nil
		}
		diagnostics = repairDiagnostics(err, "licence", "Return a complete licence matching the observed references and explicit request.")
	}
	return out, failure(contextir.CodeModelOutput, "licence", "licence", "Reference licence interpretation did not satisfy its typed contract.", err)
}

// A media track's presence does not license all of its contents. These are
// declared responsibility boundaries, not interpretations of caller prose.
func roleLicencesVisual(role Role, attribute string) bool {
	switch role {
	case RoleStructure, RoleStoryboard:
		return attribute == "framing"
	case RoleStyle, RoleEnvironment:
		return attribute == "medium" || attribute == "lighting" || attribute == "framing" || attribute == "palette"
	case RoleVoiceTimbre, RoleBGM, RoleMusicStyle, RoleBeatReference, RoleSFX:
		return false
	default:
		return true
	}
}
