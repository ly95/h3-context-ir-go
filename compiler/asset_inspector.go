package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ly95/contextir"
)

// AssetInspector measures file/container facts through an explicitly injected
// capability. It performs no semantic interpretation and makes no model call.
// Existing caller metadata must be returned unchanged, filling only absent
// fields. Locators remain opaque when this capability is not configured.
type AssetInspector interface {
	InspectAsset(context.Context, contextir.Asset) (contextir.MediaMetadata, error)
}

func (c *Compiler) inspectBriefAssets(ctx context.Context, brief Brief) (Brief, []Decision, error) {
	if err := c.CheckRequest(brief); err != nil {
		return brief, nil, err
	}
	// Snapshot the raw caller fields rather than normalizing them here: Intake
	// must still know which defaults were omitted to record its own decisions.
	wire, err := json.Marshal(brief)
	if err != nil {
		return brief, nil, invalid("brief", "Asset inspection requires serializable caller fields.")
	}
	var b Brief
	if err = json.Unmarshal(wire, &b); err != nil {
		return brief, nil, invalid("brief", "The inspection input could not be snapshotted.")
	}
	inspector := c.options.AssetInspector
	if isNil(inspector) {
		inspector, _ = c.perceiver.(AssetInspector)
	}
	var decisions []Decision
	for i := range b.Assets {
		a := &b.Assets[i]
		if isNil(inspector) {
			if a.Kind == contextir.MediaImage && (a.Metadata.Width == 0 || a.Metadata.Height == 0) {
				decisions = append(decisions, decision("assets."+a.ID+".metadata", "unmeasured", a.Metadata, "No asset-inspection capability was configured. Source geometry stays unknown; target canvas dimensions are not substituted as an observation and source rows remain unknown unless explicit preparation establishes them."))
			}
			continue
		}
		if ctx.Err() != nil {
			return b, decisions, failure(contextir.CodeCanceled, "inspect", "context", "Asset inspection was canceled.", ctx.Err())
		}
		measured, inspectionErr := inspector.InspectAsset(ctx, a.Asset)
		if inspectionErr != nil {
			var typed *contextir.Error
			if errors.As(inspectionErr, &typed) && typed != nil {
				return b, decisions, inspectionErr
			}
			return b, decisions, failure(contextir.CodePerception, "inspect", "assets."+a.ID, "The configured inspector could not measure the supplied asset.", inspectionErr)
		}
		if ctx.Err() != nil {
			return b, decisions, failure(contextir.CodeCanceled, "inspect", "context", "Asset inspection was canceled.", ctx.Err())
		}
		before := a.Metadata
		if measured.Width < 0 || measured.Height < 0 || measured.Duration < 0 || measured.FPS.Numerator < 0 || measured.FPS.Denominator < 0 {
			return b, decisions, invalid("assets.metadata", "Asset inspection returned invalid negative metadata.")
		}
		if before.Width != 0 && measured.Width != before.Width || before.Height != 0 && measured.Height != before.Height || before.Duration != 0 && measured.Duration != before.Duration || before.FPS != (contextir.Rational{}) && measured.FPS != before.FPS {
			return b, decisions, invalid("assets."+a.ID+".metadata", "Asset inspection cannot overwrite explicit caller metadata; resolve conflicting measurement or prepared-source declarations explicitly.")
		}
		a.Metadata = measured
		decisions = append(decisions, decision(fmt.Sprintf("assets.%s.metadata", a.ID), "configured_asset_inspector", measured, "Container/header facts were measured before target preparation, budgeting and mode geometry. Explicit caller metadata is preserved; this is not semantic media observation."))
	}
	return b, decisions, nil
}
