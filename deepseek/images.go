package deepseek

import (
	"encoding/base64"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/ly95/h3-context-ir-go"
)

func validateImage(image Image) error {
	switch image.Detail {
	case "", "auto", "low", "high", "original":
	default:
		return failure(contextir.CodeUnsupported, "request", "images.detail", "Choose a supported image detail level.", nil)
	}
	if image.URI == "" || !utf8.ValidString(image.URI) {
		return failure(contextir.CodeInvalid, "request", "images.uri", "A valid UTF-8 image locator is required.", nil)
	}
	if len(image.URI) > maxRequestBytes {
		return failure(contextir.CodeLimit, "request", "images.uri", "The image locator exceeds the request-size limit.", nil)
	}
	if strings.HasPrefix(image.URI, "data:") {
		header, payload, found := strings.Cut(image.URI, ",")
		if !found || payload == "" {
			return failure(contextir.CodeInvalid, "request", "images.uri", "Provide a complete base64 image data URL.", nil)
		}
		switch header {
		case "data:image/jpeg;base64", "data:image/png;base64", "data:image/gif;base64", "data:image/webp;base64":
		default:
			return failure(contextir.CodeUnsupported, "request", "images.uri", "Inline images must use a supported image MIME type and base64 encoding.", nil)
		}
		const maxImageBytes = 32 << 20
		count, err := io.Copy(io.Discard, io.LimitReader(base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(payload)), maxImageBytes+1))
		if err != nil || count == 0 {
			return failure(contextir.CodeInvalid, "request", "images.uri", "The inline image must contain valid nonempty base64 data.", nil)
		}
		if count > maxImageBytes {
			return failure(contextir.CodeLimit, "request", "images.uri", "The inline image exceeds DeepSeek's 32 MiB per-image limit.", nil)
		}
		return nil
	}
	u, err := url.Parse(image.URI)
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.Fragment != "" || !secureURL(u) {
		return failure(contextir.CodeInvalid, "request", "images.uri", "Image URLs must use HTTPS or loopback HTTP without embedded credentials.", nil)
	}
	if len(image.URI) > 8192 {
		return failure(contextir.CodeLimit, "request", "images.uri", "External image URLs must not exceed 8192 bytes.", nil)
	}
	return nil
}
