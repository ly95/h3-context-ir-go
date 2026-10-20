package contextir

import "context"

// CallUsage is the observable ledger of one actual model or opaque capability invocation. Valid means
// reported token evidence is usable, not that generated content was accepted.
// It contains identifiers and counts, never prompts or private media locations.
type CallUsage struct {
	AssetID string `json:"asset_id,omitempty"`
	Stage   string `json:"stage"`
	Attempt int    `json:"attempt"`
	Model   string `json:"model"`
	Usage   Usage  `json:"usage"`
	Valid   bool   `json:"valid"`
	// Unmetered distinguishes an invoked capability that cannot report its cost
	// from a metered response with invalid token evidence. It requires Valid=false
	// and zero Usage; the zero counts mean unknown, never a free model call.
	Unmetered bool `json:"unmetered,omitempty"`
}

// MeteredPerceiver optionally returns every model call, including rejected
// attempts. Implementations must not invoke perception twice to obtain usage.
type MeteredPerceiver interface {
	PerceiveWithCalls(context.Context, Asset) (Evidence, []CallUsage, error)
}
