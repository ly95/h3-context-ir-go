package compiler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func TestIntakeSnapshotAndRefinementRecomputeOnlyDerivedPreparation(t *testing.T) {
	c, _ := New(nil, nil, Options{Profile: h3.ComfyUIProfile(), FramePolicy: FramePolicySnapUp})
	b := Brief{Intent: "the person walks", Seconds: 8, Assets: []Asset{
		{Asset: contextir.Asset{ID: "image", Kind: contextir.MediaImage, Metadata: contextir.MediaMetadata{Width: 4096, Height: 4096}}, Role: RoleSubject},
		{Asset: contextir.Asset{ID: "video", Kind: contextir.MediaVideo, Metadata: contextir.MediaMetadata{Width: 1280, Height: 720}}, Role: RoleEditSource, Frames: 500},
	}}
	before, _ := json.Marshal(b)
	intake, err := c.ValidateIntake(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(b)
	if string(before) != string(after) || b.Assets[0].Preparation != (contextir.MediaPreparation{}) {
		t.Fatal("intake reused caller-owned slice memory")
	}
	change := "make it square at 640 pixels and four seconds"
	amended, err := applyBriefUpdates(intake.Brief, change, []BriefUpdate{
		{Field: "aspect", Action: "set", Quote: "square", Value: json.RawMessage(`"1:1"`)},
		{Field: "canvas", Action: "set", Quote: "640 pixels", Value: json.RawMessage(`[640,640]`)},
		{Field: "seconds", Action: "set", Quote: "four seconds", Value: json.RawMessage(`4`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := c.ValidateIntake(context.Background(), amended)
	if err != nil || next.Brief.Assets[0].Preparation.Width != 640 || next.Brief.Assets[1].Preparation.Width != 640 || next.Brief.Assets[1].Preparation.Frames != next.Target.FrameCount || next.Brief.Assets[1].Preparation.Frames >= intake.Brief.Assets[1].Preparation.Frames || next.Brief.Assets[0].Metadata.Width != 4096 {
		t.Fatal(next, err)
	}
	b.Assets[0].Preparation = contextir.MediaPreparation{Sizing: "match", Width: 1024, Height: 1024}
	explicit, err := c.ValidateIntake(context.Background(), b)
	if err != nil || explicit.Brief.Assets[0].PreparationInput == nil {
		t.Fatal(explicit, err)
	}
	explicit.Brief.Canvas = &[2]int{640, 640}
	if _, err = c.ValidateIntake(context.Background(), explicit.Brief); err == nil {
		t.Fatal("explicit caller preparation was silently discarded")
	}
}
