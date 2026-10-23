package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/chatcompat"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const maxPromptBytes = 16 << 20

func main() {
	os.Exit(run())
}

func run() int {
	flags := flag.NewFlagSet("manual", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	baseURL := flags.String("base-url", "", "Explicit OpenAI-compatible API base URL")
	modelID := flags.String("model", "", "Explicit model ID")
	keyEnv := flags.String("key-env", "", "Optional environment variable containing the API key")
	api := flags.String("api", "responses", "responses or chat_completions")
	prompt := flags.String("prompt", "", "Complete video request; otherwise read stdin")
	seconds := flags.Float64("seconds", 5, "Requested duration in seconds")
	aspect := flags.String("aspect", "16:9", "Requested aspect ratio")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(os.Stderr)
			flags.PrintDefaults()
			return 0
		}
		return stop("invalid_arguments")
	}
	if flags.NArg() != 0 || *baseURL == "" || *modelID == "" {
		return stop("base_url_and_model_required")
	}
	text := *prompt
	selected := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { selected[f.Name] = true })
	if !selected["prompt"] {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, maxPromptBytes+1))
		if err != nil {
			return stop("input_read_failed")
		}
		text = string(data)
	}
	if len(text) > maxPromptBytes {
		return stop("prompt_too_large")
	}
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" {
		return stop("invalid_prompt")
	}
	key := ""
	if *keyEnv != "" {
		var exists bool
		key, exists = os.LookupEnv(*keyEnv)
		if !exists || key == "" {
			return stop("credential_unavailable")
		}
	}
	httpClient := &http.Client{
		Transport: &http.Transport{ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second},
		Timeout:   2 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer httpClient.CloseIdleConnections()
	client := sdk.Client{Options: []option.RequestOption{
		option.WithBaseURL(*baseURL), option.WithAPIKey(key), option.WithHTTPClient(httpClient),
	}}
	model, err := chatcompat.NewModel(client, chatcompat.ModelOptions{
		API: chatcompat.API(*api), Model: *modelID, MaxOutputTokens: 8192,
	})
	if err != nil {
		return failed(err)
	}
	c, err := compiler.New(model, nil, compiler.Options{
		Profile: h3.OfficialProfile(), FramePolicy: compiler.FramePolicyExact,
	})
	if err != nil {
		return failed(err)
	}
	brief := compiler.Brief{Intent: text}
	if selected["seconds"] {
		brief.Seconds, brief.SecondsExplicit = *seconds, true
	}
	if selected["aspect"] {
		brief.Aspect, brief.AspectExplicit = *aspect, true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := c.Compile(ctx, brief)
	for _, call := range result.Calls {
		fmt.Fprintf(os.Stderr, "call_usage: stage=%s attempt=%d input_tokens=%d output_tokens=%d valid=%t unmetered=%t\n", call.Stage, call.Attempt, call.Usage.InputTokens, call.Usage.OutputTokens, call.Valid, call.Unmetered)
	}
	if err != nil {
		return failed(err)
	}
	if result.Status == "needs_input" {
		fmt.Fprintln(os.Stderr, "needs_input")
		if result.ModeDecision != nil {
			if err := json.NewEncoder(os.Stderr).Encode(result.ModeDecision.Questions); err != nil {
				return stop("question_output_failed")
			}
		}
		return 3
	}
	if result.Status != "compiled" {
		return stop("unexpected_compile_status")
	}
	if _, err := io.WriteString(os.Stdout, result.Artifact.Prompt); err != nil {
		return stop("prompt_output_failed")
	}
	return 0
}

func failed(err error) int {
	var typed *contextir.Error
	if errors.As(err, &typed) {
		return stop(string(typed.Code))
	}
	return stop("operation_failed")
}

func stop(code string) int {
	fmt.Fprintln(os.Stderr, code)
	return 1
}
