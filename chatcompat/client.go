package chatcompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ly95/h3-context-ir-go/internal/strictjson"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func New(c Config) (*Client, error) {
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("explicit HTTP(S) API base URL without credentials, query or fragment is required")
	}
	if u.Scheme == "http" && !c.AllowInsecureHTTP {
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, errors.New("remote HTTP requires explicit AllowInsecureHTTP")
		}
	}
	if c.Discovery == "" {
		c.Discovery = ExplicitModel
	}
	if c.Discovery != ExplicitModel && c.Discovery != SingleModel && c.Discovery != ServerDefault {
		return nil, errors.New("unsupported model discovery policy")
	}
	if c.Model == "" && c.Discovery != SingleModel && c.Discovery != ServerDefault {
		return nil, errors.New("an explicit model or single_model discovery policy is required")
	}
	if c.StructuredMode != PromptSchema && c.StructuredMode != GuidedSchema {
		return nil, errors.New("choose an explicit structured mode: prompt or json_schema")
	}
	if c.MaxOutputTokens <= 0 || c.Timeout <= 0 || c.MaxResponseBytes <= 0 || c.MaxResponseBytes == math.MaxInt64 || c.JSONRetries < 0 {
		return nil, errors.New("positive output/timeout/response limits and nonnegative retry count are required")
	}
	key := c.APIKey
	if key == "not-needed" {
		key = ""
	}
	if key != "" {
		for _, b := range []byte(key) {
			if b < 0x21 || b > 0x7e {
				return nil, errors.New("API key contains invalid header characters")
			}
		}
	}
	if c.DefaultOutputTokens == 0 {
		c.DefaultOutputTokens = c.MaxOutputTokens
	}
	if c.DefaultOutputTokens <= 0 || c.DefaultOutputTokens > c.MaxOutputTokens {
		return nil, errors.New("default output tokens must fit the explicit hard output limit")
	}
	var h http.Client
	if c.HTTPClient != nil {
		h = *c.HTTPClient
	}
	h.Timeout = c.Timeout
	if h.Transport == nil {
		h.Transport = &http.Transport{ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second}
	}
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	base := strings.TrimRight(c.BaseURL, "/")
	root := strings.TrimSuffix(base, "/v1")
	origin := *u
	origin.Path, origin.RawPath = "/", ""
	sdk := openaisdk.Client{Options: []option.RequestOption{
		option.WithBaseURL(origin.String()),
		option.WithAPIKey(key),
		option.WithHTTPClient(&h),
		option.WithMaxRetries(0),
		option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			res, err := next(req)
			if res != nil && res.Body != nil {
				res.Body = &boundedResponseBody{ReadCloser: res.Body, remaining: c.MaxResponseBytes}
			}
			return res, err
		}),
	}}
	return &Client{defaultTokens: c.DefaultOutputTokens, closeOwned: c.HTTPClient == nil || c.HTTPClient.Transport == nil, base: base, root: root, key: key, configuredModel: c.Model, discovery: c.Discovery, structured: c.StructuredMode, maxTokens: c.MaxOutputTokens, maxResponse: c.MaxResponseBytes, http: &h, sdk: sdk, thinking: c.Thinking, jsonRetries: c.JSONRetries, grow: c.GrowOnTruncation}, nil
}
func (c *Client) BaseURL() string { return c.base }
func (c *Client) RootURL() string { return c.root }
func (c *Client) ModelID() string {
	if c.configuredModel != "" {
		return c.configuredModel
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.resolved
}
func (c *Client) Headers() http.Header {
	h := http.Header{}
	if c.key != "" {
		h.Set("Authorization", "Bearer "+c.key)
	}
	return h
}
func (c *Client) Close() {
	if c.closeOwned {
		c.http.CloseIdleConnections()
	}
}
func (c *Client) request(ctx context.Context, method, target string, body any) ([]byte, int, error) {
	var data any
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, 0, e
		}
		data = b
	}
	var r *http.Response
	u, e := url.Parse(target)
	if e != nil {
		return nil, 0, e
	}
	e = c.sdk.Execute(ctx, method, strings.TrimPrefix(u.EscapedPath(), "/"), data, &r)
	if r == nil {
		return nil, 0, &Error{Code: "network", Message: "model endpoint request failed", Cause: e}
	}
	defer r.Body.Close()
	if errors.Is(e, errResponseTooLarge) {
		return nil, r.StatusCode, &Error{Code: "response_too_large", Message: "model response exceeds the configured byte limit"}
	}
	if limited, ok := r.Body.(*boundedResponseBody); e != nil && ok && limited.readErr != nil {
		return nil, r.StatusCode, &Error{Code: "read", Message: "cannot read model response", Cause: limited.readErr}
	}
	if e != nil && r.StatusCode < 400 {
		return nil, r.StatusCode, &Error{Code: "read", Message: "cannot read model response"}
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, c.maxResponse+1))
	if e != nil {
		if errors.Is(e, errResponseTooLarge) {
			return nil, r.StatusCode, &Error{Code: "response_too_large", Message: "model response exceeds the configured byte limit"}
		}
		return nil, r.StatusCode, &Error{Code: "read", Message: "cannot read model response", Cause: e}
	}
	if int64(len(b)) > c.maxResponse {
		return nil, r.StatusCode, &Error{Code: "response_too_large", Message: "model response exceeds the configured byte limit"}
	}
	return b, r.StatusCode, nil
}

var errResponseTooLarge = errors.New("response exceeds configured byte limit")

type boundedResponseBody struct {
	io.ReadCloser
	remaining int64
	readErr   error
}

func (b *boundedResponseBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, errResponseTooLarge
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		b.readErr = errResponseTooLarge
		return n, errResponseTooLarge
	}
	if err != nil && err != io.EOF {
		b.readErr = err
	}
	return n, err
}
func object(b []byte) (map[string]any, error) {
	if e := strictjson.Check(b, 512); e != nil {
		return nil, e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	if e := d.Decode(&m); e != nil {
		return nil, e
	}
	if m == nil {
		return nil, errors.New("response must be a JSON object")
	}
	var x any
	if e := d.Decode(&x); e != io.EOF {
		return nil, errors.New("response must contain one JSON object")
	}
	return m, nil
}
func ServerSentence(body string) string {
	text := body
	for i := 0; i < 4; i++ {
		var v any
		if e := json.Unmarshal([]byte(text), &v); e != nil {
			return strings.TrimSpace(text)
		}
		switch x := v.(type) {
		case string:
			text = x
			continue
		case map[string]any:
			if s, ok := x["error"].(string); ok {
				text = s
				continue
			}
			if m, ok := x["error"].(map[string]any); ok {
				x = m
			}
			if s, ok := x["message"].(string); ok && s != "" {
				text = s
				continue
			}
			if s, ok := x["detail"].(string); ok && s != "" {
				text = s
				continue
			}
		}
		return strings.TrimSpace(body)
	}
	return strings.TrimSpace(text)
}
func endpointError(status int, b []byte) *Error {
	return &Error{Code: "endpoint_refused", StatusCode: status, Message: fmt.Sprintf("HTTP %d", status), ServerMessage: ServerSentence(string(b))}
}
func num(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	x, e := n.Int64()
	return x, e == nil && x >= 0
}
