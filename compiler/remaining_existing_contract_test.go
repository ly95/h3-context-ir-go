package compiler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

// Original SFX/BGM ROLE_CASES retain their Brief/cards. The source
// audio_relations helper recomputes a marker in Python; assert the actual native
// Document, typed roles, task and final H3 instead of counting that helper as Go.
func TestRemainingExistingAudioRoleRelationships(t *testing.T) {
	corpus := loadUpstream(t)
	for _, tc := range []struct {
		role      Role
		retention contextir.Retention
		task      string
		id        string
	}{{RoleSFX, contextir.AudioReference, "audio reference", "compiler-00677"}, {RoleBGM, contextir.PartiallyCopy, "audio reuse", "compiler-00641"}} {
		t.Run(string(tc.role), func(t *testing.T) {
			var b Brief
			found := false
			for _, call := range corpus.Calls {
				if call.ID == tc.id {
					b = sourceBrief(call)
					found = true
					break
				}
			}
			if !found {
				t.Fatal("pinned original role input missing", tc.id)
			}
			c, err := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp, Planner: plannerPartial()})
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.Draft(context.Background(), b)
			if err != nil {
				t.Fatalf("actual native Draft: %#v", err)
			}
			if len(r.Calls) != 0 || len(r.Document.References) != 1 {
				t.Fatal("draft invented a model call or audio relation")
			}
			ref := r.Document.References[0]
			if ref.Kind != contextir.ReferenceAudio || ref.Source.Track != contextir.TrackAudio || ref.Retention != tc.retention {
				t.Fatal("actual native relationship differs", ref)
			}
			tasks := h3.TaskTypes(r.Document)
			if len(tasks) != 1 || tasks[0] != tc.task || !strings.Contains(r.Artifact.Prompt, "["+tc.task+"]") {
				t.Fatal("derived task/actual H3 differs", tasks, r.Artifact.Prompt)
			}
			if len(r.Artifact.Manifest.Media) != 1 || len(r.Artifact.Manifest.Media[0].TargetUses) != 1 || r.Artifact.Manifest.Media[0].TargetUses[0].Retention != tc.retention {
				t.Fatal("manifest does not carry the actual relationship")
			}
			if tc.role == RoleSFX {
				if !strings.Contains(strings.ToLower(r.Artifact.Prompt), "sound texture") || strings.Contains(r.Artifact.Prompt, "fully_copy") || strings.Contains(r.Artifact.Prompt, "partially_copy") {
					t.Fatal("SFX became source signal copying", r.Artifact.Prompt)
				}
			} else if !strings.Contains(r.Artifact.Prompt, "partially_copy") {
				t.Fatal("BGM default stopped reporting a partial copy")
			}
		})
	}
}

type remainingNeverObserve struct{ calls int }

func (p *remainingNeverObserve) Perceive(context.Context, contextir.Asset) (contextir.Evidence, error) {
	p.calls++
	return contextir.Evidence{}, fmt.Errorf("expensive perception reached before capacity refusal")
}

// Original test_capacity.py::test_the_refusal_happens_before_anything_is_analysed.
// Inject fail-loud ports required by Go; do not substitute a hand-checked limit.
func TestRemainingExistingCapacityBeforeObservation(t *testing.T) {
	b := Brief{Intent: "Put all of these together.", Mode: contextir.ModeReference, Seconds: 8}
	for i := 0; i < 10; i++ {
		b.Assets = append(b.Assets, Asset{Asset: contextir.Asset{ID: fmt.Sprintf("i%d", i), Kind: contextir.MediaImage, Metadata: contextir.MediaMetadata{Width: 1024, Height: 576}}, Role: RoleSubject})
	}
	observer := &remainingNeverObserve{}
	modelCalls := 0
	model := modelFunc(func(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error) {
		modelCalls++
		return contextir.ModelResponse{}, fmt.Errorf("model/probe reached before capacity refusal")
	})
	c, err := New(model, observer, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Compile(context.Background(), b)
	var typed *contextir.Error
	if !errors.As(err, &typed) || typed.Stage != "capacity" {
		t.Fatalf("wrong early refusal: %#v", err)
	}
	message := ""
	for _, issue := range typed.Issues {
		message += issue.Message
	}
	if !strings.Contains(message, "at most 9") {
		t.Fatal("capacity refusal does not name the actual socket ceiling")
	}
	if !contextir.HasCode(err, contextir.CodeLimit) || observer.calls != 0 || modelCalls != 0 || len(result.Calls) != 0 || result.Artifact.Prompt != "" {
		t.Fatalf("not refused before paid ports: %#v observation=%d model=%d", err, observer.calls, modelCalls)
	}
}
