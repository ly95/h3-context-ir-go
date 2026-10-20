package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ly95/contextir/budget"
	"github.com/ly95/contextir/compiler"
)

const ContractVersion = 2

var creativity = []string{"restrained", "balanced", "bold", "extreme"}
var effort = []string{"fast", "standard", "max"}
var sizing = []string{"match", "max"}
var aspects = []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}
var languages = []string{"Arabic", "Chinese", "English", "French", "German", "Italian", "Japanese", "Korean", "Portuguese", "Russian", "Spanish"}
var rolesByKind = map[string][]string{"image": {"frame_anchor_first", "frame_anchor_last", "subject", "environment", "style", "storyboard", "placed_subject", "replacement_subject"}, "video": {"edit_source", "continuation_source", "subject", "environment", "style", "storyboard", "structure"}, "audio": {"voice_timbre", "bgm", "music_style", "beat_reference", "sfx"}}
var sections = []string{"asset_fields", "brief_fields", "dialogue_fields", "error_codes", "roles", "directors", "camera_moves", "limits"}
var errorSpecs = map[string]map[string]any{
	"analysis-tool-missing": {"status": 503, "on": []string{"briefs"}}, "aspect-invalid": {"status": 422, "on": []string{"briefs"}},
	"asset-digest-mismatch": {"status": 422, "on": []string{"assets"}}, "asset-missing": {"status": 422, "on": []string{"briefs"}},
	"asset-name-not-a-digest": {"status": 422, "on": []string{"assets", "briefs"}}, "asset-no-path": {"status": 422, "on": []string{"briefs"}},
	"asset-not-uploaded": {"status": 422, "on": []string{"briefs"}}, "asset-paths-disabled": {"status": 422, "on": []string{"briefs"}},
	"asset-too-large": {"status": 413, "on": []string{"assets"}}, "asset-two-sources": {"status": 422, "on": []string{"briefs"}},
	"asset-unreadable": {"status": 422, "on": []string{"briefs"}}, "change-empty": {"status": 422, "on": []string{"briefs"}},
	"director-profile-invalid": {"status": 422, "on": []string{"briefs"}}, "duration-invalid": {"status": 422, "on": []string{"briefs"}},
	"intent-empty": {"status": 422, "on": []string{"briefs"}}, "llm-error": {"status": 502, "on": []string{"briefs"}},
	"llm-unavailable": {"status": 503, "on": []string{"briefs"}}, "malformed-request": {"status": 422, "on": []string{"briefs"}},
	"over-capacity": {"status": 422, "on": []string{"briefs"}}, "replacement-subject-undefined": {"status": 422, "on": []string{"briefs"}},
	"replacement-target-ambiguous": {"status": 422, "on": []string{"briefs"}}, "replacement-target-unnamed": {"status": 422, "on": []string{"briefs"}},
	"replaces-without-the-role": {"status": 422, "on": []string{"briefs"}}, "shots-do-not-fit": {"status": 422, "on": []string{"briefs"}},
	"shots-invalid": {"status": 422, "on": []string{"briefs"}}, "swap-without-edit-source": {"status": 422, "on": []string{"briefs"}},
	"unknown-brief": {"status": 404, "on": []string{"briefs"}}, "unknown-field": {"status": 422, "on": []string{"briefs"}},
	"unknown-role": {"status": 422, "on": []string{"briefs"}}, "upload-store-full": {"status": 507, "on": []string{"assets"}},
}

func directorMappings() []map[string]string {
	out := []map[string]string{}
	for _, d := range compiler.Directors() {
		out = append(out, map[string]string{"id": d.ID, "name": d.Name, "notes": d.Notes, "origin": "builtin"})
	}
	return out
}
func maxAssets() map[string]int {
	return map[string]int{"images": budget.MaxRefImages, "videos": budget.MaxRefVideos, "audios": budget.MaxRefAudios, "video_soundtracks": budget.MaxRefVideoSoundtracks}
}
func contractSections() map[string]any {
	return map[string]any{"asset_fields": fieldNames(AssetIn{}), "brief_fields": fieldNames(BriefIn{}), "dialogue_fields": fieldNames(DialogueIn{}), "error_codes": errorSpecs, "roles": rolesByKind, "directors": directorMappings(), "camera_moves": compiler.CameraTypes, "limits": map[string]any{"director_notes_max_chars": compiler.MaxDirectorNotesChars, "max_pinned_shots": compiler.MaxPinnedShots, "fps": budget.FPS, "trained_frames": []int{budget.TrainedMinFrames, budget.TrainedMaxFrames}, "max_assets": maxAssets(), "aspects": aspects, "creativity": creativity, "effort": effort, "sizing": sizing, "dialogue_languages": languages}}
}

// Digest uses the original contract's sorted, UTF-8, compact JSON convention.
func Digest(v any) (string, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes.TrimSuffix(b.Bytes(), []byte("\n")))
	return hex.EncodeToString(sum[:8]), nil
}
func Digests() map[string]string {
	out := map[string]string{}
	c := contractSections()
	for _, k := range sections {
		out[k], _ = Digest(c[k])
	}
	return out
}

// Snapshot contains no deployment-dependent build version. The field lists are
// derived from the exact request structs used by PrepareBrief.
func Snapshot() map[string]any {
	c := contractSections()
	c["contract_version"] = ContractVersion
	c["field_lists_describe"] = "the keys of a POST /v1/briefs request. Not the fields of h3ir.models.Brief and h3ir.models.AssetRef, which carry more and are the compiler's own, not a caller's."
	c["digests"] = Digests()
	data, _ := json.Marshal(c)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}
