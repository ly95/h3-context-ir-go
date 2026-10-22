package chatcompat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	contextir "github.com/ly95/h3-context-ir-go"
	"net/http"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"
)

func ImageDataURL(data []byte) (string, error) {
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return "", errors.New("image bytes are not a supported PNG/JPEG/GIF/WebP format")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
func ImageDataURLFile(path string) (string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return "", e
	}
	return ImageDataURL(b)
}
func UserMessage(text string, images []Image) (Message, error) {
	if !utf8.ValidString(text) {
		return Message{}, errors.New("message text must be valid UTF-8")
	}
	if len(images) == 0 {
		return Message{Role: "user", Content: text}, nil
	}
	parts := []Part{{Type: "text", Text: text}}
	for i, img := range images {
		if img.URI == "" {
			return Message{}, errors.New("image URI is required")
		}
		switch img.Detail {
		case "", "auto", "low", "high", "original":
		default:
			return Message{}, errors.New("unsupported image detail")
		}
		if strings.HasPrefix(img.URI, "data:") {
			prefix, payload, ok := strings.Cut(img.URI, ",")
			if !ok || !strings.HasSuffix(prefix, ";base64") {
				return Message{}, errors.New("image data URL must be base64")
			}
			mime := strings.TrimSuffix(strings.TrimPrefix(prefix, "data:"), ";base64")
			switch mime {
			case "image/png", "image/jpeg", "image/gif", "image/webp":
			default:
				return Message{}, errors.New("unsupported image data URL MIME")
			}
			b, e := base64.StdEncoding.Strict().DecodeString(payload)
			if e != nil || len(b) == 0 {
				return Message{}, errors.New("image data URL bytes are empty or invalid")
			}
			if actual := http.DetectContentType(b); actual != mime {
				return Message{}, errors.New("declared image MIME does not match its bytes")
			}
		} else {
			u, e := url.Parse(img.URI)
			if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
				return Message{}, errors.New("image must be an explicit HTTP(S) or data URI")
			}
		}
		parts = append(parts, Part{Type: "text", Text: fmt.Sprintf("Image %d:", i+1)}, Part{Type: "image_url", ImageURL: &ImageURL{URL: img.URI, Detail: img.Detail}})
	}
	return Message{Role: "user", Content: parts}, nil
}
func (c *Client) Generate(ctx context.Context, req contextir.ModelRequest) (contextir.ModelResponse, error) {
	return c.generate(ctx, req, nil)
}
func (c *Client) GenerateWithImages(ctx context.Context, req contextir.ModelRequest, images []Image) (contextir.ModelResponse, error) {
	if len(images) == 0 {
		return contextir.ModelResponse{}, errors.New("at least one actual image is required")
	}
	return c.generate(ctx, req, images)
}
func (c *Client) generate(ctx context.Context, req contextir.ModelRequest, images []Image) (contextir.ModelResponse, error) {
	response := contextir.ModelResponse{}
	if c.discovery == ServerDefault && c.ModelID() == "" {
		return response, errors.New("planner Model adapter requires a named model; server_default is a low-level Chat policy only")
	}
	if c.jsonRetries != 0 {
		return response, errors.New("planner Model adapter requires JSONRetries=0; use JSONCall for explicit per-attempt retry metering")
	}
	if !json.Valid(req.Input) {
		return response, errors.New("model input must be valid JSON")
	}
	schema, e := object(req.ResponseSchema)
	if e != nil {
		return response, errors.New("model response schema must be a JSON object")
	}
	var required []string
	if raw, ok := schema["required"].([]any); ok {
		for _, v := range raw {
			s, ok := v.(string)
			if !ok {
				return response, errors.New("schema required items must be strings")
			}
			required = append(required, s)
		}
	}
	user, e := UserMessage(string(req.Input), images)
	if e != nil {
		return response, e
	}
	messages := []Message{{Role: "system", Content: req.System}, user}
	thinking := c.thinking
	reply, e := c.JSONCall(ctx, messages, req.ResponseSchema, JSONOptions{Required: required, MaxTokens: c.defaultTokens, Thinking: &thinking, Retries: c.jsonRetries, RequireUsage: true, RequireModel: true})
	r := reply.Reply
	response.JSON = reply.JSON
	response.Model = r.Model
	response.Usage = contextir.Usage{InputTokens: r.TotalPromptTokens, OutputTokens: r.TotalCompletionTokens}
	response.UsageInvalid = r.UsageInvalid || !r.UsageReported
	return response, e
}
