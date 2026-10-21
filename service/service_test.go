package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	contextir "github.com/ly95/contextir"
	"github.com/ly95/contextir/compiler"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/plan"
	"github.com/ly95/contextir/uploads"
)

func testService(t *testing.T, compile CompileFunc, refine RefineFunc) *Service {
	t.Helper()
	store, e := uploads.New(uploads.Config{Directory: t.TempDir(), MaxBytes: 5000, StoreBytes: 12000, TTL: 48 * time.Hour})
	if e != nil {
		t.Fatal(e)
	}
	if compile == nil {
		compile = func(context.Context, plan.Brief) (compiler.Result, error) {
			return compiler.Result{}, refusal(503, "llm-unavailable", "test has no model")
		}
	}
	if refine == nil {
		refine = func(context.Context, compiler.Result, string) (compiler.Result, error) {
			return compiler.Result{}, refusal(503, "llm-unavailable", "test has no model")
		}
	}
	s, e := New(Config{Store: store, MaxRequestBytes: 1 << 20, Compile: compile, Refine: refine})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func call(t *testing.T, s *Service, method, path string, body []byte) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var out map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e, w.Body.String())
	}
	return w.Code, out
}

// These eight values are the unchanged original test_contract.py PINNED table.
func TestUpstreamContractDigests(t *testing.T) {
	want := map[string]string{"asset_fields": "38190799fcfb6cf1", "brief_fields": "cf2fb5307dd7d72f", "dialogue_fields": "a63047779f907e58", "error_codes": "19906fa6afaf0a20", "roles": "4b85814fed5254ab", "directors": "8e50d53980fd8777", "camera_moves": "e0ebdca0a4dcaea8", "limits": "78554cef21934947"}
	// Native v3 adds an explicit BGM-copy field and publishes actual extra
	// refusals. The frozen source-v2 digests remain an independent subset
	// oracle; they do not describe the actual native contract.
	legacy := contractSections()
	fields := []string{}
	for _, field := range fieldNames(BriefIn{}) {
		if field != "audio_copy_modes" && field != "transcript_uses" && field != "subject_projections" {
			fields = append(fields, field)
		}
	}
	legacy["brief_fields"] = fields
	codes := map[string]map[string]any{}
	for code, spec := range errorSpecs {
		if code == "internal-error" {
			continue
		}
		codes[code] = map[string]any{"status": spec["status"], "on": spec["on"]}
	}
	codes["malformed-request"]["on"] = []string{"briefs"}
	legacy["error_codes"] = codes
	for k, w := range want {
		if g, err := Digest(legacy[k]); err != nil || g != w {
			t.Errorf("source-v2 %s: %s != %s (%v)", k, g, w, err)
		}
	}
	if ContractVersion != 3 || len(errorSpecs) != 31 || Digests()["error_codes"] == want["error_codes"] || Digests()["brief_fields"] == want["brief_fields"] {
		t.Fatal("native version3 additions were hidden behind the source-v2 oracle")
	}
}

