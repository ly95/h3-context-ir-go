package contextir

import (
	"encoding/hex"
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Inspect returns deterministic diagnostics containing field paths and safe
// descriptions only. It never embeds submitted text, IDs, or media locators.
func Inspect(d Document) []Issue {
	v := newValidation()
	if d.Version != Version {
		v.issues = append(v.issues, Issue{Code: CodeVersion, Path: "version", Message: "unsupported document version"})
	}
	v.mode(d.Mode)
	v.target(d.Target)
	v.loras(d.LoRAs, d.Mode)
	v.assetsCheck(d.Assets)
	v.constraintsCheck(d.Constraints)
	v.modeAssets(d.Mode, d.Assets)
	for i, s := range d.Subjects {
		v.addID(v.subjects, s.ID, vp("subjects", i)+".id", i)
	}
	for i, r := range d.References {
		v.addID(v.refs, r.ID, vp("references", i)+".id", i)
	}
	for i, s := range d.Speakers {
		v.addID(v.speakers, s.ID, vp("speakers", i)+".id", i)
	}
	for i, s := range d.Shots {
		v.addID(v.shots, s.ID, vp("shots", i)+".id", i)
	}
	v.evidenceCheck(d.Evidence)
	for i, s := range d.Subjects {
		v.subject(s, vp("subjects", i), d)
	}
	for i, r := range d.References {
		v.reference(r, vp("references", i), d)
	}
	v.soundtrackPairs(d)
	for i, s := range d.Speakers {
		v.speaker(s, vp("speakers", i), d)
	}
	v.dialogueCheck(d.Dialogue, true)
	v.rich(d.Summary, "summary", false)
	v.text(d.Style, "style", false)
	v.text(d.Soundscape, "soundscape", false)
	v.text(d.Music, "music", false)
	v.shotsCheck(d)
	v.modeReferences(d)
	v.claimsCheck(d.Claims)
	v.constraintConflicts(d)
	return v.issues
}

// Validate checks the provider-neutral document contract without IO or mutation.
func Validate(d Document) error { return validationError(Inspect(d), "validation") }

// ValidateEvidence validates independently supplied perception output before a
// model consumes it. Coverage of requested assets is a caller policy.
func ValidateEvidence(assets []Asset, evidence []Evidence) error {
	v := newValidation()
	v.assetsCheck(assets)
	v.evidenceCheck(evidence)
	return validationError(v.issues, "evidence")
}

// ValidateRequest validates caller-owned inputs before perception or model work.
func ValidateRequest(r Request) error {
	v := newValidation()
	v.text(r.Instruction, "instruction", true)
	v.mode(r.Mode)
	v.target(r.Target)
	v.loras(r.LoRAs, r.Mode)
	v.assetsCheck(r.Assets)
	v.modeAssets(r.Mode, r.Assets)
	v.bindingsCheck(r)
	v.constraintsCheck(r.Constraints)
	v.dialogueCheck(r.Dialogue, false)
	v.requestControls(r)
	if v.constraints[ConstraintSilence] || v.constraints[ConstraintNoDialogue] {
		if len(r.Dialogue) != 0 {
			v.bad("dialogue", "dialogue conflicts with an explicit constraint")
		}
	}
	return validationError(v.issues, "request")
}

func validationError(issues []Issue, stage string) error {
	if len(issues) == 0 {
		return nil
	}
	code := CodeInvalid
	for _, issue := range issues {
		if issue.Code == CodeVersion {
			code = CodeVersion
			break
		}
	}
	return &Error{Code: code, Stage: stage, Issues: issues}
}

type validationState struct {
	issues                                 []Issue
	assets                                 map[string]Asset
	subjects, refs, speakers, shots, lines map[string]int
	facts                                  map[string]Fact
	constraints                            map[ConstraintKind]bool
}

func newValidation() *validationState {
	return &validationState{assets: map[string]Asset{}, subjects: map[string]int{}, refs: map[string]int{}, speakers: map[string]int{}, shots: map[string]int{}, lines: map[string]int{}, facts: map[string]Fact{}, constraints: map[ConstraintKind]bool{}}
}
func vp(field string, i int) string { return field + "[" + strconv.Itoa(i) + "]" }
func (v *validationState) bad(path, message string) {
	v.issues = append(v.issues, Issue{Code: CodeInvalid, Path: path, Message: message})
}
func (v *validationState) text(s, path string, required bool) {
	if !utf8.ValidString(s) {
		v.bad(path, "must be valid UTF-8")
		return
	}
	if required && strings.TrimSpace(s) == "" {
		v.bad(path, "must not be empty")
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			v.bad(path, "contains a prohibited control character")
			break
		}
	}
}
func (v *validationState) id(s, path string) {
	if s == "" || !utf8.ValidString(s) {
		v.bad(path, "must be a non-empty UTF-8 identifier")
		return
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' {
			v.bad(path, "identifier contains a prohibited character")
			break
		}
	}
}
func (v *validationState) addID(dst map[string]int, s, path string, i int) {
	v.id(s, path)
	if _, found := dst[s]; found {
		v.bad(path, "identifier must be unique in its collection")
	} else {
		dst[s] = i
	}
}
func (v *validationState) ids(ids []string, path string, known map[string]int) {
	seen := map[string]bool{}
	for i, id := range ids {
		p := vp(path, i)
		v.id(id, p)
		if seen[id] {
			v.bad(p, "identifier must not repeat")
		}
		seen[id] = true
		if known != nil {
			if _, ok := known[id]; !ok {
				v.bad(p, "references an unknown identifier")
			}
		}
	}
}
func (v *validationState) mode(m Mode) {
	switch m {
	case ModeText, ModeFirstFrame, ModeLastFrame, ModeFirstLastFrame, ModeReference:
	default:
		v.bad("mode", "unsupported mode")
	}
}
func (v *validationState) rational(r Rational, path string, optional bool) bool {
	if optional && r.Numerator == 0 && r.Denominator == 0 {
		return true
	}
	if r.Numerator <= 0 || r.Denominator <= 0 {
		v.bad(path, "numerator and denominator must both be positive")
		return false
	}
	return true
}
func (v *validationState) dimensions(w, h int, path string) {
	if w < 0 || h < 0 || (w == 0) != (h == 0) {
		v.bad(path, "width and height must both be positive or both omitted")
	}
	if w > 0 && h > 0 && w > int(^uint(0)>>1)/h {
		v.bad(path, "pixel area exceeds the representable dimension budget")
	}
}
func (v *validationState) target(t Target) {
	if t.Duration <= 0 {
		v.bad("target.duration_ms", "must be positive")
	}
	fpsOK := v.rational(t.FPS, "target.fps", false)
	v.dimensions(t.Width, t.Height, "target")
	v.text(t.AspectRatio, "target.aspect_ratio", true)
	if t.AspectRatio != "adaptive" && t.AspectRatio != "auto" {
		parts := strings.Split(t.AspectRatio, ":")
		valid := len(parts) == 2
		for _, part := range parts {
			n, err := strconv.ParseUint(part, 10, 32)
			if err != nil || n == 0 {
				valid = false
			}
		}
		if !valid {
			v.bad("target.aspect_ratio", "must be a positive ratio or an adaptive ratio")
		}
	}
	if t.FrameCount < 0 {
		v.bad("target.frame_count", "must not be negative")
	}
	if t.FrameCount > 0 && fpsOK {
		// Use exact arithmetic so even maximal caller-owned integers cannot wrap.
		n := new(big.Int).Mul(big.NewInt(t.FrameCount), big.NewInt(t.FPS.Denominator))
		n.Mul(n, big.NewInt(1000))
		den := big.NewInt(t.FPS.Numerator)
		q, rem := new(big.Int), new(big.Int)
		q.QuoRem(n, den, rem)
		if rem.Mul(rem, big.NewInt(2)).Cmp(den) >= 0 {
			q.Add(q, big.NewInt(1))
		}
		if !q.IsInt64() || q.Int64() != int64(t.Duration) {
			v.bad("target.duration_ms", "must equal the nearest-millisecond frame playback duration")
		}
	}
}
func (v *validationState) assetsCheck(assets []Asset) {
	for i, a := range assets {
		p := vp("assets", i)
		v.id(a.ID, p+".id")
		if _, exists := v.assets[a.ID]; exists {
			v.bad(p+".id", "asset identifier must be unique")
		} else {
			v.assets[a.ID] = a
		}
		switch a.Kind {
		case MediaImage, MediaVideo, MediaAudio:
		default:
			v.bad(p+".kind", "unsupported media kind")
		}
		v.text(a.URI, p+".uri", false)
		v.text(a.Description, p+".description", false)
		if a.SHA256 != "" {
			b, err := hex.DecodeString(a.SHA256)
			if err != nil || len(b) != 32 {
				v.bad(p+".sha256", "must encode a 32-byte SHA-256 digest")
			}
		}
		m := a.Metadata
		prep := a.Preparation
		v.dimensions(prep.Width, prep.Height, p+".preparation")
		switch prep.Sizing {
		case "", "match", "max":
		default:
			v.bad(p+".preparation.sizing", "unsupported explicit conditioning sizing policy")
		}
		if prep.Frames < 0 || a.Kind != MediaVideo && prep.Frames != 0 {
			v.bad(p+".preparation.frames", "a nonnegative conditioning frame count is valid only for a video")
		}
		if a.Kind == MediaAudio && prep != (MediaPreparation{}) {
			v.bad(p+".preparation", "audio conditioning cannot carry visual preparation instructions")
		}
		if m.Duration < 0 {
			v.bad(p+".metadata.duration_ms", "must not be negative")
		}
		v.dimensions(m.Width, m.Height, p+".metadata")
		v.rational(m.FPS, p+".metadata.fps", true)
		if a.Kind == MediaImage && (m.Duration != 0 || m.FPS != (Rational{})) {
			v.bad(p+".metadata", "still images cannot declare a duration or frame rate")
		}
		if a.Kind == MediaAudio && (m.Width != 0 || m.Height != 0 || m.FPS != (Rational{})) {
			v.bad(p+".metadata", "audio cannot declare visual dimensions or frame rate")
		}
	}
}
func (v *validationState) modeAssets(mode Mode, assets []Asset) {
	switch mode {
	case ModeText:
		if len(assets) != 0 {
			v.bad("assets", "text mode cannot contain media assets")
		}
	case ModeFirstFrame, ModeLastFrame, ModeFirstLastFrame:
		want := 1
		if mode == ModeFirstLastFrame {
			want = 2
		}
		if len(assets) != want {
			v.bad("assets", "keyframe mode requires its exact number of image assets")
		}
		for i, a := range assets {
			if a.Kind != MediaImage {
				v.bad(vp("assets", i)+".kind", "keyframe mode requires image assets")
			}
		}
	case ModeReference:
		if len(assets) == 0 {
			v.bad("assets", "reference mode requires at least one media asset")
		}
	}
}
func (v *validationState) source(s SourceRef, path string) {
	v.id(s.AssetID, path+".asset_id")
	a, found := v.assets[s.AssetID]
	if !found {
		v.bad(path+".asset_id", "references an unknown asset")
	}
	if s.Track != TrackVisual && s.Track != TrackAudio {
		v.bad(path+".track", "unsupported source track")
	}
	if found && ((a.Kind == MediaImage && s.Track != TrackVisual) || (a.Kind == MediaAudio && s.Track != TrackAudio)) {
		v.bad(path+".track", "track is incompatible with the source media kind")
	}
	if s.Interval != nil {
		if s.Interval.Start < 0 || s.Interval.End <= s.Interval.Start {
			v.bad(path+".interval", "must be a non-empty non-negative interval")
		}
		if found && a.Kind == MediaImage {
			v.bad(path+".interval", "a still image cannot have a temporal interval")
		}
		if found && a.Metadata.Duration > 0 && s.Interval.End > a.Metadata.Duration {
			v.bad(path+".interval", "extends beyond the declared source duration")
		}
	}
}
func (v *validationState) evidenceCheck(evidence []Evidence) {
	seen := map[string]bool{}
	for i, e := range evidence {
		if e.Card != nil {
			v.card(*e.Card, e)
		}
		p := vp("evidence", i)
		v.id(e.AssetID, p+".asset_id")
		if _, exists := v.assets[e.AssetID]; !exists {
			v.bad(p+".asset_id", "references an unknown asset")
		}
		if seen[e.AssetID] {
			v.bad(p+".asset_id", "evidence for an asset must be unique")
		}
		seen[e.AssetID] = true
		v.text(e.Model, p+".model", false)
		v.text(e.AnalyzerVersion, p+".analyzer_version", true)
		for j, limitation := range e.Limitations {
			v.text(limitation, vp(p+".limitations", j), true)
		}
		checkedTracks := v.coverage(e, p+".coverage")
		for j, f := range e.Facts {
			q := vp(p+".facts", j)
			v.id(f.ID, q+".id")
			v.text(f.Text, q+".text", true)
			v.text(f.AnalyzerVersion, q+".analyzer_version", false)
			v.source(f.Source, q+".source")
			if f.Source.AssetID != e.AssetID {
				v.bad(q+".source.asset_id", "fact source must match its evidence asset")
			}
			if e.Coverage != nil && !checkedTracks[f.Source.Track] {
				v.bad(q+".source.track", "facts must use a track declared as inspected by coverage")
			}
			if _, exists := v.facts[f.ID]; exists {
				v.bad(q+".id", "fact identifier must be unique")
			} else {
				v.facts[f.ID] = f
			}
		}
	}
}

