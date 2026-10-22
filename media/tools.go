package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ly95/h3-context-ir-go"
)

const AnalyzerVersion = "contextir-media/v1"

type Options struct {
	FFprobe       string
	FFmpeg        string
	MaxInputBytes int64
	MaxFrameBytes int64
}
type Tools struct {
	probe, ffmpeg      string
	maxInput, maxFrame int64
}
type Info struct {
	Kind       contextir.MediaKind     `json:"kind"`
	Metadata   contextir.MediaMetadata `json:"metadata"`
	Frames     int64                   `json:"frames"`
	HasAudio   bool                    `json:"has_audio"`
	SampleRate int                     `json:"sample_rate,omitempty"`
	Channels   int                     `json:"channels,omitempty"`
	Codec      string                  `json:"codec,omitempty"`
}
type Frame struct {
	Time contextir.Time `json:"time_ms"`
	PNG  []byte         `json:"-"`
}

func NewTools(options Options) (*Tools, error) {
	if options.FFprobe == "" || options.FFmpeg == "" {
		return nil, mediaError(contextir.CodeInvalid, "tools", "Explicit ffprobe and ffmpeg executable paths are required.", nil)
	}
	probe, err := exec.LookPath(options.FFprobe)
	if err != nil {
		return nil, mediaError(contextir.CodeUnsupported, "ffprobe", "ffprobe is not installed or the configured executable is unavailable. Install ffmpeg, which includes ffprobe, and configure its executable path.", nil)
	}
	ffmpeg, err := exec.LookPath(options.FFmpeg)
	if err != nil {
		return nil, mediaError(contextir.CodeUnsupported, "ffmpeg", "ffmpeg is not installed or the configured executable is unavailable. Install ffmpeg and configure its executable path.", nil)
	}
	if options.MaxInputBytes < 0 || options.MaxFrameBytes < 0 {
		return nil, mediaError(contextir.CodeInvalid, "limits", "Media byte limits must be positive when supplied.", nil)
	}
	if options.MaxInputBytes == 0 {
		options.MaxInputBytes = 1 << 30
	}
	if options.MaxFrameBytes == 0 {
		options.MaxFrameBytes = 32 << 20
	}
	return &Tools{probe: probe, ffmpeg: ffmpeg, maxInput: options.MaxInputBytes, maxFrame: options.MaxFrameBytes}, nil
}

func (t *Tools) localFile(path string) error {
	if !filepath.IsAbs(path) {
		return mediaError(contextir.CodeInvalid, "path", "A caller-resolved absolute local file path is required.", nil)
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return mediaError(contextir.CodeInvalid, "path", "The caller-resolved media file does not exist.", nil)
	}
	if err != nil {
		return mediaError(contextir.CodeInvalid, "path", "The caller-resolved media file is not accessible.", nil)
	}
	if !info.Mode().IsRegular() {
		return mediaError(contextir.CodeInvalid, "path", "The caller-resolved media must be a regular file.", nil)
	}
	if info.Size() <= 0 || info.Size() > t.maxInput {
		return mediaError(contextir.CodeLimit, "asset", "The media is empty or exceeds the configured byte limit.", nil)
	}
	return nil
}