// Supplementary: a full published Document survives create/read/prompt/refine
// through the real HTTP boundary. Drafting here is an explicit injected choice.
func TestSupplementaryPublishedLifecycle(t *testing.T) {
	c, e := compiler.New(nil, nil, compiler.Options{Profile: h3.ComfyUIProfile(), FramePolicy: compiler.FramePolicySnapUp})
	if e != nil {
		t.Fatal(e)
	}
	var seed *int64
	compile := func(ctx context.Context, b plan.Brief) (compiler.Result, error) {
		seed = b.Seed
		native, e := compiler.FromOriginal(b, nil)
		if e != nil {
			return compiler.Result{}, e
		}
		return c.Draft(ctx, native)
	}
	refine := func(ctx context.Context, old compiler.Result, change string) (compiler.Result, error) {
		b := old.Brief
		switch change {
		case "explicit test edit":
			b.Seconds = 8
			nextSeed := int64(43)
			b.Seed = &nextSeed
			b.Effort = "fast"
			b.Mode = contextir.ModeText
		case "clear sampling choices":
			if b.Seed == nil || *b.Seed != 43 || b.Effort != "fast" || b.Mode != contextir.ModeText {
				t.Fatal("the next refinement received stale parameters", b)
			}
			b.Seed = nil
			b.Effort = "standard"
			b.Mode = ""
		default:
			t.Fatal(change)
		}
		return c.Draft(ctx, b)
	}
	s := testService(t, compile, refine)
	code, out := call(t, s, "POST", "/v1/briefs", []byte(`{"intent":"a car pulls away","seed":42}`))
	if code != 201 {
		t.Fatalf("%d %+v", code, out)
	}
	if seed == nil || *seed != 42 {
		t.Fatal("wire choices lost")
	}
	if out["status"] != "degraded" {
		t.Fatal(out)
	}
	id := out["id"].(string)
	code, prompt := call(t, s, "GET", "/v1/briefs/"+id+"/prompt", nil)
	if code != 200 || prompt["prompt"] == "" || len(prompt["render_hash"].(string)) != 64 {
		t.Fatal(code, prompt)
	}
	hash := prompt["render_hash"]
	code, out = call(t, s, "PATCH", "/v1/briefs/"+id, []byte(`{"change":"explicit test edit"}`))
	if code != 200 || out["version"] != float64(2) {
		t.Fatal(code, out)
	}
	code, prompt = call(t, s, "GET", "/v1/briefs/"+id+"/prompt", nil)
	if code != 200 || prompt["render_hash"] == hash {
		t.Fatal("refinement did not publish changed artifact")
	}
	rec, ok, e := s.cfg.Records.Get(context.Background(), id)
	if e != nil || !ok || rec.Brief.Seed == nil || *rec.Brief.Seed != 43 || rec.Brief.Seconds != 8 || rec.Brief.Effort != "fast" || rec.Brief.Mode == nil || *rec.Brief.Mode != "t2va" {
		t.Fatal(rec, e)
	}
	code, out = call(t, s, "PATCH", "/v1/briefs/"+id, []byte(`{"change":"clear sampling choices"}`))
	if code != 200 || out["version"] != float64(3) {
		t.Fatal(code, out)
	}
	rec, ok, e = s.cfg.Records.Get(context.Background(), id)
	if e != nil || !ok || rec.Brief.Seed != nil || rec.Brief.Effort != "standard" || rec.Brief.Mode != nil || rec.Result.Brief.Seed != nil {
		t.Fatal("cleared parameters were resurrected in storage", rec, e)
	}
	code, out = call(t, s, "PATCH", "/v1/briefs/"+id, []byte(`{"change":"  "}`))
	if code != 422 || out["detail"].(map[string]any)["code"] != "change-empty" {
		t.Fatal(code, out)
	}
}

