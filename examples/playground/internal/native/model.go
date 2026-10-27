package native

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	contextir "github.com/ly95/h3-context-ir-go"
)

type Options struct {
	Client                *http.Client
	BaseURL               string
	Model                 string
	APIKey                string
	Session               string
	AllowedResponseModels []string
}

type Model struct {
	client                *http.Client
	baseURL               string
	model                 string
	apiKey                string
	session               string
	allowedResponseModels []string
}

func New(options Options) (*Model, error) {
	u, err := url.Parse(options.BaseURL)
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, failure(contextir.CodeInvalid, "setup", "base_url", "Supply an explicit native Gemini base URL.", nil)
	}
	loopback := strings.EqualFold(u.Hostname(), "localhost")
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, failure(contextir.CodeInvalid, "setup", "base_url", "Use HTTPS or a loopback test server.", nil)
	}
	if options.Client == nil || !headerValue(options.APIKey) || !headerValue(options.Session) || !modelID(options.Model) {
		return nil, failure(contextir.CodeInvalid, "setup", "configuration", "Provide an HTTP client, model, credential and stable conversation session explicitly.", nil)
	}
	for _, model := range options.AllowedResponseModels {
		if !modelID(model) {
			return nil, failure(contextir.CodeInvalid, "setup", "allowed_response_models", "Each explicitly allowed response model must be a valid native identifier.", nil)
		}
	}
	client := *options.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Model{client: &client, baseURL: strings.TrimRight(options.BaseURL, "/"), model: options.Model, apiKey: options.APIKey, session: options.Session, allowedResponseModels: slices.Clone(options.AllowedResponseModels)}, nil
}

func (m *Model) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return m.enveloped(ctx, request, "", nil)
}

func (m *Model) GenerateMedia(ctx context.Context, request contextir.ModelRequest, mime string, data []byte) (contextir.ModelResponse, error) {
	if len(data) == 0 || mime == "" {
		return emptyResponse(), failure(contextir.CodeInvalid, "request", "media", "Supply explicit media bytes and MIME type.", nil)
	}
	switch mime {
	case "video/mp4", "video/webm", "audio/flac", "audio/x-flac", "audio/wav", "audio/x-wav", "audio/mpeg", "audio/mp3", "audio/ogg", "audio/aac", "audio/webm":
	default:
		return emptyResponse(), failure(contextir.CodeUnsupported, "request", "media_mime", "The native adapter has no declared support for this media MIME type.", nil)
	}
	return m.enveloped(ctx, request, mime, data)
}

func (m *Model) GenerateWithImages(ctx context.Context, request contextir.ModelRequest, images []contextir.ImageInput) (contextir.ModelResponse, error) {
	if len(images) == 0 {
		return m.Generate(ctx, request)
	}
	if len(images) != 1 {
		return emptyResponse(), failure(contextir.CodeUnsupported, "request", "images", "The native playground adapter accepts exactly one explicit image attachment.", nil)
	}
	input := images[0]
	if input.Detail != "" && input.Detail != "original" {
		return emptyResponse(), failure(contextir.CodeUnsupported, "request", "image_detail", "The native image path accepts original pixels without an implicit image-detail conversion.", nil)
	}
	if len(input.URI) > maxRequestBytes {
		return emptyResponse(), failure(contextir.CodeLimit, "request", "image", "The image data URI exceeds the explicit native request limit.", nil)
	}
	header, encoded, found := strings.Cut(input.URI, ",")
	if !found || header != "data:image/png;base64" && header != "data:image/jpeg;base64" {
		return emptyResponse(), failure(contextir.CodeUnsupported, "request", "image", "Supply one PNG or JPEG as an explicit base64 data URI.", nil)
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) == 0 {
		return emptyResponse(), failure(contextir.CodeInvalid, "request", "image", "The image data URI contains invalid or empty base64 bytes.", nil)
	}
	if len(data) > maxRequestBytes/2 {
		return emptyResponse(), failure(contextir.CodeLimit, "request", "image", "The original image exceeds the explicit native media limit.", nil)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || header == "data:image/png;base64" && format != "png" || header == "data:image/jpeg;base64" && format != "jpeg" {
		return emptyResponse(), failure(contextir.CodeInvalid, "request", "image", "The original image bytes do not match the declared PNG or JPEG container.", nil)
	}
	mime := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	return m.enveloped(ctx, request, mime, data)
}

func (m *Model) acceptsModel(model string) bool {
	return model == m.model || slices.Contains(m.allowedResponseModels, model)
}

func modelID(model string) bool {
	if model == "" {
		return false
	}
	for _, r := range model {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func headerValue(value string) bool {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}

var _ interface {
	Generate(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error)
	GenerateMedia(context.Context, contextir.ModelRequest, string, []byte) (contextir.ModelResponse, error)
	GenerateWithImages(context.Context, contextir.ModelRequest, []contextir.ImageInput) (contextir.ModelResponse, error)
} = (*Model)(nil)
