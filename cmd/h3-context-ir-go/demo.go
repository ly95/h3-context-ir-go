package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

const defaultDemoPrompt = "A baker opens the wooden shutters of a small bakery in warm morning light. The camera slowly pushes in as the baker sets a tray of fresh bread on the counter."

// runDemo supplies a small text-only request to the existing CLI pipeline.
// Choosing the model path is explicit and never falls back to an offline draft.
func runDemo(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var provider briefFlags
	registerProviderFlags(fs, &provider)
	providerFlags := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) { providerFlags[f.Name] = true })
	mode := fs.String("mode", "draft", "Demo operation: draft (offline) or brief (explicit model compilation)")
	prompt := fs.String("prompt", defaultDemoPrompt, "Complete text-only video request; preserved exactly")
	seconds := fs.Float64("seconds", 8, "Requested video duration in seconds; no implicit frame rounding")
	aspect := fs.String("aspect", "16:9", "Requested aspect ratio")
	profile := fs.String("profile", "official", "H3 profile: official or comfyui")
	framePolicy := fs.String("frame-policy", "exact", "Frame policy: exact or explicit snap_up")
	maxRetries := fs.Int("max-retries", 0, "Additional bounded compiler/schema repair calls (0..3)")
	fs.Usage = func() {
		fmt.Fprintln(errOut, "usage: h3-context-ir-go demo [flags]")
		fmt.Fprintln(errOut, "With no flags, runs an offline bakery draft and prints the complete result JSON.")
		fmt.Fprintln(errOut, "Use -mode brief -base-url URL -model MODEL for model compilation (official SDK model conversion by default).")
		fmt.Fprintln(errOut, "Choose -api chat_completions for a compatible Chat Completions endpoint; -key-env names an optional key variable.")
		fmt.Fprintln(errOut, "The demo accepts no attachments and prints result JSON to stdout; it does not create files or generate video.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		return messageError(errOut, "invalid-arguments", "Demo accepts flags only; pass the complete request with -prompt")
	}
	if *mode != "draft" && *mode != "brief" {
		return messageError(errOut, "invalid-mode", "Choose demo -mode draft or brief")
	}
	if !utf8.ValidString(*prompt) || strings.TrimSpace(*prompt) == "" {
		return messageError(errOut, "invalid-prompt", "Demo prompt must be nonempty valid UTF-8")
	}
	if math.IsNaN(*seconds) || math.IsInf(*seconds, 0) || *seconds <= 0 {
		return messageError(errOut, "invalid-duration", "Demo seconds must be finite and positive")
	}
	providerArgs := []string{}
	providerSelected := false
	retriesSelected := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "max-retries" {
			retriesSelected = true
		}
		if providerFlags[f.Name] {
			providerArgs = append(providerArgs, "-"+f.Name+"="+f.Value.String())
			if f.Name == "provider" {
				providerSelected = true
			}
		}
	})
	if *mode == "draft" && (len(providerArgs) > 0 || retriesSelected) {
		return messageError(errOut, "invalid-configuration", "Offline demo draft accepts no model flags; choose -mode brief explicitly")
	}
	if *mode == "brief" && !providerSelected {
		providerArgs = append(providerArgs, "-provider=openai")
	}
	// This is the public service Brief input, decoded by PrepareRequest. With
	// no attachments, typed media routing selects t2va without interpreting text.
	payload, err := json.Marshal(struct {
		Intent  string  `json:"intent"`
		Seconds float64 `json:"seconds"`
		Aspect  string  `json:"aspect"`
	}{Intent: *prompt, Seconds: *seconds, Aspect: *aspect})
	if err != nil {
		return messageError(errOut, "invalid-input", "Demo request cannot be encoded")
	}
	briefArgs := []string{"-input=-", "-profile=" + *profile, "-frame-policy=" + *framePolicy, "-max-retries=" + strconv.Itoa(*maxRetries)}
	briefArgs = append(briefArgs, providerArgs...)
	if *mode == "draft" {
		fmt.Fprintln(errOut, "Offline draft demo: no model calls; output is a deterministic draft.")
	}
	return runBrief(*mode, briefArgs, bytes.NewReader(payload), out, errOut)
}
