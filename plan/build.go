package plan

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/ly95/contextir/budget"
)

var wordTargets = map[string]int{"ref2va": 400, "t2va": 330, "i2va": 460, "fl2va": 380, "l2va": 380}

func ShotCount(target budget.Target, brief Brief, mode string, opts ProfileOptions, manifest []ManifestEntry) (int, error) {
	if brief.Shots != nil && *brief.Shots != 0 {
		if *brief.Shots < 1 {
			return 0, fmt.Errorf("pinned shot count must be positive")
		}
		return *brief.Shots, nil
	}
	for _, m := range manifest {
		if m.Role == "edit_source" || m.Role == "continuation_source" {
			return 1, nil
		}
	}
	if mode == "fl2va" {
		return 1, nil
	}
	if opts.SecondsPerShot <= 0 || opts.MaxShots < 1 {
		return 0, fmt.Errorf("shot allocation profile must have positive duration and ceiling")
	}
	n := max(1, min(opts.MaxShots, int(math.RoundToEven(target.EffectiveSeconds()/opts.SecondsPerShot))))
	for n > 1 && target.EffectiveSeconds()*1000/float64(n) < MinShotMS {
		n--
	}
	return n, nil
}
func AllocateShots(target budget.Target, n int, mode string, opts ProfileOptions) ([]ShotPlan, error) {
	if n < 1 {
		return nil, fmt.Errorf("shot count must be positive")
	}
	if _, ok := wordTargets[mode]; !ok {
		return nil, fmt.Errorf("unsupported mode %q", mode)
	}
	total := int(target.EffectiveSeconds() * 1000)
	if total <= 0 {
		return nil, fmt.Errorf("target duration must be positive")
	}
	bounds := []int{0}
	for i := 1; i < n; i++ {
		raw := float64(total) * float64(i) / float64(n)
		bounds = append(bounds, max(bounds[len(bounds)-1]+MinShotMS, int(math.RoundToEven(raw/100))*100))
	}
	bounds = append(bounds, total)
	for i := len(bounds) - 2; i > 0; i-- {
		if bounds[i] >= bounds[i+1]-MinShotMS {
			bounds[i] = bounds[i+1] - MinShotMS
		}
	}
	words := wordTargets[mode]
	if opts.WordTargetOverride != nil && *opts.WordTargetOverride != 0 {
		words = *opts.WordTargetOverride
	}
	out := []ShotPlan{}
	for i := 0; i < n; i++ {
		start, end := bounds[i], bounds[i+1]
		if start < 0 || end <= start {
			return nil, fmt.Errorf("%d shots do not fit target duration", n)
		}
		share := float64(end-start) / float64(total)
		out = append(out, ShotPlan{N: i + 1, StartMS: start, EndMS: end, Subjects: []string{}, Dialogue: []DialogueLine{}, SyncSound: []string{}, OnscreenText: []string{}, WordTarget: max(MinShotWords, int(math.RoundToEven(float64(words)*share)))})
	}
	return out, nil
}
func AssignDialogue(shots []ShotPlan, lines []DialogueLine) error {
	if len(lines) == 0 {
		return nil
	}
	if len(shots) == 0 {
		return fmt.Errorf("dialogue requires a shot")
	}
	caps, used := make([]float64, len(shots)), make([]float64, len(shots))
	for i, s := range shots {
		caps[i] = math.Max(1, float64(s.EndMS-s.StartMS)/1000*WordsPerSecond)
	}
	cursor := 0
	for _, line := range lines {
		w := float64(len(strings.Fields(line.Text)))
		at := -1
		for i := cursor; i < len(shots); i++ {
			if used[i]+w <= caps[i] {
				at = i
				break
			}
		}
		if at < 0 {
			at = 0
			for i := 1; i < len(shots); i++ {
				if caps[i]-used[i] > caps[at]-used[at] {
					at = i
				}
			}
		}
		shots[at].Dialogue = append(shots[at].Dialogue, line)
		used[at] += w
		cursor = at
	}
	return nil
}
func SplitSound(events []SoundEvent) (map[int][]string, []string) {
	sync := map[int][]string{}
	ambient := []string{}
	for _, e := range events {
		text := strings.TrimSpace(e.Text)
		if text == "" {
			continue
		}
		if e.Layer == "ambient" || e.Shot == 0 {
			if !contains(ambient, text) {
				ambient = append(ambient, text)
			}
		} else {
			sync[e.Shot] = append(sync[e.Shot], text)
		}
	}
	return sync, ambient
}

