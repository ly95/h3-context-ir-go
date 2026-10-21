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

func TestActualTranscriptReperformanceReviewSeparatesWordsAndSignal(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_transcript_review_v2.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "821ac92df5fd9c7d20f91a2bbbc7892912fa1e2d6e5437851f1a1456f18a034f" {
		t.Fatal("actual second-wave candidate provenance changed")
	}
	for _, variant := range []string{"actual-post-speech-reaction", "source-signal-copy", "second-performance", "caller-claim-is-not-execution"} {
		t.Run(variant, func(t *testing.T) {
			var recorded struct {
				Brief     Brief             `json:"brief"`
				Structure Structure         `json:"structure"`
				Request   contextir.Request `json:"request"`
				Writer    json.RawMessage   `json:"writer_document"`
				Critic    struct {
					Fragments map[string]string `json:"fragments"`
				} `json:"critic_input"`
			}
			if err := json.Unmarshal(raw, &recorded); err != nil {
				t.Fatal(err)
			}
			contract, err := canonicalReferenceContract(recorded.Brief, recorded.Structure, recorded.Request, nil)
			if err != nil {
				t.Fatal(err)
			}
			assembled, err := contract.assemble(recorded.Writer)
			if err != nil {
				t.Fatal(err)
			}
			slots, err := fixedDialogueSlots(recorded.Request, recorded.Structure)
			if err != nil {
				t.Fatal(err)
			}
			var d contextir.Document
			if err := json.Unmarshal(assembleFixedEvents(assembled, recorded.Request, recorded.Structure, slots), &d); err != nil {
				t.Fatal(err)
			}
			d, err = projectSubjectAppearances(d, nil)
			if err != nil {
				t.Fatal(err)
			}
			d.Evidence = recorded.Brief.Evidence
			fragments, _ := compositionFragments(d)
			for id, text := range fragments {
				if recorded.Critic.Fragments[id] != text {
					t.Fatalf("rebuilt host candidate changed actual narrative %s", id)
				}
			}
			if variant == "source-signal-copy" {
				d.Soundscape = "Play the original source recording unchanged as the shop's soundtrack."
			}
			if variant == "second-performance" {
				d.Shots[1].Description = contextir.Text("The man delivers the complete fixed line again in this second shot.")
			}
			c, err := New(modelFunc(func(_ context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
				var input struct {
					Fragments      map[string]string `json:"fragments"`
					Claims         []contextir.Claim `json:"claims"`
					TranscriptUses []TranscriptUse   `json:"transcript_uses"`
				}
				if err := json.Unmarshal(req.Input, &input); err != nil {
					t.Fatal(err)
				}
				if input.Fragments["claim:claim-1"] != "" || len(input.Claims) != 4 || input.Claims[0].Origin != contextir.OriginUser || len(input.TranscriptUses) != 1 || input.TranscriptUses[0].Text != "We close at six, not half past." {
					t.Fatal("caller authorization, source words and executable narrative were conflated", string(req.Input))
				}
				if !strings.Contains(req.System, "Post-speech mouth closing or settling") || !strings.Contains(req.System, "Reperforming these authorized words is independent") || !strings.Contains(string(req.Input), `"source_signal_is_used":false`) || strings.Contains(string(req.ResponseSchema), `"claim:claim-1"`) {
					t.Fatal("critic lacks the conservative typed words/signal boundary")
				}
				findings := []CompositionFinding{}
				switch variant {
				case "source-signal-copy":
					findings = append(findings, CompositionFinding{Code: "audio_signal_misuse", FragmentID: "soundscape", Quote: d.Soundscape})
				case "second-performance":
					findings = append(findings, CompositionFinding{Code: "dialogue_placement_mismatch", FragmentID: "shot:shot-2:description", Quote: "delivers the complete fixed line again", SubjectID: "man-at-counter"})
				case "caller-claim-is-not-execution":
					findings = append(findings, CompositionFinding{Code: "audio_signal_misuse", FragmentID: "claim:claim-1", Quote: input.Claims[0].Text})
				}
				wire, _ := json.Marshal(map[string]any{"findings": findings})
				return contextir.ModelResponse{JSON: wire, Model: "explicit-typed-review-double", Usage: contextir.Usage{InputTokens: 17, OutputTokens: 5}}, nil
			}), nil, Options{})
			if err != nil {
				t.Fatal(err)
			}
			review, err := c.ReviewComposition(context.Background(), recorded.Brief, recorded.Structure, d)
			if variant == "actual-post-speech-reaction" {
				if err != nil || !review.Passed {
					t.Fatal("authorized words and residual reaction rejected", review, err)
				}
			} else if err == nil || review.Passed {
				t.Fatal("copy/repeated performance or invalid finding escaped", review, err)
			}
		})
	}
}
