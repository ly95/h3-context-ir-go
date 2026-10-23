package tokens

import (
	"container/heap"
	"crypto/sha1"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

//go:embed data/vocab.json
var data embed.FS

const VocabularyGitBlobSHA1 = "4783fe10ac3adce15ac8f358ef5462739852c569"

var addedTokens = map[string]int{"<|endoftext|>": 151643, "<|im_start|>": 151644, "<|im_end|>": 151645, "<|object_ref_start|>": 151646, "<|object_ref_end|>": 151647, "<|box_start|>": 151648, "<|box_end|>": 151649, "<|quad_start|>": 151650, "<|quad_end|>": 151651, "<|vision_start|>": 151652, "<|vision_end|>": 151653, "<|vision_pad|>": 151654, "<|image_pad|>": 151655, "<|video_pad|>": 151656, "<tool_call>": 151657, "</tool_call>": 151658, "<|fim_prefix|>": 151659, "<|fim_middle|>": 151660, "<|fim_suffix|>": 151661, "<|fim_pad|>": 151662, "<|repo_name|>": 151663, "<|file_sep|>": 151664, "<tool_response>": 151665, "</tool_response>": 151666, "<think>": 151667, "</think>": 151668}
var once sync.Once
var ranks map[string]int
var loadError error

func load() {
	b, e := data.ReadFile("data/vocab.json")
	if e != nil {
		loadError = e
		return
	}
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(b))
	h.Write(b)
	if hex.EncodeToString(h.Sum(nil)) != VocabularyGitBlobSHA1 {
		loadError = errors.New("embedded H3 vocabulary checksum mismatch")
		return
	}
	var vocab map[string]int
	if e = json.Unmarshal(b, &vocab); e != nil {
		loadError = e
		return
	}
	unicodeToByte := map[rune]byte{}
	n := 0
	for i := 0; i < 256; i++ {
		if (i >= 33 && i <= 126) || (i >= 161 && i <= 172) || (i >= 174) {
			unicodeToByte[rune(i)] = byte(i)
		} else {
			unicodeToByte[rune(256+n)] = byte(i)
			n++
		}
	}
	ranks = make(map[string]int, len(vocab))
	for token, id := range vocab {
		if _, special := addedTokens[token]; special {
			continue
		}
		buf := make([]byte, 0, len(token))
		okay := true
		for _, r := range token {
			v, ok := unicodeToByte[r]
			if !ok {
				okay = false
				break
			}
			buf = append(buf, v)
		}
		if okay {
			ranks[string(buf)] = id
		}
	}
	for i := 0; i < 256; i++ {
		if _, ok := ranks[string([]byte{byte(i)})]; !ok {
			loadError = fmt.Errorf("embedded H3 vocabulary lacks byte %d", i)
			return
		}
	}
}

func Count(text string) (int, error) { ids, e := Encode(text); return len(ids), e }

func AddedTokens() map[string]int {
	m := make(map[string]int, len(addedTokens))
	for k, v := range addedTokens {
		m[k] = v
	}
	return m
}

func Encode(text string) ([]int, error) {
	if !utf8.ValidString(text) {
		return nil, errors.New("tokenizer input must be valid UTF-8")
	}
	once.Do(load)
	if loadError != nil {
		return nil, loadError
	}
	ids := []int{}
	for len(text) > 0 {
		pos := len(text)
		special := ""
		specialID := 0
		for s, id := range addedTokens {
			if p := strings.Index(text, s); p >= 0 && (p < pos || (p == pos && len(s) > len(special))) {
				pos, special, specialID = p, s, id
			}
		}
		ordinary := text[:pos]
		for len(ordinary) > 0 {
			n := pieceLength(ordinary)
			if n <= 0 {
				return nil, errors.New("H3 tokenizer pre-tokenization made no progress")
			}
			part, e := encodePiece([]byte(ordinary[:n]))
			if e != nil {
				return nil, e
			}
			ids = append(ids, part...)
			ordinary = ordinary[n:]
		}
		text = text[pos:]
		if special != "" {
			ids = append(ids, specialID)
			text = text[len(special):]
		}
	}
	return ids, nil
}

