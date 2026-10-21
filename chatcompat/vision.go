package chatcompat

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
	"unicode"
)

const VisionProbeDigits = "473"

var digitGlyphs = map[byte][7]string{'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"}, '1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"}, '2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"}, '3': {"11111", "00010", "00100", "00010", "00001", "10001", "01110"}, '4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"}, '5': {"11111", "10000", "11110", "00001", "00001", "10001", "01110"}, '6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"}, '7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"}, '8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"}, '9': {"01110", "10001", "10001", "01111", "00001", "00010", "01100"}}

func DigitsPNG(digits string, scale, pad int) ([]byte, error) {
	if digits == "" || scale <= 0 || pad < 0 || len(digits) > 100 || scale > 1000 || pad > 10000 {
		return nil, errors.New("invalid digit probe geometry")
	}
	for i := range digits {
		if _, ok := digitGlyphs[digits[i]]; !ok {
			return nil, errors.New("probe accepts ASCII digits only")
		}
	}
	w, h := 2*pad+len(digits)*5*scale+(len(digits)-1)*scale, 2*pad+7*scale
	if int64(w+1)*int64(h) > 64<<20 {
		return nil, errors.New("digit probe exceeds image byte limit")
	}
	raw := make([]byte, 0, (w+1)*h)
	for y := 0; y < h; y++ {
		row := bytes.Repeat([]byte{255}, w)
		gy := (y - pad) / scale
		if y >= pad && gy < 7 {
			for i := range digits {
				g := digitGlyphs[digits[i]]
				x0 := pad + i*6*scale
				for gx, cell := range g[gy] {
					if cell == '1' {
						for x := x0 + gx*scale; x < x0+(gx+1)*scale; x++ {
							row[x] = 0
						}
					}
				}
			}
		}
		raw = append(raw, 0)
		raw = append(raw, row...)
	}
	var compressed bytes.Buffer
	z, e := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	if e != nil {
		return nil, e
	}
	if _, e = z.Write(raw); e != nil {
		return nil, e
	}
	if e = z.Close(); e != nil {
		return nil, e
	}
	var out bytes.Buffer
	out.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
	chunk := func(tag string, data []byte) {
		binary.Write(&out, binary.BigEndian, uint32(len(data)))
		out.WriteString(tag)
		out.Write(data)
		crc := crc32.NewIEEE()
		crc.Write([]byte(tag))
		crc.Write(data)
		binary.Write(&out, binary.BigEndian, crc.Sum32())
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr, uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(h))
	ihdr[8] = 8
	chunk("IHDR", ihdr)
	chunk("IDAT", compressed.Bytes())
	chunk("IEND", nil)
	return out.Bytes(), nil
}
func JudgesThePicture(status int) bool { return status == 400 || status == 415 || status == 422 }
func (c *Client) VisionCheck(ctx context.Context) (bool, string, error) {
	b, e := DigitsPNG(VisionProbeDigits, 16, 16)
	if e != nil {
		return false, "", e
	}
	uri, e := ImageDataURL(b)
	if e != nil {
		return false, "", e
	}
	m, e := UserMessage("This picture shows 3 digits. Reply with only those digits.", []Image{{URI: uri}})
	if e != nil {
		return false, "", e
	}
	thinking := false
	temp := 0.
	r, e := c.Chat(ctx, []Message{m}, ChatOptions{Thinking: &thinking, MaxTokens: 256, Temperature: &temp})
	if e != nil {
		return false, "", e
	}
	var digits strings.Builder
	for _, v := range r.Content {
		if unicode.IsDigit(v) {
			digits.WriteRune(v)
		}
	}
	return strings.Contains(digits.String(), VisionProbeDigits), strings.TrimSpace(r.Content), nil
}
func (c *Client) Doctor(ctx context.Context) (map[string]any, error) {
	out := map[string]any{"url": c.base, "credential": "none sent"}
	if c.key != "" {
		out["credential"] = "Authorization: Bearer sent"
	}
	hp := c.HealthProbe(ctx)
	out["health"] = hp.OK
	via := hp.Via
	if via == "" {
		via = "(nothing answered)"
	}
	out["health_via"] = via
	tried := []string{}
	for _, a := range hp.Attempts {
		tried = append(tried, a.URL+" -> "+a.Result)
	}
	out["health_tried"] = strings.Join(tried, "; ")
	if !hp.OK {
		return out, nil
	}
	models, e := c.Models(ctx)
	ids := []string{}
	out["max_model_len"] = "(this endpoint does not report one)"
	if e != nil {
		out["models_error"] = e.Error()
	} else {
		for _, m := range models {
			ids = append(ids, m.ID)
			if m.MaxModelLen != nil {
				if _, ok := out["max_model_len"].(string); ok {
					out["max_model_len"] = *m.MaxModelLen
				}
			}
		}
		out["model_ids"] = ids
	}
	if c.configuredModel != "" {
		out["model"] = c.configuredModel
		out["model_from"] = "H3IR_LLM_MODEL"
		listed := false
		for _, id := range ids {
			if id == c.configuredModel {
				listed = true
			}
		}
		if len(ids) > 0 && !listed {
			out["model_warning"] = "H3IR_LLM_MODEL is set and this endpoint does not list it; use one of the listed model ids"
		}
	} else {
		id, e := c.DiscoverModel(ctx)
		if e != nil {
			out["model"] = "(none: H3IR_LLM_MODEL is unset and this will not guess)"
			out["model_from"] = e.Error()
			return out, nil
		}
		out["model"] = id
		out["model_from"] = "H3IR_LLM_MODEL is unset and this endpoint serves one model, so there was nothing to choose"
	}
	thinking := false
	temp := 0.
	r, e := c.Chat(ctx, []Message{{Role: "user", Content: "Reply with the single word: ready"}}, ChatOptions{Thinking: &thinking, MaxTokens: 32, Temperature: &temp})
	if e != nil {
		out["chat_error"] = e.Error()
	} else {
		out["chat_ok"] = strings.Contains(strings.ToLower(r.Content), "ready")
		lat, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", r.WallSeconds), 64)
		out["latency_s"] = lat
	}
	ok, said, e := c.VisionCheck(ctx)
	if e != nil {
		var endpoint *Error
		if errors.As(e, &endpoint) && endpoint.Code == "endpoint_refused" && JudgesThePicture(endpoint.StatusCode) {
			ok, said = false, e.Error()
		} else {
			out["vision_error"] = e.Error()
			if errors.As(e, &endpoint) && endpoint.Code == "endpoint_refused" {
				switch endpoint.StatusCode {
				case 404:
					out["vision_note"] = "this endpoint has no model called " + c.ModelID() + ", so nothing was asked. Check H3IR_LLM_MODEL"
				case 401, 403:
					out["vision_note"] = "this endpoint wants a credential; set H3IR_LLM_KEY; the model was never asked"
				default:
					out["vision_note"] = "the endpoint answered with an error about the server rather than about the model; nothing is known about vision"
				}
			}
			return out, nil
		}
	}
	out["vision_ok"], out["vision_reply"] = ok, said
	if !ok {
		out["vision_note"] = "the model did not read the test picture through the configured image path; point H3IR_LLM_MODEL at a model that can see"
	}
	return out, nil
}