// Coverage records an adapter's explicit declaration. It cannot establish that
// a model actually inspected the media or that its observations are accurate.
func (v *validationState) coverage(e Evidence, path string) map[Track]bool {
	checked := map[Track]bool{}
	if e.Coverage == nil {
		return checked
	}
	c := e.Coverage
	if c.Status != CoverageComplete && c.Status != CoveragePartial {
		v.bad(path+".status", "unsupported coverage status")
	}
	a, known := v.assets[e.AssetID]
	allowed := func(track Track) bool {
		if track != TrackVisual && track != TrackAudio {
			return false
		}
		if !known {
			return true
		}
		return a.Kind == MediaVideo || (a.Kind == MediaImage && track == TrackVisual) || (a.Kind == MediaAudio && track == TrackAudio)
	}
	for i, track := range c.Tracks {
		p := vp(path+".tracks", i)
		if !allowed(track) {
			v.bad(p, "inspected track is incompatible with the media kind")
		}
		if checked[track] {
			v.bad(p, "inspected track must not repeat")
		}
		checked[track] = true
	}
	absent := map[Track]bool{}
	for i, track := range c.AbsentTracks {
		p := vp(path+".absent_tracks", i)
		if track != TrackAudio || (known && a.Kind != MediaVideo) {
			v.bad(p, "only a video's optional audio track may be declared absent")
		}
		if absent[track] {
			v.bad(p, "absent track must not repeat")
		}
		absent[track] = true
		if checked[track] {
			v.bad(p, "a track cannot be both inspected and absent")
		}
	}
	if len(c.Tracks) == 0 && len(c.AbsentTracks) == 0 {
		v.bad(path+".tracks", "coverage must declare inspected or explicitly absent tracks")
	}
	if known && c.Status == CoverageComplete {
		switch a.Kind {
		case MediaImage:
			if !checked[TrackVisual] {
				v.bad(path+".tracks", "complete image coverage must inspect the visual track")
			}
		case MediaAudio:
			if !checked[TrackAudio] {
				v.bad(path+".tracks", "complete audio coverage must inspect the audio track")
			}
		case MediaVideo:
			if !checked[TrackVisual] || (!checked[TrackAudio] && !absent[TrackAudio]) {
				v.bad(path+".tracks", "complete video coverage must inspect visuals and inspect or probe the audio track")
			}
		}
	}
	for i, at := range c.SampleTimes {
		p := vp(path+".sample_times_ms", i)
		if known && a.Kind == MediaImage {
			v.bad(p, "still-image coverage cannot declare temporal samples")
		}
		if at < 0 || (known && a.Metadata.Duration > 0 && at >= a.Metadata.Duration) {
			v.bad(p, "sample time must fall within the source timeline")
		}
		if i > 0 && at <= c.SampleTimes[i-1] {
			v.bad(p, "sample times must be strictly increasing")
		}
	}
	for i, interval := range c.Intervals {
		p := vp(path+".intervals", i)
		if known && a.Kind == MediaImage {
			v.bad(p, "still-image coverage cannot declare temporal intervals")
		}
		if interval.Start < 0 || interval.End <= interval.Start || (known && a.Metadata.Duration > 0 && interval.End > a.Metadata.Duration) {
			v.bad(p, "coverage interval must be non-empty and within the source timeline")
		}
		if i > 0 && interval.Start < c.Intervals[i-1].End {
			v.bad(p, "coverage intervals must be ordered and non-overlapping")
		}
	}
	return checked
}