func pieceLength(s string) int {
	r, w := utf8.DecodeRuneInString(s)
	if r == '\'' {
		for _, tail := range []string{"s", "t", "re", "ve", "m", "ll", "d"} {
			n, matched := 1, true
			for _, want := range tail {
				if n >= len(s) {
					matched = false
					break
				}
				got, k := utf8.DecodeRuneInString(s[n:])
				if !strings.EqualFold(string(got), string(want)) {
					matched = false
					break
				}
				n += k
			}
			if matched {
				return n
			}
		}
	}
	isLN := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }
	if unicode.IsLetter(r) {
		n := w
		for n < len(s) {
			x, k := utf8.DecodeRuneInString(s[n:])
			if !unicode.IsLetter(x) {
				break
			}
			n += k
		}
		return n
	}
	if r != '\r' && r != '\n' && !isLN(r) && w < len(s) {
		x, k := utf8.DecodeRuneInString(s[w:])
		if unicode.IsLetter(x) {
			n := w + k
			for n < len(s) {
				y, l := utf8.DecodeRuneInString(s[n:])
				if !unicode.IsLetter(y) {
					break
				}
				n += l
			}
			return n
		}
	}
	if unicode.IsNumber(r) {
		return w
	}
	n := 0
	if r == ' ' && w < len(s) {
		x, _ := utf8.DecodeRuneInString(s[w:])
		if !unicode.IsSpace(x) && !isLN(x) {
			n = w
		}
	}
	if n > 0 || (!unicode.IsSpace(r) && !isLN(r)) {
		for n < len(s) {
			x, k := utf8.DecodeRuneInString(s[n:])
			if unicode.IsSpace(x) || isLN(x) {
				break
			}
			n += k
		}
		for n < len(s) {
			x, k := utf8.DecodeRuneInString(s[n:])
			if x != '\r' && x != '\n' {
				break
			}
			n += k
		}
		return n
	}
	if unicode.IsSpace(r) {
		n, lastNL, lastWidth := 0, 0, 0
		for n < len(s) {
			x, k := utf8.DecodeRuneInString(s[n:])
			if !unicode.IsSpace(x) {
				break
			}
			n += k
			lastWidth = k
			if x == '\r' || x == '\n' {
				lastNL = n
			}
		}
		if lastNL > 0 {
			return lastNL
		}
		if n == len(s) {
			return n
		}
		if n > lastWidth {
			return n - lastWidth
		}
		return n
	}
	return w
}

type node struct {
	start, end, prev, next int
	alive                  bool
}
type candidate struct{ rank, left, right, leftEnd, rightEnd int }
type queue []candidate

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	if q[i].rank != q[j].rank {
		return q[i].rank < q[j].rank
	}
	return q[i].left < q[j].left
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)   { *q = append(*q, x.(candidate)) }
func (q *queue) Pop() any     { a := *q; n := len(a) - 1; x := a[n]; *q = a[:n]; return x }
func encodePiece(b []byte) ([]int, error) {
	if id, ok := ranks[string(b)]; ok {
		return []int{id}, nil
	}
	nodes := make([]node, len(b))
	for i := range nodes {
		nodes[i] = node{i, i + 1, i - 1, i + 1, true}
	}
	if len(nodes) > 0 {
		nodes[len(nodes)-1].next = -1
	}
	q := queue{}
	push := func(l, r int) {
		if l < 0 || r < 0 {
			return
		}
		if rank, ok := ranks[string(b[nodes[l].start:nodes[r].end])]; ok {
			heap.Push(&q, candidate{rank, l, r, nodes[l].end, nodes[r].end})
		}
	}
	for i := 0; i+1 < len(nodes); i++ {
		push(i, i+1)
	}
	for q.Len() > 0 {
		c := heap.Pop(&q).(candidate)
		l, r := &nodes[c.left], &nodes[c.right]
		if !l.alive || !r.alive || l.next != c.right || l.end != c.leftEnd || r.end != c.rightEnd {
			continue
		}
		l.end, l.next = r.end, r.next
		r.alive = false
		if l.next >= 0 {
			nodes[l.next].prev = c.left
		}
		push(l.prev, c.left)
		push(c.left, l.next)
	}
	ids := make([]int, 0, len(nodes))
	for i := 0; i < len(nodes); {
		n := nodes[i]
		id, ok := ranks[string(b[n.start:n.end])]
		if !ok {
			return nil, errors.New("H3 BPE produced an unregistered byte sequence")
		}
		ids = append(ids, id)
		if n.next < 0 {
			break
		}
		i = n.next
	}
	return ids, nil
}
func WordCount(text string) int {
	n := 0
	inside := false
	for _, r := range text {
		word := unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
		if word {
			if !inside {
				n++
				inside = true
			}
		} else if r != '\'' && r != '-' {
			inside = false
		}
	}
	return n
}
func LabelCost() (map[string]int, error) {
	m := map[string]int{}
	for _, s := range []string{"<Picture 1>: ", "<Video 1>: ", "<Audio 1>: ", "<0.5 seconds>", "<Subject 1>"} {
		n, e := Count(s)
		if e != nil {
			return nil, e
		}
		m[s] = n
	}
	return m, nil
}
