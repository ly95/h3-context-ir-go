package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/chatcompat"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
)

const maxImageBytes = 10 << 20
const maxPromptBytes = 64 << 10
const maxRequestBytes = maxImageBytes + (256 << 10)

//go:embed web
var webFiles embed.FS

type imageInput struct {
	bytes  []byte
	width  int
	height int
	mime   string
}

type compileInput struct {
	prompt        string
	seconds       float64
	aspect        string
	shots         int
	creativity    compiler.Creativity
	noDialogue    bool
	noMusic       bool
	silence       bool
	role          compiler.Role
	image         *imageInput
	baseURL       string
	model         string
	api           string
	allowedModels []string
	key           string
}

type stageEvent struct {
	Stage        string `json:"stage"`
	Attempt      int    `json:"attempt"`
	Phase        string `json:"phase"`
	Model        string `json:"model,omitempty"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Valid        bool   `json:"valid"`
	Failed       bool   `json:"failed"`
}

type publicFailure struct {
	Code       string            `json:"code"`
	Stage      string            `json:"stage,omitempty"`
	Message    string            `json:"message"`
	Issues     []contextir.Issue `json:"issues,omitempty"`
	HTTPStatus int               `json:"http_status,omitempty"`
}

type compileResponse struct {
	compiler.Result
	Prompt    string                   `json:"prompt"`
	ElapsedMS int64                    `json:"elapsed_ms"`
	Error     *publicFailure           `json:"error,omitempty"`
	Questions []compiler.Clarification `json:"questions,omitempty"`
}

type compilePipeline func(context.Context, compileInput, func(stageEvent)) (compiler.Result, error)

type playground struct {
	settings settings
	sample   *imageInput
	run      compilePipeline
	static   http.Handler
}

func newServer(cfg settings) (*playground, error) {
	if !validEndpoint(cfg.baseURL) || strings.TrimSpace(cfg.model) == "" || cfg.api != string(chatcompat.Responses) && cfg.api != string(chatcompat.ChatCompletions) {
		return nil, errors.New("invalid provider settings")
	}
	files, err := fs.Sub(webFiles, "web")
	if err != nil {
		return nil, err
	}
	s := &playground{settings: cfg, run: compilePrompt, static: http.FileServer(http.FS(files))}
	if cfg.samplePath != "" {
		file, err := os.Open(cfg.samplePath)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
			_ = file.Close()
			if readErr == nil {
				s.sample, _ = decodeImage(data)
			}
		}
	}
	return s, nil
}

func (s *playground) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob: data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	host := r.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	if !loopbackHost(host) || r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
		writeFailure(w, http.StatusForbidden, "origin_rejected", "请从本机测试页发起请求。")
		return
	}
	switch r.URL.Path {
	case "/api/config":
		if r.Method != http.MethodGet {
			methodError(w, http.MethodGet)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"base_url": s.settings.baseURL, "model": s.settings.model, "api": s.settings.api, "allowed_response_models": s.settings.allowedModels, "credential_configured": s.settings.key != "", "max_image_bytes": maxImageBytes, "sample_available": s.sample != nil})
	case "/api/compile":
		if r.Method != http.MethodPost {
			methodError(w, http.MethodPost)
			return
		}
		s.compile(w, r)
	case "/sample.png":
		if r.Method != http.MethodGet || s.sample == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", s.sample.mime)
		_, _ = w.Write(s.sample.bytes)
	case "/", "/app.js", "/app.css":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodError(w, http.MethodGet+", "+http.MethodHead)
			return
		}
		s.static.ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
}

func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	value := r.Header.Get("Origin")
	if value == "" {
		return true
	}
	origin, err := url.Parse(value)
	return err == nil && origin.User == nil && origin.Scheme == "http" && origin.Host == r.Host && origin.Path == "" && origin.RawQuery == "" && origin.Fragment == ""
}

func (s *playground) compile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	input, err := s.readInput(r)
	if err != nil {
		writeFailure(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if _, ok := w.(http.Flusher); !ok {
		writeFailure(w, http.StatusInternalServerError, "stream_unavailable", "当前服务器不支持阶段进度。")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	writer := &eventWriter{response: w}
	w.(http.Flusher).Flush()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	started := time.Now()
	result, compileErr := s.run(ctx, input, func(event stageEvent) { writer.send("stage", event) })
	out := compileResponse{Result: result, ElapsedMS: time.Since(started).Milliseconds()}
	if compileErr != nil {
		out.Status = "failed"
		out.Error = safeFailure(compileErr)
		out.Document = contextir.Document{}
		out.Artifact = h3.Artifact{}
	} else if result.Status == "compiled" {
		out.Prompt = result.Artifact.Prompt
	} else if result.Status == "needs_input" {
		if result.ModeDecision != nil {
			out.Questions = result.ModeDecision.Questions
		}
	} else {
		out.Status = "failed"
		out.Error = &publicFailure{Code: "unexpected_status", Message: "编译没有生成可用结果。"}
		out.Document = contextir.Document{}
		out.Artifact = h3.Artifact{}
	}
	writer.send("result", out)
}

type eventWriter struct {
	mu       sync.Mutex
	response http.ResponseWriter
}

func (w *eventWriter) send(event string, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := fmt.Fprintf(w.response, "event: %s\ndata: %s\n\n", event, encoded); err == nil {
		w.response.(http.Flusher).Flush()
	}
}

func (s *playground) readInput(r *http.Request) (compileInput, error) {
	var out compileInput
	reader, err := r.MultipartReader()
	if err != nil {
		return out, errors.New("请使用表单提交提示词和参考图。")
	}
	fields := map[string]string{}
	allowed := map[string]bool{"prompt": true, "seconds": true, "aspect": true, "shots": true, "creativity": true, "no_dialogue": true, "no_music": true, "silence": true, "reference_role": true, "base_url": true, "model": true, "api": true, "allowed_response_models": true, "api_key": true, "sample": true}
	imageSeen := false
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return out, errors.New("请求不完整或超过上传大小限制。")
		}
		name := part.FormName()
		if part.FileName() != "" {
			if name != "image" || imageSeen {
				_ = part.Close()
				return out, errors.New("一次只接受一张参考图。")
			}
			imageSeen = true
			data, readErr := io.ReadAll(io.LimitReader(part, maxImageBytes+1))
			_ = part.Close()
			if readErr != nil {
				return out, errors.New("参考图读取失败。")
			}
			out.image, err = decodeImage(data)
			if err != nil {
				return out, err
			}
			continue
		}
		if !allowed[name] {
			_ = part.Close()
			return out, errors.New("请求包含未知字段。")
		}
		if _, exists := fields[name]; exists {
			_ = part.Close()
			return out, errors.New("请求包含重复字段。")
		}
		limit := maxPromptBytes
		if name != "prompt" {
			limit = 4096
		}
		data, readErr := io.ReadAll(io.LimitReader(part, int64(limit+1)))
		_ = part.Close()
		if readErr != nil || len(data) > limit || !utf8.Valid(data) {
			return out, errors.New("表单字段超长或不是有效的 UTF-8。")
		}
		fields[name] = string(data)
	}
	out.prompt = fields["prompt"]
	if strings.TrimSpace(out.prompt) == "" {
		return out, errors.New("请输入原始提示词。")
	}
	out.seconds, err = strconv.ParseFloat(defaultField(fields, "seconds", "5"), 64)
	if err != nil || math.IsNaN(out.seconds) || math.IsInf(out.seconds, 0) || out.seconds < 1 || out.seconds > 60 || math.Abs(out.seconds*24-math.Round(out.seconds*24)) > 1e-7 {
		return out, errors.New("时长须在 1–60 秒之间，并精确对应 24 fps 的整数帧。")
	}
	out.shots, err = strconv.Atoi(defaultField(fields, "shots", "1"))
	if err != nil || out.shots < 1 || out.shots > 10 || out.shots > 1 && out.seconds/float64(out.shots) < 1.2 {
		return out, errors.New("镜头数须为 1–10，多个镜头平均时长至少为 1.2 秒。")
	}
	out.aspect = defaultField(fields, "aspect", "16:9")
	if out.aspect != "16:9" && out.aspect != "9:16" && out.aspect != "1:1" && out.aspect != "4:3" && out.aspect != "3:4" {
		return out, errors.New("请选择支持的画幅。")
	}
	out.creativity = compiler.Creativity(defaultField(fields, "creativity", string(compiler.CreativityRestrained)))
	if out.creativity != compiler.CreativityRestrained && out.creativity != compiler.CreativityBalanced && out.creativity != compiler.CreativityBold && out.creativity != compiler.CreativityExtreme {
		return out, errors.New("请选择有效的创意程度。")
	}
	for _, item := range []struct {
		name   string
		target *bool
		preset string
	}{{"no_dialogue", &out.noDialogue, "true"}, {"no_music", &out.noMusic, "true"}, {"silence", &out.silence, "false"}} {
		value := defaultField(fields, item.name, item.preset)
		if value != "true" && value != "false" {
			return out, errors.New("音频选项须明确为 true 或 false。")
		}
		*item.target = value == "true"
	}
	out.role = compiler.Role(defaultField(fields, "reference_role", string(compiler.RoleSubject)))
	if out.role != compiler.RoleSubject && out.role != compiler.RoleEnvironment && out.role != compiler.RoleStyle && out.role != compiler.RoleFirstFrame {
		return out, errors.New("请选择明确的参考图用途。")
	}
	if value, exists := fields["sample"]; exists {
		if value != "true" && value != "false" {
			return out, errors.New("示例选项须为 true 或 false。")
		}
		if value == "true" {
			if s.sample == nil || out.image != nil {
				return out, errors.New("示例图不可用，或与上传图片重复。")
			}
			out.image = s.sample
		}
	}
	out.baseURL = defaultField(fields, "base_url", s.settings.baseURL)
	out.model = defaultField(fields, "model", s.settings.model)
	out.api = defaultField(fields, "api", s.settings.api)
	out.allowedModels = s.settings.allowedModels
	if value, exists := fields["allowed_response_models"]; exists {
		out.allowedModels = splitModels(value)
	}
	if !validEndpoint(out.baseURL) || strings.TrimSpace(out.model) == "" || out.api != string(chatcompat.Responses) && out.api != string(chatcompat.ChatCompletions) {
		return out, errors.New("请检查 API 地址、模型名称和接口类型。")
	}
	out.key = strings.TrimSpace(fields["api_key"])
	if out.key == "" {
		if out.baseURL != s.settings.baseURL {
			return out, errors.New("修改 API 地址时，请提供该地址使用的 API key。")
		}
		out.key = s.settings.key
	}
	if out.key == "" {
		return out, errors.New("请在连接设置中填写 API key，或在启动服务时配置密钥环境变量。")
	}
	return out, nil
}

func defaultField(fields map[string]string, key, preset string) string {
	if value, exists := fields[key]; exists {
		return value
	}
	return preset
}

func validEndpoint(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

func decodeImage(data []byte) (*imageInput, error) {
	if len(data) == 0 || len(data) > maxImageBytes {
		return nil, errors.New("参考图须小于或等于 10 MiB。")
	}
	config, kind, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || kind != "png" && kind != "jpeg" {
		return nil, errors.New("请上传真实的 PNG 或 JPEG 图片。")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16000000 {
		return nil, errors.New("参考图不得超过 1600 万像素。")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return nil, errors.New("参考图文件损坏或不完整。")
	}
	return &imageInput{bytes: data, width: config.Width, height: config.Height, mime: "image/" + kind}, nil
}

func safeFailure(err error) *publicFailure {
	out := &publicFailure{Code: "compile_failed", Message: "优化失败，请查看诊断与调用用量。"}
	var typed *contextir.Error
	if errors.As(err, &typed) {
		out.Code, out.Stage, out.Issues = string(typed.Code), typed.Stage, typed.Issues
	}
	var provider *chatcompat.HTTPError
	if errors.As(err, &provider) {
		out.HTTPStatus = provider.StatusCode
		out.Message = fmt.Sprintf("模型服务返回 HTTP %d，请检查连接设置或账户状态。", provider.StatusCode)
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeFailure(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"status": "failed", "error": publicFailure{Code: code, Message: message}})
}

func methodError(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeFailure(w, http.StatusMethodNotAllowed, "method_not_allowed", "不支持此请求方法。")
}
