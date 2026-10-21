package lora

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ParseFrontMatter reads the documented small YAML subset: scalars, inline
// lists/maps, list-of-maps triggers, and one mapping nesting level. Unsupported
// shapes return an error instead of being silently ignored.
func ParseFrontMatter(text string) (map[string]any, string, error) {
	if !utf8.ValidString(text) {
		return nil, "", fmt.Errorf("front matter document must be valid UTF-8")
	}
	if !strings.HasPrefix(text, "---") {
		return map[string]any{}, text, nil
	}
	i := strings.Index(text[3:], "\n---")
	if i < 0 {
		return map[string]any{}, text, nil
	}
	end := 3 + i
	head, body := text[3:end], text[end+4:]
	data := map[string]any{}
	current := ""
	var items []any
	var nested map[string]any
	flush := func() {
		if current != "" {
			if nested != nil {
				data[current] = nested
			} else {
				if items == nil {
					items = []any{}
				}
				data[current] = items
			}
		}
		current = ""
		items = nil
		nested = nil
	}
	for _, raw := range strings.Split(head, "\n") {
		line := strings.TrimRight(raw, " \r\t")
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indented := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
		if indented && strings.HasPrefix(trim, "- ") && current != "" {
			if nested != nil {
				return nil, "", fmt.Errorf("front matter field %s mixes a mapping and a list", current)
			}
			item := strings.TrimSpace(trim[1:])
			if strings.HasPrefix(item, "{") {
				m, e := inlineMap(item)
				if e != nil {
					return nil, "", e
				}
				items = append(items, m)
			} else if j := strings.Index(item, ":"); j >= 0 && !strings.HasPrefix(strings.TrimSpace(item[:j]), `"`) && !strings.HasPrefix(strings.TrimSpace(item[:j]), "'") {
				v, e := parseValue(strings.TrimSpace(item[j+1:]))
				if e != nil {
					return nil, "", e
				}
				items = append(items, map[string]any{strings.TrimSpace(item[:j]): v})
			} else {
				v, e := parseValue(item)
				if e != nil {
					return nil, "", e
				}
				items = append(items, v)
			}
			continue
		}
		if indented && current != "" && nestedField.MatchString(trim) {
			k, v, ok := strings.Cut(trim, ":")
			if !ok {
				return nil, "", fmt.Errorf("invalid nested field")
			}
			parsed, e := parseValue(strings.TrimSpace(v))
			if e != nil {
				return nil, "", e
			}
			if len(items) > 0 {
				if m, ok := items[len(items)-1].(map[string]any); ok {
					m[strings.TrimSpace(k)] = parsed
					continue
				}
				return nil, "", fmt.Errorf("nested mapping cannot follow a scalar list item")
			}
			if nested == nil {
				nested = map[string]any{}
			}
			nested[strings.TrimSpace(k)] = parsed
			continue
		}
		flush()
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, "", fmt.Errorf("front matter line must be key: value")
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" {
			return nil, "", fmt.Errorf("empty front matter key")
		}
		if v == "" {
			current = k
			continue
		}
		value, e := parseValue(v)
		if e != nil {
			return nil, "", e
		}
		data[k] = value
	}
	flush()
	return data, body, nil
}
func inlineMap(s string) (map[string]any, error) {
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return nil, fmt.Errorf("unclosed inline mapping")
	}
	parts, e := splitInline(s[1 : len(s)-1])
	if e != nil {
		return nil, e
	}
	out := map[string]any{}
	for _, part := range parts {
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			if strings.TrimSpace(part) != "" {
				return nil, fmt.Errorf("inline mapping item lacks colon")
			}
			continue
		}
		x, e := parseValue(strings.TrimSpace(v))
		if e != nil {
			return nil, e
		}
		out[strings.TrimSpace(k)] = x
	}
	return out, nil
}
func parseValue(s string) (any, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		if !strings.HasSuffix(s, "]") {
			return nil, fmt.Errorf("unclosed inline list")
		}
		parts, e := splitInline(s[1 : len(s)-1])
		if e != nil {
			return nil, e
		}
		out := []any{}
		for _, p := range parts {
			if strings.TrimSpace(p) == "" {
				continue
			}
			v, e := parseValue(p)
			if e != nil {
				return nil, e
			}
			out = append(out, v)
		}
		return out, nil
	}
	if strings.HasPrefix(s, "{") {
		return inlineMap(s)
	}
	return scalar(s)
}

// splitInline preserves commas inside quoted trigger bytes and nested literals.
func splitInline(s string) ([]string, error) {
	var out []string
	start, depth := 0, 0
	quote := byte(0)
	escaped := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if b == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if b == quote {
				quote = 0
			}
			continue
		}
		if b == '"' || b == '\'' {
			quote = b
			continue
		}
		switch b {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced inline value")
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if quote != 0 || depth != 0 {
		return nil, fmt.Errorf("unclosed quoted or nested inline value")
	}
	out = append(out, s[start:])
	return out, nil
}

func scalar(s string) (any, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == s[len(s)-1] && (s[0] == '"' || s[0] == '\'') {
		return s[1 : len(s)-1], nil
	}
	switch strings.ToLower(s) {
	case "null", "none", "~", "":
		return nil, nil
	case "true", "yes":
		return true, nil
	case "false", "no":
		return false, nil
	}
	if n, e := strconv.ParseInt(s, 10, 64); e == nil {
		return n, nil
	}
	if n, e := strconv.ParseFloat(s, 64); e == nil {
		return n, nil
	}
	return s, nil
}

var nestedField = regexp.MustCompile(`^\w+\s*:`)
