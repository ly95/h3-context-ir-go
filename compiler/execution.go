package compiler

import (
	"context"
	"fmt"

	"github.com/ly95/contextir"
)

// ExecutionOptions are caller choices affecting the model/analyzer, not film
// content. An adapter factory must explicitly implement requested seed/effort;
// the compiler never assumes an injected backend supports a sampling parameter.
type ExecutionOptions struct {
	Seed   *int64 `json:"seed,omitempty"`
	Effort string `json:"effort"`
}
type ConfigureExecution func(context.Context, ExecutionOptions, Model, Perceiver) (Model, Perceiver, error)

func (c *Compiler) executionFor(ctx context.Context, b Brief) (*Compiler, error) {
	if c == nil {
		return nil, invalid("compiler", "Initialize the compiler with New.")
	}
	if ctx.Err() != nil {
		return nil, failure(contextir.CodeCanceled, "execution", "context", "Execution configuration was canceled.", ctx.Err())
	}
	if c.executionConfigured {
		if sameExecution(c.executionChoice, b) {
			return c, nil
		}
		return c.executionParent.executionFor(ctx, b)
	}
	effort := b.Effort
	if effort == "" {
		effort = "standard"
	}
	if effort != "fast" && effort != "standard" && effort != "max" {
		return nil, invalid("effort", "Choose fast, standard or max.")
	}
	if b.Seed == nil && effort == "standard" {
		return c, nil
	}
	if c.options.ConfigureExecution == nil {
		return nil, failure(contextir.CodeUnsupported, "execution", "execution_options", "An explicit seed or nonstandard effort needs an adapter configuration policy that implements those parameters.", nil)
	}
	seed := b.Seed
	if seed != nil {
		copySeed := *seed
		seed = &copySeed
	}
	model, perceiver, err := c.options.ConfigureExecution(ctx, ExecutionOptions{Seed: seed, Effort: effort}, c.model, c.perceiver)
	if err != nil {
		return nil, failure(contextir.CodeUnsupported, "execution", "execution_options", "The selected adapters cannot implement the requested seed/effort.", err)
	}
	if isNil(model) {
		return nil, invalid("execution.model", "Execution configuration must return a model adapter.")
	}
	next := *c
	next.model = model
	next.perceiver = perceiver
	next.executionConfigured = true
	next.executionParent = c
	next.executionChoice = ExecutionOptions{Effort: effort}
	if b.Seed != nil {
		copySeed := *b.Seed
		next.executionChoice.Seed = &copySeed
	}
	return &next, nil
}
func (c *Compiler) needsExecution(b Brief) bool {
	if c == nil {
		return false
	}
	if c.executionConfigured {
		return !sameExecution(c.executionChoice, b)
	}
	return b.Seed != nil || (b.Effort != "" && b.Effort != "standard")
}

func sameExecution(choice ExecutionOptions, b Brief) bool {
	effort := b.Effort
	if effort == "" {
		effort = "standard"
	}
	if choice.Effort != effort || (choice.Seed == nil) != (b.Seed == nil) {
		return false
	}
	return choice.Seed == nil || *choice.Seed == *b.Seed
}

func validateTranscripts(b Brief) error {
	for id, text := range b.Transcripts {
		asset, exists := assetID(b.Assets, id)
		if !exists || asset.Kind != contextir.MediaAudio {
			return invalid("transcripts", "Caller transcripts must identify an attached audio asset by its stable ID.")
		}
		if text == "" {
			return invalid("transcripts", "A supplied transcript must contain the caller's words.")
		}
	}
	return nil
}

func executionDecision(b Brief) Decision {
	effort := b.Effort
	if effort == "" {
		effort = "standard"
	}
	explanation := "The injected adapters and explicit compiler repair configuration implement standard effort."
	if b.Seed != nil || effort != "standard" {
		explanation = "The caller's explicit adapter configuration policy implements these seed/effort choices; no unsupported sampling parameter was silently discarded."
	}
	return decision("execution", "explicit_adapter_policy", ExecutionOptions{Seed: b.Seed, Effort: effort}, explanation)
}

// transcriptInputs deliberately labels words as caller supplied. They do not
// become observed Fact records or evidence of voice, timbre, music or modality.
func transcriptInputs(b Brief) []map[string]string {
	var result []map[string]string
	for _, asset := range b.Assets {
		if text, ok := b.Transcripts[asset.ID]; ok {
			result = append(result, map[string]string{"asset_id": asset.ID, "text": text, "origin": "caller_transcript", "scope": "words_only", "note": fmt.Sprintf("No voice or acoustic observation is established for %s by this text.", asset.ID)})
		}
	}
	return result
}
