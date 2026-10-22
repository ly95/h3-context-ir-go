package contextir

import "context"

type CallUsage struct {
	AssetID   string `json:"asset_id,omitempty"`
	Stage     string `json:"stage"`
	Attempt   int    `json:"attempt"`
	Model     string `json:"model"`
	Usage     Usage  `json:"usage"`
	Valid     bool   `json:"valid"`
	Unmetered bool   `json:"unmetered,omitempty"`
}

type MeteredPerceiver interface {
	PerceiveWithCalls(context.Context, Asset) (Evidence, []CallUsage, error)
}
