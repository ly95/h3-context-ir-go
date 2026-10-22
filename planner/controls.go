package planner

import (
	"fmt"
	"github.com/ly95/h3-context-ir-go"
)

func validateControls(request contextir.Request, doc contextir.Document) error {
	if len(request.PinnedShots) > 0 {
		if len(request.PinnedShots) != len(doc.Shots) {
			return issueError(contextir.CodeModelOutput, "plan", "shots", "Preserve the exact pinned shot count.", nil)
		}
		for i, want := range request.PinnedShots {
			got := doc.Shots[i]
			if got.ID != want.ID || got.Start != want.Start || got.End != want.End {
				return issueError(contextir.CodeModelOutput, "plan", fmt.Sprintf("shots[%d]", i), "Preserve the pinned shot ID, order, and exact time boundaries.", nil)
			}
		}
	}
	written := map[string]int{}
	for _, shot := range doc.Shots {
		for _, event := range shot.Events {
			if event.Kind == contextir.EventVisibleText {
				written[event.VisibleText]++
			}
		}
	}
	for _, text := range request.OnscreenText {
		if written[text] == 0 {
			return issueError(contextir.CodeModelOutput, "plan", "onscreen_text", "Preserve every exact requested visible text string.", nil)
		}
		written[text]--
	}
	if (request.AdditionScope != nil || len(request.OnscreenText) > 0) && !request.AllowGeneratedVisibleText {
		for _, count := range written {
			if count > 0 {
				return issueError(contextir.CodeModelOutput, "plan", "onscreen_text", "Additional visible text requires explicit permission.", nil)
			}
		}
	}
	if scope := request.AdditionScope; scope != nil {
		if !scope.Dialogue && len(doc.Dialogue) > 0 {
			return issueError(contextir.CodeModelOutput, "plan", "dialogue", "The resolved request does not permit dialogue.", nil)
		}
		if !scope.Music && doc.Music != "N/A" {
			return issueError(contextir.CodeModelOutput, "plan", "music", "The resolved request does not permit music; use N/A.", nil)
		}
		if !scope.VisibleText {
			for _, count := range written {
				if count > 0 {
					return issueError(contextir.CodeModelOutput, "plan", "onscreen_text", "The resolved request does not permit additional visible text.", nil)
				}
			}
		}
	}
	for _, ref := range doc.References {
		for _, role := range ref.Roles {
			for _, binding := range request.Bindings {
				if binding.Role == role && key(binding.Source) == key(ref.Source) && (ref.Replaces != binding.Replaces || ref.PairedVideoID != binding.PairedVideoID) {
					return issueError(contextir.CodeModelOutput, "plan", "references", "Preserve explicit replacement and paired-soundtrack relationships.", nil)
				}
			}
		}
	}
	return nil
}
