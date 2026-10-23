package comfy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strings"
	"unicode/utf8"
)

type Graph map[string]any

var h3NodeClasses = []string{"MiniMaxH3ReferenceToVideo", "MiniMaxH3ImageToVideo"}

func Decode(data []byte) (Graph, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("graph JSON must be valid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var g Graph
	if e := d.Decode(&g); e != nil {
		return nil, e
	}
	if g == nil {
		return nil, errors.New("graph must be an object")
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return nil, errors.New("graph must contain exactly one JSON object")
	}
	return g, nil
}
func clone(g Graph) (Graph, error) {
	b, e := json.Marshal(g)
	if e != nil {
		return nil, e
	}
	return Decode(b)
}
func isH3(s string) bool {
	for _, c := range h3NodeClasses {
		if s == c {
			return true
		}
	}
	return false
}
func FindH3Nodes(g Graph) []string {
	ids := []string{}
	for id, n := range g {
		if m, ok := n.(map[string]any); ok {
			if s, ok := m["class_type"].(string); ok && isH3(s) {
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	return ids
}
func SetPrompt(g Graph, text, nodeID string) (Graph, error) {
	if !utf8.ValidString(text) {
		return nil, errors.New("H3 prompt must be valid UTF-8")
	}
	out, e := clone(g)
	if e != nil {
		return nil, e
	}
	ids := []string{nodeID}
	if nodeID == "" {
		ids = FindH3Nodes(out)
	}
	if len(ids) == 0 {
		return nil, errors.New("no MiniMaxH3 conditioning node in this graph")
	}
	if len(ids) > 1 {
		return nil, fmt.Errorf("graph has several H3 nodes (%v); pass node_id to choose one", ids)
	}
	n, ok := out[ids[0]].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("node %s does not exist or is not a node object", ids[0])
	}
	class, _ := n["class_type"].(string)
	if !isH3(class) {
		return nil, fmt.Errorf("node %s is not a MiniMaxH3 conditioning node", ids[0])
	}
	inputs, ok := n["inputs"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("node %s has no prompt input", ids[0])
	}
	if _, ok = inputs["prompt"]; !ok {
		return nil, fmt.Errorf("node %s has no prompt input", ids[0])
	}
	inputs["prompt"] = text
	return out, nil
}
func numeric(v any) (json.Number, bool) {
	switch x := v.(type) {
	case json.Number:
		if _, ok := new(big.Rat).SetString(string(x)); ok {
			return x, true
		}
	case int:
		return json.Number(fmt.Sprint(x)), true
	case int64:
		return json.Number(fmt.Sprint(x)), true
	case uint64:
		return json.Number(fmt.Sprint(x)), true
	case float64:
		return json.Number(fmt.Sprint(x)), true
	case float32:
		return json.Number(fmt.Sprint(x)), true
	}
	return "", false
}
func SetSeed(g Graph, seed uint64) (Graph, error) {
	out, e := clone(g)
	if e != nil {
		return nil, e
	}
	for _, n := range out {
		m, ok := n.(map[string]any)
		if !ok {
			continue
		}
		inputs, ok := m["inputs"].(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"seed", "noise_seed"} {
			if _, ok := numeric(inputs[key]); ok {
				inputs[key] = seed
			}
		}
	}
	return out, nil
}

type GraphDescription struct {
	Nodes     int           `json:"nodes"`
	H3Nodes   []string      `json:"h3_nodes"`
	Seeds     []json.Number `json:"seeds"`
	LoRANodes int           `json:"lora_nodes"`
	Classes   []string      `json:"classes"`
}

func DescribeGraph(g Graph) GraphDescription {
	d := GraphDescription{Nodes: len(g), H3Nodes: FindH3Nodes(g), Seeds: []json.Number{}, Classes: []string{}}
	classes, seeds := map[string]bool{}, map[string]json.Number{}
	for _, n := range g {
		m, ok := n.(map[string]any)
		if !ok {
			continue
		}
		if c, ok := m["class_type"].(string); ok && c != "" {
			classes[c] = true
			if strings.Contains(c, "Lora") {
				d.LoRANodes++
			}
		}
		inputs, _ := m["inputs"].(map[string]any)
		for _, k := range []string{"seed", "noise_seed"} {
			if v, ok := numeric(inputs[k]); ok {
				r, valid := new(big.Rat).SetString(string(v))
				if valid {
					seeds[r.RatString()] = v
				}
			}
		}
	}
	for c := range classes {
		d.Classes = append(d.Classes, c)
	}
	for _, v := range seeds {
		d.Seeds = append(d.Seeds, v)
	}
	sort.Strings(d.Classes)
	sort.Slice(d.Seeds, func(i, j int) bool {
		a, _ := new(big.Rat).SetString(string(d.Seeds[i]))
		b, _ := new(big.Rat).SetString(string(d.Seeds[j]))
		return a.Cmp(b) < 0
	})
	return d
}
func Outputs(history map[string]any) ([]map[string]any, error) {
	out := []map[string]any{}
	raw, exists := history["outputs"]
	if !exists || raw == nil {
		return out, nil
	}
	nodes, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("history outputs must be an object")
	}
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		payload, ok := nodes[id].(map[string]any)
		if !ok {
			return nil, errors.New("history output payload must be an object")
		}
		for _, key := range []string{"images", "gifs", "videos", "audio"} {
			raw, ok := payload[key]
			if !ok || raw == nil {
				continue
			}
			items, ok := raw.([]any)
			if !ok {
				return nil, errors.New("history media output must be an array")
			}
			for _, item := range items {
				m, ok := item.(map[string]any)
				if !ok {
					return nil, errors.New("history media item must be an object")
				}
				copy := map[string]any{}
				for k, v := range m {
					copy[k] = v
				}
				copy["node"], copy["kind"] = id, key
				out = append(out, copy)
			}
		}
	}
	return out, nil
}

func H3NodeClasses() []string { return append([]string{}, h3NodeClasses...) }
