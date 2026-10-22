package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/uploads"
)

func heldUpload(t *testing.T, s *Service) (plan.Brief, string) {
	t.Helper()
	data := []byte("the same caller-owned original media bytes")
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	if code, out := call(t, s, "PUT", "/v1/assets/"+digest, data); code != 201 {
		t.Fatal(code, out)
	}
	path, exists, err := s.cfg.Store.Stored(digest)
	if err != nil || !exists {
		t.Fatal(path, exists, err)
	}
	return plan.Brief{Assets: []plan.AssetRef{{Kind: "image", SHA256: digest, Path: &path}}}, path
}

func TestRefineMissingUploadsRefusesBeforeModelAndCanBeReuploaded(t *testing.T) {
	refines := 0
	s := testService(t, nil, func(context.Context, compiler.Result, string) (compiler.Result, error) {
		refines++
		return compiler.Result{}, refusal(503, "llm-unavailable", "injected boundary reached")
	})
	brief, path := heldUpload(t, s)
	if _, err := s.cfg.Records.Save(context.Background(), "saved", Record{Brief: brief, Result: compiler.Result{Status: "compiled"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	code, out := call(t, s, "PATCH", "/v1/briefs/saved", []byte(`{"change":"make the same scene longer"}`))
	detail := out["detail"].(map[string]any)
	if code != 422 || detail["code"] != "asset-not-uploaded" || refines != 0 || !strings.Contains(detail["message"].(string), "PUT /v1/assets/{sha256}") {
		t.Fatal("missing upload spent a model call or has no recovery", code, out, refines)
	}
	missing := detail["missing"].([]any)
	if len(missing) != 1 || missing[0] != brief.Assets[0].SHA256 {
		t.Fatal("missing upload identity was not returned", out)
	}
	_, restoredPath := heldUpload(t, s)
	if restoredPath != path {
		t.Fatal("content-addressed reupload changed the source identity")
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	code, out = call(t, s, "PATCH", "/v1/briefs/saved", []byte(`{"change":"make the same scene longer"}`))
	if code != 503 || refines != 1 {
		t.Fatal("the reuploaded source did not unblock refinement", code, out, refines)
	}
	stat, err := os.Stat(path)
	if err != nil || !stat.ModTime().After(old.Add(time.Minute)) {
		t.Fatal("active upload was not touched", stat, err)
	}
	rec, _, err := s.cfg.Records.Get(context.Background(), "saved")
	if err != nil || rec.Version != 1 {
		t.Fatal("a refused refinement changed the saved version", rec, err)
	}
}

func TestInFlightUploadsRemainRecentAndMissingSourceCancels(t *testing.T) {
	s := testService(t, nil, nil)
	store, err := uploads.New(uploads.Config{Directory: t.TempDir(), MaxBytes: 5000, StoreBytes: 12000, TTL: 90 * time.Millisecond, Grace: 90 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Store = store
	brief, path := heldUpload(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = s.withUploadedDependencies(ctx, brief, func(ctx context.Context) (compiler.Result, error) {
		for i := 0; i < 5; i++ {
			select {
			case <-ctx.Done():
				return compiler.Result{}, ctx.Err()
			case <-time.After(45 * time.Millisecond):
			}
			if _, err := store.Sweep(0, time.Now()); err != nil {
				return compiler.Result{}, err
			}
			if _, err := os.Stat(path); err != nil {
				return compiler.Result{}, errors.New("an active source expired during its model operation")
			}
		}
		return compiler.Result{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []contextir.CallUsage{{Stage: "refine", Attempt: 1, Model: "local-audit-double", Usage: contextir.Usage{InputTokens: 23, OutputTokens: 7}, Valid: true}}
	result, err := s.withUploadedDependencies(ctx, brief, func(ctx context.Context) (compiler.Result, error) {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		<-ctx.Done()
		return compiler.Result{Calls: wantCalls}, ctx.Err()
	})
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != "asset-not-uploaded" || len(result.Calls) != 1 || result.Calls[0] != wantCalls[0] {
		t.Fatal("disappearing source did not cancel or lost partial usage", result, err)
	}
}
