package deepseek

import (
	"bytes"
	"encoding/json"
	"github.com/ly95/h3-context-ir-go"
)

// ImagePerceptionRequest exposes the exact production prompt/schema/input contract
// for explicit observation-cache identity. It performs no model or media IO and
// never includes caller notes, opaque locators or credentials.
func ImagePerceptionRequest(asset contextir.Asset, diagnostics []contextir.Issue, includeCard bool) (contextir.ModelRequest, error) {
	if asset.ID == "" || asset.Kind != contextir.MediaImage {
		return contextir.ModelRequest{}, perceptionFailure(contextir.CodeInvalid, "asset", "A named still image is required.", nil)
	}
	// Deliberately omit URI, Description and all user-authored asset notes. The
	// actual pixels travel exclusively through the model's image input channel.
	input, _ := json.Marshal(struct {
		AssetID     string                  `json:"asset_id"`
		Metadata    contextir.MediaMetadata `json:"metadata"`
		Diagnostics []contextir.Issue       `json:"diagnostics,omitempty"`
	}{AssetID: asset.ID, Metadata: asset.Metadata, Diagnostics: diagnostics})
	request := contextir.ModelRequest{Stage: "image_perception", System: "Prompt version: " + ImagePerceptionPromptVersion + "\n\n" + imagePerceptionSystem, Input: input, ResponseSchema: contextir.EvidenceSchema()}
	if includeCard {
		request.System += "\n\nCard contract contextir-image-card/v2: Also return card with the exact asset_id and analyzer_version=adapter-assigned. Populate summary, subjects, environment, lighting, palette, framing, style, visible_text, composition, is_reference_sheet. Each subject has kind (person|animal|object|environment|style), descriptor, attributes and pose. Attributes are stable identity (hair, build, clothing, material, markings); pose is transient posture, gestures, gaze and expression and must not be put in attributes. A character reference sheet is one identity across panels, not separate subjects or a location to reproduce. composition is bare_plate, composed_scene or unknown. A still image cannot establish motion, camera movement, audio, music, voice or transcript; leave these absent. Do not invent hidden or uncertain attributes. Omit card.model and card.coverage; the adapter assigns them."
		var schema map[string]any
		dec := json.NewDecoder(bytes.NewReader(request.ResponseSchema))
		dec.UseNumber()
		if err := dec.Decode(&schema); err != nil {
			return contextir.ModelRequest{}, perceptionFailure(contextir.CodeInvalid, "schema", "image-card schema could not be constructed", nil)
		}
		defs := schema["$defs"].(map[string]any)
		definition := defs["Evidence"].(map[string]any)
		definition["required"] = append(definition["required"].([]any), "card")
		props := definition["properties"].(map[string]any)
		props["asset_id"] = map[string]any{"type": "string", "const": asset.ID}
		// A still has no optional audio track or temporal sampling. Constrain the
		// actual model contract, then check the decoded response locally as well.
		props["coverage"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"status", "tracks"}, "properties": map[string]any{"status": map[string]any{"type": "string", "enum": []string{"complete", "partial"}}, "tracks": map[string]any{"const": []string{"visual"}}}}
		request.ResponseSchema, _ = json.Marshal(schema)
	}
	return request, nil
}