func Build(brief Brief, mode string, cards map[string]AssetCard, options BuildOptions) (Plan, error) {
	if _, ok := wordTargets[mode]; !ok {
		return Plan{}, fmt.Errorf("unsupported mode %q", mode)
	}
	opts := DefaultProfile()
	if options.Profile != nil {
		opts = *options.Profile
	}
	var target budget.Target
	var err error
	switch options.AlignmentPolicy {
	case budget.SnapUp:
		target, err = budget.Build(brief.Seconds, brief.Aspect, brief.Canvas, brief.Megapixels)
	case budget.Exact:
		target, err = budget.BuildExact(brief.Seconds, brief.Aspect, brief.Canvas, brief.Megapixels)
	default:
		return Plan{}, fmt.Errorf("frame alignment policy must be explicitly exact or snap_up")
	}
	if err != nil {
		return Plan{}, err
	}
	manifest, err := BuildManifest(brief, target)
	if err != nil {
		return Plan{}, err
	}
	subjects := BuildSubjects(manifest, cards)
	var unresolved []string
	if options.DeferUnresolved {
		subjects, _, unresolved, err = BindReplacementsForInspection(subjects, manifest, options.ReplacementTargets)
	} else {
		subjects, _, err = BindReplacements(subjects, manifest, options.ReplacementTargets)
	}
	if err != nil {
		return Plan{}, err
	}
	speakers := BuildSpeakers(brief, subjects, manifest)
	var shots []ShotPlan
	if options.Proposal != nil {
		shots, err = AllocateProposal(target, options.Proposal, mode, opts)
	} else {
		var n int
		n, err = ShotCount(target, brief, mode, opts, manifest)
		if err == nil && len(options.Beats) > 0 {
			ceiling := opts.MaxShots
			if brief.Shots != nil && *brief.Shots != 0 {
				ceiling = *brief.Shots
			}
			n = max(1, min(len(options.Beats), ceiling))
		}
		if err == nil {
			shots, err = AllocateShots(target, n, mode, opts)
		}
	}
	if err != nil {
		return Plan{}, err
	}
	if err = AssignDialogue(shots, brief.Dialogue); err != nil {
		return Plan{}, err
	}
	sync, ambient := SplitSound(options.SoundEvents)
	labels := []string{}
	for _, s := range subjects {
		labels = append(labels, s.Label)
	}
	for i := range shots {
		s := &shots[i]
		s.SyncSound = copyStrings(sync[s.N])
		if options.Proposal == nil {
			if len(options.Beats) > i {
				b := options.Beats[i]
				s.Beat = strings.TrimSpace(b.Beat)
				if b.Camera != nil && b.Camera.Type != "" {
					camera := *b.Camera
					s.Camera = &camera
				}
				s.Subjects = copyStrings(b.Subjects)
			} else {
				s.Subjects = copyStrings(labels)
			}
		}
		valid := []string{}
		for _, label := range s.Subjects {
			if label != "" && contains(labels, label) {
				valid = append(valid, label)
			}
		}
		if len(valid) == 0 {
			valid = copyStrings(labels)
		}
		s.Subjects = valid
	}
	for i := range subjects {
		s := &subjects[i]
		for _, shot := range shots {
			if contains(shot.Subjects, s.Label) {
				s.AppearsIn = append(s.AppearsIn, shot.N)
			}
		}
		if len(s.AppearsIn) == 0 {
			for _, shot := range shots {
				s.AppearsIn = append(s.AppearsIn, shot.N)
			}
		}
		s.RetentionNote = RetentionNote(*s)
	}
	if len(brief.OnscreenText) > 0 && len(shots) > 0 {
		shots[0].OnscreenText = copyStrings(brief.OnscreenText)
	}
	music := strings.TrimSpace(options.Music)
	if music == "" && brief.Silent {
		music = "N/A"
	}
	loras := append([]json.RawMessage{}, options.LoRAs...)
	return Plan{UnresolvedReplacements: unresolved, Mode: mode, Target: target, Manifest: manifest, Subjects: subjects, Speakers: speakers, Shots: shots, TaskTypes: TaskTypes(manifest, brief), StylePhrase: strings.TrimSpace(options.StylePhrase), AmbientSound: ambient, Music: music, LoRAs: loras, ModeDecision: options.ModeDecision, PlannerVersion: "1"}, nil
}

