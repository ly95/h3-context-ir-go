package compiler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ly95/h3-context-ir-go"
)

// RenderingPermission settles authorization without observing the source look.
// A source/target taxonomy mismatch cannot grant a rendering change.
type RenderingPermission struct {
	Kind           string                `json:"kind"`
	Disposition    string                `json:"disposition"`
	OperationQuote string                `json:"operation_quote"`
	Target         string                `json:"target"`
	Reasoning      string                `json:"reasoning"`
	Calls          []contextir.CallUsage `json:"calls,omitempty"`
}

func (c *Compiler) decideRenderingPermission(ctx context.Context, b Brief, visual bool) (RenderingPermission, error) {
	return c.renderingPermissionAttempts(ctx, b, visual, nil, c.options.MaxStructureRetries+1, 0)
}

func (c *Compiler) renderingPermissionAttempts(ctx context.Context, b Brief, visual bool, diagnostics []contextir.Issue, maxAttempts, previousAttempts int) (RenderingPermission, error) {
	if !visual {
		return RenderingPermission{Kind: "no_reference", Disposition: "no_reference", Reasoning: "No supplied reference has authority over the rendering medium."}, nil
	}
	if b.Style != "" {
		return RenderingPermission{Kind: "structured_style", Disposition: "convert", Target: b.Style, Reasoning: "The caller explicitly supplied the authoritative structured Style field."}, nil
	}
	out := RenderingPermission{}
	schema := json.RawMessage(`{"type":"object","additionalProperties":false,"required":["kind","operation_quote","target","reasoning"],"properties":{"kind":{"type":"string","enum":["preserve","rendering_transformation","target_rendition"]},"operation_quote":{"type":"string"},"target":{"type":"string"},"reasoning":{"type":"string"}}}`)
	const system = `Resolve only whether the caller authorizes departing from an attached visual reference's rendering. You receive no source look, taxonomy, target geometry or director: none can grant permission. Read the complete caller request and constraints.
Choose rendering_transformation when the caller expressly asks to transform/reinterpret/restyle the rendering, with an authorizing operation and its target. Choose target_rendition when a grammatical instruction directly specifies the result's rendering in a target style or medium, even without a separate transformation verb. Choose preserve when no such instruction exists. A detached adjective or noun-phrase style tag appended to scene action is descriptive metadata, not a rendering operation or a complete target-rendition directive. A fragment does not become authoritative simply because it has its own sentence punctuation. Do not treat every style mention as authorization.
Interpret grammatical and semantic relationships, not keyword lists. A rendering operation can target a technique of unknown taxonomy and can stay within the same coarse family as the source. A change in depicted subject, motion, setting, dreamlike content or mood alone is not a rendering conversion. For an authorized instruction, operation_quote is its exact complete authorizing caller phrase and target is the complete requested rendition noun phrase. For preserve, operation_quote and target must be empty. Return only typed JSON. Do not infer what the unseen reference looks like.`
	var lastErr error
	for current := 1; current <= maxAttempts; current++ {
		attempt := previousAttempts + current
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "rendering_permission", "context", "Rendering permission interpretation was canceled.", ctx.Err())
		}
		input, _ := json.Marshal(map[string]any{"intent": b.Intent, "constraints": b.Constraints, "has_medium_reference": true, "diagnostics": diagnostics})
		response, callErr := c.model.Generate(ctx, contextir.ModelRequest{Stage: "rendering_permission", System: system, Input: input, ResponseSchema: append(json.RawMessage(nil), schema...)})
		valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
		out.Calls = append(out.Calls, contextir.CallUsage{Stage: "rendering_permission", Attempt: attempt, Model: response.Model, Usage: response.Usage, Valid: valid})
		if ctx.Err() != nil {
			return out, failure(contextir.CodeCanceled, "rendering_permission", "context", "Rendering permission interpretation was canceled.", ctx.Err())
		}
		if callErr != nil && !repairableModelOutput(callErr) {
			return out, failure(contextir.CodeModel, "rendering_permission", "model", "Rendering permission interpretation failed.", callErr)
		}
		if !valid {
			return out, failure(contextir.CodeModelOutput, "rendering_permission", "usage", "Rendering permission interpretation returned invalid usage.", callErr)
		}
		if callErr != nil {
			lastErr = callErr
			diagnostics = repairDiagnostics(callErr, "rendering_permission", "Return the exact rendering-permission contract.")
			continue
		}
		var reply struct {
			Kind           string `json:"kind"`
			OperationQuote string `json:"operation_quote"`
			Target         string `json:"target"`
			Reasoning      string `json:"reasoning"`
		}
		lastErr = validateWire(response.JSON, schema)
		if lastErr == nil {
			lastErr = strictDecode(response.JSON, &reply)
		}
		if lastErr == nil {
			if reply.Kind == "preserve" {
				if reply.OperationQuote != "" || reply.Target != "" {
					lastErr = invalid("rendering_permission", "Preserve has no claimed authorizing operation or transferred target.")
				}
			} else if reply.OperationQuote == "" || !scopeTextContains(b, reply.OperationQuote, nil) || strings.TrimSpace(reply.Target) == "" {
				lastErr = invalid("rendering_permission.operation_quote", "An authorized rendition needs exact caller words and its complete supported target.")
			}
		}
		if lastErr == nil {
			out.Kind, out.OperationQuote, out.Target, out.Reasoning = reply.Kind, reply.OperationQuote, reply.Target, reply.Reasoning
			out.Disposition = "preserve"
			if reply.Kind != "preserve" {
				out.Disposition = "convert"
			}
			return out, nil
		}
		diagnostics = repairDiagnostics(lastErr, "rendering_permission", "Distinguish an actual caller rendering instruction from a detached descriptive style fragment.")
	}
	return out, failure(contextir.CodeModelOutput, "rendering_permission", "permission", "Rendering permission did not satisfy the typed contract.", lastErr)
}
