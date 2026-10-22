package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/ly95/h3-context-ir-go"
)

// FrameCache is an explicit caller-owned cache of decoded sample artifacts.
// It never caches perception or a model answer. The content hash, exact sampling
// fractions and analyzer/cache version are all in the key. Corrupt entries fail
// explicitly; they are never treated as successful observations or ignored.
type FrameCache struct {
	tools *Tools
	dir   string
	mu    sync.Mutex
}
type CachedSamples struct {
	CacheHit bool     `json:"cache_hit"`
	Info     Info     `json:"info"`
	Frames   []Frame  `json:"-"`
	Paths    []string `json:"paths"`
}
type cacheRecord struct {
	Version      string       `json:"version"`
	SourceSHA256 string       `json:"source_sha256"`
	Fractions    []float64    `json:"fractions"`
	Info         Info         `json:"info"`
	Frames       []cacheFrame `json:"frames"`
}
type cacheFrame struct {
	Time   contextir.Time `json:"time_ms"`
	Name   string         `json:"name"`
	SHA256 string         `json:"sha256"`
	Bytes  int            `json:"bytes"`
}

const frameCacheVersion = AnalyzerVersion + "/frame-cache/v1"

func NewFrameCache(tools *Tools, directory string) (*FrameCache, error) {
	if tools == nil {
		return nil, fmt.Errorf("frame cache requires initialized media tools")
	}
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("frame cache requires an explicit absolute directory")
	}
	if e := os.MkdirAll(directory, 0700); e != nil {
		return nil, e
	}
	return &FrameCache{tools: tools, dir: directory}, nil
}
func (c *FrameCache) SampleVideo(ctx context.Context, path string, fractions []float64) (CachedSamples, error) {
	if c == nil || c.tools == nil {
		return CachedSamples{}, fmt.Errorf("initialize frame cache first")
	}
	if len(fractions) == 0 || len(fractions) > 600 {
		return CachedSamples{}, mediaError(contextir.CodeInvalid, "samples", "Choose between one and 600 explicit frame fractions.", nil)
	}
	previous := -1.0
	for _, fraction := range fractions {
		if math.IsNaN(fraction) || math.IsInf(fraction, 0) || fraction <= 0 || fraction >= 1 || fraction <= previous {
			return CachedSamples{}, mediaError(contextir.CodeInvalid, "samples", "Fractions must be distinct, increasing, and strictly inside the video.", nil)
		}
		previous = fraction
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.tools.localFile(path); e != nil {
		return CachedSamples{}, e
	}
	source, e := hashFile(ctx, path)
	if e != nil {
		return CachedSamples{}, e
	}
	keyData, e := json.Marshal(struct {
		Version, Source string
		Fractions       []float64
	}{frameCacheVersion, source, fractions})
	if e != nil {
		return CachedSamples{}, e
	}
	keyHash := sha256.Sum256(keyData)
	directory := filepath.Join(c.dir, hex.EncodeToString(keyHash[:]))
	recordPath := filepath.Join(directory, "manifest.json")
	raw, e := os.ReadFile(recordPath)
	if e == nil {
		var record cacheRecord
		if e = json.Unmarshal(raw, &record); e != nil {
			return CachedSamples{}, fmt.Errorf("invalid sample cache manifest: %w", e)
		}
		if record.Version != frameCacheVersion || record.SourceSHA256 != source || len(record.Frames) != len(fractions) || len(record.Fractions) != len(fractions) || record.Info.Kind != contextir.MediaVideo || record.Info.Metadata.Duration <= 0 {
			return CachedSamples{}, fmt.Errorf("sample cache contract mismatch")
		}
		for i, fraction := range fractions {
			if record.Fractions[i] != fraction {
				return CachedSamples{}, fmt.Errorf("sample cache fraction mismatch")
			}
		}
		out := CachedSamples{CacheHit: true, Info: record.Info, Frames: []Frame{}, Paths: []string{}}
		for _, item := range record.Frames {
			if filepath.Base(item.Name) != item.Name {
				return CachedSamples{}, fmt.Errorf("invalid sample cache filename")
			}
			p := filepath.Join(directory, item.Name)
			b, e := os.ReadFile(p)
			if e != nil {
				return CachedSamples{}, e
			}
			h := sha256.Sum256(b)
			if len(b) != item.Bytes || int64(len(b)) > c.tools.maxFrame || hex.EncodeToString(h[:]) != item.SHA256 {
				return CachedSamples{}, fmt.Errorf("sample cache content hash mismatch")
			}
			out.Frames = append(out.Frames, Frame{Time: item.Time, PNG: b})
			out.Paths = append(out.Paths, p)
		}
		return out, nil
	}
	if !errors.Is(e, os.ErrNotExist) {
		return CachedSamples{}, e
	}
	info, frames, e := c.tools.SampleVideo(ctx, path, fractions)
	if e != nil {
		return CachedSamples{}, e
	}
	after, e := hashFile(ctx, path)
	if e != nil {
		return CachedSamples{}, e
	}
	if after != source {
		return CachedSamples{}, fmt.Errorf("source changed while sampling")
	}
	temporary, e := os.MkdirTemp(c.dir, "samples-")
	if e != nil {
		return CachedSamples{}, e
	}
	defer os.RemoveAll(temporary)
	record := cacheRecord{Version: frameCacheVersion, SourceSHA256: source, Fractions: append([]float64{}, fractions...), Info: info, Frames: []cacheFrame{}}
	for i, frame := range frames {
		name := fmt.Sprintf("frame-%03d.png", i)
		if e = os.WriteFile(filepath.Join(temporary, name), frame.PNG, 0600); e != nil {
			return CachedSamples{}, e
		}
		h := sha256.Sum256(frame.PNG)
		record.Frames = append(record.Frames, cacheFrame{Time: frame.Time, Name: name, SHA256: hex.EncodeToString(h[:]), Bytes: len(frame.PNG)})
	}
	raw, e = json.Marshal(record)
	if e != nil {
		return CachedSamples{}, e
	}
	if e = os.WriteFile(filepath.Join(temporary, "manifest.json"), raw, 0600); e != nil {
		return CachedSamples{}, e
	}
	if e = os.Rename(temporary, directory); e != nil {
		return CachedSamples{}, fmt.Errorf("cannot publish sample cache entry: %w", e)
	}
	out := CachedSamples{Info: info, Frames: frames, Paths: []string{}}
	for _, f := range record.Frames {
		out.Paths = append(out.Paths, filepath.Join(directory, f.Name))
	}
	return out, nil
}
func hashFile(ctx context.Context, path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	b := make([]byte, 64<<10)
	for {
		if e = ctx.Err(); e != nil {
			return "", e
		}
		n, readErr := f.Read(b)
		if n > 0 {
			if _, e = h.Write(b[:n]); e != nil {
				return "", e
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
