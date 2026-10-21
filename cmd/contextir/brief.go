package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	contextir "github.com/ly95/contextir"
	"github.com/ly95/contextir/chatcompat"
	"github.com/ly95/contextir/compiler"
	"github.com/ly95/contextir/deepseek"
	"github.com/ly95/contextir/lora"
	"github.com/ly95/contextir/media"
	"github.com/ly95/contextir/planner"
	"github.com/ly95/contextir/service"
	"github.com/ly95/contextir/uploads"
)

type pathsFlag []string

func (p *pathsFlag) String() string { return strings.Join(*p, ",") }
func (p *pathsFlag) Set(s string) error {
	if s == "" {
		return errors.New("path must not be empty")
	}
	*p = append(*p, s)
	return nil
}

type briefFlags struct {
	provider, base, model, keyEnv, reasoning, structured string
	tokens                                               int
	timeout                                              time.Duration
	insecure                                             bool
}
type providers struct {
	flags    briefFlags
	key      string
	http     *http.Client
	cleanups []func()
}

func (p *providers) make(effort string) (compiler.Model, deepseek.ImageModel, error) {
	reason := p.flags.reasoning
	if effort == "fast" {
		reason = "none"
	}
	if effort == "max" {
		if p.flags.provider == "deepseek" {
			reason = "max"
		} else {
			reason = "thinking"
		}
	}
	switch p.flags.provider {
	case "deepseek":
		c, e := deepseek.New(deepseek.Config{BaseURL: p.flags.base, APIKey: p.key, Model: p.flags.model, MaxOutputTokens: p.flags.tokens, ReasoningEffort: reason, Timeout: p.flags.timeout, MaxResponseBytes: 8 << 20, HTTPClient: p.http})
		return c, c, e
	case "chatcompat":
		if reason != "none" && reason != "thinking" {
			return nil, nil, errors.New("chatcompat requires reasoning-effort none or thinking")
		}
		if reason == "thinking" && p.flags.structured == chatcompat.GuidedSchema {
			return nil, nil, errors.New("thinking requires prompt structured mode; no execution mode was silently changed")
		}
		c, e := chatcompat.New(chatcompat.Config{BaseURL: p.flags.base, APIKey: p.key, Model: p.flags.model, Discovery: chatcompat.ExplicitModel, StructuredMode: p.flags.structured, MaxOutputTokens: p.flags.tokens, Timeout: p.flags.timeout, MaxResponseBytes: 8 << 20, HTTPClient: p.http, AllowInsecureHTTP: p.flags.insecure, Thinking: reason == "thinking"})
		if e == nil {
			p.cleanups = append(p.cleanups, c.Close)
		}
		return c, c, e
	}
	return nil, nil, errors.New("choose an explicit provider: deepseek or chatcompat")
}
func (p *providers) close() {
	for _, f := range p.cleanups {
		f()
	}
	p.http.CloseIdleConnections()
}

