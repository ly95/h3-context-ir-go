package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ly95/contextir"
)

func TestActualDialoguePlacementNarrativeReceivesTypedReview(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_dialogue_placement_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "7d0503add8acf03e12d6e9b1b1384906e3d15a2898ff9e22206086e57cc842fa" {
		t.Fatal("actual first-wave candidate provenance changed")
	}
	for _, variant := range []string{"misaligned", "aligned", "invented-quote"} {
		t.Run(variant, func(t *testing.T) {
			var recorded struct {
				Brief     Brief              `json:"brief"`
				Structure Structure          `json:"structure"`
				Document  contextir.Document `json:"document"`
			}
			if err := json.Unmarshal(raw, &recorded); err != nil {
				t.Fatal(err)
			}
			d := recorded.Document
			if variant == "aligned" {
				d.Shots[0].Description = append(d.Shots[0].Description, contextir.Span{Text: " Before the cut, he turns to the listener and delivers the fixed line once."})
				d.Shots[1].Description = contextir.Text("The man lowers his gaze to the folded cloth and remains quiet while the camera holds on his face and hands.")
				d.Shots[1].Events = nil
			}
			calls := 0
			c, err := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				var input struct {
					Fragments map[string]string `json:"fragments"`
					Dialogue  struct {
						Lines []struct {
							Text string `json:"text"`
						} `json:"lines"`
						Shots []struct {
							ID    string                   `json:"shot_id"`
							Parts []contextir.DialoguePart `json:"dialogue_parts"`
						} `json:"shots"`
					} `json:"dialogue_contract"`
				}
				if err := json.Unmarshal(req.Input, &input); err != nil {
					t.Fatal(err)
				}
				if req.Stage != "semantic_review" || len(input.Dialogue.Lines) != 1 || input.Dialogue.Lines[0].Text != "We close at six, not half past." || len(input.Dialogue.Shots) != 2 || len(input.Dialogue.Shots[0].Parts) != 1 || len(input.Dialogue.Shots[1].Parts) != 0 {
					t.Fatal("critic did not receive actual fixed line and per-shot emission", string(req.Input))
				}
				if !strings.Contains(string(req.ResponseSchema), "dialogue_placement_mismatch") || !strings.Contains(req.System, "listening reaction") || !strings.Contains(req.System, "actual dialogue parts") {
					t.Fatal("typed contradiction or conservative review threshold absent")
				}
				findings := []CompositionFinding{}
				if variant != "aligned" {
					quote := "A single small nod punctuates the line"
					if !strings.Contains(input.Fragments["shot:shot-2:description"], quote) {
						t.Fatal("actual first-wave contradiction is not in the final narrative")
					}
					if variant == "invented-quote" {
						quote = "This sentence never appeared in the candidate."
					}
					findings = append(findings, CompositionFinding{Code: "dialogue_placement_mismatch", FragmentID: "shot:shot-2:description", Quote: quote, SubjectID: "subject-man"})
				}
				wire, _ := json.Marshal(map[string]any{"findings": findings})
				return contextir.ModelResponse{JSON: wire, Model: "explicit-typed-review-double", Usage: contextir.Usage{InputTokens: 17, OutputTokens: 5}}, nil
			}), nil, Options{})
			if err != nil {
				t.Fatal(err)
			}
			review, err := c.ReviewComposition(context.Background(), recorded.Brief, recorded.Structure, d)
			if calls != 1 || len(review.Calls) != 1 || !review.Required {
				t.Fatal("required actual-call ledger was lost", review, err)
			}
			if variant == "aligned" {
				if err != nil || !review.Passed {
					t.Fatal("aligned fixed performance was rejected", review, err)
				}
			} else if err == nil || review.Passed {
				t.Fatal("misaligned or ungrounded narrative finding was accepted", review, err)
			}
			if variant == "misaligned" && (len(review.Findings) != 1 || review.Findings[0].Code != "dialogue_placement_mismatch" || !contextir.HasCode(err, contextir.CodeModelOutput)) {
				t.Fatal("real contradiction did not reach bounded content-repair boundary", review, err)
			}
		})
	}
}

func TestDialogueReviewTriggerUsesTypedPerformanceNotWords(t *testing.T) {
	if CompositionReviewRequired(Brief{Intent: "dialogue placement speech voiceover"}, Structure{}) {
		t.Fatal("intent keywords chose the review path")
	}
	if !CompositionReviewRequired(Brief{Dialogue: []Dialogue{{Text: "Exact."}}}, Structure{}) ||
		!CompositionReviewRequired(Brief{}, Structure{Dialogue: []Dialogue{{Text: "Exact."}}}) ||
		!CompositionReviewRequired(Brief{}, Structure{}, contextir.Document{Dialogue: []contextir.DialogueLine{{ID: "generated", Text: "Exact."}}}) {
		t.Fatal("actual typed dialogue skipped semantic placement review")
	}
}
