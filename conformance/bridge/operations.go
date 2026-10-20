package main

import (
	"encoding/json"
	"strings"

	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/plan"
	"github.com/ly95/contextir/tokens"
)

func operation(name string, b json.RawMessage) (any, error) {
	switch name {
	case "h3ir.draft.deterministic_draft", "h3ir.render.native_draft":
		return draftOperation(name, b)
	case "h3ir.tokens.count":
		var a struct {
			Text string `json:"text"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return tokens.Count(a.Text)
	case "h3ir.grid.Target.build":
		var a struct {
			Seconds    float64  `json:"seconds"`
			Aspect     string   `json:"aspect"`
			Canvas     *[2]int  `json:"canvas"`
			Megapixels *float64 `json:"megapixels"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return budget.Build(a.Seconds, a.Aspect, a.Canvas, a.Megapixels)
	case "h3ir.grid.align_frame_count":
		var a struct {
			N int `json:"n"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return budget.AlignFrameCount(a.N), nil
	case "h3ir.grid.video_latent_t", "h3ir.grid.audio_latent_t":
		var a struct {
			Frames int `json:"frames"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		if name == "h3ir.grid.video_latent_t" {
			return budget.VideoLatentT(a.Frames), nil
		}
		return budget.AudioLatentT(a.Frames), nil
	case "h3ir.grid.legal_frames":
		var a struct {
			Trained bool `json:"trained_only"`
			Max     int  `json:"max_frames"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return budget.LegalFrames(a.Trained, a.Max), nil
	case "h3ir.grid.frames_for_seconds":
		var a struct {
			Seconds float64 `json:"seconds"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return budget.FramesForSeconds(a.Seconds), nil
	case "h3ir.grid.rows_per_latent_frame", "h3ir.grid.adapt_canvas":
		var a struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		if name == "h3ir.grid.rows_per_latent_frame" {
			return budget.RowsPerLatentFrame(a.Width, a.Height), nil
		}
		return budget.AdaptCanvas(a.Width, a.Height)
	case "h3ir.grid.canvas_for_aspect":
		var a struct {
			Aspect     string   `json:"aspect"`
			Megapixels *float64 `json:"megapixels"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return budget.CanvasForAspect(a.Aspect, a.Megapixels)
	case "h3ir.grid.s_ss_text":
		var a struct {
			Seconds float64 `json:"seconds"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return budget.SSecondsText(a.Seconds)
	case "h3ir.grid.instruction_line_for":
		var a struct {
			Mode    string `json:"mode"`
			Last    int    `json:"last_shot"`
			Seconds string `json:"s_ss"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return budget.InstructionLineFor(a.Mode, a.Last, a.Seconds), nil
	case "h3ir.grid.ms_to_timestamp":
		var a struct {
			MS int64 `json:"ms"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return budget.MSToTimestamp(a.MS)
	case "h3ir.grid.timestamp_to_ms":
		var a struct {
			TS string `json:"ts"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return budget.TimestampToMS(a.TS)
	case "h3ir.plan.build_manifest":
		var a struct {
			Brief  plan.Brief    `json:"brief"`
			Target budget.Target `json:"target"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return plan.BuildManifest(a.Brief, a.Target)
	case "h3ir.plan.build_subjects":
		var a struct {
			Manifest []plan.ManifestEntry      `json:"manifest"`
			Cards    map[string]plan.AssetCard `json:"cards"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return plan.BuildSubjects(a.Manifest, a.Cards), nil
	case "h3ir.plan.build_speakers":
		var a struct {
			Brief    plan.Brief           `json:"brief"`
			Subjects []plan.SubjectPlan   `json:"subjects"`
			Manifest []plan.ManifestEntry `json:"manifest"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return plan.BuildSpeakers(a.Brief, a.Subjects, a.Manifest), nil
	case "h3ir.plan.allocate_shots":
		var a struct {
			Target  budget.Target       `json:"target"`
			N       int                 `json:"n_shots"`
			Mode    string              `json:"mode"`
			Options plan.ProfileOptions `json:"opts"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return plan.AllocateShots(a.Target, a.N, a.Mode, a.Options)
	case "h3ir.plan.derive_task_types":
		var a struct {
			Manifest []plan.ManifestEntry `json:"manifest"`
			Brief    plan.Brief           `json:"brief"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return plan.TaskTypes(a.Manifest, a.Brief), nil
	case "h3ir.plan.build_plan":
		var a struct {
			Brief       plan.Brief                `json:"brief"`
			Mode        string                    `json:"mode"`
			Cards       map[string]plan.AssetCard `json:"cards"`
			Proposal    []plan.ProposedShot       `json:"proposal"`
			Beats       []plan.Beat               `json:"beats"`
			SoundEvents []plan.SoundEvent         `json:"sound_events"`
			Style       string                    `json:"style_phrase"`
			Music       string                    `json:"music"`
			Options     *plan.ProfileOptions      `json:"opts"`
			LoRAs       []json.RawMessage         `json:"loras"`
			Decision    *plan.ModeDecision        `json:"mode_decision"`
			Targets     map[string]string         `json:"replacement_targets"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		return plan.Build(a.Brief, a.Mode, a.Cards, plan.BuildOptions{ReplacementTargets: a.Targets, DeferUnresolved: true, Profile: a.Options, AlignmentPolicy: budget.SnapUp, Proposal: a.Proposal, Beats: a.Beats, SoundEvents: a.SoundEvents, StylePhrase: a.Style, Music: a.Music, LoRAs: a.LoRAs, ModeDecision: a.Decision})
	}
	if strings.HasPrefix(name, "h3ir.uploads.") {
		return uploadOperation(name, b)
	}
	if strings.HasPrefix(name, "h3ir.evalloop.") {
		return evaluateOperation(name, b)
	}
	if strings.HasPrefix(name, "h3ir.analyse.") {
		return mediaOperation(name, b)
	}
	if strings.HasPrefix(name, "h3ir.service.") || strings.HasPrefix(name, "h3ir.contract.") || name == "h3ir.compile.check_request" || name == "h3ir.compile.check_capacity" {
		return serviceOperation(name, b)
	}
	if name == "h3ir.mode.infer_mode" || name == "h3ir.creativity.build" || name == "h3ir.licence.resolve_licence" || name == "h3ir.style.resolve_style" {
		return semanticOperation(name, b)
	}
	return extensionOperation(name, b)
}
