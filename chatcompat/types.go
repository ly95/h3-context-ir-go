// Package chatcompat implements explicit Chat Completions text/vision adapters.
// Portability and probe contracts are ported from OpenH3-IR backend.py at
// fd031e136ae3d89147324c0d1b2aa65e838c21f5 (Apache-2.0).
package chatcompat

import (
	"encoding/json"
	"fmt"
	"github.com/ly95/contextir"
	"net/http"
	"sync"
	"time"
)

const (
	ExplicitModel = "explicit"
	SingleModel   = "single_model"
	ServerDefault = "server_default"
	PromptSchema  = "prompt"
	GuidedSchema  = "json_schema"
)

type Config struct {
	DefaultOutputTokens                               int
	BaseURL, APIKey, Model, Discovery, StructuredMode string
	MaxOutputTokens                                   int
	Timeout                                           time.Duration
	MaxResponseBytes                                  int64
	HTTPClient                                        *http.Client
	AllowInsecureHTTP                                 bool
	Thinking                                          bool
	JSONRetries                                       int
	GrowOnTruncation                                  bool
}
type Client struct {
	defaultTokens                                           int
	closeOwned                                              bool
	base, root, key, configuredModel, discovery, structured string
	maxTokens                                               int
	maxResponse                                             int64
	http                                                    *http.Client
	thinking                                                bool
	jsonRetries                                             int
	grow                                                    bool
	mu                                                      sync.RWMutex
	resolved                                                string
}
type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}
type Image = contextir.ImageInput
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}
type Part struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}
type ChatOptions struct {
	Thinking         *bool
	MaxTokens        int
	Temperature      *float64
	Seed             *int64
	ResponseFormat   json.RawMessage
	Stop             []string
	Retries          int
	GrowOnTruncation bool
	RequireUsage     bool
	RequireModel     bool
}
type Reply struct {
	Content               string  `json:"content"`
	Reasoning             string  `json:"reasoning"`
	PromptTokens          int64   `json:"prompt_tokens"`
	CompletionTokens      int64   `json:"completion_tokens"`
	FinishReason          string  `json:"finish_reason"`
	WallSeconds           float64 `json:"wall_s"`
	Model                 string  `json:"model"`
	UsageReported         bool    `json:"usage_reported"`
	UsageInvalid          bool    `json:"usage_invalid"`
	Attempts              int     `json:"attempts"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
}

func (r Reply) ReasoningShare() float64 {
	if r.CompletionTokens == 0 {
		return 0
	}
	total := len([]rune(r.Content)) + len([]rune(r.Reasoning))
	if total == 0 {
		return 0
	}
	return float64(len([]rune(r.Reasoning))) / float64(total)
}

type Error struct {
	Code, Message string
	StatusCode    int
	ServerMessage string
	Cause         error
}

func (e *Error) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.ServerMessage)
	}
	return e.Message
}
func (e *Error) Unwrap() error { return e.Cause }

type Attempt struct {
	URL    string `json:"url"`
	Result string `json:"result"`
}
type HealthProbe struct {
	OK       bool      `json:"ok"`
	Via      string    `json:"via"`
	Attempts []Attempt `json:"attempts"`
}
type ModelInfo struct {
	ID          string `json:"id"`
	Root        string `json:"root,omitempty"`
	MaxModelLen *int64 `json:"max_model_len,omitempty"`
}
type JSONOptions struct {
	Required     []string
	MaxTokens    int
	Seed         *int64
	Temperature  *float64
	Thinking     *bool
	Retries      int
	RequireUsage bool
	RequireModel bool
}
type JSONReply struct {
	Value    map[string]any
	JSON     json.RawMessage
	Reply    Reply
	Attempts []Reply
}
