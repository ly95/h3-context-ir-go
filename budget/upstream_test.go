package budget

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestUpstreamGridCalls(t *testing.T) {
	b, e := os.ReadFile("../conformance/fixtures/budget_calls.json")
	if e != nil {
		t.Fatal(e)
	}
	var p struct {
		SourceCommit string `json:"source_commit"`
		Calls        []struct {
			ID         string          `json:"id"`
			Function   string          `json:"function"`
			Arguments  json.RawMessage `json:"arguments"`
			Result     json.RawMessage `json:"result"`
			Raised     json.RawMessage `json:"raised"`
			Properties json.RawMessage `json:"target_properties"`
		} `json:"calls"`
	}
	if e = json.Unmarshal(b, &p); e != nil {
		t.Fatal(e)
	}
	if p.SourceCommit != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" {
		t.Fatal("unexpected source")
	}
	n := 0
	for _, x := range p.Calls {
		if !strings.HasPrefix(x.Function, "h3ir.grid.") {
			continue
		}
		n++
		t.Run(x.ID, func(t *testing.T) {
			var a struct {
				Seconds     float64  `json:"seconds"`
				Aspect      string   `json:"aspect"`
				Canvas      *[2]int  `json:"canvas"`
				Megapixels  *float64 `json:"megapixels"`
				N           int      `json:"n"`
				Frames      int      `json:"frames"`
				Width       int      `json:"width"`
				Height      int      `json:"height"`
				MS          int64    `json:"ms"`
				TS          string   `json:"ts"`
				Mode        string   `json:"mode"`
				LastShot    int      `json:"last_shot"`
				SS          string   `json:"s_ss"`
				TrainedOnly bool     `json:"trained_only"`
				MaxFrames   int      `json:"max_frames"`
			}
			if e := json.Unmarshal(x.Arguments, &a); e != nil {
				t.Fatal(e)
			}
			var got any
			var err error
			switch strings.TrimPrefix(x.Function, "h3ir.grid.") {
			case "Target.build":
				var v Target
				v, err = Build(a.Seconds, a.Aspect, a.Canvas, a.Megapixels)
				got = map[string]any{"nominal_seconds": v.NominalSeconds, "frames": v.Frames, "canvas": v.Canvas, "fps": v.FPS}
				if err == nil {
					sn, _ := v.SS("snapped")
					nom, _ := v.SS("nominal")
					props := map[string]any{"effective_seconds": v.EffectiveSeconds(), "latent_t": v.LatentT(), "video_rows": v.VideoRows(), "audio_rows": v.AudioRows(), "in_trained_range": v.InTrainedRange(), "s_ss_snapped": sn, "s_ss_nominal": nom}
					assertJSON(t, props, x.Properties)
				}
			case "frames_for_seconds":
				got = FramesForSeconds(a.Seconds)
			case "align_frame_count":
				got = AlignFrameCount(a.N)
			case "canvas_for_aspect":
				got, err = CanvasForAspect(a.Aspect, a.Megapixels)
			case "adapt_canvas":
				got, err = AdaptCanvas(a.Width, a.Height)
			case "video_latent_t":
				got = VideoLatentT(a.Frames)
			case "rows_per_latent_frame":
				got = RowsPerLatentFrame(a.Width, a.Height)
			case "audio_latent_t":
				got = AudioLatentT(a.Frames)
			case "s_ss_text":
				got, err = SSecondsText(a.Seconds)
			case "ms_to_timestamp":
				got, err = MSToTimestamp(a.MS)
			case "instruction_line_for":
				got = InstructionLineFor(a.Mode, a.LastShot, a.SS)
			case "legal_frames":
				got = LegalFrames(a.TrainedOnly, a.MaxFrames)
			case "timestamp_to_ms":
				got, err = TimestampToMS(a.TS)
			default:
				t.Fatal("unhandled function", x.Function)
			}
			if len(x.Raised) > 0 && string(x.Raised) != "null" {
				if err == nil {
					t.Fatal("expected upstream rejection", string(x.Raised))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertJSON(t, got, x.Result)
		})
	}
	if n < 4800 {
		t.Fatal("missing source grid coverage", n)
	}
}
func assertJSON(t *testing.T, got any, want json.RawMessage) {
	t.Helper()
	b, e := json.Marshal(got)
	if e != nil {
		t.Fatal(e)
	}
	var a, c any
	if e = json.Unmarshal(b, &a); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(want, &c); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(a, c) {
		t.Errorf("got %s want %s", b, want)
	}
}
func TestHalfUpAndExplicitAlignment(t *testing.T) {
	s, e := SSecondsText(1.005)
	if e != nil || s != "1.01" {
		t.Fatal(s, e)
	}
	if _, e = BuildExact(10, "16:9", nil, nil); e == nil {
		t.Fatal("exact policy accepted 240 off-grid frames")
	}
	v, e := Build(10, "16:9", nil, nil)
	if e != nil || v.AlignmentPolicy != SnapUp || v.Frames != 243 {
		t.Fatal(v, e)
	}
	if _, e = TimestampToMS("00:99.000"); e == nil {
		t.Fatal("invalid timestamp accepted")
	}
}