// Supplementary: original request digests and compiler asset IDs are different
// namespaces. Removing an earlier attachment must not move a transcript to the
// wrong source or collapse two occurrences of the same picture into one slot.
func TestAmendedRequestPreservesStableSourceIdentity(t *testing.T) {
	path := "/caller/portrait.png"
	audioPath, videoPath := "/caller/voice.wav", "/caller/clip.mp4"
	shaImage, shaAudio, shaVideo := strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)
	firstNote, secondNote := "opening composition", "closing composition"
	previous := plan.Brief{Assets: []plan.AssetRef{
		{Kind: "image", SHA256: shaImage, Path: &path, Note: &firstNote, Role: "frame_anchor_first", RoleStated: true},
		{Kind: "image", SHA256: shaImage, Path: &path, Note: &secondNote, Role: "frame_anchor_last", RoleStated: true},
		{Kind: "audio", SHA256: shaAudio, Path: &audioPath, Role: "bgm", PairedVideoSHA256: &shaVideo},
		{Kind: "video", SHA256: shaVideo, Path: &videoPath, Role: "edit_source"},
	}, Transcripts: map[string]string{shaAudio: "the old exact words"}}
	before := compiler.Brief{Assets: []compiler.Asset{
		{Asset: contextir.Asset{ID: "asset-1", Kind: contextir.MediaImage, SHA256: shaImage, URI: path}, Role: compiler.RoleFirstFrame},
		{Asset: contextir.Asset{ID: "asset-2", Kind: contextir.MediaImage, SHA256: shaImage, URI: path}, Role: compiler.RoleLastFrame},
		{Asset: contextir.Asset{ID: "asset-3", Kind: contextir.MediaAudio, SHA256: shaAudio, URI: audioPath}, Role: compiler.RoleBGM, PairedVideoID: "asset-4"},
		{Asset: contextir.Asset{ID: "asset-4", Kind: contextir.MediaVideo, SHA256: shaVideo, URI: videoPath}, Role: compiler.RoleEditSource},
	}}
	after := before
	after.Assets = append([]compiler.Asset(nil), before.Assets[1:]...)
	after.Transcripts = map[string]string{"asset-3": "new words, byte-for-byte：保留"}
	got, err := amendedBrief(previous, before, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Assets) != 3 || value(got.Assets[0].Note) != secondNote || value(got.Assets[1].PairedVideoSHA256) != shaVideo || got.Transcripts[shaAudio] != after.Transcripts["asset-3"] {
		t.Fatal("source identity or caller words were lost", got)
	}
	// The compacted original request is valid input again, even though its native
	// IDs will now be assigned in a different order by FromOriginal.
	got.Seconds, got.Aspect = 8, "16:9"
	rebuilt, err := compiler.FromOriginal(got, nil)
	if err != nil || rebuilt.Transcripts["asset-2"] != after.Transcripts["asset-3"] || rebuilt.Assets[1].PairedVideoID != "asset-3" {
		t.Fatal("stored request cannot be reconstructed faithfully", rebuilt, err)
	}
	// A second projection uses the previous native IDs, not guessed positions in
	// the compacted original request.
	after2 := after
	after2.Assets = append([]compiler.Asset(nil), after.Assets[1:]...)
	after2.Transcripts = nil
	got, err = amendedBrief(got, after, after2)
	if err != nil || len(got.Assets) != 2 || len(got.Transcripts) != 0 || got.Assets[0].Kind != "audio" {
		t.Fatal("the second refinement restored removed data", got, err)
	}
}

func TestSupplementaryProtocolAndUploadBoundary(t *testing.T) {
	var calls int
	var received plan.Brief
	s := testService(t, func(_ context.Context, brief plan.Brief) (compiler.Result, error) {
		calls++
		received = brief
		return compiler.Result{}, refusal(503, "llm-unavailable", "none")
	}, nil)
	for _, b := range []string{`{"intent":"x","invented":true}`, `{"intent":"x","assets":[{"path":"/etc/hostname","invented":1}]}`, `{"intent":"x","dialogue":[{"text":"exact words","speaker_hint":"x"}]}`, `{"intent":"x","seconds":"soon"}`, `{"intent":"x","effort":"turbo"}`, `{"intent":"x","assets":[null]}`, `{"intent":"x","dialogue":[{"text":null}]}`} {
		if code, _ := call(t, s, "POST", "/v1/briefs", []byte(b)); code != 422 {
			t.Fatal(code, b)
		}
	}
	for _, b := range []string{`{"intent":"first","intent":"second"}`, `{"intent":"x","dialogue":[{"text":"\ud800"}]}`, `{"intent":"x","dialogue":[{"text":"\udc00"}]}`} {
		if code, _ := call(t, s, "POST", "/v1/briefs", []byte(b)); code != 422 {
			t.Fatal("ambiguous or altered exact words accepted", code, b)
		}
	}
	if code, out := call(t, s, "POST", "/v1/briefs", []byte(`{"intent":"x","assets":[{"path":"/etc/hostname"}]}`)); code != 422 || out["detail"].(map[string]any)["code"] != "asset-paths-disabled" {
		t.Fatal(code, out)
	}
	if calls != 0 {
		t.Fatal("invalid request reached compile")
	}
	body := []byte("unchanged source bytes")
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	if code, _ := call(t, s, "PUT", "/v1/assets/"+digest, body); code != 201 {
		t.Fatal(code)
	}
	if code, _ := call(t, s, "PUT", "/v1/assets/"+digest, []byte("wrong replacement")); code != 200 {
		t.Fatal(code)
	}
	p, have, e := s.cfg.Store.Stored(digest)
	if e != nil || !have {
		t.Fatal(e)
	}
	_ = p
	for _, m := range []string{"GET", "HEAD", "DELETE", "POST", "PATCH"} {
		if code, _ := call(t, s, m, "/v1/assets/"+digest, nil); code != 405 {
			t.Fatal(m, code)
		}
	}
	code, out := call(t, s, "POST", "/v1/briefs", []byte(`{"intent":"x","assets":[{"sha256":"`+digest+`","role":"subject"}]}`))
	if code != 503 || calls != 1 {
		t.Fatal(code, out, calls)
	}
	code, out = call(t, s, "POST", "/v1/briefs", []byte(`{"intent":"x","assets":[{"sha256":"`+digest+`","kind":"audio","role":"voice_timbre"}],"transcripts":{"`+digest+`":"unaltered caller words"}}`))
	if code != 503 || received.Transcripts[digest] != "unaltered caller words" {
		t.Fatal("caller transcript lost at compile port", code, out, received)
	}
}

