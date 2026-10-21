package media

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/contextir"
)

func TestOriginalGoldenHeaderGeometry(t *testing.T) {
	for _, name := range []string{"ref1.png", "ref2.png"} {
		path, e := filepath.Abs(filepath.Join("../third_party/open-h3-ir/testdata/golden/assets", name))
		if e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		header, e := ReadHeader(path)
		if e != nil {
			t.Fatal(e)
		}
		if header.Kind != contextir.MediaImage || header.MIME != "image/png" || header.Width != int(binary.BigEndian.Uint32(b[16:20])) || header.Height != int(binary.BigEndian.Uint32(b[20:24])) {
			t.Fatalf("%s header does not match source IHDR: %+v", name, header)
		}
	}
}
func TestVisualDeclarationUsesBytesAndLeavesAudioContainersAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pretend-photo.jpg")
	if e := os.WriteFile(path, append([]byte("\x00\x00\x00\x20ftypM4A "), make([]byte, 32)...), 0600); e != nil {
		t.Fatal(e)
	}
	_, err := ValidateDeclaration(path, contextir.MediaImage)
	var typed *contextir.Error
	if !errors.As(err, &typed) || typed.Code != contextir.CodePerception || !strings.Contains(typed.Issues[0].Message, "Attach it with kind: video") {
		t.Fatalf("declaration mismatch: %v", err)
	}
	if _, e := ValidateDeclaration(path, contextir.MediaAudio); e != nil {
		t.Fatalf("audio-only ISO container refused: %v", e)
	}
}
func TestExplicitFrameCacheInvalidationAndIntegrity(t *testing.T) {
	probe, e := exec.LookPath("ffprobe")
	if e != nil {
		t.Skip("ffprobe unavailable: native video-cache feature cannot be exercised on this host")
	}
	decoder, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip("ffmpeg unavailable: native video-cache feature cannot be exercised on this host")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "source.mp4")
	cmd := exec.Command(decoder, "-v", "error", "-nostdin", "-y", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=24:duration=2", "-pix_fmt", "yuv420p", path)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("make original test-pattern fixture: %v %s", e, out)
	}
	tools, e := NewTools(Options{FFprobe: probe, FFmpeg: decoder})
	if e != nil {
		t.Fatal(e)
	}
	cache, e := NewFrameCache(tools, filepath.Join(dir, "cache"))
	if e != nil {
		t.Fatal(e)
	}
	fractions := []float64{.1, .5, .9}
	first, e := cache.SampleVideo(context.Background(), path, fractions)
	if e != nil {
		t.Fatal(e)
	}
	if first.CacheHit || len(first.Frames) != 3 {
		t.Fatal("first explicit cache request did not decode all samples")
	}
	stat, e := os.Stat(first.Paths[0])
	if e != nil {
		t.Fatal(e)
	}
	again, e := cache.SampleVideo(context.Background(), path, fractions)
	if e != nil {
		t.Fatal(e)
	}
	current, e := os.Stat(first.Paths[0])
	if e != nil {
		t.Fatal(e)
	}
	if !again.CacheHit || !reflect.DeepEqual(first.Paths, again.Paths) || !stat.ModTime().Equal(current.ModTime()) {
		t.Fatal("same immutable input was decoded again or rewritten")
	}
	changed, e := cache.SampleVideo(context.Background(), path, []float64{.2, .5, .8})
	if e != nil {
		t.Fatal(e)
	}
	if changed.CacheHit || reflect.DeepEqual(changed.Paths, first.Paths) || changed.Frames[0].Time != 400 {
		t.Fatal("changed fractions reused stale samples")
	}
	if e = os.WriteFile(first.Paths[0], []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = cache.SampleVideo(context.Background(), path, fractions); e == nil {
		t.Fatal("corrupt cached PNG silently accepted or regenerated")
	}
}
