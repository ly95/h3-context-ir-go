package compiler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/planner"
)

type videoTranscriptObserver struct {
	evidence contextir.Evidence
	calls    int
}

func (p *videoTranscriptObserver) Perceive(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
	evidence, _, err := p.PerceiveWithCalls(ctx, asset)
	return evidence, err
}

func (p *videoTranscriptObserver) PerceiveWithCalls(_ context.Context, asset contextir.Asset) (contextir.Evidence, []contextir.CallUsage, error) {
	p.calls++
	return p.evidence, []contextir.CallUsage{{AssetID: asset.ID, Stage: "video_perception", Attempt: 1, Model: "video-observation-double", Usage: contextir.Usage{InputTokens: 13, OutputTokens: 7}, Valid: true}}, nil
}

func visualVideoWithSpeech() (Brief, contextir.Evidence) {
	coverage := &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{contextir.TrackVisual, contextir.TrackAudio}}
	card := AssetCard{AssetID: "clip", Summary: "A person walks down a corridor.", Style: "Live-action", Subjects: []SubjectCard{{Kind: "person", Descriptor: "the observed person", Attributes: []string{"green jacket"}}}, Transcript: "Keep these recorded words as source evidence.", Language: "en", AnalyzerVersion: "video-observation-double/v1", Coverage: coverage}
	evidence := contextir.Evidence{AssetID: "clip", AnalyzerVersion: card.AnalyzerVersion, Model: "video-observation-double", Card: &card, Coverage: coverage, Facts: []contextir.Fact{{ID: "clip-visual", Text: card.Summary, Source: contextir.SourceRef{AssetID: "clip", Track: contextir.TrackVisual}}, {ID: "clip-audio", Text: card.Transcript, Source: contextir.SourceRef{AssetID: "clip", Track: contextir.TrackAudio}}}}
	brief := Brief{Intent: "Preserve the visual scene in a silent video edit.", Mode: contextir.ModeReference, Seconds: 8, Creativity: CreativityRestrained, SilenceAll: true, Assets: []Asset{{Asset: contextir.Asset{ID: "clip", Kind: contextir.MediaVideo, Metadata: contextir.MediaMetadata{Duration: 8000}}, Role: RoleEditSource, Frames: 192}}}
	return brief, evidence
}

func TestVisualVideoSpeechRemainsObservedEvidenceThroughCompile(t *testing.T) {
	brief, evidence := visualVideoWithSpeech()
	observer := &videoTranscriptObserver{evidence: evidence}
	stages := []string{}
	engine, err := New(remainingCompileDouble(t, brief, &stages, false), observer, Options{Profile: h3.OfficialProfile(), FailurePolicy: FailurePolicyFail, Planner: planner.Options{AllowPartialPerception: true}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Compile(t.Context(), brief)
	if err != nil || result.Status != "compiled" || result.Artifact.Prompt == "" {
		t.Fatal("observing source speech prevented a valid silent visual video edit", err)
	}
	if observer.calls != 1 || len(result.Evidence) != 1 || !reflect.DeepEqual(result.Evidence[0], evidence) || !reflect.DeepEqual(result.Document.Evidence, result.Evidence) || len(result.Brief.Cards) != 1 || result.Brief.Cards[0].Transcript != evidence.Card.Transcript {
		t.Fatal("the successful edit dropped or changed original audio observations")
	}
	if len(result.Document.Dialogue) != 0 || len(result.Brief.TranscriptUses) != 0 || strings.Contains(result.Artifact.Prompt, evidence.Card.Transcript) || result.Document.Soundscape != "N/A" || result.Document.Music != "N/A" {
		t.Fatal("visual video responsibility imported observed speech into the silent target")
	}
	if len(result.Calls) != len(stages)+1 || result.Calls[0].Stage != "video_perception" || result.Calls[0].Usage != (contextir.Usage{InputTokens: 13, OutputTokens: 7}) || !result.UsageComplete {
		t.Fatal("successful compilation lost or duplicated the actual observation ledger")
	}
}

func TestVisualVideoTranscriptScopeAndSelectionBoundaries(t *testing.T) {
	brief, evidence := visualVideoWithSpeech()
	brief.Cards = []AssetCard{*evidence.Card}
	for _, scenario := range []string{"observation_only", "explicit_transcript", "explicit_selection", "model_selection"} {
		t.Run(scenario, func(t *testing.T) {
			input := brief
			calls := 0
			use := TranscriptUse{AssetID: "clip", Text: evidence.Card.Transcript, Language: "en", SpeakerHint: "narrator", Voiceover: true}
			if scenario == "explicit_transcript" {
				input.Transcripts = map[string]string{"clip": evidence.Card.Transcript}
			}
			if scenario == "explicit_selection" {
				input.TranscriptUses = []TranscriptUse{use}
			}
			model := modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				if request.Stage != "scope" {
					t.Fatal("unexpected stage", request.Stage)
				}
				value := map[string]any{"requested": Scope{}, "forbidden": Scope{}, "dialogue": []Dialogue{}, "onscreen_text": []string{}, "reasoning": "Observed words do not authorize a new performance."}
				if scenario == "model_selection" {
					value["transcript_uses"] = []TranscriptUse{use}
				}
				data, _ := json.Marshal(value)
				return contextir.ModelResponse{JSON: data, Model: "scope-observation-double", Usage: contextir.Usage{InputTokens: 5, OutputTokens: 2}}, nil
			})
			engine, err := New(model, nil, Options{Planner: planner.Options{AllowPartialPerception: true}})
			if err != nil {
				t.Fatal(err)
			}
			scope, err := engine.DecideScope(t.Context(), input)
			if scenario == "observation_only" {
				if err != nil || calls != 1 || len(scope.Dialogue) != 0 || len(scope.TranscriptUses) != 0 || !reflect.DeepEqual(input.Cards[0], *evidence.Card) {
					t.Fatal("visual video observations did not remain intact through valid scope interpretation", err)
				}
				return
			}
			if err == nil || len(scope.Dialogue) != 0 || len(scope.TranscriptUses) != 0 {
				t.Fatal("visual video speech became an authorized transcript source")
			}
			if scenario == "model_selection" {
				if calls != 1 || len(scope.Calls) != 1 || scope.Calls[0].Usage != (contextir.Usage{InputTokens: 5, OutputTokens: 2}) {
					t.Fatal("rejected model-selected source words lost actual usage")
				}
			} else if calls != 0 || len(scope.Calls) != 0 {
				t.Fatal("invalid explicit video transcript authorization reached a model")
			}
		})
	}
}
