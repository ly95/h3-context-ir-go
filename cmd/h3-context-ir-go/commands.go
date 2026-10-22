package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ly95/h3-context-ir-go/budget"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/h3check"
	"github.com/ly95/h3-context-ir-go/service"
	"github.com/ly95/h3-context-ir-go/tokens"
)

const maxCLIInputBytes int64 = 16 << 20

func readInput(path string, in io.Reader, limit int64) ([]byte, error) {
	var closeFile *os.File
	if path != "-" {
		f, e := os.Open(path)
		if e != nil {
			return nil, errors.New("input file could not be opened")
		}
		closeFile = f
		defer closeFile.Close()
		in = f
	}
	b, e := io.ReadAll(io.LimitReader(in, limit+1))
	if e != nil {
		return nil, errors.New("input could not be read")
	}
	if int64(len(b)) > limit {
		return nil, errors.New("input exceeds the configured byte limit; no content was truncated")
	}
	if !utf8.Valid(b) {
		return nil, errors.New("input must be valid UTF-8")
	}
	return b, nil
}
func encodeOutput(out io.Writer, v any) int {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	if e.Encode(v) != nil {
		return 1
	}
	if _, err := out.Write(b.Bytes()); err != nil {
		return 1
	}
	return 0
}
func messageError(out io.Writer, code, msg string) int {
	_ = json.NewEncoder(out).Encode(map[string]any{"code": code, "message": msg})
	return 1
}
func selectedProfile(name string) (h3.Profile, error) {
	switch name {
	case "official":
		return h3.OfficialProfile(), nil
	case "comfyui":
		return h3.ComfyUIProfile(), nil
	}
	return h3.Profile{}, errors.New("choose profile official or comfyui")
}
func parseCanvas(raw string) (*[2]int, error) {
	if raw == "" {
		return nil, nil
	}
	p := strings.Split(raw, "x")
	if len(p) != 2 {
		return nil, errors.New("canvas must be WIDTHxHEIGHT")
	}
	w, e := strconv.Atoi(p[0])
	if e != nil || w <= 0 {
		return nil, errors.New("canvas width must be positive")
	}
	h, e := strconv.Atoi(p[1])
	if e != nil || h <= 0 {
		return nil, errors.New("canvas height must be positive")
	}
	return &[2]int{w, h}, nil
}
func runDeterministic(command string, args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(errOut)
	switch command {
	case "contract":
		js := fs.Bool("js", false, "Export the standalone browser module instead of JSON")
		if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
			return 2
		}
		var text string
		if *js {
			text = service.AsJS()
		} else {
			var e error
			text, e = service.AsJSON()
			if e != nil {
				return report(errOut, e)
			}
		}
		if _, e := io.WriteString(out, text); e != nil {
			return 1
		}
		return 0
	case "directors":
		id := fs.String("id", "", "Show one shipped profile by its exact ID")
		if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
			return 2
		}
		profiles := compiler.Directors()
		if *id != "" {
			for _, d := range profiles {
				if d.ID == *id {
					return encodeOutput(out, d)
				}
			}
			return messageError(errOut, "unknown-director", "No shipped director has that ID; run h3-context-ir-go directors")
		}
		return encodeOutput(out, profiles)
	case "budget":
		seconds := fs.Float64("seconds", 5, "Nominal duration in seconds")
		aspect := fs.String("aspect", "16:9", "Target aspect ratio")
		canvasText := fs.String("canvas", "", "Explicit WIDTHxHEIGHT canvas")
		mp := fs.Float64("megapixels", 0, "Explicit pixel area ask; omit for native size")
		policy := fs.String("frame-policy", "exact", "Frame policy: exact or snap_up")
		mode := fs.String("mode", "t2va", "H3 task mode for the instruction line")
		if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
			return 2
		}
		canvas, e := parseCanvas(*canvasText)
		if e != nil {
			return messageError(errOut, "invalid-budget", e.Error())
		}
		var area *float64
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "megapixels" {
				area = mp
			}
		})
		var target budget.Target
		switch *policy {
		case "exact":
			target, e = budget.BuildExact(*seconds, *aspect, canvas, area)
		case "snap_up":
			target, e = budget.Build(*seconds, *aspect, canvas, area)
		default:
			return messageError(errOut, "invalid-frame-policy", "Choose exact or snap_up")
		}
		if e != nil {
			return messageError(errOut, "invalid-budget", e.Error())
		}
		sss, e := target.SS("snapped")
		if e != nil {
			return report(errOut, e)
		}
		if !enumCLI(*mode, []string{"t2va", "i2va", "l2va", "fl2va", "ref2va"}) {
			return messageError(errOut, "invalid-mode", "Choose a supported H3 mode")
		}
		instruction := budget.InstructionLineFor(*mode, 1, sss)
		return encodeOutput(out, map[string]any{"target": target, "effective_seconds": target.EffectiveSeconds(), "latent_t": target.LatentT(), "video_rows": target.VideoRows(), "audio_rows": target.AudioRows(), "in_trained_range": target.InTrainedRange(), "instruction_line": instruction})
	case "tokens":
		path := fs.String("input", "-", "UTF-8 input file, or - for stdin")
		ids := fs.Bool("ids", false, "Include exact Qwen2.5 token IDs")
		if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
			return 2
		}
		b, e := readInput(*path, in, maxCLIInputBytes)
		if e != nil {
			return messageError(errOut, "invalid-input", e.Error())
		}
		encoded, e := tokens.Encode(string(b))
		if e != nil {
			return report(errOut, e)
		}
		value := map[string]any{"tokens": len(encoded)}
		if *ids {
			value["ids"] = encoded
		}
		return encodeOutput(out, value)
	case "check-h3", "checkH3":
		path := fs.String("input", "-", "H3 prompt file, or - for stdin")
		contextPath := fs.String("context", "", "Optional checker Context JSON file")
		mode := fs.String("mode", "ref2va", "Explicit H3 mode, defaults to upstream ref2va")
		profile := fs.String("profile", "comfyui", "Grid policy: official or comfyui")
		if e := fs.Parse(args); e != nil || fs.NArg() != 0 {
			return 2
		}
		ctx := h3check.NewContext()
		if *contextPath != "" {
			b, e := readInput(*contextPath, in, maxCLIInputBytes)
			if e != nil {
				return messageError(errOut, "invalid-context", e.Error())
			}
			var obj map[string]json.RawMessage
			if e = json.Unmarshal(b, &obj); e != nil || obj == nil {
				return messageError(errOut, "invalid-context", "Context must be a JSON object")
			}
			fields := map[string]bool{}
			t := reflect.TypeOf(ctx)
			for i := 0; i < t.NumField(); i++ {
				fields[strings.Split(t.Field(i).Tag.Get("json"), ",")[0]] = true
			}
			for k := range obj {
				if !fields[k] {
					return messageError(errOut, "invalid-context", "Context contains an unknown field")
				}
			}
			if e = json.Unmarshal(b, &ctx); e != nil {
				return messageError(errOut, "invalid-context", "Context fields do not match the public checker schema")
			}
		}
		visited := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
		if *contextPath == "" || visited["mode"] {
			ctx.Mode = *mode
		}
		if !enumCLI(ctx.Mode, []string{"t2va", "i2va", "l2va", "fl2va", "ref2va"}) {
			return messageError(errOut, "invalid-mode", "Choose a supported H3 mode")
		}
		p, e := selectedProfile(*profile)
		if e != nil {
			return messageError(errOut, "invalid-profile", e.Error())
		}
		grid := p.Grid != nil
		ctx.CheckFrameGrid = &grid
		b, e := readInput(*path, in, maxCLIInputBytes)
		if e != nil {
			return messageError(errOut, "invalid-input", e.Error())
		}
		findings := h3check.Validate(string(b), ctx)
		failed := false
		for _, f := range findings {
			if f.Severity == "ERROR" {
				failed = true
			}
		}
		code := encodeOutput(out, map[string]any{"valid": !failed, "findings": findings})
		if failed {
			return 1
		}
		return code
	}
	return 2
}
func enumCLI(s string, values []string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
