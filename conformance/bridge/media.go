package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/media"
)

func mediaOperation(name string, b json.RawMessage) (any, error) {
	switch name {
	case "h3ir.analyse._cache_key", "h3ir.analyse._cache_path", "h3ir.analyse.save_cached", "h3ir.analyse.load_cached", "h3ir.analyse.analyse_audio", "h3ir.analyse.analyse_all":
		value, err := cardCacheOperation(name, b)
		return value, readableMediaError(err)
	}
	var a struct {
		Path      string    `json:"path"`
		FFprobe   string    `json:"ffprobe"`
		FFmpeg    string    `json:"ffmpeg"`
		Fractions []float64 `json:"fractions"`
		CacheDir  string    `json:"cache_dir"`
	}
	if e := json.Unmarshal(b, &a); e != nil {
		return nil, e
	}
	switch name {
	case "h3ir.analyse.image_size", "h3ir.analyse.image_mime", "h3ir.analyse.sniff_container":
		header, e := media.ReadHeader(a.Path)
		if e != nil {
			return nil, readableMediaError(e)
		}
		switch name {
		case "h3ir.analyse.image_size":
			if header.Width <= 0 || header.Height <= 0 {
				return nil, nil
			}
			return [2]int{header.Width, header.Height}, nil
		case "h3ir.analyse.image_mime":
			if header.MIME == "" {
				return nil, nil
			}
			return header.MIME, nil
		default:
			if header.Kind == "" {
				return nil, nil
			}
			return header.Kind, nil
		}
	}
	tools, e := media.NewTools(media.Options{FFprobe: a.FFprobe, FFmpeg: a.FFmpeg})
	if e != nil {
		return nil, readableMediaError(e)
	}
	switch name {
	case "h3ir.analyse.probe_seconds":
		info, e := tools.Inspect(context.Background(), a.Path)
		if e != nil {
			return nil, readableMediaError(e)
		}
		return float64(info.Metadata.Duration) / 1000, nil
	case "h3ir.analyse.sample_frames":
		cache, e := media.NewFrameCache(tools, a.CacheDir)
		if e != nil {
			return nil, readableMediaError(e)
		}
		sample, e := cache.SampleVideo(context.Background(), a.Path, a.Fractions)
		if e != nil {
			return nil, readableMediaError(e)
		}
		return sample.Paths, nil
	}
	return nil, fmt.Errorf("unsupported media conformance operation %q", name)
}
func readableMediaError(err error) error {
	var e *contextir.Error
	if errors.As(err, &e) && len(e.Issues) > 0 {
		return fmt.Errorf("%s: %s: %w", e.Issues[0].Path, e.Issues[0].Message, err)
	}
	return err
}
