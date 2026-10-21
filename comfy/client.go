package comfy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	BaseURL          string
	HTTPClient       *http.Client
	Timeout          time.Duration
	MaxResponseBytes int64
	Headers          http.Header
}
type Client struct {
	base           string
	http           *http.Client
	maxBytes       int64
	headers        http.Header
	ownedTransport *http.Transport
}
type Error struct {
	Operation  string
	StatusCode int
	Code       string
}

// RequestError retains the cause without including private locators in Error().
type RequestError struct {
	Operation string
	Cause     error
}

func (e *RequestError) Error() string { return "ComfyUI " + e.Operation + " network request failed" }
func (e *RequestError) Unwrap() error { return e.Cause }

func (e *Error) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("ComfyUI %s failed: HTTP %d (%s)", e.Operation, e.StatusCode, e.Code)
	}
	return fmt.Sprintf("ComfyUI %s failed: %s", e.Operation, e.Code)
}
func New(c Config) (*Client, error) {
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("ComfyUI base URL must be an explicit http(s) URL without credentials, query or fragment")
	}
	if c.Timeout <= 0 {
		return nil, errors.New("ComfyUI HTTP timeout must be positive")
	}
	if c.MaxResponseBytes <= 0 || c.MaxResponseBytes == 1<<63-1 {
		return nil, errors.New("ComfyUI response byte limit must be positive")
	}
	var h http.Client
	var owned *http.Transport
	if c.HTTPClient != nil {
		h = *c.HTTPClient
	} else {
		transport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, errors.New("explicit HTTP client is required when the default transport is customized")
		}
		owned = transport.Clone()
		owned.Proxy = nil // Proxy policy is an explicit injected-transport choice, never an environment lookup.
		h.Transport = owned
	}
	h.Timeout = c.Timeout
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{strings.TrimRight(c.BaseURL, "/"), &h, c.MaxResponseBytes, c.Headers.Clone(), owned}, nil
}
func (c *Client) Close() {
	if c.ownedTransport != nil {
		c.ownedTransport.CloseIdleConnections()
	}
}
func (c *Client) request(ctx context.Context, method, path, contentType string, body io.Reader) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if e != nil {
		return nil, e
	}
	for k, values := range c.headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, e := c.http.Do(req)
	operation := strings.SplitN(path, "?", 2)[0]
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &RequestError{Operation: operation, Cause: e}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &Error{operation, res.StatusCode, "http_status"}
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, c.maxBytes+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > c.maxBytes {
		return nil, &Error{operation, 0, "response_too_large"}
	}
	return b, nil
}
func decodeObject(b []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	if e := d.Decode(&m); e != nil {
		return nil, e
	}
	if m == nil {
		return nil, errors.New("ComfyUI response must be a JSON object")
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return nil, errors.New("ComfyUI response must contain one JSON object")
	}
	return m, nil
}
func (c *Client) object(ctx context.Context, method, path string, payload any) (map[string]any, error) {
	var body io.Reader
	contentType := ""
	if payload != nil {
		b, e := json.Marshal(payload)
		if e != nil {
			return nil, e
		}
		body = bytes.NewReader(b)
		contentType = "application/json"
	}
	b, e := c.request(ctx, method, path, contentType, body)
	if e != nil {
		return nil, e
	}
	m, e := decodeObject(b)
	if e != nil {
		return nil, &Error{path, 0, "invalid_json_response"}
	}
	return m, nil
}
func (c *Client) Stats(ctx context.Context) (map[string]any, error) {
	return c.object(ctx, http.MethodGet, "/system_stats", nil)
}
func (c *Client) HasH3Nodes(ctx context.Context) (map[string]bool, error) {
	info, e := c.object(ctx, http.MethodGet, "/object_info", nil)
	if e != nil {
		return nil, e
	}
	m := map[string]bool{}
	for _, k := range h3NodeClasses {
		_, m[k] = info[k]
	}
	return m, nil
}