func (v *validationState) bindingsCheck(r Request) {
	if len(r.Bindings) == 0 {
		return
	}
	first, last := 0, 0
	usedAssets := map[string]bool{}
	for i, binding := range r.Bindings {
		p := vp("bindings", i)
		v.source(binding.Source, p+".source")
		usedAssets[binding.Source.AssetID] = true
		a, known := v.assets[binding.Source.AssetID]
		if (binding.Source.Track == TrackVisual && !visualRole(binding.Role)) || (binding.Source.Track == TrackAudio && !audioRole(binding.Role)) {
			v.bad(p+".role", "binding role is incompatible with the source track")
		}
		if !visualRole(binding.Role) && !audioRole(binding.Role) {
			v.bad(p+".role", "unsupported binding role")
		}
		frame := binding.Role == RoleFirstFrame || binding.Role == RoleLastFrame || binding.Role == RoleKeyframe
		if frame && (binding.Source.Track != TrackVisual || (known && a.Kind != MediaImage)) {
			v.bad(p+".source", "frame bindings require a still-image visual source")
		}
		if (binding.Role == RoleEditSource || binding.Role == RoleContinuation) && (binding.Source.Track != TrackVisual || (known && a.Kind != MediaVideo)) {
			v.bad(p+".source", "whole-video bindings require a video visual source")
		}
		if frame && binding.Anchor == nil {
			v.bad(p+".anchor_ms", "frame bindings require an explicit target anchor")
		}
		if binding.Anchor != nil {
			if !frame {
				v.bad(p+".anchor_ms", "only frame bindings may have a target anchor")
			}
			if *binding.Anchor < 0 || *binding.Anchor > r.Target.Duration {
				v.bad(p+".anchor_ms", "anchor must fall within the target timeline")
			}
			if binding.Role == RoleFirstFrame && *binding.Anchor != 0 {
				v.bad(p+".anchor_ms", "first frame must anchor the start")
			}
			if binding.Role == RoleLastFrame && *binding.Anchor != r.Target.Duration {
				v.bad(p+".anchor_ms", "last frame must anchor the end")
			}
		}
		if binding.Role == RoleFirstFrame {
			first++
			if first > 1 {
				v.bad(p+".role", "first-frame responsibility must be unique")
			}
		}
		if binding.Role == RoleLastFrame {
			last++
			if last > 1 {
				v.bad(p+".role", "last-frame responsibility must be unique")
			}
		}
		if r.Mode == ModeReference && (binding.Role == RoleFirstFrame || binding.Role == RoleLastFrame) {
			v.bad(p+".role", "reference mode uses explicit keyframes rather than first/last-frame mode roles")
		}
		if r.Mode != ModeReference && binding.Role != RoleFirstFrame && binding.Role != RoleLastFrame {
			v.bad(p+".role", "base-mode bindings may only assign first/last-frame responsibilities")
		}
		for j := 0; j < i; j++ {
			previous := r.Bindings[j]
			sameAnchor := binding.Anchor == nil && previous.Anchor == nil || binding.Anchor != nil && previous.Anchor != nil && *binding.Anchor == *previous.Anchor
			if previous.Role == binding.Role && sameAnchor && sameValidationSource(previous.Source, binding.Source) {
				v.bad(p, "source responsibility must not repeat")
				break
			}
		}
	}
	for i, asset := range r.Assets {
		if !usedAssets[asset.ID] {
			v.bad(vp("assets", i), "explicit bindings must assign a responsibility to every input asset")
		}
	}
	switch r.Mode {
	case ModeText:
		v.bad("bindings", "text mode cannot contain media bindings")
	case ModeFirstFrame:
		if first != 1 || last != 0 || len(r.Bindings) != 1 {
			v.bad("bindings", "first-frame mode requires exactly one first-frame binding")
		}
	case ModeLastFrame:
		if first != 0 || last != 1 || len(r.Bindings) != 1 {
			v.bad("bindings", "last-frame mode requires exactly one last-frame binding")
		}
	case ModeFirstLastFrame:
		if first != 1 || last != 1 || len(r.Bindings) != 2 {
			v.bad("bindings", "first/last-frame mode requires exactly one binding for each endpoint")
		}
	}
}
func visualRole(role Role) bool {
	switch role {
	case RoleIdentity, RoleAppearance, RoleClothing, RoleScene, RoleStyle, RoleAction, RolePose, RoleCamera, RoleStoryboard, RoleFirstFrame, RoleLastFrame, RoleKeyframe, RoleEditSource, RoleContinuation, RoleStructure, RolePlacedSubject, RoleReplacementSubject:
		return true
	}
	return false
}
func audioRole(role Role) bool {
	switch role {
	case RoleVoice, RoleMusic, RoleRhythm, RoleSoundtrack, RoleSoundEffect, RoleMusicStyle, RoleBeatReference, RoleSoundTexture:
		return true
	}
	return false
}
func visualRetention(r Retention) bool {
	return r == FullyPreserved || r == PartiallyPreserved || r == AttributeTransfer || r == WeakReference
}
func audioRetention(r Retention) bool {
	return r == FullyCopy || r == PartiallyCopy || r == AudioReference || r == WeakReference
}
func hasRole(roles []Role, role Role) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
func (v *validationState) subject(s Subject, p string, d Document) {
	v.text(s.Description, p+".description", true)
	v.text(s.Descriptor, p+".descriptor", false)
	if s.Kind != "" {
		switch s.Kind {
		case "subject", "person", "animal", "object", "environment", "style", "action":
		default:
			v.bad(p+".kind", "unsupported subject kind")
		}
	}
	for i, text := range s.Attributes {
		v.text(text, vp(p+".attributes", i), true)
	}
	for i, text := range s.Pose {
		v.text(text, vp(p+".pose", i), true)
	}
	if len(s.Pose) > 0 && !s.PoseLicensed {
		v.bad(p+".pose", "target pose requires an explicit pose license; unlicensed observations belong in Evidence.Card")
	}
	switch s.Origin {
	case "", "observed", "user", "creative", "user_symbolic":
	default:
		v.bad(p+".origin", "unsupported subject origin")
	}
	v.text(s.RetentionDetails, p+".retention_details", false)
	v.ids(s.ShotIDs, p+".shot_ids", v.shots)
	if s.TakenOverBy != "" {
		v.id(s.TakenOverBy, p+".taken_over_by")
		if _, exists := v.subjects[s.TakenOverBy]; !exists || s.TakenOverBy == s.ID {
			v.bad(p+".taken_over_by", "transfer must identify a different existing target subject")
		}
		if s.Retention != AttributeTransfer {
			v.bad(p+".retention", "a transferred subject requires attribute_transfer")
		}
	}
	if len(s.Sources) > 0 && !visualRetention(s.Retention) {
		v.bad(p+".retention", "referenced subjects require a visual retention relationship")
	}
	if len(s.Sources) == 0 && s.Retention != "" {
		v.bad(p+".retention", "a retention relationship requires a source")
	}
	for i, attribution := range s.Sources {
		q := vp(p+".sources", i)
		v.source(attribution.Source, q+".source")
		v.text(attribution.Details, q+".details", false)
		if attribution.Source.Track != TrackVisual || !visualRole(attribution.Role) {
			v.bad(q+".role", "subject attribution must describe a visual source role")
		}
		if attribution.Role == RoleFirstFrame || attribution.Role == RoleLastFrame || attribution.Role == RoleKeyframe || attribution.Role == RoleStructure {
			v.bad(q+".role", "frame anchors and whole-video relationships require a reference")
		}
		if attribution.Role == RoleEditSource || attribution.Role == RoleContinuation {
			found := false
			for _, ref := range d.References {
				if ref.Kind == ReferenceVideo && hasRole(ref.Roles, attribution.Role) && sameValidationSource(ref.Source, attribution.Source) {
					found = true
				}
			}
			if !found {
				v.bad(q+".role", "video-owned subjects require the corresponding explicit whole-video reference")
			}
		}
		v.ids(attribution.FactIDs, q+".fact_ids", nil)
		for j, id := range attribution.FactIDs {
			fact, found := v.facts[id]
			if !found {
				v.bad(vp(q+".fact_ids", j), "references an unknown evidence fact")
				continue
			}
			if !sameValidationSource(attribution.Source, fact.Source) {
				v.bad(vp(q+".fact_ids", j), "fact provenance is incompatible with the attribution source")
			}
		}
	}
}
func sameValidationSource(a, b SourceRef) bool {
	if a.AssetID != b.AssetID || a.Track != b.Track {
		return false
	}
	if a.Interval == nil || b.Interval == nil {
		return a.Interval == nil && b.Interval == nil
	}
	return *a.Interval == *b.Interval
}
func (v *validationState) reference(r Reference, p string, d Document) {
	v.source(r.Source, p+".source")
	v.text(r.Description, p+".description", true)
	v.text(r.RetentionDetails, p+".retention_details", false)
	v.ids(r.ShotIDs, p+".shot_ids", v.shots)
	v.text(r.Replaces, p+".replaces", false)
	if r.Replaces != "" && !hasRole(r.Roles, RoleReplacementSubject) {
		v.bad(p+".replaces", "only replacement_subject may identify a replaced source figure")
	}
	if hasRole(r.Roles, RoleReplacementSubject) && strings.TrimSpace(r.Replaces) == "" {
		resolved := false
		for _, original := range d.Subjects {
			if original.TakenOverBy != "" {
				for _, incoming := range d.Subjects {
					if incoming.ID == original.TakenOverBy {
						for _, source := range incoming.Sources {
							if source.Source.AssetID == r.Source.AssetID {
								resolved = true
							}
						}
					}
				}
			}
		}
		if !resolved {
			v.bad(p+".replaces", "replacement_subject requires an explicit source figure or a structurally resolved subject transfer")
		}
	}
	if hasRole(r.Roles, RolePlacedSubject) || hasRole(r.Roles, RoleReplacementSubject) {
		edit := false
		for _, other := range d.References {
			if other.Kind == ReferenceVideo && hasRole(other.Roles, RoleEditSource) {
				edit = true
			}
		}
		if !edit {
			v.bad(p+".roles", "placement and replacement require an edit-source video")
		}
		if r.Kind != ReferencePicture {
			v.bad(p+".kind", "placement and replacement require a picture reference")
		}
	}
	if hasRole(r.Roles, RoleStructure) && r.Kind != ReferenceVideo {
		v.bad(p+".kind", "structure requires a video reference")
	}
	if r.PairedVideoID != "" {
		paired, exists := v.assets[r.PairedVideoID]
		if r.Kind != ReferenceAudio || !exists || paired.Kind != MediaVideo {
			v.bad(p+".paired_video_id", "paired soundtrack must identify an existing video asset")
		}
	}
	a, exists := v.assets[r.Source.AssetID]
	switch r.Kind {
	case ReferencePicture:
		if r.Source.Track != TrackVisual || (exists && a.Kind != MediaImage) {
			v.bad(p+".source", "picture references require a still-image visual source")
		}
	case ReferenceVideo:
		if r.Source.Track != TrackVisual || (exists && a.Kind != MediaVideo) {
			v.bad(p+".source", "video references require a video visual source")
		}
	case ReferenceAudio:
		if r.Source.Track != TrackAudio {
			v.bad(p+".source.track", "audio references require an explicit audio source track")
		}
	default:
		v.bad(p+".kind", "unsupported reference kind")
	}
	if r.Kind == ReferenceAudio {
		if !audioRetention(r.Retention) {
			v.bad(p+".retention", "audio reference requires an audio retention relationship")
		}
	} else if !visualRetention(r.Retention) {
		v.bad(p+".retention", "visual reference requires a visual retention relationship")
	}
	if len(r.Roles) == 0 {
		v.bad(p+".roles", "must contain at least one role")
	}
	seen := map[Role]bool{}
	for i, role := range r.Roles {
		q := vp(p+".roles", i)
		if seen[role] {
			v.bad(q, "role must not repeat")
		}
		seen[role] = true
		if (r.Kind == ReferenceAudio && !audioRole(role)) || (r.Kind != ReferenceAudio && !visualRole(role)) {
			v.bad(q, "role is incompatible with the reference kind")
		}
		if r.Kind == ReferencePicture && (role == RoleEditSource || role == RoleContinuation) {
			v.bad(q, "whole-video roles require a video reference")
		}
		if r.Kind == ReferenceVideo && (role == RoleFirstFrame || role == RoleLastFrame || role == RoleKeyframe) {
			v.bad(q, "frame anchors require a picture reference")
		}
	}
	anchorRole := hasRole(r.Roles, RoleFirstFrame) || hasRole(r.Roles, RoleLastFrame) || hasRole(r.Roles, RoleKeyframe)
	if anchorRole {
		if r.Retention != FullyPreserved {
			v.bad(p+".retention", "frame anchors require fully_preserved retention")
		}
	} else if r.Kind == ReferencePicture && r.Retention == AttributeTransfer {
		v.bad(p+".retention", "picture references cannot transfer attributes; describe subject transfers on subjects")
	}
	if anchorRole && r.Anchor == nil {
		v.bad(p+".anchor_ms", "frame roles require an explicit anchor")
	}
	if r.Anchor != nil {
		if r.Kind != ReferencePicture || !anchorRole {
			v.bad(p+".anchor_ms", "anchor is only valid for a frame reference")
		}
		if *r.Anchor < 0 || *r.Anchor > d.Target.Duration {
			v.bad(p+".anchor_ms", "anchor must fall within the target timeline")
		}
		if hasRole(r.Roles, RoleFirstFrame) && *r.Anchor != 0 {
			v.bad(p+".anchor_ms", "first frame must anchor the start")
		}
		if hasRole(r.Roles, RoleLastFrame) && *r.Anchor != d.Target.Duration {
			v.bad(p+".anchor_ms", "last frame must anchor the end")
		}
		if len(r.ShotIDs) > 0 {
			for i, shot := range d.Shots {
				ownsAnchor := *r.Anchor >= shot.Start && *r.Anchor < shot.End
				if i == len(d.Shots)-1 && *r.Anchor == d.Target.Duration && shot.End == d.Target.Duration {
					ownsAnchor = true
				}
				if ownsAnchor {
					listed := false
					for _, id := range r.ShotIDs {
						if id == shot.ID {
							listed = true
							break
						}
					}
					if !listed {
						v.bad(p+".shot_ids", "must include the shot that contains the frame anchor")
					}
					break
				}
			}
		}
	}
	if hasRole(r.Roles, RoleFirstFrame) && hasRole(r.Roles, RoleLastFrame) {
		v.bad(p+".roles", "one reference cannot be both first and last frame")
	}
	if r.SpeakerID != "" {
		v.id(r.SpeakerID, p+".speaker_id")
		speakerIndex, found := v.speakers[r.SpeakerID]
		if !found {
			v.bad(p+".speaker_id", "references an unknown speaker")
		} else if d.Speakers[speakerIndex].VoiceReferenceID != r.ID {
			v.bad(p+".speaker_id", "speaker and voice reference bindings must be reciprocal")
		}
		if r.Kind != ReferenceAudio || !hasRole(r.Roles, RoleVoice) {
			v.bad(p+".speaker_id", "speaker bindings require an audio voice reference")
		}
	}
	if hasRole(r.Roles, RoleVoice) && r.SpeakerID == "" {
		v.bad(p+".speaker_id", "voice references require an explicit target speaker binding")
	}
	if hasRole(r.Roles, RoleVoice) && r.Retention != AudioReference && r.Retention != WeakReference {
		v.bad(p+".retention", "voice timbre references cannot copy the source audio signal")
	}
	if (hasRole(r.Roles, RoleMusicStyle) || hasRole(r.Roles, RoleBeatReference) || hasRole(r.Roles, RoleSoundTexture)) && r.Retention != AudioReference && r.Retention != WeakReference {
		v.bad(p+".retention", "property-only audio references cannot copy the source signal")
	}
}
func (v *validationState) speaker(s Speaker, p string, d Document) {
	v.text(s.Description, p+".description", true)
	if s.SubjectID != "" {
		v.id(s.SubjectID, p+".subject_id")
		if _, found := v.subjects[s.SubjectID]; !found {
			v.bad(p+".subject_id", "references an unknown subject")
		}
	}
	if s.VoiceReferenceID != "" {
		v.id(s.VoiceReferenceID, p+".voice_reference_id")
		i, found := v.refs[s.VoiceReferenceID]
		if !found {
			v.bad(p+".voice_reference_id", "references an unknown reference")
		} else {
			r := d.References[i]
			if r.Kind != ReferenceAudio || !hasRole(r.Roles, RoleVoice) || r.SpeakerID != s.ID {
				v.bad(p+".voice_reference_id", "must identify a reciprocally bound audio voice reference")
			}
		}
	}
}
func (v *validationState) dialogueCheck(lines []DialogueLine, resolveSpeakers bool) {
	for i, line := range lines {
		p := vp("dialogue", i)
		v.addID(v.lines, line.ID, p+".id", i)
		v.text(line.Language, p+".language", true)
		v.text(line.Text, p+".text", true)
		v.text(line.Delivery, p+".delivery", false)
		if line.Kind != "" && line.Kind != VocalSpeech && line.Kind != VocalSinging {
			v.bad(p+".kind", "unsupported vocal kind")
		}
		if line.Kind == VocalSinging && line.Voiceover {
			v.bad(p+".voiceover", "off-screen singing requires a separately supported vocal contract")
		}
		if len(line.SpeakerIDs) == 0 {
			v.bad(p+".speaker_ids", "dialogue requires at least one explicit speaker")
		}
		known := v.speakers
		if !resolveSpeakers {
			known = nil
		}
		v.ids(line.SpeakerIDs, p+".speaker_ids", known)
	}
}
func (v *validationState) rich(text RichText, path string, required bool) {
	if required && len(text) == 0 {
		v.bad(path, "must contain rich-text content")
	}
	for i, span := range text {
		p := vp(path, i)
		count := 0
		if span.Text != "" {
			count++
			v.text(span.Text, p+".text", true)
		}
		if span.SubjectID != "" {
			count++
			v.id(span.SubjectID, p+".subject_id")
			if _, found := v.subjects[span.SubjectID]; !found {
				v.bad(p+".subject_id", "references an unknown subject")
			}
		}
		if span.ReferenceID != "" {
			count++
			v.id(span.ReferenceID, p+".reference_id")
			if _, found := v.refs[span.ReferenceID]; !found {
				v.bad(p+".reference_id", "references an unknown reference")
			}
		}
		if count != 1 {
			v.bad(p, "span must set exactly one content field")
		}
	}
}