// Supplementary: failed perception and writing can consume provider tokens.
// The HTTP boundary must preserve every reported attempt without publishing a
// result, both when the compiler refuses and when publication checks refuse.
func TestSupplementaryFailedCompilationPreservesUsage(t *testing.T) {
	result := compiler.Result{
		Calls: []contextir.CallUsage{
			{AssetID: "image1", Stage: "perception", Attempt: 1, Model: "vision", Usage: contextir.Usage{InputTokens: 37, OutputTokens: 5}, Valid: true},
			{Stage: "writer", Attempt: 1, Model: "writer", Usage: contextir.Usage{InputTokens: 83, OutputTokens: 11}, Valid: true},
		},
		UsageComplete:     false,
		UnmeteredAssetIDs: []string{"video1"},
	}
	compileErr := &contextir.Error{Code: contextir.CodeModel, Stage: "writer", Cause: errors.New("private provider body")}
	var failure error = compileErr
	compile := func(context.Context, plan.Brief) (compiler.Result, error) { return result, failure }
	refine := func(context.Context, compiler.Result, string) (compiler.Result, error) { return result, failure }
	s := testService(t, compile, refine)
	if _, err := s.cfg.Records.Save(context.Background(), "existing", Record{Result: compiler.Result{Status: "ready"}}); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"compiler", "publication"} {
		if phase == "publication" {
			failure = nil // The empty document must fail native publication checks.
		}
		for _, request := range []struct{ method, path, body string }{
			{"POST", "/v1/briefs", `{"intent":"an explicit scene"}`},
			{"PATCH", "/v1/briefs/existing", `{"change":"an explicit edit"}`},
		} {
			code, out := call(t, s, request.method, request.path, []byte(request.body))
			wantCode := 502
			if phase == "publication" {
				wantCode = 500
			}
			if code != wantCode || out["usage_complete"] != false {
				t.Fatalf("%s %s: %d %#v", phase, request.method, code, out)
			}
			var got struct {
				Calls             []contextir.CallUsage `json:"calls"`
				UnmeteredAssetIDs []string              `json:"unmetered_asset_ids"`
			}
			raw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Calls) != 2 || got.Calls[0] != result.Calls[0] || got.Calls[1] != result.Calls[1] || len(got.UnmeteredAssetIDs) != 1 || got.UnmeteredAssetIDs[0] != "video1" {
				t.Fatalf("lost or duplicated usage: %#v", got)
			}
			if strings.Contains(string(raw), "private provider body") {
				t.Fatal("provider response body leaked into the HTTP error")
			}
		}
	}
	rec, ok, err := s.cfg.Records.Get(context.Background(), "existing")
	if err != nil || !ok || rec.Version != 1 {
		t.Fatal("failed refinement published a new record", rec, err)
	}
}

type disconnectReader struct{ n int }

