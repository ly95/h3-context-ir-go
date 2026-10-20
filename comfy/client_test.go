package comfy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func clientFor(t *testing.T, h http.HandlerFunc, limit int64) *Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c, e := New(Config{BaseURL: s.URL, Timeout: time.Second, MaxResponseBytes: limit})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(c.Close)
	return c
}

// Remote protocol tests supplement the upstream graph assertions; no live GPU is used.
func TestRemoteProtocol(t *testing.T) {
	var history atomic.Int32
	var uploaded atomic.Bool
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/system_stats":
			io.WriteString(w, `{"system":{"comfyui_version":"test","python_version":"3.12"},"devices":[{"name":"test-gpu","vram_total":8589934592,"vram_free":4294967296}]}`)
		case "/object_info":
			io.WriteString(w, `{"MiniMaxH3ReferenceToVideo":{},"MiniMaxH3ImageToVideo":{}}`)
		case "/upload/image":
			if e := r.ParseMultipartForm(4096); e != nil {
				t.Error(e)
				http.Error(w, "bad form", 400)
				return
			}
			f, header, e := r.FormFile("image")
			if e != nil {
				t.Error(e)
				return
			}
			defer f.Close()
			b, _ := io.ReadAll(f)
			if header.Filename != "input.png" || string(b) != "image-bytes" || r.FormValue("subfolder") != "h3ir" || r.FormValue("overwrite") != "true" {
				t.Error("multipart differs from requested upload")
			}
			uploaded.Store(true)
			io.WriteString(w, `{"name":"input.png","subfolder":"h3ir"}`)
		case "/prompt":
			var p struct {
				Prompt   Graph  `json:"prompt"`
				ClientID string `json:"client_id"`
			}
			if e := json.NewDecoder(r.Body).Decode(&p); e != nil || p.ClientID != "test-client" || p.Prompt["3"] == nil {
				t.Errorf("invalid submit %v %v", p, e)
			}
			io.WriteString(w, `{"prompt_id":"job"}`)
		case "/history/job":
			if history.Add(1) == 1 {
				io.WriteString(w, `{}`)
			} else {
				io.WriteString(w, `{"job":{"status":{"completed":true},"outputs":{"9":{"videos":[{"filename":"out.mp4","subfolder":"h3ir","type":"output"}]}}}}`)
			}
		case "/view":
			if r.URL.Query().Get("filename") != "out.mp4" || r.URL.Query().Get("type") != "output" {
				t.Error("view parameters changed")
			}
			w.Write([]byte{1, 2, 3})
		default:
			http.NotFound(w, r)
		}
	}, 1<<20)
	ctx := context.Background()
	name, e := c.UploadImage(ctx, "input.png", strings.NewReader("image-bytes"), "h3ir", true)
	if e != nil || name != "h3ir/input.png" || !uploaded.Load() {
		t.Fatal(name, e)
	}
	g, e := Decode([]byte(`{"3":{"class_type":"MiniMaxH3ReferenceToVideo","inputs":{"prompt":"x"}}}`))
	if e != nil {
		t.Fatal(e)
	}
	id, e := c.Submit(ctx, g, "test-client")
	if e != nil || id != "job" {
		t.Fatal(id, e)
	}
	h, e := c.Wait(ctx, id, WaitOptions{PollInterval: time.Millisecond, Timeout: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	out, e := Outputs(h)
	if e != nil || len(out) != 1 || out[0]["kind"] != "videos" || out[0]["node"] != "9" {
		t.Fatal(out, e)
	}
	b, e := c.View(ctx, "out.mp4", "h3ir", "output")
	if e != nil || len(b) != 3 {
		t.Fatal(b, e)
	}
	p, e := c.Probe(ctx)
	if e != nil || !p.Reachable || p.VRAMTotalGiB != 8 || !p.H3Nodes["MiniMaxH3ReferenceToVideo"] {
		t.Fatal(p, e)
	}
}
func TestFailuresAreExplicitAndSafe(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{{"server failure", "private signed locator must not appear in error", 500}, {"redirect", "", 302}, {"invalid json", "{broken", 200}, {"missing id", "{}", 200}, {"oversize", strings.Repeat("x", 100), 200}} {
		t.Run(tc.name, func(t *testing.T) {
			c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "http://127.0.0.1:1/private")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}, 32)
			_, e := c.Submit(context.Background(), Graph{"3": map[string]any{}}, "client")
			if e == nil || strings.Contains(e.Error(), "signed locator") || strings.Contains(e.Error(), "127.0.0.1:1") {
				t.Fatal(e)
			}
		})
	}
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"job":{"status":{"status_str":"error","completed":true}}}`)
	}, 1024)
	if _, e := c.Wait(context.Background(), "job", WaitOptions{time.Millisecond, time.Second}); e == nil {
		t.Fatal("failed job accepted as completed")
	}
	c = clientFor(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }, 1024)
	_, e := c.Wait(context.Background(), "job", WaitOptions{time.Millisecond, 10 * time.Millisecond})
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
}
func TestGraphEditingPreservesExactValues(t *testing.T) {
	g, e := Decode([]byte(`{"3":{"class_type":"MiniMaxH3ReferenceToVideo","inputs":{"prompt":"old"}},"4":{"class_type":"KSampler","inputs":{"seed":18446744073709551615}}}`))
	if e != nil {
		t.Fatal(e)
	}
	out, e := SetPrompt(g, "new", "")
	if e != nil {
		t.Fatal(e)
	}
	if g["3"].(map[string]any)["inputs"].(map[string]any)["prompt"] != "old" || out["3"].(map[string]any)["inputs"].(map[string]any)["prompt"] != "new" {
		t.Fatal("mutated source graph")
	}
	out, e = SetSeed(g, 18446744073709551614)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(out)
	if !strings.Contains(string(b), "18446744073709551614") {
		t.Fatal(string(b))
	}
	d := DescribeGraph(g)
	if len(d.Seeds) != 1 || d.Seeds[0] != "18446744073709551615" {
		t.Fatal(d)
	}
}