type locatedDialoguePart struct {
	part DialoguePart
	shot int
	path string
}

func (v *validationState) shotsCheck(d Document) {
	if len(d.Shots) == 0 {
		v.bad("shots", "must cover the target timeline")
	}
	parts := map[string][]locatedDialoguePart{}
	previousEnd := Time(0)
	for i, shot := range d.Shots {
		p := vp("shots", i)
		if shot.Start < 0 || shot.End <= shot.Start || shot.End > d.Target.Duration {
			v.bad(p, "shot must be a non-empty interval within the target timeline")
		}
		if shot.Start != previousEnd {
			v.bad(p+".start_ms", "shots must continuously cover the target timeline in order")
		}
		previousEnd = shot.End
		v.text(shot.Transition, p+".transition", false)
		v.rich(shot.Description, p+".description", true)
		lastAt := shot.Start
		for j, event := range shot.Events {
			q := vp(p+".events", j)
			if event.At != nil {
				if *event.At < shot.Start || *event.At >= shot.End {
					v.bad(q+".at_ms", "event onset must fall within its shot")
				}
				if *event.At < lastAt {
					v.bad(q+".at_ms", "event times must be nondecreasing in playback order")
				}
				lastAt = *event.At
			}
			v.rich(event.Description, q+".description", event.Kind != EventDialogue && event.Kind != EventVisibleText && event.Kind != EventAudioCue)
			switch event.Kind {
			case EventVisual, EventAction, EventCamera, EventSound, EventDialogue, EventVisibleText, EventAudioCue:
			default:
				v.bad(q+".kind", "unsupported event kind")
			}
			if event.Kind == EventDialogue {
				if event.Dialogue == nil {
					v.bad(q+".dialogue", "dialogue events require a dialogue part")
				}
			} else if event.Dialogue != nil {
				v.bad(q+".dialogue", "only dialogue events may contain a dialogue part")
			}
			if event.Kind == EventAudioCue {
				if event.AudioCue == nil {
					v.bad(q+".audio_cue", "audio-cue events require an existing audio phrase cue")
				}
			} else if event.AudioCue != nil {
				v.bad(q+".audio_cue", "only audio-cue events may contain an audio cue")
			}
			if event.AudioCue != nil {
				v.audioCue(*event.AudioCue, q+".audio_cue", shot.ID, d)
			}
			if event.Kind == EventVisibleText {
				v.text(event.VisibleText, q+".visible_text", true)
			} else if event.VisibleText != "" {
				v.bad(q+".visible_text", "only visible-text events may declare visible text")
			}
			if event.Dialogue != nil {
				dp := event.Dialogue
				v.id(dp.LineID, q+".dialogue.line_id")
				v.text(dp.Text, q+".dialogue.text", true)
				if _, found := v.lines[dp.LineID]; !found {
					v.bad(q+".dialogue.line_id", "references an unknown dialogue line")
				}
				parts[dp.LineID] = append(parts[dp.LineID], locatedDialoguePart{part: *dp, shot: i, path: q + ".dialogue"})
			}
		}
	}
	if len(d.Shots) > 0 && previousEnd != d.Target.Duration {
		v.bad("shots", "shots must end at the target duration")
	}
	for i, line := range d.Dialogue {
		p := vp("dialogue", i)
		list := parts[line.ID]
		if len(list) == 0 {
			v.bad(p, "dialogue line must appear on the target timeline")
			continue
		}
		var joined strings.Builder
		for j, item := range list {
			joined.WriteString(item.part.Text)
			from := j > 0 && item.shot != list[j-1].shot
			to := j+1 < len(list) && item.shot != list[j+1].shot
			if item.part.ContinuesFromPrevious != from {
				v.bad(item.path+".continues_from_previous", "continuation marker must match a preceding part across a cut")
			}
			if item.part.ContinuesToNext != to {
				v.bad(item.path+".continues_to_next", "continuation marker must match a following part across a cut")
			}
			if from && item.shot != list[j-1].shot+1 {
				v.bad(item.path, "cross-cut dialogue parts must span consecutive shots")
			}
			if item.part.CutOff && (j != len(list)-1 || item.shot != len(d.Shots)-1 || item.part.ContinuesToNext) {
				v.bad(item.path+".cut_off", "cutoff is only valid on a line's final part in the final shot")
			}
		}
		if joined.String() != line.Text {
			v.bad(p+".text", "dialogue parts must preserve the complete line byte-for-byte")
		}
	}
}

