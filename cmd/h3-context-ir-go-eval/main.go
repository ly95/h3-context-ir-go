package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/chatcompat"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/perception"
	"github.com/ly95/h3-context-ir-go/planner"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type callRecord struct {
	Stage            string          `json:"stage"`
	Model            string          `json:"model"`
	Usage            contextir.Usage `json:"usage"`
	UsageValid       bool            `json:"usage_valid"`
	AdapterSucceeded bool            `json:"adapter_succeeded"`
}

type caseResult struct {
	ID      string            `json:"id"`
	Passed  bool              `json:"passed"`
	Stage   string            `json:"stage"`
	Issues  []contextir.Issue `json:"issues,omitempty"`
	Calls   []callRecord      `json:"calls"`
	Shots   int               `json:"shots,omitempty"`
	Frames  int64             `json:"frames,omitempty"`
	Elapsed int64             `json:"elapsed_ms"`
}

type report struct {
	StartedAt              string       `json:"started_at"`
	API                    string       `json:"api"`
	RequestedModel         string       `json:"requested_model"`
	ReasoningEffort        string       `json:"reasoning_effort"`
	MaxOutputTokens        int          `json:"max_output_tokens"`
	PlannerVersion         string       `json:"planner_version"`
	MaxRetries             int          `json:"max_retries"`
	Cases                  []caseResult `json:"cases"`
	RequiresSemanticReview bool         `json:"requires_semantic_review"`
}

type recordedModel struct {
	client  *chatcompat.Model
	calls   *[]callRecord
	dir     string
	attempt int
}

func (m *recordedModel) Generate(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
	m.attempt++
	if err := os.WriteFile(filepath.Join(m.dir, fmt.Sprintf("model-input-%d.json", m.attempt)), req.Input, 0600); err != nil {
		return contextir.ModelResponse{Usage: contextir.Usage{InputTokens: -1, OutputTokens: -1}, UsageInvalid: true}, err
	}
	response, err := m.client.Generate(ctx, req)
	*m.calls = append(*m.calls, callRecord{Stage: req.Stage, Model: response.Model, Usage: response.Usage, UsageValid: !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0, AdapterSucceeded: err == nil})
	if len(response.JSON) != 0 {
		if writeErr := os.WriteFile(filepath.Join(m.dir, fmt.Sprintf("model-output-%d.json", m.attempt)), response.JSON, 0600); writeErr != nil {
			return response, writeErr
		}
	}
	return response, err
}

type recordedPerceiver struct {
	inner *perception.ImagePerceiver
	calls *[]callRecord
}

