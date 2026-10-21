package contextir_test

import (
	"testing"

	ir "github.com/ly95/contextir"
	"github.com/ly95/contextir/h3"
)

// The upstream R6/R7 preservation rules concern the IR relationship itself,
// including base modes whose prompt does not print a retention section.
func TestReferenceRetentionPreservesFrameAndPictureContracts(t *testing.T) {
	for _, role := range []ir.Role{ir.RoleFirstFrame, ir.RoleLastFrame, ir.RoleKeyframe} {
		t.Run(string(role), func(t *testing.T) {
			mode := ir.ModeFirstFrame
			if role == ir.RoleLastFrame {
				mode = ir.ModeLastFrame
			} else if role == ir.RoleKeyframe {
				mode = ir.ModeReference
			}
			d := validationDocument(mode)
			if role == ir.RoleKeyframe {
				at := d.Target.Duration / 2
				d.References = []ir.Reference{{
					ID: "frame", Kind: ir.ReferencePicture,
					Source: ir.SourceRef{AssetID: "reference-image", Track: ir.TrackVisual},
					Roles:  []ir.Role{role}, Description: "The supplied frame anchors the target.",
					Retention: ir.FullyPreserved, Anchor: &at,
				}}
			}
			if err := ir.Validate(d); err != nil {
				t.Fatalf("valid frame relationship rejected: %v", err)
			}
			for _, marker := range []ir.Retention{ir.PartiallyPreserved, ir.WeakReference, ir.AttributeTransfer} {
				t.Run(string(marker), func(t *testing.T) {
					d.References[0].Retention = marker
					assertValidationInvalid(t, d, "references[0].retention")
					artifact, err := h3.Compile(d, h3.OfficialProfile())
					if !ir.HasCode(err, ir.CodeInvalid) || artifact.Prompt != "" {
						t.Fatalf("invalid frame relationship was published: %v", err)
					}
				})
			}
		})
	}
	t.Run("ordinary_picture", func(t *testing.T) {
		d := validationDocument(ir.ModeReference)
		d.References = []ir.Reference{{
			ID: "style", Kind: ir.ReferencePicture,
			Source: ir.SourceRef{AssetID: "reference-image", Track: ir.TrackVisual},
			Roles:  []ir.Role{ir.RoleStyle}, Description: "The supplied style reference.",
		}}
		for _, marker := range []ir.Retention{ir.FullyPreserved, ir.PartiallyPreserved, ir.WeakReference} {
			d.References[0].Retention = marker
			if err := ir.Validate(d); err != nil {
				t.Fatalf("valid ordinary picture marker %s rejected: %v", marker, err)
			}
		}
		d.References[0].Retention = ir.AttributeTransfer
		assertValidationInvalid(t, d, "references[0].retention")
		artifact, err := h3.Compile(d, h3.OfficialProfile())
		if !ir.HasCode(err, ir.CodeInvalid) || artifact.Prompt != "" {
			t.Fatalf("picture attribute transfer was published: %v", err)
		}
	})
}