func Contract(packageVersion string) map[string]any {
	out := Snapshot()
	out["package_version"] = packageVersion
	return out
}
func AsJSON() (string, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	err := e.Encode(orderedSnapshot())
	return b.String(), err
}

type pair struct {
	key   string
	value any
}
type ordered []pair

func (o ordered) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(p.key)
		b.Write(k)
		b.WriteByte(':')
		var v bytes.Buffer
		e := json.NewEncoder(&v)
		e.SetEscapeHTML(false)
		if err := e.Encode(p.value); err != nil {
			return nil, err
		}
		b.Write(bytes.TrimSuffix(v.Bytes(), []byte("\n")))
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
func orderedSnapshot() ordered {
	c := contractSections()
	out := ordered{{"contract_version", ContractVersion}, {"field_lists_describe", Snapshot()["field_lists_describe"]}}
	for _, name := range sections {
		value := c[name]
		switch name {
		case "error_codes":
			keys := []string{}
			for k := range errorSpecs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			obj := ordered{}
			for _, k := range keys {
				obj = append(obj, pair{k, ordered{{"status", errorSpecs[k]["status"]}, {"on", errorSpecs[k]["on"]}}})
			}
			value = obj
		case "roles":
			value = ordered{{"image", rolesByKind["image"]}, {"video", rolesByKind["video"]}, {"audio", rolesByKind["audio"]}}
		case "limits":
			l := c[name].(map[string]any)
			obj := ordered{}
			for _, k := range []string{"director_notes_max_chars", "max_pinned_shots", "fps", "trained_frames", "max_assets", "aspects", "creativity", "effort", "sizing", "dialogue_languages"} {
				v := l[k]
				if k == "max_assets" {
					a := maxAssets()
					v = ordered{{"images", a["images"]}, {"videos", a["videos"]}, {"audios", a["audios"]}, {"video_soundtracks", a["video_soundtracks"]}}
				}
				obj = append(obj, pair{k, v})
			}
			value = obj
		}
		out = append(out, pair{name, value})
	}
	d := Digests()
	obj := ordered{}
	for _, name := range sections {
		obj = append(obj, pair{name, d[name]})
	}
	return append(out, pair{"digests", obj})
}

func jsString(s string) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// AsJS publishes the source-compatible standalone browser vocabulary. It carries
// display profiles, motion names and the notes cap, with no runtime dependency.
func AsJS() string {
	parts := []string{"/* GENERATED by `h3ir contract --js`. Do not edit.\n *\n * The seven directions, the twenty camera moves H3 has names for, and the cap the\n * compiler refuses a longer direction at. The compiler owns all three; this file is the\n * copy the panel draws from so the picker works with no service running.\n *\n * To refresh: run the command above with the matching open-h3-ir installed, and write its\n * output over this file. `tests/test_contract_drift.py` fails while it is stale.\n */\n", fmt.Sprintf("export const CONTRACT_VERSION = %d;", ContractVersion), "export const DIRECTORS_DIGEST = " + jsString(Digests()["directors"]) + ";", fmt.Sprintf("export const MAX_NOTES = %d;", compiler.MaxDirectorNotesChars), ""}
	rows := []string{"["}
	row := " "
	for _, move := range compiler.CameraTypes {
		item := " " + jsString(move) + ","
		if len(row)+len(item) > 96 {
			rows = append(rows, row)
			row = " "
		}
		row += item
	}
	rows = append(rows, row, "]")
	parts = append(parts, "export const CAMERA_MOVES = "+strings.Join(rows, "\n")+";", "", "export const DIRECTORS = [")
	for _, d := range compiler.Directors() {
		parts = append(parts, "  {", "    id: "+jsString(d.ID)+",", "    name: "+jsString(d.Name)+",", "    notes: "+jsString(d.Notes)+",", "  },")
	}
	parts = append(parts, "];")
	return strings.Join(parts, "\n") + "\n"
}
func RolesByKind() map[string][]string {
	out := map[string][]string{}
	for k, v := range rolesByKind {
		out[k] = append([]string(nil), v...)
	}
	return out
}
func SectionNames() []string { return append([]string(nil), sections...) }

func (s *Service) Capabilities() map[string]any {
	durations := []float64{}
	for _, n := range budget.LegalFrames(true, 0) {
		durations = append(durations, round(float64(n)/budget.FPS, 3))
	}
	sort.Float64s(durations)
	a := s.cfg.Store.Config()
	counts := maxAssets()
	counts["total_files"] = budget.MaxRefImages + budget.MaxRefVideos + budget.MaxRefAudios + budget.MaxRefVideoSoundtracks
	return map[string]any{"durations_seconds": durations, "trained_band_seconds": []float64{round(float64(budget.TrainedMinFrames)/budget.FPS, 3), round(float64(budget.TrainedMaxFrames)/budget.FPS, 3)}, "longer_durations": "supported if frame-aligned; slower, not rejected", "canvas": "any multiple of 32; 768 short edge is the default, larger renders fine", "aspects": aspects, "max_assets": counts, "dialogue_languages": languages, "output": map[string]any{"short_edge": budget.BaseShortEdge, "fps": budget.FPS, "audio": "32 kHz stereo"}, "contract": map[string]any{"version": ContractVersion, "endpoint": "GET /v1/contract"}, "assets": map[string]any{"paths": s.cfg.AllowPaths, "uploads": a.MaxBytes > 0, "upload_endpoint": "PUT /v1/assets/{sha256}", "upload_max_bytes": a.MaxBytes, "upload_store_bytes": a.StoreBytes, "upload_ttl_hours": a.TTL.Hours()}, "note": "2K output needs MiniMax's closed regeneration stage; local output is 768p."}
}
