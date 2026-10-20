// Package planner interprets explicit requests and observed media evidence into
// a versioned ContextIR document. It performs no provider, network, or media IO
// itself. Injected adapters own model selection, credentials, and metering.
package planner

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/ly95/contextir"
)

// PromptVersion identifies the interpretation contract embedded in this package.
const PromptVersion = "contextir/planner/v1"

//go:embed prompts/plan_v1.txt
var systemPrompt string

// Model must honor context cancellation and must not silently select another
// model or remove input modalities. Usage should be returned even on failure.
type Model interface {
	Generate(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error)
}

// Perceiver observes one entire asset, including the modalities it supports.
// Unsupported media or unavailable perception must produce an error. Its
// implementation owns media access, decoding, and any perception-call metering.
type Perceiver interface {
	Perceive(context.Context, contextir.Asset) (contextir.Evidence, error)
}

// Options configures bounded repair of malformed or invalid documents. Model
// transport failures and invalid usage are never automatically retried.
type Options struct {
	// MaxRetries is the number of extra interpretation calls, from 0 through 3.
	// The default, zero, makes exactly one model call after perception succeeds.
	MaxRetries int
}

// CallUsage records every planner Model invocation, including failed attempts.
// Valid is false for impossible negative token counts; callers must not charge
// such reports. Perceiver calls are metered separately by the injected adapter.
type CallUsage struct {
	Stage   string          `json:"stage"`
	Attempt int             `json:"attempt"`
	Model   string          `json:"model"`
	Usage   contextir.Usage `json:"usage"`
	Valid   bool            `json:"valid"`
}

// Result retains reported usage and collected evidence even when Plan returns an
// error. Document is populated only after the complete contract is validated.
type Result struct {
	Document contextir.Document   `json:"document"`
	Evidence []contextir.Evidence `json:"evidence,omitempty"`
	Calls    []CallUsage          `json:"calls,omitempty"`
}

// Planner is immutable after New. It can be used concurrently when its adapters
// can be used concurrently. It never reads process-wide environment variables.
type Planner struct {
	model     Model
	perceiver Perceiver
	options   Options
}

func New(model Model, perceiver Perceiver, options Options) (*Planner, error) {
	if isNil(model) {
		return nil, issueError(contextir.CodeModel, "setup", "model", "A model adapter is required.", nil)
	}
	if options.MaxRetries < 0 || options.MaxRetries > 3 {
		return nil, issueError(contextir.CodeInvalid, "setup", "options.max_retries", "MaxRetries must be between zero and three.", nil)
	}
	return &Planner{model: model, perceiver: perceiver, options: options}, nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	switch reflect.ValueOf(value).Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflect.ValueOf(value).IsNil()
	default:
		return false
	}
}

// Plan perceives every input asset before interpreting the request. For already
// analyzed media, use PlanWithEvidence to avoid another perception call.
func (p *Planner) Plan(ctx context.Context, request contextir.Request) (Result, error) {
	return p.PlanWithEvidence(ctx, request, nil)
}

// PlanWithEvidence accepts complete observations for any subset of input assets.
// Missing assets are passed to Perceiver. Supplied observations are validated
// before any adapter call; unknown assets and duplicate evidence are rejected.
func (p *Planner) PlanWithEvidence(ctx context.Context, request contextir.Request, supplied []contextir.Evidence) (Result, error) {
	var result Result
	if err := canceled(ctx, "request"); err != nil {
		return result, err
	}
	if err := contextir.ValidateRequest(request); err != nil {
		return result, err
	}
	if err := contextir.ValidateEvidence(request.Assets, supplied); err != nil {
		return result, perceptionError(err)
	}
	// Preserve a private snapshot before calling any adapter: callbacks may own
	// and edit the caller's slices, including dialogue speakers and fact ranges.
	request = cloneRequest(request)
	supplied = cloneEvidence(supplied)
	if p == nil || isNil(p.model) {
		return result, issueError(contextir.CodeModel, "setup", "model", "A model adapter is required.", nil)
	}
	evidence, err := p.collectEvidence(ctx, request.Assets, supplied)
	result.Evidence = cloneEvidence(evidence)
	if err != nil {
		return result, err
	}
	var diagnostics []contextir.Issue
	for attempt := 1; attempt <= p.options.MaxRetries+1; attempt++ {
		if err := canceled(ctx, "plan"); err != nil {
			return result, err
		}
		input, err := json.Marshal(struct {
			PromptVersion string               `json:"prompt_version"`
			Request       contextir.Request    `json:"request"`
			Evidence      []contextir.Evidence `json:"evidence"`
			Diagnostics   []contextir.Issue    `json:"diagnostics,omitempty"`
		}{PromptVersion, request, evidence, diagnostics})
		if err != nil {
			return result, issueError(contextir.CodeInvalid, "request", "request", "The request cannot be encoded.", err)
		}
		if err := canceled(ctx, "plan"); err != nil {
			return result, err
		}
		response, callErr := p.model.Generate(ctx, contextir.ModelRequest{
			Stage:          "plan",
			System:         systemPrompt,
			Input:          input,
			ResponseSchema: contextir.DocumentSchema(),
		})
		validUsage := response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		result.Calls = append(result.Calls, CallUsage{
			Stage: "plan", Attempt: attempt, Model: response.Model,
			Usage: response.Usage, Valid: validUsage,
		})
		if err := canceled(ctx, "plan"); err != nil {
			return result, err
		}
		if errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded) {
			return result, issueError(contextir.CodeCanceled, "plan", "model", "The model call was canceled.", callErr)
		}
		if !validUsage {
			return result, issueError(contextir.CodeModelOutput, "plan", "usage", "Reported token counts must be nonnegative.", callErr)
		}
		if callErr != nil {
			return result, issueError(contextir.CodeModel, "plan", "model", "The model call failed.", callErr)
		}
		document, err := contextir.Decode(bytes.NewReader(response.JSON))
		if err == nil {
			err = validateLocked(request, document, evidence)
		}
		if err == nil {
			// The actual observations are authoritative; they are attached by the
			// host rather than accepted as model-invented observations.
			document.Evidence = cloneEvidence(evidence)
			err = contextir.Validate(document)
		}
		if err == nil {
			result.Document = document
			return result, nil
		}
		outputErr := modelOutputError(err)
		if attempt > p.options.MaxRetries {
			return result, outputErr
		}
		// A configured retry requests a new complete document using the original
		// inputs and safe diagnostics. No invalid draft is returned or reused.
		diagnostics = outputErr.Issues
	}
	panic("planner: unreachable bounded attempt loop")
}

func canceled(ctx context.Context, stage string) error {
	if err := ctx.Err(); err != nil {
		return issueError(contextir.CodeCanceled, stage, "context", "The operation was canceled.", err)
	}
	return nil
}

func issueError(code contextir.Code, stage, path, message string, cause error) *contextir.Error {
	return &contextir.Error{Code: code, Stage: stage, Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}, Cause: cause}
}

func modelOutputError(cause error) *contextir.Error {
	var typed *contextir.Error
	if errors.As(cause, &typed) && len(typed.Issues) > 0 {
		issues := append([]contextir.Issue(nil), typed.Issues...)
		return &contextir.Error{Code: contextir.CodeModelOutput, Stage: "plan", Issues: issues, Cause: cause}
	}
	// Do not echo a provider or JSON decoder error: it can contain model text,
	// user text, unknown field names, or private media locators.
	return issueError(contextir.CodeModelOutput, "plan", "document", "Return exactly one valid Document matching the supplied schema.", cause)
}
