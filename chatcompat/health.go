package chatcompat

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

func (c *Client) LivenessURLs() []string { return []string{c.base + "/models", c.root + "/health"} }
func (c *Client) HealthProbe(ctx context.Context) HealthProbe {
	h := HealthProbe{Attempts: []Attempt{}}
	for _, target := range c.LivenessURLs() {
		_, status, e := c.request(ctx, http.MethodGet, target, nil)
		if e != nil {
			kind := "HTTPError"
			var op *net.OpError
			var ne net.Error
			if errors.As(e, &op) {
				kind = "ConnectError"
			} else if errors.As(e, &ne) && ne.Timeout() {
				kind = "ConnectTimeout"
			}
			h.Attempts = append(h.Attempts, Attempt{target, kind})
			if kind == "ConnectError" || kind == "ConnectTimeout" {
				break
			}
			continue
		}
		h.Attempts = append(h.Attempts, Attempt{target, fmt.Sprintf("HTTP %d", status)})
		if status == 200 {
			h.OK, h.Via = true, target
			return h
		}
	}
	return h
}
func (c *Client) Models(ctx context.Context) ([]ModelInfo, error) {
	b, status, e := c.request(ctx, http.MethodGet, c.base+"/models", nil)
	if e != nil {
		return nil, e
	}
	if status != 200 {
		return nil, endpointError(status, b)
	}
	m, e := object(b)
	if e != nil {
		return nil, e
	}
	if m["data"] == nil {
		return []ModelInfo{}, nil
	}
	raw, ok := m["data"].([]any)
	if !ok {
		return nil, errors.New("model list data must be an array or null")
	}
	models := []ModelInfo{}
	for _, x := range raw {
		entry, ok := x.(map[string]any)
		if !ok {
			return nil, errors.New("model list entry must be an object")
		}
		id, ok := entry["id"].(string)
		if !ok || id == "" {
			continue
		}
		r, _ := entry["root"].(string)
		info := ModelInfo{ID: id, Root: r}
		if v, exists := entry["max_model_len"]; exists && v != nil {
			n, valid := num(v)
			if !valid || n <= 0 {
				return nil, errors.New("reported model context length must be a positive integer")
			}
			info.MaxModelLen = &n
		}
		models = append(models, info)
	}
	return models, nil
}
func (c *Client) DiscoverModel(ctx context.Context) (string, error) {
	if c.configuredModel != "" {
		return c.configuredModel, nil
	}
	if c.discovery != SingleModel {
		return "", errors.New("H3IR_LLM_MODEL is not set; explicit single_model discovery permission is required")
	}
	entries, e := c.Models(ctx)
	if e != nil {
		return "", &Error{Code: "unavailable", Message: "H3IR_LLM_MODEL is not set and /models could not be read to discover it", Cause: e}
	}
	if len(entries) == 0 {
		return "", &Error{Code: "unavailable", Message: "H3IR_LLM_MODEL is not set and /models named no models"}
	}
	distinct := map[string]bool{}
	ids := make([]string, 0, len(entries))
	for _, m := range entries {
		root := m.Root
		if root == "" {
			root = m.ID
		}
		distinct[root] = true
		ids = append(ids, m.ID)
	}
	if len(distinct) > 1 {
		return "", &Error{Code: "unavailable", Message: "H3IR_LLM_MODEL is not set and several model weights are served; this will not guess. Set H3IR_LLM_MODEL to one of: " + strings.Join(ids, ", ")}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resolved != "" && c.resolved != ids[0] {
		return "", errors.New("model discovery changed after resolution; construct a new explicitly configured client")
	}
	c.resolved = ids[0]
	return ids[0], nil
}
func (c *Client) RequireAvailable(ctx context.Context) error {
	h := c.HealthProbe(ctx)
	if !h.OK {
		a := []string{}
		for _, v := range h.Attempts {
			a = append(a, v.URL+" -> "+v.Result)
		}
		return &Error{Code: "unavailable", Message: "the reasoning model is not reachable. Tried: " + strings.Join(a, "; ")}
	}
	if c.ModelID() == "" {
		_, e := c.DiscoverModel(ctx)
		return e
	}
	return nil
}

func (c *Client) ServerVersion(ctx context.Context) (string, []Attempt) {
	attempts := []Attempt{}
	for _, target := range []string{c.root + "/version", c.root + "/api/version"} {
		b, status, e := c.request(ctx, http.MethodGet, target, nil)
		if e != nil {
			attempts = append(attempts, Attempt{target, "request failed"})
			continue
		}
		attempts = append(attempts, Attempt{target, fmt.Sprintf("HTTP %d", status)})
		if status != 200 {
			continue
		}
		m, e := object(b)
		if e != nil {
			attempts[len(attempts)-1].Result = "invalid JSON"
			continue
		}
		if v, ok := m["version"].(string); ok && v != "" {
			return v, attempts
		}
	}
	return "", attempts
}
