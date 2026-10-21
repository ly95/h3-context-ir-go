package media_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/media"
)

func TestAudioAnalyzerMeteringSurvivesSuccessAndFailure(t *testing.T) {
	tools, ffmpeg := realTools(t)
	path := filepath.Join(t.TempDir(), "speech.wav")
	if output, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.1", "-c:a", "pcm_s16le", path).CombinedOutput(); err != nil {
		t.Fatalf("create actual audio: %v %s", err, output)
	}
	for _, mode := range []string{"opaque", "metered", "failure", "invalid", "local"} {
		t.Run(mode, func(t *testing.T) {
			callbackCount := 0
			options := media.PerceiverOptions{Tools: tools, Resolve: func(context.Context, contextir.Asset) (string, error) { return path, nil }, OnAudioCall: func(contextir.CallUsage) { callbackCount++ }}
			observation := media.AudioObservation{Transcript: "Exact caller analyzer words.", Language: "en", AnalyzerVersion: "test-asr/v1"}
			if mode == "opaque" {
				options.Audio = func(context.Context, string, media.Info) (media.AudioObservation, error) { return observation, nil }
			} else {
				options.MeteredAudio = func(context.Context, string, media.Info) (media.AudioObservation, []contextir.CallUsage, error) {
					if mode == "local" {
						return observation, nil, nil
					}
					calls := []contextir.CallUsage{{Model: "declared-asr", Usage: contextir.Usage{InputTokens: 23, OutputTokens: 5}, Valid: mode != "invalid"}}
					if mode == "failure" {
						return media.AudioObservation{}, calls, errors.New("ASR failed after its model call")
					}
					return observation, calls, nil
				}
			}
			perceiver, err := media.NewPerceiver(options)
			if err != nil {
				t.Fatal(err)
			}
			evidence, calls, err := perceiver.PerceiveWithCalls(context.Background(), contextir.Asset{ID: "speech", Kind: contextir.MediaAudio})
			if (err != nil) != (mode == "failure" || mode == "invalid") {
				t.Fatal("wrong analyzer outcome", err)
			}
			if mode == "local" {
				if len(calls) != 0 || callbackCount != 0 {
					t.Fatal("explicit local analyzer invented a paid call", calls)
				}
				return
			}
			if len(calls) != 1 || callbackCount != 1 || calls[0].AssetID != "speech" || calls[0].Stage != "audio_perception" || calls[0].Attempt != 1 {
				t.Fatal("actual analyzer invocation missing or counted twice", calls, callbackCount)
			}
			if mode == "opaque" {
				if !calls[0].Unmetered || calls[0].Valid || calls[0].Usage != (contextir.Usage{}) {
					t.Fatal("unknown ASR cost represented as valid free usage", calls)
				}
			} else if calls[0].Usage.InputTokens != 23 || calls[0].Unmetered || calls[0].Valid != (mode != "invalid") {
				t.Fatal("reported usage changed on success/failure", calls)
			}
			if err == nil && (evidence.Card.Transcript != observation.Transcript || evidence.Facts[len(evidence.Facts)-1].AnalyzerVersion != "test-asr/v1") {
				t.Fatal("transcript or analyzer provenance lost", evidence)
			}
		})
	}
}

func TestAudioAnalyzerChoiceIsExplicit(t *testing.T) {
	_, err := media.NewPerceiver(media.PerceiverOptions{
		Resolve: func(context.Context, contextir.Asset) (string, error) { return "unused", nil },
		Audio: func(context.Context, string, media.Info) (media.AudioObservation, error) {
			return media.AudioObservation{}, nil
		},
		MeteredAudio: func(context.Context, string, media.Info) (media.AudioObservation, []contextir.CallUsage, error) {
			return media.AudioObservation{}, nil, nil
		},
	})
	if !contextir.HasCode(err, contextir.CodeInvalid) {
		t.Fatal("ambiguous ASR capability accepted", err)
	}
}