func (t *Tools) Inspect(ctx context.Context, path string) (Info, error) {
	if t == nil {
		return Info{}, mediaError(contextir.CodeInvalid, "tools", "Initialize media tools first.", nil)
	}
	if err := t.localFile(path); err != nil {
		return Info{}, err
	}
	data, err := run(ctx, 4<<20, t.probe, "-v", "error", "-show_streams", "-show_format", "-of", "json", path)
	if err != nil {
		return Info{}, err
	}
	var wire struct {
		Streams []struct {
			CodecType   string `json:"codec_type"`
			CodecName   string `json:"codec_name"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Rate        string `json:"avg_frame_rate"`
			Frames      string `json:"nb_frames"`
			Duration    string `json:"duration"`
			SampleRate  string `json:"sample_rate"`
			Channels    int    `json:"channels"`
			Disposition struct {
				AttachedPic int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
		} `json:"format"`
	}
	if err = json.Unmarshal(data, &wire); err != nil {
		return Info{}, mediaError(contextir.CodePerception, "probe", "The media probe returned malformed metadata.", nil)
	}
	var out Info
	for _, s := range wire.Streams {
		if s.CodecType == "audio" {
			out.HasAudio = true
			var sampleErr error
			out.SampleRate, sampleErr = strconv.Atoi(s.SampleRate)
			if sampleErr != nil || out.SampleRate <= 0 || s.Channels <= 0 {
				return Info{}, mediaError(contextir.CodePerception, "audio_metadata", "The decoded audio stream has invalid sample rate or channel metadata.", nil)
			}
			out.Channels = s.Channels
			if out.Codec == "" {
				out.Codec = s.CodecName
			}
			continue
		}
		if s.CodecType != "video" || s.Disposition.AttachedPic != 0 {
			continue
		}
		if out.Metadata.Width != 0 {
			return Info{}, mediaError(contextir.CodeUnsupported, "streams", "Multiple visual streams require an explicit stream-selection adapter.", nil)
		}
		out.Kind = contextir.MediaVideo
		out.Codec = s.CodecName
		out.Metadata.Width = s.Width
		out.Metadata.Height = s.Height
		if s.Width <= 0 || s.Height <= 0 {
			return Info{}, mediaError(contextir.CodePerception, "geometry", "The visual stream has invalid geometry.", nil)
		}
		out.Frames, _ = strconv.ParseInt(s.Frames, 10, 64)
		if rat, ok := rational(s.Rate); ok {
			out.Metadata.FPS = rat
		}
		if duration, ok := secondsMS(s.Duration); ok {
			out.Metadata.Duration = duration
		}
	}
	if duration, ok := secondsMS(wire.Format.Duration); ok && out.Metadata.Duration == 0 {
		out.Metadata.Duration = duration
	}
	if out.Kind == "" {
		if !out.HasAudio {
			return Info{}, mediaError(contextir.CodeUnsupported, "streams", "No supported visual or audio stream was found.", nil)
		}
		out.Kind = contextir.MediaAudio
	}
	if !out.HasAudio && out.Metadata.Width > 0 {
		still := wire.Format.Name == "image2" || wire.Format.Name == "png_pipe" || wire.Format.Name == "jpeg_pipe" || wire.Format.Name == "webp_pipe" || wire.Format.Name == "bmp_pipe" || wire.Format.Name == "tiff_pipe"
		if still {
			out.Kind = contextir.MediaImage
			out.Metadata.Duration = 0
			out.Metadata.FPS = contextir.Rational{}
		}
	}
	return out, nil
}

func rational(s string) (contextir.Rational, bool) {
	a, b, ok := strings.Cut(s, "/")
	if !ok {
		return contextir.Rational{}, false
	}
	n, e1 := strconv.ParseInt(a, 10, 64)
	d, e2 := strconv.ParseInt(b, 10, 64)
	return contextir.Rational{Numerator: n, Denominator: d}, e1 == nil && e2 == nil && n > 0 && d > 0
}
func secondsMS(s string) (contextir.Time, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > float64(math.MaxInt64)/1000 {
		return 0, false
	}
	return contextir.Time(math.Round(v * 1000)), true
}

func (t *Tools) SampleVideo(ctx context.Context, path string, fractions []float64) (Info, []Frame, error) {
	info, err := t.Inspect(ctx, path)
	if err != nil {
		return Info{}, nil, err
	}
	if info.Kind != contextir.MediaVideo || info.Metadata.Duration <= 0 {
		return info, nil, mediaError(contextir.CodeUnsupported, "video", "A timed video stream is required for frame sampling.", nil)
	}
	if len(fractions) == 0 || len(fractions) > 600 {
		return info, nil, mediaError(contextir.CodeInvalid, "samples", "Choose between one and 600 explicit frame fractions.", nil)
	}
	frames := make([]Frame, 0, len(fractions))
	previous := -1.0
	for _, f := range fractions {
		if math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 || f >= 1 || f <= previous {
			return info, nil, mediaError(contextir.CodeInvalid, "samples", "Fractions must be distinct, increasing, and strictly inside the video.", nil)
		}
		previous = f
		at := contextir.Time(math.Round(float64(info.Metadata.Duration) * f))
		if at >= info.Metadata.Duration {
			at = info.Metadata.Duration - 1
		}
		data, err := run(ctx, t.maxFrame, t.ffmpeg, "-v", "error", "-nostdin", "-ss", fmt.Sprintf("%.6f", float64(at)/1000), "-i", path, "-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "pipe:1")
		if err != nil {
			return info, nil, err
		}
		if len(data) == 0 {
			return info, nil, mediaError(contextir.CodePerception, "frame", "The decoder returned an empty sampled frame.", nil)
		}
		frames = append(frames, Frame{Time: at, PNG: data})
	}
	return info, frames, nil
}

type limitedBuffer struct {
	bytes.Buffer
	max int64
}

var errOutputLimit = errors.New("media tool output limit")

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if int64(b.Len())+int64(len(p)) > b.max {
		return 0, errOutputLimit
	}
	return b.Buffer.Write(p)
}
func run(ctx context.Context, limit int64, path string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, mediaError(contextir.CodeCanceled, "context", "Media inspection was canceled.", err)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	buffer := &limitedBuffer{max: limit}
	cmd.Stdout = buffer
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, mediaError(contextir.CodeCanceled, "context", "Media inspection was canceled.", ctx.Err())
		}
		if errors.Is(err, errOutputLimit) {
			return nil, mediaError(contextir.CodeLimit, "output", "Decoded media exceeds the configured byte limit.", nil)
		}
		return nil, mediaError(contextir.CodePerception, "decoder", "The media tool could not inspect or decode this source.", nil)
	}
	return append([]byte(nil), buffer.Bytes()...), nil
}
func mediaError(code contextir.Code, path, message string, cause error) *contextir.Error {
	return &contextir.Error{Code: code, Stage: "media", Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}, Cause: cause}
}
