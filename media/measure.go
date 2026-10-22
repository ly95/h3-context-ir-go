package media

import (
	"context"

	"github.com/ly95/h3-context-ir-go"
)

// MeasureOptions explicitly authorizes resource resolution. Images need only
// the stdlib header reader; video/audio stream metadata needs configured tools.
type MeasureOptions struct {
	Resolve Resolver
	Tools   *Tools
}

// MeasureAssets fills missing source metadata without mutating caller assets.
// It validates visual kind against bytes before any model sees the source.
// A caller-provided tensor size is authoritative even if its temporary encoded
// representation has a different size, matching the original measure_assets.
func MeasureAssets(ctx context.Context, assets []contextir.Asset, options MeasureOptions) ([]contextir.Asset, error) {
	out := append([]contextir.Asset(nil), assets...)
	for i := range out {
		measured, err := MeasureAsset(ctx, out[i], options)
		if err != nil {
			return nil, err
		}
		out[i].Metadata = measured
	}
	return out, nil
}

func MeasureAsset(ctx context.Context, asset contextir.Asset, options MeasureOptions) (contextir.MediaMetadata, error) {
	if asset.Kind != contextir.MediaImage && asset.Kind != contextir.MediaVideo && asset.Kind != contextir.MediaAudio {
		return asset.Metadata, mediaError(contextir.CodeInvalid, "kind", "Declare a supported image, video or audio kind before measuring this asset.", nil)
	}
	if options.Resolve == nil {
		return asset.Metadata, mediaError(contextir.CodeInvalid, "resolver", "An explicit local asset resolver is required for measurement.", nil)
	}
	if err := ctx.Err(); err != nil {
		return asset.Metadata, err
	}
	path, err := options.Resolve(ctx, asset)
	if err != nil {
		return asset.Metadata, mediaError(contextir.CodePerception, "resolver", "The asset resolver could not resolve measurement input.", err)
	}
	header, err := ValidateDeclaration(path, asset.Kind)
	if err != nil {
		return asset.Metadata, err
	}
	out := asset.Metadata
	if asset.Kind == contextir.MediaImage {
		if out.Width == 0 {
			out.Width = header.Width
		}
		if out.Height == 0 {
			out.Height = header.Height
		}
		return out, nil
	}
	// Like the original header intake, an audio declaration is never rejected
	// merely because an ISO container was identified as video. Decode/probe is
	// an explicit additional capability, rather than an inferred visual track.
	if options.Tools == nil {
		return out, nil
	}
	info, err := options.Tools.Inspect(ctx, path)
	if err != nil {
		return out, err
	}
	if out.Width == 0 {
		out.Width = info.Metadata.Width
	}
	if out.Height == 0 {
		out.Height = info.Metadata.Height
	}
	if out.Duration == 0 {
		out.Duration = info.Metadata.Duration
	}
	if out.FPS == (contextir.Rational{}) {
		out.FPS = info.Metadata.FPS
	}
	return out, nil
}

// InspectAsset implements compiler.AssetInspector structurally, avoiding a
// dependency from the media adapter back to the high-level compiler package.
func (p *Perceiver) InspectAsset(ctx context.Context, asset contextir.Asset) (contextir.MediaMetadata, error) {
	if p == nil {
		return asset.Metadata, mediaError(contextir.CodeInvalid, "perceiver", "Initialize media perception first.", nil)
	}
	return MeasureAsset(ctx, asset, MeasureOptions{Resolve: p.options.Resolve, Tools: p.options.Tools})
}