func (p *recordedPerceiver) Perceive(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
	result, err := p.inner.PerceiveWithUsage(ctx, asset)
	if result.Called {
		*p.calls = append(*p.calls, callRecord{Stage: "perception", Model: result.Response.Model, Usage: result.Response.Usage, UsageValid: !result.Response.UsageInvalid && result.Response.Usage.InputTokens >= 0 && result.Response.Usage.OutputTokens >= 0, AdapterSucceeded: err == nil})
	}
	return result.Evidence, err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	baseURL := flag.String("base-url", "", "Explicit OpenAI-compatible API root")
	model := flag.String("model", "", "Explicit image-capable model; the evaluation sends image inputs")
	output := flag.String("out", "", "New output directory for synthetic requests, documents, prompts, and report")
	keyEnv := flag.String("api-key-env", "", "Explicit environment variable containing the API key; omit for an unauthenticated endpoint")
	maxRetries := flag.Int("max-retries", 0, "Explicit extra planner calls per case on invalid model output, from 0 to 3")
	api := flag.String("api", string(chatcompat.Responses), "OpenAI-compatible API: responses or chat_completions")
	reasoning := flag.String("reasoning-effort", "", "Optional reasoning effort; omitted means do not send")
	flag.Parse()
	if flag.NArg() != 0 || *baseURL == "" || *model == "" || *output == "" {
		return errors.New("explicit -base-url, -model, and -out are required")
	}
	if *maxRetries < 0 || *maxRetries > 3 {
		return errors.New("-max-retries must be between 0 and 3")
	}
	key := ""
	if *keyEnv != "" {
		var exists bool
		key, exists = os.LookupEnv(*keyEnv)
		if !exists || key == "" {
			return errors.New("the explicitly named key environment variable is unset or empty")
		}
	}
	endpoint, endpointErr := url.Parse(*baseURL)
	secure := endpointErr == nil && endpoint.Scheme == "https"
	if endpointErr == nil && endpoint.Scheme == "http" {
		address := net.ParseIP(endpoint.Hostname())
		secure = strings.EqualFold(endpoint.Hostname(), "localhost") || address != nil && address.IsLoopback()
	}
	if endpointErr != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Opaque != "" || !secure {
		return errors.New("provide an HTTPS API root or a loopback HTTP API root without credentials, query or fragment")
	}
	httpClient := &http.Client{Transport: &http.Transport{ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second}, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer httpClient.CloseIdleConnections()
	client, err := chatcompat.NewModel(sdk.Client{Options: []option.RequestOption{option.WithBaseURL(*baseURL), option.WithAPIKey(key), option.WithHTTPClient(httpClient)}}, chatcompat.ModelOptions{API: chatcompat.API(*api), SupportsImages: true, Model: *model, MaxOutputTokens: 8192, ReasoningEffort: *reasoning})
	if err != nil {
		return err
	}
	if err := os.Mkdir(*output, 0700); err != nil {
		return errors.New("create a new evaluation output directory: destination must not already exist")
	}
	imageBytes, err := syntheticCircle()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*output, "synthetic-red-circle.png"), imageBytes, 0600); err != nil {
		return err
	}
	imageURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	perceiver, err := perception.NewImagePerceiver(client, func(_ context.Context, asset contextir.Asset) (perception.Image, error) {
		if asset.ID != "synthetic-red-circle" {
			return perception.Image{}, errors.New("unknown synthetic asset")
		}
		return perception.Image{URI: imageURI, Detail: "original"}, nil
	}, perception.PerceptionOptions{})
	if err != nil {
		return err
	}
	r := report{API: *api, StartedAt: time.Now().UTC().Format(time.RFC3339), RequestedModel: *model, ReasoningEffort: *reasoning, MaxOutputTokens: 8192, PlannerVersion: planner.PromptVersion, MaxRetries: *maxRetries, RequiresSemanticReview: true}
	for _, item := range corpus() {
		started := time.Now()
		cr := caseResult{ID: item.id, Stage: "plan", Calls: []callRecord{}}
		dir := filepath.Join(*output, item.id)
		if err := os.Mkdir(dir, 0700); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(dir, "request.json"), item.request); err != nil {
			return err
		}
		m := &recordedModel{client: client, calls: &cr.Calls, dir: dir}
		p, err := planner.New(m, &recordedPerceiver{inner: perceiver, calls: &cr.Calls}, planner.Options{MaxRetries: *maxRetries})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		result, planErr := p.Plan(ctx, item.request)
		cancel()
		if len(result.Evidence) != 0 {
			if err := writeJSON(filepath.Join(dir, "evidence.json"), result.Evidence); err != nil {
				return err
			}
		}
		if planErr == nil {
			cr.Stage = "compile"
			artifact, compileErr := h3.Compile(result.Document, h3.OfficialProfile())
			planErr = compileErr
			if planErr == nil {
				cr.Stage = "replay"
				encoded, encodeErr := contextir.Encode(result.Document)
				planErr = encodeErr
				if planErr == nil {
					reloaded, decodeErr := contextir.Decode(bytes.NewReader(encoded))
					planErr = decodeErr
					if planErr == nil {
						replay, replayErr := h3.Compile(reloaded, h3.OfficialProfile())
						planErr = replayErr
						if planErr == nil && replay.SHA256 != artifact.SHA256 {
							planErr = errors.New("persisted artifact changed on replay")
						}
					}
				}
				if planErr == nil {
					if err := writeJSON(filepath.Join(dir, "document.json"), result.Document); err != nil {
						return err
					}
					if err := writeJSON(filepath.Join(dir, "artifact.json"), artifact); err != nil {
						return err
					}
					if err := os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(artifact.Prompt), 0600); err != nil {
						return err
					}
					cr.Passed, cr.Stage, cr.Shots, cr.Frames = true, "passed", len(result.Document.Shots), artifact.Frames
				}
			}
		}
		if planErr != nil {
			var typed *contextir.Error
			if errors.As(planErr, &typed) && typed != nil {
				cr.Issues = append([]contextir.Issue(nil), typed.Issues...)
			}
		}
		cr.Elapsed = time.Since(started).Milliseconds()
		r.Cases = append(r.Cases, cr)
		if err := writeJSON(filepath.Join(*output, "report.json"), r); err != nil {
			return err
		}
		line, _ := json.Marshal(cr)
		fmt.Println(string(line))
	}
	for _, cr := range r.Cases {
		if !cr.Passed {
			return errors.New("evaluation failed; inspect report.json and the synthetic outputs")
		}
	}
	return nil
}