func (r *disconnectReader) Read(p []byte) (int, error) {
	if r.n == 0 {
		r.n++
		return copy(p, "first"), nil
	}
	return 0, ErrClientDisconnected
}
func TestSupplementaryDrainAndMemoryOwnership(t *testing.T) {
	if e := Drain(&disconnectReader{}, 100); e != nil {
		t.Fatal(e)
	}
	if e := Drain(strings.NewReader("x"), 0); e == nil {
		t.Fatal("unbounded drain allowed")
	}
	sentinel := errors.New("disk read failure")
	if e := Drain(errorReader{sentinel}, 20); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	m := NewMemoryRecords()
	r := Record{Brief: plan.Brief{Intent: "owned"}, Result: compiler.Result{ModelError: errors.New("explicit fallback reason"), Document: contextir.Document{Shots: []contextir.Shot{{ID: "unchanged"}}}}}
	if _, e := m.Save(context.Background(), "a", r); e != nil {
		t.Fatal(e)
	}
	r.Result.Document.Shots[0].ID = "mutated"
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := m.Save(context.Background(), "counter", Record{}); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	got, _, e := m.Get(context.Background(), "a")
	if e != nil || got.Result.Document.Shots[0].ID != "unchanged" || got.Result.ModelError.Error() != "explicit fallback reason" {
		t.Fatal(got, e)
	}
	got, _, _ = m.Get(context.Background(), "counter")
	if got.Version != 20 {
		t.Fatal(got.Version)
	}
	_ = io.EOF
}

func TestSupplementarySubjectRetentionInPromptWiring(t *testing.T) {
	source := contextir.SourceRef{AssetID: "image", Track: contextir.TrackVisual}
	r := compiler.Result{Document: contextir.Document{Subjects: []contextir.Subject{{ID: "person", Retention: contextir.WeakReference, Sources: []contextir.Attribution{{Source: source, Role: contextir.RoleAppearance}}}}}}
	r.Artifact.Manifest.Media = []h3.MediaBinding{{Label: "<Picture 1>", Kind: contextir.ReferencePicture, Source: source, Socket: "ref_image_1"}}
	w := promptLayer(r)["wiring"].([]map[string]any)[0]
	if w["retention"] != "weak_reference" || w["kind"] != "image" {
		t.Fatal("subject retention or wire media kind was dropped", w)
	}
	r.Document.Subjects = append(r.Document.Subjects, contextir.Subject{ID: "second", Retention: contextir.FullyPreserved, Sources: []contextir.Attribution{{Source: source, Role: contextir.RoleAppearance}}})
	w = promptLayer(r)["wiring"].([]map[string]any)[0]
	if w["retention"] != "mixed" || len(w["retention_relationships"].([]map[string]string)) != 2 {
		t.Fatal("different subject relationships were silently collapsed", w)
	}
}

