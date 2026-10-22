package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ly95/h3-context-ir-go/chatcompat"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type backendEnvelope struct {
	Result any             `json:"result"`
	Model  string          `json:"model,omitempty"`
	Error  *backendFailure `json:"error,omitempty"`
}
type backendFailure struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	Status        int    `json:"status,omitempty"`
	ServerMessage string `json:"server_message,omitempty"`
}
type fixtureTransport struct{ inner http.RoundTripper }

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, e := f.inner.RoundTrip(r)
	if e != nil {
		return resp, e
	}
	kind := resp.Header.Get("X-Conformance-Transport-Error")
	if kind == "" {
		return resp, nil
	}
	resp.Body.Close()
	if kind == "ConnectError" {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused by test transport")}
	}
	if kind == "ConnectTimeout" {
		return nil, fixtureTimeout{}
	}
	return nil, errors.New("test transport request failed")
}

type fixtureTimeout struct{}

func (fixtureTimeout) Error() string   { return "test transport timed out" }
func (fixtureTimeout) Timeout() bool   { return true }
func (fixtureTimeout) Temporary() bool { return true }
func backendOperation(name string, raw json.RawMessage) (any, error) {
	var a struct {
		Config struct {
			BaseURL       string  `json:"base_url"`
			APIKey        string  `json:"api_key"`
			Model         string  `json:"model"`
			ResolvedModel string  `json:"resolved_model"`
			MaxTokens     int     `json:"max_tokens"`
			Timeout       float64 `json:"timeout_s"`
			Thinking      bool    `json:"default_thinking"`
			Guided        bool    `json:"guided_decoding"`
		} `json:"config"`
		ProxyURL       string               `json:"proxy_url"`
		Messages       []chatcompat.Message `json:"messages"`
		Thinking       *bool                `json:"thinking"`
		MaxTokens      *int                 `json:"max_tokens"`
		Temperature    *float64             `json:"temperature"`
		Seed           *int64               `json:"seed"`
		ResponseFormat json.RawMessage      `json:"response_format"`
		Stop           []string             `json:"stop"`
		Retries        int                  `json:"retries"`
		Schema         json.RawMessage      `json:"schema"`
		Required       []string             `json:"required"`
		Text           string               `json:"text"`
		Body           string               `json:"body"`
		Content        any                  `json:"content"`
		Extra          string               `json:"extra"`
		Object         map[string]any       `json:"obj"`
		Path           string               `json:"path"`
		Images         []string             `json:"images"`
		Digits         string               `json:"digits"`
		Scale          int                  `json:"scale"`
		Pad            int                  `json:"pad"`
		Status         int                  `json:"status"`
	}
	if e := json.Unmarshal(raw, &a); e != nil {
		return nil, e
	}
	var value any
	var err error
	switch strings.TrimPrefix(name, "h3ir.backend.") {
	case "_server_sentence":
		return backendEnvelope{Result: chatcompat.ServerSentence(a.Body)}, nil
	case "_extract_json":
		return backendEnvelope{Result: chatcompat.ExtractJSON(a.Text)}, nil
	case "_append_text":
		v, e := chatcompat.AppendText(a.Content, a.Extra)
		return wrapBackend(v, "", e), nil
	case "_is_the_schema_itself":
		return backendEnvelope{Result: chatcompat.IsSchemaItself(a.Object)}, nil
	case "image_data_url":
		v, e := chatcompat.ImageDataURLFile(a.Path)
		return wrapBackend(v, "", e), nil
	case "user_message":
		images := []chatcompat.Image{}
		for _, path := range a.Images {
			uri, e := chatcompat.ImageDataURLFile(path)
			if e != nil {
				return wrapBackend(nil, "", e), nil
			}
			images = append(images, chatcompat.Image{URI: uri})
		}
		v, e := chatcompat.UserMessage(a.Text, images)
		return wrapBackend(v, "", e), nil
	case "digits_png":
		v, e := chatcompat.DigitsPNG(a.Digits, a.Scale, a.Pad)
		return wrapBackend(v, "", e), nil
	case "judges_the_picture":
		return backendEnvelope{Result: chatcompat.JudgesThePicture(a.Status)}, nil
	}
	// The production URL is preserved. This test-only dialer sends its HTTP bytes
	// to the loopback server that invokes the original pytest MockTransport.
	proxy, e := url.Parse(a.ProxyURL)
	if e != nil || proxy.Hostname() != "127.0.0.1" {
		return nil, errors.New("backend conformance proxy must be loopback")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", proxy.Host)
	}, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	hard := a.Config.MaxTokens
	if hard < 32000 {
		hard = 32000
	}
	def := a.Config.MaxTokens
	if def <= 0 {
		def = 16000
	}
	model := a.Config.Model
	if model == "" {
		model = a.Config.ResolvedModel
	}
	discovery := chatcompat.SingleModel
	if model == "" && (name == "h3ir.backend.Backend.chat" || name == "h3ir.backend.Backend.json_call") {
		discovery = chatcompat.ServerDefault
	}
	structured := chatcompat.PromptSchema
	if a.Config.Guided {
		structured = chatcompat.GuidedSchema
	}
	c, e := chatcompat.New(chatcompat.Config{BaseURL: a.Config.BaseURL, APIKey: a.Config.APIKey, Model: model, Discovery: discovery, StructuredMode: structured, MaxOutputTokens: hard, DefaultOutputTokens: def, Timeout: time.Duration(a.Config.Timeout * float64(time.Second)), MaxResponseBytes: 16 << 20, HTTPClient: &http.Client{Transport: fixtureTransport{transport}}, AllowInsecureHTTP: true, Thinking: a.Config.Thinking})
	if e != nil {
		return wrapBackend(nil, "", e), nil
	}
	defer c.Close()
	ctx := context.Background()
	max := 0
	if a.MaxTokens != nil {
		max = *a.MaxTokens
	}
	if string(a.ResponseFormat) == "null" {
		a.ResponseFormat = nil
	}
	switch strings.TrimPrefix(name, "h3ir.backend.") {
	case "Backend.base_url":
		value = c.BaseURL()
	case "Backend._root_url":
		value = c.RootURL()
	case "Backend.model_id":
		value = c.ModelID()
	case "Backend._headers":
		h := c.Headers()
		m := map[string]string{}
		if v := h.Get("Authorization"); v != "" {
			m["Authorization"] = v
		}
		value = m
	case "Backend.liveness_urls":
		value = c.LivenessURLs()
	case "Backend.health_probe":
		h := c.HealthProbe(ctx)
		pairs := [][]string{}
		for _, x := range h.Attempts {
			pairs = append(pairs, []string{x.URL, x.Result})
		}
		value = map[string]any{"ok": h.OK, "via": h.Via, "attempts": pairs}
	case "Backend.health":
		value = c.HealthProbe(ctx).OK
	case "Backend._discover_model":
		value, err = c.DiscoverModel(ctx)
	case "Backend.require_available":
		err = c.RequireAvailable(ctx)
	case "Backend.server_version":
		v, _ := c.ServerVersion(ctx)
		if v == "" {
			v = "?"
		}
		value = v
	case "Backend.chat":
		value, err = c.Chat(ctx, a.Messages, chatcompat.ChatOptions{Thinking: a.Thinking, MaxTokens: max, Temperature: a.Temperature, Seed: a.Seed, ResponseFormat: a.ResponseFormat, Stop: a.Stop, Retries: a.Retries, GrowOnTruncation: true})
	case "Backend.json_call":
		var r chatcompat.JSONReply
		r, err = c.JSONCall(ctx, a.Messages, a.Schema, chatcompat.JSONOptions{Thinking: a.Thinking, Required: a.Required, MaxTokens: max, Temperature: a.Temperature, Seed: a.Seed, Retries: a.Retries})
		value = r.Value
	case "vision_check":
		var ok bool
		var said string
		ok, said, err = c.VisionCheck(ctx)
		value = []any{ok, said}
	case "probe":
		value, err = c.Doctor(ctx)
	default:
		return nil, fmt.Errorf("unsupported backend operation %q", name)
	}
	return wrapBackend(value, c.ModelID(), err), nil
}
func wrapBackend(value any, model string, err error) backendEnvelope {
	e := backendEnvelope{Result: value, Model: model}
	if err != nil {
		f := &backendFailure{Code: "backend", Message: err.Error()}
		var typed *chatcompat.Error
		if errors.As(err, &typed) {
			f.Code = typed.Code
			f.Status = typed.StatusCode
			f.ServerMessage = typed.ServerMessage
		}
		e.Error = f
	}
	return e
}