type evalCase struct {
	id      string
	request contextir.Request
}

func corpus() []evalCase {
	target := contextir.Target{Duration: 8000, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}}
	silence := []contextir.Constraint{{ID: "silent", Kind: contextir.ConstraintSilence, Text: "全片静音。"}}
	zero := contextir.Time(0)
	return []evalCase{
		{"text-red-ball", contextir.Request{Instruction: "白色背景中只有一个红色小球。固定镜头，红球从画面左侧滚到右侧，持续8秒，不出现人物、文字或其他物体。", Mode: contextir.ModeText, Target: target, Constraints: silence}},
		{"exact-dialogue", contextir.Request{Instruction: "两段各4秒的镜头：先拍安静的车站空景，再拍一辆缓缓驶离的列车。画外音narrator只说给定的一句话，贯穿两段镜头；保留中文原文，不增加其他台词或配乐。", Mode: contextir.ModeText, Target: target, Dialogue: []contextir.DialogueLine{{ID: "farewell", SpeakerIDs: []string{"narrator"}, Language: "zh-CN", Text: "我们明天见，好吗？", Voiceover: true}}, Constraints: []contextir.Constraint{{ID: "no-music", Kind: contextir.ConstraintNoMusic, Text: "不要配乐。"}}}},
		{"image-first-frame", contextir.Request{Instruction: "使用给定图片的实际内容作为完整起始帧。固定镜头，让图中圆形向右平移，保持它的真实颜色和白色背景，全片静音，不增加其他物体。素材description故意写错，请以实际图片观察证据为准。", Mode: contextir.ModeFirstFrame, Target: target, Assets: []contextir.Asset{{ID: "synthetic-red-circle", Kind: contextir.MediaImage, URI: "synthetic:red-circle", Description: "A blue square on a black background (deliberately incorrect fixture metadata).", Metadata: contextir.MediaMetadata{Width: 320, Height: 180}}}, Bindings: []contextir.AssetBinding{{Source: contextir.SourceRef{AssetID: "synthetic-red-circle", Track: contextir.TrackVisual}, Role: contextir.RoleFirstFrame, Anchor: &zero}}, Constraints: silence}},
	}
}

func syntheticCircle() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			pixel := color.RGBA{R: 255, G: 255, B: 255, A: 255}
			if (x-80)*(x-80)+(y-90)*(y-90) <= 30*30 {
				pixel = color.RGBA{R: 235, G: 24, B: 24, A: 255}
			}
			img.SetRGBA(x, y, pixel)
		}
	}
	var out bytes.Buffer
	err := png.Encode(&out, img)
	return out.Bytes(), err
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