func runBrief(command string, args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(errOut)
	input := fs.String("input", "-", "Original Brief request JSON file, or - for stdin")
	profileName := fs.String("profile", "official", "H3 profile: official or comfyui")
	framePolicy := fs.String("frame-policy", "exact", "Frame policy: exact or snap_up")
	clarification := fs.String("clarification-policy", "require", "Question policy: require or explicitly continue_with_recorded_choice")
	maxRetries := fs.Int("max-retries", 0, "Additional bounded compiler/schema repair calls (0..3)")
	allowPaths := fs.Bool("allow-paths", false, "Authorize reading local paths named in the request")
	uploadDirectory := fs.String("upload-store-dir", "", "Explicit existing content-addressed upload store directory")
	evidencePath := fs.String("evidence", "", "Optional JSON array of actual observed root Evidence records")
	ffprobe := fs.String("ffprobe", "", "Explicit absolute ffprobe executable path for video/audio")
	ffmpeg := fs.String("ffmpeg", "", "Explicit absolute ffmpeg executable path for video sampling")
	fractionsText := fs.String("frame-fractions", "0.15,0.5,0.85", "Increasing interior video sample fractions")
	partial := fs.Bool("allow-partial-perception", false, "Explicitly admit sampled visual or metadata-only audio coverage")
	cacheDirectory := fs.String("frame-cache-dir", "", "Optional explicit frame-cache directory")
	var loraRoots pathsFlag
	fs.Var(&loraRoots, "lora-dir", "Explicit LoRA registry directory; may be repeated")
	var f briefFlags
	fs.StringVar(&f.provider, "provider", "", "Explicit model provider: deepseek or chatcompat; omitted draft is offline")
	fs.StringVar(&f.base, "base-url", "", "Explicit provider API root")
	fs.StringVar(&f.model, "model", "", "Explicit provider model ID")
	fs.StringVar(&f.keyEnv, "key-env", "", "Name of the one environment variable to read for the API key")
	fs.StringVar(&f.reasoning, "reasoning-effort", "", "DeepSeek none|low|high|max; chatcompat none|thinking")
	fs.StringVar(&f.structured, "structured-mode", chatcompat.PromptSchema, "Chat schema protocol: prompt or json_schema")
	fs.IntVar(&f.tokens, "max-output-tokens", 8192, "Hard output-token ceiling for each provider call")
	fs.DurationVar(&f.timeout, "timeout", 5*time.Minute, "Total compile timeout and maximum request duration")
	fs.BoolVar(&f.insecure, "allow-insecure-http", false, "Authorize a non-loopback HTTP chatcompat endpoint")
	if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
		return 2
	}
	if *maxRetries < 0 || *maxRetries > 3 || f.tokens <= 0 || f.timeout <= 0 {
		return messageError(errOut, "invalid-configuration", "Retries must be 0..3 and token/time limits must be positive")
	}
	profile, e := selectedProfile(*profileName)
	if e != nil {
		return messageError(errOut, "invalid-profile", e.Error())
	}
	if *framePolicy != "exact" && *framePolicy != "snap_up" {
		return messageError(errOut, "invalid-frame-policy", "Choose exact or snap_up")
	}
	if *clarification != "require" && *clarification != "continue_with_recorded_choice" {
		return messageError(errOut, "invalid-clarification-policy", "Choose require or continue_with_recorded_choice")
	}
	if f.provider == "" && (command == "brief" || f.base != "" || f.model != "" || f.keyEnv != "" || f.reasoning != "") {
		return messageError(errOut, "provider-required", "Choose -provider deepseek or chatcompat explicitly; draft without provider is offline")
	}
	if f.provider != "" && (f.base == "" || f.model == "" || f.reasoning == "") {
		return messageError(errOut, "invalid-configuration", "A selected provider requires -base-url, -model and -reasoning-effort")
	}
	if f.provider != "" && f.provider != "deepseek" && f.provider != "chatcompat" {
		return messageError(errOut, "invalid-provider", "Choose deepseek or chatcompat")
	}
	if f.provider == "deepseek" && f.keyEnv == "" {
		return messageError(errOut, "key-env-required", "DeepSeek requires an explicitly named -key-env; keys are never read from dotenv")
	}
	payload, e := readInput(*input, in, maxCLIInputBytes)
	if e != nil {
		return messageError(errOut, "invalid-input", e.Error())
	}
	var store *uploads.Store
	if *uploadDirectory != "" {
		directory, err := filepath.Abs(*uploadDirectory)
		if err != nil {
			return messageError(errOut, "invalid-store", "Upload-store directory cannot be resolved")
		}
		store, e = uploads.New(uploads.Config{Directory: directory})
		if e != nil {
			return report(errOut, e)
		}
	}
	original, e := service.PrepareRequest(service.PrepareConfig{Store: store, AllowPaths: *allowPaths, MaxRequestBytes: maxCLIInputBytes}, payload)
	if e != nil {
		return reportBoundary(errOut, e)
	}
	for i := range original.Assets {
		if original.Assets[i].Path != nil {
			absolute, err := filepath.Abs(*original.Assets[i].Path)
			if err != nil {
				return messageError(errOut, "invalid-path", "An attachment path cannot be resolved")
			}
			original.Assets[i].Path = &absolute
		}
	}
	native, e := compiler.FromOriginal(original, nil)
	if e != nil {
		return report(errOut, e)
	}
	if command == "draft" && (native.Seed != nil || native.Effort != "" && native.Effort != "standard") {
		return messageError(errOut, "unsupported-execution", "Deterministic drafting does not implement model seed/effort changes; omit those choices or use brief")
	}
	if *evidencePath != "" {
		b, err := readInput(*evidencePath, in, maxCLIInputBytes)
		if err != nil {
			return messageError(errOut, "invalid-evidence", err.Error())
		}
		var rows []json.RawMessage
		if err = json.Unmarshal(b, &rows); err != nil {
			return messageError(errOut, "invalid-evidence", "Evidence must be a JSON array")
		}
		for _, row := range rows {
			item, err := contextir.DecodeEvidence(bytes.NewReader(row))
			if err != nil {
				return report(errOut, err)
			}
			native.Evidence = append(native.Evidence, item)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	var tools *media.Tools
	if *ffprobe != "" || *ffmpeg != "" {
		if !filepath.IsAbs(*ffprobe) || !filepath.IsAbs(*ffmpeg) {
			return messageError(errOut, "invalid-tools", "Supply both ffprobe and ffmpeg as explicit absolute executable paths")
		}
		tools, e = media.NewTools(media.Options{FFprobe: *ffprobe, FFmpeg: *ffmpeg, MaxInputBytes: 1 << 30, MaxFrameBytes: 32 << 20})
		if e != nil {
			return report(errOut, e)
		}
	}
	for i := range native.Assets {
		a := &native.Assets[i]
		if a.URI == "" {
			return messageError(errOut, "invalid-path", "Each attachment must resolve to a local file")
		}
		header, err := media.ValidateDeclaration(a.URI, a.Kind)
		if err != nil {
			return report(errOut, err)
		}
		if a.Kind == contextir.MediaImage {
			a.Metadata.Width = header.Width
			a.Metadata.Height = header.Height
		} else {
			if tools == nil {
				return messageError(errOut, "tools-required", "Video and audio require explicit -ffprobe and -ffmpeg paths")
			}
			info, err := tools.Inspect(ctx, a.URI)
			if err != nil {
				return report(errOut, err)
			}
			a.Metadata = info.Metadata
			a.Frames = info.Frames
		}
	}
	var registry *lora.Registry
	if len(loraRoots) > 0 {
		roots := []string{}
		for _, p := range loraRoots {
			absolute, err := filepath.Abs(p)
			if err != nil {
				return messageError(errOut, "invalid-lora-registry", "A registry path cannot be resolved")
			}
			roots = append(roots, absolute)
		}
		registry, e = lora.LoadRegistry(roots)
		if e != nil {
			return report(errOut, e)
		}
	}
	makePerceiver := func(imageModel deepseek.ImageModel) (compiler.Perceiver, error) {
		if len(native.Assets) == 0 {
			return nil, nil
		}
		fractions, err := parseFractions(*fractionsText)
		if err != nil {
			return nil, err
		}
		var cache *media.FrameCache
		if *cacheDirectory != "" {
			if tools == nil {
				return nil, errors.New("a frame cache requires explicit ffprobe and ffmpeg tools")
			}
			absolute, err := filepath.Abs(*cacheDirectory)
			if err != nil {
				return nil, errors.New("frame-cache path cannot be resolved")
			}
			cache, err = media.NewFrameCache(tools, absolute)
			if err != nil {
				return nil, err
			}
		}
		return media.NewPerceiver(media.PerceiverOptions{Tools: tools, FrameCache: cache, Model: imageModel, Resolve: func(_ context.Context, a contextir.Asset) (string, error) {
			if !filepath.IsAbs(a.URI) {
				return "", errors.New("media resolver requires an authorized absolute local path")
			}
			return a.URI, nil
		}, FrameFractions: fractions, MaxImageRetries: *maxRetries})
	}
	var model compiler.Model
	var perceiver compiler.Perceiver
	var provider *providers
	if f.provider != "" {
		key := ""
		if f.keyEnv != "" {
			var exists bool
			key, exists = os.LookupEnv(f.keyEnv)
			if !exists || key == "" {
				return messageError(errOut, "credential-unavailable", "The explicitly named key environment variable is unset or empty")
			}
		}
		transport := &http.Transport{ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second}
		provider = &providers{flags: f, key: key, http: &http.Client{Transport: transport, Timeout: f.timeout}}
		defer provider.close()
		var imageModel deepseek.ImageModel
		model, imageModel, e = provider.make("standard")
		if e != nil {
			return reportBoundary(errOut, e)
		}
		perceiver, e = makePerceiver(imageModel)
		if e != nil {
			return reportBoundary(errOut, e)
		}
	}
	if f.provider == "" && len(native.Assets) > 0 {
		perceiver, e = makePerceiver(nil)
		if e != nil {
			return reportBoundary(errOut, e)
		}
	}
	options := compiler.Options{ClarificationPolicy: compiler.ClarificationPolicy(*clarification), Profile: profile, FramePolicy: compiler.FramePolicy(*framePolicy), MaxStructureRetries: *maxRetries, Planner: planner.Options{MaxRetries: *maxRetries, AllowPartialPerception: *partial}, Registry: registry}
	if provider != nil {
		options.ConfigureExecution = func(_ context.Context, exec compiler.ExecutionOptions, _ compiler.Model, _ compiler.Perceiver) (compiler.Model, compiler.Perceiver, error) {
			if exec.Seed != nil {
				return nil, nil, errors.New("this CLI has no declared provider capability for a model seed; explicit seed is unsupported")
			}
			m, im, err := provider.make(exec.Effort)
			if err != nil {
				return nil, nil, err
			}
			p, err := makePerceiver(im)
			return m, p, err
		}
	}
	c, e := compiler.New(model, perceiver, options)
	if e != nil {
		return report(errOut, e)
	}
	var result compiler.Result
	if command == "draft" {
		result, e = c.Draft(ctx, native)
	} else {
		result, e = c.Compile(ctx, native)
	}
	if e != nil {
		_ = json.NewEncoder(errOut).Encode(map[string]any{"status": "failed", "calls": result.Calls, "usage_complete": result.UsageComplete, "unmetered_asset_ids": result.UnmeteredAssetIDs})
		return reportBoundary(errOut, e)
	}
	return encodeOutput(out, map[string]any{"result": result})
}
func parseFractions(raw string) ([]float64, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 || len(parts) > 600 {
		return nil, errors.New("choose 1..600 frame fractions")
	}
	out := make([]float64, 0, len(parts))
	previous := 0.
	for _, p := range parts {
		n, e := strconv.ParseFloat(p, 64)
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= previous || n >= 1 {
			return nil, errors.New("frame fractions must be increasing and strictly between zero and one")
		}
		out = append(out, n)
		previous = n
	}
	return out, nil
}
func reportBoundary(out io.Writer, err error) int {
	var se *service.Error
	if errors.As(err, &se) && se != nil {
		return messageError(out, se.Code, se.Message)
	}
	var ce *contextir.Error
	if errors.As(err, &ce) && ce != nil {
		return report(out, err)
	}
	return messageError(out, "operation-failed", err.Error())
}
