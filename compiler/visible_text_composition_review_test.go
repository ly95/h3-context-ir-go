package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func TestActualVisiblePropContinuationReceivesOriginalTextContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_visible_text_placement_v3.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "041620abc9b6bc6433603ff2073a35bc8d8e5ef9d769a07fda507b9240f7bd7f" {
		t.Fatal("actual third-wave semantic failure provenance changed")
	}
	for _, variant := range []string{"actual-visible-continuation", "wrong-shot", "changed-word", "blank-back", "unreadable-face", "unrestricted-continuation", "invented-quote"} {
		t.Run(variant, func(t *testing.T) {
			var recorded struct {
				Brief     Brief              `json:"brief"`
				Structure Structure          `json:"structure"`
				Document  contextir.Document `json:"document"`
				Prompt    string             `json:"prompt"`
			}
			if err := json.Unmarshal(raw, &recorded); err != nil {
				t.Fatal(err)
			}
			d := recorded.Document
			published, err := h3.Compile(d, h3.OfficialProfile())
			if err != nil || published.Prompt != recorded.Prompt {
				t.Fatal("the actual failed semantic candidate must remain a valid, exactly reproduced publication", err)
			}
			original := "The hanging sign remains above the platform"
			replaceSecond := func(replacement string) {
				for i := range d.Shots[1].Description {
					d.Shots[1].Description[i].Text = strings.ReplaceAll(d.Shots[1].Description[i].Text, original, replacement)
				}
			}
			var shown contextir.Event
			visibleFragmentID := ""
			for i, event := range d.Shots[0].Events {
				if event.VisibleText != "" {
					shown = event
					visibleFragmentID = fmt.Sprintf("shot:%s:visible_text:%d", d.Shots[0].ID, i)
				}
			}
			if shown.VisibleText != "出口 EXIT 07" {
				t.Fatal("real source-visible words changed")
			}
			switch variant {
			case "blank-back":
				replaceSecond("Only the blank back of the hanging sign is visible, with the lettered face entirely hidden")
			case "unreadable-face":
				replaceSecond("The hanging sign is blurred and its lettering is completely unreadable")
			case "unrestricted-continuation":
				recorded.Brief.Intent = "Show the supplied sign lettering in the station; it may remain readable in both shots."
			case "wrong-shot":
				filtered := []contextir.Event{}
				for _, event := range d.Shots[0].Events {
					if event.VisibleText == "" {
						filtered = append(filtered, event)
					}
				}
				d.Shots[0].Events = filtered
				shown.At = nil
				d.Shots[1].Events = append(d.Shots[1].Events, shown)
				replaceSecond("The same lettering is presented in a clear frontal view in the second shot")
			case "changed-word":
				for i := range d.Shots[0].Events {
					if d.Shots[0].Events[i].VisibleText != "" {
						d.Shots[0].Events[i].VisibleText = "出口 EXIT 70"
					}
				}
			}
			calls := 0
			c, err := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				var input struct {
					Intent    string            `json:"intent"`
					Fragments map[string]string `json:"fragments"`
					Visible   struct {
						Caller   []string `json:"caller_words"`
						Accepted []string `json:"accepted_words"`
						Shots    []struct {
							ID     string `json:"shot_id"`
							Events []struct {
								Text string `json:"text"`
							} `json:"visible_text_events"`
						} `json:"shots"`
					} `json:"onscreen_text_contract"`
				}
				if err := json.Unmarshal(req.Input, &input); err != nil {
					t.Fatal(err)
				}
				if req.Stage != "semantic_review" || input.Intent != recorded.Brief.Intent || len(input.Visible.Caller) != 1 || input.Visible.Caller[0] != "出口 EXIT 07" || len(input.Visible.Accepted) != 1 || input.Visible.Accepted[0] != "出口 EXIT 07" || len(input.Visible.Shots) != 2 {
					t.Fatal("critic lost independent original words, caller placement or actual shot contract", string(req.Input))
				}
				if !strings.Contains(string(req.ResponseSchema), "visible_text_placement_mismatch") || !strings.Contains(req.System, "event counts alone do not establish compliance") || !strings.Contains(req.System, "Do not demand one-shot-only display unless the caller actually requires it") {
					t.Fatal("critic lacks conservative prop-continuity/display contract")
				}
				findings := []CompositionFinding{}
				if variant == "actual-visible-continuation" || variant == "invented-quote" {
					if len(input.Visible.Shots[0].Events) != 1 || len(input.Visible.Shots[1].Events) != 0 || !strings.Contains(input.Fragments["shot:shot-2:description"], original) {
						t.Fatal("real one-event but continued-visible-prop failure was not preserved")
					}
					quote := original
					if variant == "invented-quote" {
						quote = "The following words were never in the candidate."
					}
					findings = append(findings, CompositionFinding{Code: "visible_text_placement_mismatch", FragmentID: "shot:shot-2:description", Quote: quote})
				}
				if variant == "wrong-shot" {
					if len(input.Visible.Shots[0].Events) != 0 || len(input.Visible.Shots[1].Events) != 1 {
						t.Fatal("critic actual shot mapping drifted")
					}
					findings = append(findings, CompositionFinding{Code: "visible_text_placement_mismatch", FragmentID: "shot:shot-2:description", Quote: "The same lettering is presented in a clear frontal view in the second shot"})
				}
				if variant == "changed-word" {
					if input.Visible.Shots[0].Events[0].Text != "出口 EXIT 70" {
						t.Fatal("changed final word self-authorized original contract")
					}
					findings = append(findings, CompositionFinding{Code: "visible_text_placement_mismatch", FragmentID: visibleFragmentID, Quote: "出口 EXIT 70"})
				}
				wire, _ := json.Marshal(map[string]any{"findings": findings})
				return contextir.ModelResponse{JSON: wire, Model: "explicit-typed-visible-review-double", Usage: contextir.Usage{InputTokens: 17, OutputTokens: 5}}, nil
			}), nil, Options{})
			if err != nil {
				t.Fatal(err)
			}
			review, err := c.ReviewComposition(context.Background(), recorded.Brief, recorded.Structure, d)
			if calls != 1 || len(review.Calls) != 1 || !review.Required {
				t.Fatal("visible contract did not reach one auditable semantic review", review, err)
			}
			negative := variant == "actual-visible-continuation" || variant == "wrong-shot" || variant == "changed-word" || variant == "invented-quote"
			if negative && (err == nil || review.Passed) {
				t.Fatal("visible contradiction or ungrounded finding escaped", review, err)
			}
			if !negative && (err != nil || !review.Passed) {
				t.Fatal("non-display or caller-authorized continuity was rejected", review, err)
			}
		})
	}
}

func TestVisibleTextReviewTriggerUsesTypedWordsAndEvents(t *testing.T) {
	if CompositionReviewRequired(Brief{Intent: "sign readable visible text in shot one"}, Structure{}) {
		t.Fatal("intent keywords chose the review path")
	}
	if !CompositionReviewRequired(Brief{OnscreenText: []string{"Fixed."}}, Structure{}) ||
		!CompositionReviewRequired(Brief{}, Structure{OnscreenText: []string{"Fixed."}}) ||
		!CompositionReviewRequired(Brief{}, Structure{}, contextir.Document{Shots: []contextir.Shot{{Events: []contextir.Event{{VisibleText: "Fixed."}}}}}) {
		t.Fatal("actual visible text skipped review")
	}
}