// UploadImage sends caller-supplied bytes over HTTP; filename must be a basename.
// It does not read a ComfyUI path or assume a shared filesystem.
func (c *Client) UploadImage(ctx context.Context, filename string, data io.Reader, subfolder string, overwrite bool) (string, error) {
	if filename == "" || filepath.Base(filename) != filename || strings.ContainsAny(filename, "/\\") {
		return "", errors.New("upload filename must be an explicit basename")
	}
	if data == nil {
		return "", errors.New("upload data reader is required")
	}
	pr, pw := io.Pipe()
	defer pr.Close()
	mw := multipart.NewWriter(pw)
	go func() {
		part, e := mw.CreateFormFile("image", filename)
		if e == nil {
			_, e = io.Copy(part, data)
		}
		if e == nil {
			e = mw.WriteField("subfolder", subfolder)
		}
		if e == nil {
			e = mw.WriteField("overwrite", fmt.Sprint(overwrite))
		}
		if e == nil {
			e = mw.Close()
		}
		pw.CloseWithError(e)
	}()
	b, e := c.request(ctx, http.MethodPost, "/upload/image", mw.FormDataContentType(), pr)
	if e != nil {
		return "", e
	}
	m, e := decodeObject(b)
	if e != nil {
		return "", &Error{"upload", 0, "invalid_json_response"}
	}
	name, ok := m["name"].(string)
	if !ok || name == "" {
		return "", &Error{"upload", 0, "missing_name"}
	}
	sub, exists := m["subfolder"]
	if !exists {
		return "", &Error{"upload", 0, "missing_subfolder"}
	}
	folder, ok := sub.(string)
	if !ok {
		return "", &Error{"upload", 0, "invalid_subfolder"}
	}
	if folder != "" {
		return folder + "/" + name, nil
	}
	return name, nil
}
func (c *Client) UploadImageFile(ctx context.Context, path, subfolder string, overwrite bool) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	return c.UploadImage(ctx, filepath.Base(path), f, subfolder, overwrite)
}
func (c *Client) View(ctx context.Context, filename, subfolder, kind string) ([]byte, error) {
	if filename == "" || kind == "" {
		return nil, errors.New("view filename and kind must be explicit")
	}
	q := url.Values{"filename": {filename}, "subfolder": {subfolder}, "type": {kind}}
	return c.request(ctx, http.MethodGet, "/view?"+q.Encode(), "", nil)
}
func (c *Client) Submit(ctx context.Context, g Graph, clientID string) (string, error) {
	if clientID == "" {
		return "", errors.New("client ID must be explicit")
	}
	if g == nil {
		return "", errors.New("graph is required")
	}
	m, e := c.object(ctx, http.MethodPost, "/prompt", map[string]any{"prompt": g, "client_id": clientID})
	if e != nil {
		return "", e
	}
	id, ok := m["prompt_id"].(string)
	if !ok || id == "" {
		return "", &Error{"submit", 0, "missing_prompt_id"}
	}
	return id, nil
}
func (c *Client) History(ctx context.Context, promptID string) (map[string]any, error) {
	if promptID == "" {
		return nil, errors.New("prompt ID must be explicit")
	}
	m, e := c.object(ctx, http.MethodGet, "/history/"+url.PathEscape(promptID), nil)
	if e != nil {
		return nil, e
	}
	v, exists := m[promptID]
	if !exists || v == nil {
		return map[string]any{}, nil
	}
	h, ok := v.(map[string]any)
	if !ok {
		return nil, &Error{"history", 0, "invalid_history"}
	}
	return h, nil
}

type WaitOptions struct {
	PollInterval time.Duration
	Timeout      time.Duration
}

func (c *Client) Wait(ctx context.Context, promptID string, o WaitOptions) (map[string]any, error) {
	if o.PollInterval <= 0 || o.Timeout <= 0 {
		return nil, errors.New("poll interval and job timeout must be positive")
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	for {
		h, e := c.History(ctx, promptID)
		if e != nil {
			return nil, e
		}
		raw, exists := h["status"]
		if exists && raw != nil {
			status, ok := raw.(map[string]any)
			if !ok {
				return nil, &Error{"wait", 0, "invalid_status"}
			}
			if state, _ := status["status_str"].(string); state == "error" {
				return nil, &Error{"wait", 0, "job_failed"}
			}
			if done, ok := status["completed"].(bool); ok && done {
				return h, nil
			}
		}
		timer := time.NewTimer(o.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

type ProbeReport struct {
	URL          string          `json:"url"`
	Reachable    bool            `json:"reachable"`
	ComfyUI      string          `json:"comfyui,omitempty"`
	Python       string          `json:"python,omitempty"`
	Device       string          `json:"device,omitempty"`
	VRAMTotalGiB float64         `json:"vram_total_gib,omitempty"`
	VRAMFreeGiB  float64         `json:"vram_free_gib,omitempty"`
	H3Nodes      map[string]bool `json:"h3_nodes,omitempty"`
}

func (c *Client) Probe(ctx context.Context) (ProbeReport, error) {
	p := ProbeReport{URL: c.base}
	stats, e := c.Stats(ctx)
	if e != nil {
		return p, e
	}
	p.Reachable = true
	system, _ := stats["system"].(map[string]any)
	p.ComfyUI, _ = system["comfyui_version"].(string)
	p.Python, _ = system["python_version"].(string)
	devices, _ := stats["devices"].([]any)
	if len(devices) > 0 {
		if d, ok := devices[0].(map[string]any); ok {
			p.Device, _ = d["name"].(string)
			if v, ok := numeric(d["vram_total"]); ok {
				x, e := v.Float64()
				if e != nil {
					return p, e
				}
				p.VRAMTotalGiB = math.RoundToEven(x/(1<<30)*100) / 100
			}
			if v, ok := numeric(d["vram_free"]); ok {
				x, e := v.Float64()
				if e != nil {
					return p, e
				}
				p.VRAMFreeGiB = math.RoundToEven(x/(1<<30)*100) / 100
			}
		}
	}
	p.H3Nodes, e = c.HasH3Nodes(ctx)
	return p, e
}
