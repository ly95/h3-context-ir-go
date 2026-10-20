package contextir

func (v *validationState) requestControls(r Request) {
	var end Time
	ids := map[string]int{}
	for i, s := range r.PinnedShots {
		p := vp("pinned_shots", i)
		v.addID(ids, s.ID, p+".id", i)
		if s.Start != end || s.End <= s.Start || s.End > r.Target.Duration {
			v.bad(p, "pinned shots must cover the target in contiguous playback order")
		}
		end = s.End
	}
	if len(r.PinnedShots) > 0 && end != r.Target.Duration {
		v.bad("pinned_shots", "pinned shots must end at the target duration")
	}
	for i, text := range r.OnscreenText {
		v.text(text, vp("onscreen_text", i), true)
	}
	if r.AdditionScope != nil {
		if !r.AdditionScope.Dialogue && (len(r.Dialogue) > 0 || r.AllowGeneratedDialogue) {
			v.bad("addition_scope.dialogue", "dialogue permission conflicts with the requested dialogue contract")
		}
		if !r.AdditionScope.VisibleText && len(r.OnscreenText) > 0 {
			v.bad("addition_scope.visible_text", "visible text permission conflicts with the supplied text")
		}
	}
}
