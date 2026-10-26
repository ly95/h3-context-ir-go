package planner

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/ly95/h3-context-ir-go"
)

const PromptVersion = "contextir/planner/v4"

//go:embed prompts/plan_v4.txt
var systemPrompt string

type Model interface {
	Generate(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error)
}

type Perceiver interface {
	Perceive(context.Context, contextir.Asset) (contextir.Evidence, error)
}

type Options struct {
	ValidateDocument         func(contextir.Request, contextir.Document) error
	MaxRetries               int
	RepairProtocol           RepairProtocol
	AllowPartialPerception   bool
	AllowUnspecifiedCoverage bool
}

type CallUsage = contextir.CallUsage

type Result struct {
	Document contextir.Document   `json:"document"`
	Evidence []contextir.Evidence `json:"evidence,omitempty"`
	Calls    []CallUsage          `json:"calls,omitempty"`
}

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
	if options.RepairProtocol == "" {
		options.RepairProtocol = RepairValidatedDocument
	}
	if options.RepairProtocol != RepairValidatedDocument && options.RepairProtocol != RepairDiagnosticsOnly {
		return nil, issueError(contextir.CodeInvalid, "setup", "options.repair_protocol", "Choose validated_document/v1 or diagnostics_only/v1 for model repair requests.", nil)
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

func (p *Planner) Plan(ctx context.Context, request contextir.Request) (Result, error) {
	return p.PlanWithEvidence(ctx, request, nil)
}

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
	schema, err := responseSchema(request)
	if err != nil {
		return result, issueError(contextir.CodeInvalid, "request", "response_schema", "The explicit request cannot be represented by the planning schema.", err)
	}
	var diagnostics []contextir.Issue
	var repair *repairContext
	for attempt := 1; attempt <= p.options.MaxRetries+1; attempt++ {
		if err := canceled(ctx, "plan"); err != nil {
			return result, err
		}
		if len(diagnostics) > 0 && p.options.RepairProtocol == RepairValidatedDocument {
			schema, err = repairResponseSchema(request)
			if err != nil {
				return result, issueError(contextir.CodeInvalid, "plan", "response_schema", "The explicit request cannot be represented by the repair schema.", err)
			}
		}
		input, err := json.Marshal(struct {
			PromptVersion string               `json:"prompt_version"`
			Request       contextir.Request    `json:"request"`
			Evidence      []contextir.Evidence `json:"evidence"`
			Diagnostics   []contextir.Issue    `json:"diagnostics,omitempty"`
			Repair        *repairContext       `json:"repair,omitempty"`
		}{PromptVersion, request, evidence, diagnostics, repair})
		if err != nil {
			return result, issueError(contextir.CodeInvalid, "request", "request", "The request cannot be encoded.", err)
		}
		if err := canceled(ctx, "plan"); err != nil {
			return result, err
		}
		system := systemPrompt
		if repair != nil {
			system += "\n" + contentRepairInstruction
		}
		repair = nil
		response, callErr := p.model.Generate(ctx, contextir.ModelRequest{
			Stage:          "plan",
			System:         system,
			Input:          input,
			ResponseSchema: append(json.RawMessage(nil), schema...),
		})
		validUsage := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
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
			return result, issueError(contextir.CodeModelOutput, "plan", "usage", "Reported usage must be valid and token counts must be nonnegative.", callErr)
		}
		if callErr != nil {
			if contextir.HasCode(callErr, contextir.CodeModelOutput) {
				outputErr := modelOutputError(callErr)
				if attempt <= p.options.MaxRetries {
					diagnostics = outputErr.Issues
					continue
				}
				return result, outputErr
			}
			return result, issueError(contextir.CodeModel, "plan", "model", "The model call failed.", callErr)
		}
		document, err := contextir.Decode(bytes.NewReader(response.JSON))
		if err == nil {
			err = validateLocked(request, document, evidence)
		}
		if err == nil {
			document.Evidence = cloneEvidence(evidence)
			err = contextir.Validate(document)
		}
		guarded := false
		if err == nil && p.options.ValidateDocument != nil {
			data, copyErr := json.Marshal(document)
			if copyErr != nil {
				return result, issueError(contextir.CodeInvalid, "plan", "document", "The guarded document cannot be encoded.", copyErr)
			}
			var snapshot contextir.Document
			if copyErr = json.Unmarshal(data, &snapshot); copyErr != nil {
				return result, issueError(contextir.CodeInvalid, "plan", "document", "The guarded document cannot be copied.", copyErr)
			}
			guarded = true
			err = p.options.ValidateDocument(cloneRequest(request), snapshot)
		}
		if canceledErr := canceled(ctx, "plan"); canceledErr != nil {
			return result, canceledErr
		}
		var stop *validationStop
		if errors.As(err, &stop) {
			return result, stop.cause
		}
		if err == nil {
			result.Document = document
			return result, nil
		}
		outputErr := modelOutputError(err)
		if attempt > p.options.MaxRetries {
			return result, outputErr
		}
		diagnostics = outputErr.Issues
		if guarded && p.options.RepairProtocol == RepairValidatedDocument {
			document.Evidence = nil
			repair = &repairContext{PreviousDocument: document}
		}
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
	if errors.As(cause, &typed) && typed != nil && len(typed.Issues) > 0 {
		issues := append([]contextir.Issue(nil), typed.Issues...)
		return &contextir.Error{Code: contextir.CodeModelOutput, Stage: "plan", Issues: issues, Cause: cause}
	}
	return issueError(contextir.CodeModelOutput, "plan", "document", "Return exactly one valid Document matching the supplied schema.", cause)
}