func (v *validationState) audioCue(cue AudioCue, path, shotID string, d Document) {
	v.id(cue.ReferenceID, path+".reference_id")
	v.text(cue.Language, path+".language", true)
	v.text(cue.Text, path+".text", true)
	index, exists := v.refs[cue.ReferenceID]
	if !exists {
		v.bad(path+".reference_id", "references an unknown audio reference")
		return
	}
	ref := d.References[index]
	if ref.Kind != ReferenceAudio || ref.Source.Track != TrackAudio {
		v.bad(path+".reference_id", "audio cues require an audio-track reference")
	}
	if ref.Retention != FullyCopy && ref.Retention != PartiallyCopy {
		v.bad(path+".reference_id", "existing audio phrase cues require an explicitly copied audio signal")
	}
	if _, exists := v.assets[ref.Source.AssetID]; !exists {
		v.bad(path+".reference_id", "audio cue source asset must exist")
	}
	if len(ref.ShotIDs) > 0 {
		listed := false
		for _, id := range ref.ShotIDs {
			if id == shotID {
				listed = true
				break
			}
		}
		if !listed {
			v.bad(path+".reference_id", "cue event must belong to an explicitly declared reference shot")
		}
	}
}
func (v *validationState) modeReferences(d Document) {
	first, last := 0, 0
	var firstAsset, lastAsset string
	var completeAudio *SourceRef
	usedAssets := map[string]bool{}
	for _, subject := range d.Subjects {
		for _, source := range subject.Sources {
			usedAssets[source.Source.AssetID] = true
		}
	}
	for i, r := range d.References {
		usedAssets[r.Source.AssetID] = true
		p := vp("references", i)
		if r.Kind == ReferenceAudio && r.Retention == FullyCopy {
			if completeAudio == nil {
				source := r.Source
				completeAudio = &source
			}
		}
		if hasRole(r.Roles, RoleFirstFrame) {
			first++
			firstAsset = r.Source.AssetID
		}
		if hasRole(r.Roles, RoleLastFrame) {
			last++
			lastAsset = r.Source.AssetID
		}
		if d.Mode == ModeReference && (hasRole(r.Roles, RoleFirstFrame) || hasRole(r.Roles, RoleLastFrame)) {
			v.bad(p+".roles", "reference mode cannot use first/last-frame mode roles; use explicit keyframes")
		}
		if d.Mode != ModeReference && !hasRole(r.Roles, RoleFirstFrame) && !hasRole(r.Roles, RoleLastFrame) {
			v.bad(p+".roles", "base modes only support their first/last-frame references")
		}
	}
	if completeAudio != nil {
		for i, ref := range d.References {
			if ref.Kind == ReferenceAudio && (ref.Retention == FullyCopy || ref.Retention == PartiallyCopy) && !sameValidationSource(*completeAudio, ref.Source) {
				v.bad(vp("references", i)+".retention", "a complete final audio track cannot be mixed with a copied signal from a different source")
			}
		}
	}
	switch d.Mode {
	case ModeText:
		if len(d.References) != 0 {
			v.bad("references", "text mode cannot declare media references")
		}
	case ModeFirstFrame:
		if first != 1 || last != 0 || len(d.References) != 1 {
			v.bad("references", "first-frame mode requires exactly one first-frame reference")
		}
	case ModeLastFrame:
		if first != 0 || last != 1 || len(d.References) != 1 {
			v.bad("references", "last-frame mode requires exactly one last-frame reference")
		}
	case ModeFirstLastFrame:
		if first != 1 || last != 1 || len(d.References) != 2 {
			v.bad("references", "first/last-frame mode requires one reference for each endpoint")
		}
		if firstAsset != "" && firstAsset == lastAsset {
			v.bad("references", "first and last frame must identify separate assets")
		}
	case ModeReference:
		for i, asset := range d.Assets {
			if !usedAssets[asset.ID] {
				v.bad(vp("assets", i), "reference-mode assets require an explicit subject or reference source binding")
			}
		}
	}
}
func (v *validationState) constraintsCheck(constraints []Constraint) {
	seen := map[string]int{}
	for i, c := range constraints {
		p := vp("constraints", i)
		v.addID(seen, c.ID, p+".id", i)
		v.text(c.Text, p+".text", true)
		switch c.Kind {
		case ConstraintRequirement, ConstraintProhibition, ConstraintSilence, ConstraintNoMusic, ConstraintNoAddedMusic, ConstraintNoDialogue:
			v.constraints[c.Kind] = true
		default:
			v.bad(p+".kind", "unsupported constraint kind")
		}
	}
}
func noAudioLayer(s string) bool { return strings.TrimSpace(s) == "" || strings.TrimSpace(s) == "N/A" }
func (v *validationState) constraintConflicts(d Document) {
	silence := v.constraints[ConstraintSilence]
	if (silence || v.constraints[ConstraintNoMusic]) && !noAudioLayer(d.Music) {
		v.bad("music", "music conflicts with an explicit constraint")
	}
	if v.constraints[ConstraintNoAddedMusic] && !noAudioLayer(d.Music) {
		v.bad("music", "the generated score layer is prohibited; copied music remains in its explicit audio reference")
	}
	if silence && !noAudioLayer(d.Soundscape) {
		v.bad("soundscape", "soundscape conflicts with an explicit silence constraint")
	}
	if (silence || v.constraints[ConstraintNoDialogue]) && len(d.Dialogue) > 0 {
		v.bad("dialogue", "dialogue conflicts with an explicit constraint")
	}
	for i, r := range d.References {
		if silence && r.Kind == ReferenceAudio {
			v.bad(vp("references", i), "audio reference conflicts with explicit silence")
		}
		if v.constraints[ConstraintNoMusic] && hasRole(r.Roles, RoleMusic) {
			v.bad(vp("references", i)+".roles", "music role conflicts with an explicit no-music constraint")
		}
	}
	for i, shot := range d.Shots {
		for j, e := range shot.Events {
			if (silence && (e.Kind == EventSound || e.Kind == EventDialogue || e.Kind == EventAudioCue)) || (v.constraints[ConstraintNoDialogue] && e.Kind == EventDialogue) {
				v.bad(vp(vp("shots", i)+".events", j)+".kind", "audio event conflicts with an explicit constraint")
			}
		}
	}
}
func (v *validationState) claimsCheck(claims []Claim) {
	seen := map[string]int{}
	for i, claim := range claims {
		p := vp("claims", i)
		v.addID(seen, claim.ID, p+".id", i)
		v.text(claim.Text, p+".text", true)
		switch claim.Origin {
		case OriginUser:
		case OriginObserved:
			if len(claim.Sources) == 0 {
				v.bad(p+".sources", "observed claims require source provenance")
			}
		case OriginCreative:
			if len(claim.Sources) != 0 {
				v.bad(p+".sources", "creative claims cannot be presented as source observations")
			}
		default:
			v.bad(p+".origin", "unsupported claim origin")
		}
		for j, source := range claim.Sources {
			v.source(source, vp(p+".sources", j))
		}
	}
}
