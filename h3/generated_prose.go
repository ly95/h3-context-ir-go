package h3

// Explicit prose-only formatters ported from open-h3-ir fd031e13
// h3ir/textnorm.py and render.py (Apache-2.0). They never run implicitly over a
// Document, dialogue, visible text or caller-authored literal RichText.
import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ProseNormalization makes the opt-in byte changes observable to the caller.
type ProseNormalization struct {
	Text    string `json:"text"`
	Changed bool   `json:"changed"`
}

var generatedProseReplacer = strings.NewReplacer(
	"‘", "'", "’", "'", "‚", "'", "‛", "'", "“", `"`, "”", `"`, "„", `"`, "‟", `"`, "′", "'", "″", `"`,
	"\u00a0", " ", "\u2007", " ", "\u202f", " ", "\u2009", " ", "\u200a", " ",
	"\u200b", "", "\u200c", "", "\u200d", "", "\ufeff", "",
	"‐", "-", "‑", "-", "‒", "-", "–", "-", "…", "...",
	"〈", "<", "〉", ">", "＜", "<", "＞", ">", "（", "(", "）", ")", "［", "[", "］", "]", "：", ":", "，", ",", "．", ".",
)
var generatedHorizontalSpace = regexp.MustCompile(`[ \t]{2,}`)

// NormalizeGeneratedProse is explicitly selected byte hygiene for generated
// prose. Em dashes remain unchanged for official H3 instructions. Its result
// still requires normal Document validation; it never makes unsafe labels legal.
func NormalizeGeneratedProse(text string) ProseNormalization {
	out := generatedHorizontalSpace.ReplaceAllString(generatedProseReplacer.Replace(text), " ")
	return ProseNormalization{Text: out, Changed: out != text}
}

// GeneratedSoundSentences formats explicitly supplied generated sound clauses.
// It does not infer events or move sounds between ambient and shot tracks.
func GeneratedSoundSentences(items []string) string {
	out := []string{}
	for _, item := range items {
		s := strings.TrimRight(formatTrim(NormalizeGeneratedProse(item).Text), ",;")
		if s == "" {
			continue
		}
		r, n := utf8.DecodeRuneInString(s)
		s = string(unicode.ToUpper(r)) + s[n:]
		if !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "!") && !strings.HasSuffix(s, "?") {
			s += "."
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

func proseWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }

// ContinueCase is an opt-in continuation grammar formatter. It lowercases only
// A/An/The, or an opening word independently present in lower case within the
// same prose sentence. It never classifies a request or changes proper names
// solely because their first rune is capitalized.
func ContinueCase(body string) string {
	first := []rune{}
	for _, r := range body {
		if !proseWordRune(r) && r != '\'' && r != '-' {
			break
		}
		first = append(first, r)
	}
	if len(first) == 0 || !unicode.IsUpper(first[0]) {
		return body
	}
	word := string(first)
	lowered := strings.ToLower(word)
	safe := word == "A" || word == "An" || word == "The"
	rest := []rune(body[len(word):])
	needle := []rune(lowered)
	for i := 0; !safe && i+len(needle) <= len(rest); i++ {
		if string(rest[i:i+len(needle)]) != lowered || i > 0 && proseWordRune(rest[i-1]) || i+len(needle) < len(rest) && proseWordRune(rest[i+len(needle)]) {
			continue
		}
		if i >= 2 && unicode.IsSpace(rest[i-1]) && strings.ContainsRune(".!?", rest[i-2]) {
			continue
		}
		safe = true
	}
	if safe {
		_, n := utf8.DecodeRuneInString(body)
		return string(unicode.ToLower(first[0])) + body[n:]
	}
	return body
}