// AllocateProposal accepts already validated typed creative decisions. It does
// not infer purpose, subjects, or a camera move from prose.
func AllocateProposal(target budget.Target, proposal []ProposedShot, mode string, opts ProfileOptions) ([]ShotPlan, error) {
	if len(proposal) == 0 {
		return nil, fmt.Errorf("proposal has no shots")
	}
	total := int(math.RoundToEven(target.EffectiveSeconds() * 1000))
	words, ok := wordTargets[mode]
	if !ok {
		return nil, fmt.Errorf("unsupported mode %q", mode)
	}
	if opts.WordTargetOverride != nil && *opts.WordTargetOverride != 0 {
		words = *opts.WordTargetOverride
	}
	out := []ShotPlan{}
	for i, p := range proposal {
		start, end := p.StartMS, total
		if i+1 < len(proposal) {
			end = proposal[i+1].StartMS
		}
		if start < 0 || end <= start || (i == 0 && start != 0) || end > total {
			return nil, fmt.Errorf("proposal boundaries must be increasing and within target")
		}
		beat := p.Purpose + ": " + p.Action
		if p.WhatChanges != "" {
			beat += " — what changes: " + p.WhatChanges
		}
		camera := p.Camera
		if camera != nil && camera.Type == "" {
			camera = nil
		}
		out = append(out, ShotPlan{N: i + 1, StartMS: start, EndMS: end, Beat: strings.TrimSpace(beat), Camera: camera, Subjects: copyStrings(p.Subjects), Dialogue: []DialogueLine{}, SyncSound: copyStrings(p.SyncSound), OnscreenText: []string{}, WordTarget: max(MinShotWords, int(math.RoundToEven(float64(words)*float64(end-start)/float64(total))))})
	}
	return out, nil
}
func (c CameraMove) Phrase(style string) (string, error) {
	if style == "prose" {
		return "", nil
	}
	if style != "" && style != "canonical" {
		return "", fmt.Errorf("unsupported camera style %q", style)
	}
	verbs := map[string]string{"Zoom In": "zooms in", "Zoom Out": "zooms out", "Push In": "pushes in", "Pull Out": "pulls out", "Pan Left": "pans left", "Pan Right": "pans right", "Truck Left": "trucks left", "Truck Right": "trucks right", "Tilt Up": "tilts up", "Tilt Down": "tilts down", "Pedestal Up": "rises on a pedestal move", "Pedestal Down": "lowers on a pedestal move", "Arc Shot": "arcs around the subject", "Tracking Shot": "tracks with the subject", "Static Shot": "holds a static shot", "Shake Slightly": "shakes slightly", "Shake Strongly": "shakes strongly", "POV": "takes the subject's point of view", "Roll Clockwise": "rolls clockwise", "Roll Counterclockwise": "rolls counterclockwise"}
	verb, ok := verbs[c.Type]
	if !ok {
		return "", fmt.Errorf("unsupported camera type %q", c.Type)
	}
	out := "The camera " + verb
	if c.Amplitude != nil {
		if *c.Amplitude != "small" && *c.Amplitude != "large" {
			return "", fmt.Errorf("invalid camera amplitude")
		}
		out += " with " + *c.Amplitude + " amplitude"
	}
	if c.Speed != nil {
		if *c.Speed != "slow" && *c.Speed != "fast" {
			return "", fmt.Errorf("invalid camera speed")
		}
		out += " at " + *c.Speed + " speed"
	}
	return out, nil
}