func TestSupplementaryClarificationEnvelopeAndAnswers(t *testing.T) {
	doc := contextir.Document{Version: contextir.Version, Mode: contextir.ModeFirstFrame, Target: contextir.Target{Duration: 8000, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}}, Assets: []contextir.Asset{{ID: "image", Kind: contextir.MediaImage, URI: "opaque:explicit-input"}}, Style: "live-action photographic", Shots: []contextir.Shot{{ID: "opening", Start: 0, End: 8000, Description: contextir.Text("The camera holds on the supplied frame.")}}, Soundscape: "N/A", Music: "N/A"}
	at := contextir.Time(0)
	doc.References = []contextir.Reference{{ID: "anchor", Kind: contextir.ReferencePicture, Source: contextir.SourceRef{AssetID: "image", Track: contextir.TrackVisual}, Roles: []contextir.Role{contextir.RoleFirstFrame}, Description: "the supplied first frame", Anchor: &at, Retention: contextir.FullyPreserved}}
	artifact, e := h3.Compile(doc, h3.OfficialProfile())
	if e != nil {
		t.Fatal(e)
	}
	q := compiler.Clarification{ID: "anchor_or_reference", Ask: "Should your image be the video's opening frame, or should what it shows appear in a new scene?", Options: []compiler.ClarificationOption{{ID: "opening_frame", Label: "Use it as the opening frame"}, {ID: "appears_in", Label: "Put what it shows into a new scene"}}, Recommended: "appears_in", DefaultIfUnanswered: "appears_in"}
	result := compiler.Result{Status: "needs_input", ModeDecision: &compiler.ModeDecision{Mode: doc.Mode, NeedsClarification: true, Questions: []compiler.Clarification{q}}}
	if e = validateResult(result); e != nil {
		t.Fatal(e)
	}
	out := envelope("clarify", Record{Brief: plan.Brief{Seconds: 8}, Result: result})
	if out["status"] != "needs_input" || out["default_if_unanswered"] != "appears_in" || out["question"].(*compiler.Clarification).ID != "anchor_or_reference" {
		t.Fatal("typed clarification was dropped", out)
	}
	invalid := result
	invalid.Artifact = artifact
	invalid.Document = doc
	if e = validateResult(invalid); e == nil {
		t.Fatal("unresolved question published a renderable artifact")
	}
	continued := invalid
	continued.Status = "compiled"
	continued.ClarificationPolicy = compiler.ClarificationContinue
	d := *continued.ModeDecision
	d.ClarificationPolicy = compiler.ClarificationContinue
	continued.ModeDecision = &d
	if e = validateResult(continued); e != nil {
		t.Fatal("explicit recorded continuation was refused", e)
	}
	continuedEnvelope := envelope("continued", Record{Brief: plan.Brief{Seconds: 8}, Result: continued})
	if continuedEnvelope["status"] != "ready" || continuedEnvelope["question"].(*compiler.Clarification).ID != "anchor_or_reference" || continuedEnvelope["clarification_policy"] != compiler.ClarificationContinue {
		t.Fatal("continuation policy or unresolved question was lost", continuedEnvelope)
	}
	result.ModeDecision.Questions = nil
	if e = validateResult(result); e == nil {
		t.Fatal("clarification flag without a question published")
	}
	result.ModeDecision.Questions = []compiler.Clarification{q}
	var calls int
	s := testService(t, func(context.Context, plan.Brief) (compiler.Result, error) { calls++; return result, nil }, nil)
	data := []byte("explicit protocol attachment")
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	if _, _, e = s.cfg.Store.Put(digest, nil, bytes.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	code, response := call(t, s, "POST", "/v1/briefs", []byte(`{"intent":"the same request","assets":[{"sha256":"`+digest+`"}]}`))
	if code != 201 || response["status"] != "needs_input" {
		t.Fatal("pending response was dropped or encoded a non-finite target", code, response)
	}
	id := response["id"].(string)
	if code, _ = call(t, s, "GET", "/v1/briefs/"+id, nil); code != 200 {
		t.Fatal(code)
	}
	if code, _ = call(t, s, "GET", "/v1/briefs/"+id+"/prompt", nil); code != 422 {
		t.Fatal("unresolved prompt was exposed", code)
	}
	if code, _ = call(t, s, "PATCH", "/v1/briefs/"+id, []byte(`{"change":"make it eight seconds"}`)); code != 422 || calls != 1 {
		t.Fatal("pending refinement spent another compile before an answer", code, calls)
	}
	for answer, role := range map[string]string{"opening_frame": "frame_anchor_first", "appears_in": "subject"} {
		b, e := s.PrepareBrief([]byte(`{"intent":"the same request","assets":[{"sha256":"` + digest + `"}],"answer":{"anchor_or_reference":"` + answer + `"}}`))
		if e != nil || b.Assets[0].Role != role || !b.Assets[0].RoleStated {
			t.Fatal("clarification answer failed to establish an explicit role", b, e)
		}
	}
	if _, e = s.PrepareBrief([]byte(`{"intent":"x","assets":[{"sha256":"` + digest + `"}],"answer":{"anchor_or_reference":"maybe"}}`)); e == nil {
		t.Fatal("unknown answer silently dropped")
	}
}

type errorReader struct{ e error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.e }
