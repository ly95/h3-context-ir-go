package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/uploads"
)

type PrepareConfig struct {
	Store           *uploads.Store
	AllowPaths      bool
	MaxRequestBytes int64
}

// PrepareRequest reuses the HTTP request boundary without constructing a server
// or a model capability. An omitted byte bound uses the documented 16 MiB cap;
// no upload store is needed unless the request actually names uploaded digests.
func PrepareRequest(cfg PrepareConfig, payload []byte) (plan.Brief, error) {
	if cfg.MaxRequestBytes == 0 {
		cfg.MaxRequestBytes = 16 << 20
	}
	if cfg.MaxRequestBytes <= 0 || cfg.MaxRequestBytes == math.MaxInt64 {
		return plan.Brief{}, malformed("configuration", "MaxRequestBytes must be a positive finite byte bound")
	}
	p := Service{cfg: Config{Store: cfg.Store, AllowPaths: cfg.AllowPaths, MaxRequestBytes: cfg.MaxRequestBytes}}
	return p.PrepareBrief(payload)
}

func fieldNames(v any) []string {
	t := reflect.TypeOf(v)
	out := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		out = append(out, strings.Split(t.Field(i).Tag.Get("json"), ",")[0])
	}
	return out
}
func knownKeys(raw json.RawMessage, fields []string, path string) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if e := json.Unmarshal(raw, &obj); e != nil || obj == nil {
		return nil, malformed(path, "Input should be a JSON object")
	}
	allowed := map[string]bool{}
	for _, f := range fields {
		allowed[f] = true
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !allowed[k] {
			where := k
			if path != "" {
				where = path + "." + k
			}
			return nil, &Error{Status: 422, Code: "unknown-field", Message: fmt.Sprintf("this build does not know the field %q. It was refused rather than ignored. The client and service contracts differ; GET /v1/contract lists every field this build takes.", where), Problems: []map[string]any{{"type": "extra_forbidden", "loc": strings.Split(where, "."), "msg": "Extra inputs are not permitted"}}}
		}
	}
	return obj, nil
}
func malformed(path, msg string) *Error {
	if path == "" {
		path = "the request"
	}
	return &Error{Status: 422, Code: "malformed-request", Message: path + ": " + msg + ".", Problems: []map[string]any{{"type": "value_error", "loc": strings.Split(path, "."), "msg": msg}}}
}
func required(obj map[string]json.RawMessage, name, path string) error {
	raw, ok := obj[name]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if path != "" {
			name = path + "." + name
		}
		return malformed(name, "Field required")
	}
	return nil
}
func enum(v string, valid []string) bool {
	for _, s := range valid {
		if v == s {
			return true
		}
	}
	return false
}
func nullableObject(raw json.RawMessage, path string) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return malformed(path, "Input should be a JSON object")
	}
	return nil
}

// ParseBrief applies the public request schema and preserves all caller values.
// Nested dictionaries explicitly declared open (provenance/director metadata)
// retain their content; no user-authored text is interpreted as intent here.
func ParseBrief(data []byte) (BriefIn, error) {
	var b BriefIn
	if !utf8.Valid(data) {
		return b, malformed("the request", "JSON must be valid UTF-8")
	}
	if e := uniqueJSON(data); e != nil {
		return b, malformed("the request", e.Error())
	}
	obj, e := knownKeys(data, fieldNames(b), "")
	if e != nil {
		return b, e
	}
	if e = required(obj, "intent", ""); e != nil {
		return b, e
	}
	b = BriefIn{Seconds: 5, Aspect: "16:9", Creativity: "balanced", Effort: "standard", Assets: []AssetIn{}, Dialogue: []DialogueIn{}, OnscreenText: []string{}, LoRAs: []json.RawMessage{}, Constraints: []string{}, Transcripts: map[string]string{}}
	if e = json.Unmarshal(data, &b); e != nil {
		path := "the request"
		if te, ok := e.(*json.UnmarshalTypeError); ok && te.Field != "" {
			path = te.Field
		}
		return b, malformed(path, e.Error())
	}
	for _, k := range []string{"intent", "assets", "seconds", "aspect", "dialogue", "onscreen_text", "loras", "silent", "constraints", "creativity", "director", "effort", "transcripts", "audio_copy_modes", "transcript_uses", "subject_projections"} {
		if raw, ok := obj[k]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return b, malformed(k, "Input cannot be null")
		}
	}
	if !enum(b.Creativity, creativity) {
		return b, malformed("creativity", "Input should be restrained, balanced, bold or extreme")
	}
	if !enum(b.Effort, effort) {
		return b, malformed("effort", "Input should be fast, standard or max")
	}
	if math.IsNaN(b.Seconds) || math.IsInf(b.Seconds, 0) {
		return b, malformed("seconds", "Input should be a finite number")
	}
	if b.Megapixels != nil && (*b.Megapixels < .25 || *b.Megapixels > 2.5 || math.IsNaN(*b.Megapixels) || math.IsInf(*b.Megapixels, 0)) {
		return b, malformed("megapixels", "Input should be between 0.25 and 2.5")
	}
	if e = nullableObject(b.DirectorProfile, "director_profile"); e != nil {
		return b, e
	}
	for id, answer := range b.Answer {
		if id != "anchor_or_reference" {
			return b, malformed("answer."+id, "Unknown clarification ID")
		}
		if answer != "opening_frame" && answer != "appears_in" {
			return b, malformed("answer."+id, "Input should be opening_frame or appears_in")
		}
	}
	for id, mode := range b.AudioCopyModes {
		if strings.TrimSpace(id) == "" || mode != "fully_copy" && mode != "partially_copy" {
			return b, malformed("audio_copy_modes", "Each asset-N key requires fully_copy or partially_copy")
		}
	}
	if e = validateWireChoices(obj, b); e != nil {
		return b, e
	}
	var assets []json.RawMessage
	if raw, ok := obj["assets"]; ok {
		_ = json.Unmarshal(raw, &assets)
	}
	for i, raw := range assets {
		path := fmt.Sprintf("assets.%d", i)
		aobj, er := knownKeys(raw, fieldNames(AssetIn{}), path)
		if er != nil {
			return b, er
		}
		a := AssetIn{Kind: "image", Sizing: "match"}
		if er = json.Unmarshal(raw, &a); er != nil {
			return b, malformed(path, er.Error())
		}
		for _, k := range []string{"kind", "sizing"} {
			if v, ok := aobj[k]; ok && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
				return b, malformed(path+"."+k, "Input cannot be null")
			}
		}
		if !enum(a.Kind, []string{"image", "video", "audio"}) {
			return b, malformed(path+".kind", "Input should be image, video or audio")
		}
		if !enum(a.Sizing, sizing) {
			return b, malformed(path+".sizing", "Input should be match or max")
		}
		if e = nullableObject(a.Provenance, path+".provenance"); e != nil {
			return b, e
		}
		b.Assets[i] = a
	}
	var dialogue []json.RawMessage
	if raw, ok := obj["dialogue"]; ok {
		_ = json.Unmarshal(raw, &dialogue)
	}
	for i, raw := range dialogue {
		path := fmt.Sprintf("dialogue.%d", i)
		dobj, er := knownKeys(raw, fieldNames(DialogueIn{}), path)
		if er != nil {
			return b, er
		}
		if er = required(dobj, "text", path); er != nil {
			return b, er
		}
		d := DialogueIn{Language: "English"}
		if er = json.Unmarshal(raw, &d); er != nil {
			return b, malformed(path, er.Error())
		}
		for _, k := range []string{"language", "voiceover"} {
			if v, ok := dobj[k]; ok && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
				return b, malformed(path+"."+k, "Input cannot be null")
			}
		}
		b.Dialogue[i] = d
	}
	for i, raw := range b.LoRAs {
		if er := nullableObject(raw, fmt.Sprintf("loras.%d", i)); er != nil {
			return b, er
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return b, malformed(fmt.Sprintf("loras.%d", i), "Input should be a JSON object")
		}
	}
	if len(b.Answer) > 0 && (len(b.Assets) == 0 || b.Assets[0].Kind != "image") {
		return b, malformed("answer", "Clarifying an image role requires an attached first image")
	}
	return b, nil
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func hashPath(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", refusal(422, "asset-missing", "no such file: "+path)
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() {
		return "", refusal(422, "asset-unreadable", "attachment is not a readable regular file")
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", refusal(422, "asset-unreadable", "could not read the attachment")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (s *Service) source(a AssetIn) (string, string, error) {
	p, d := value(a.Path), value(a.SHA256)
	if p != "" && d != "" {
		return "", "", refusal(422, "asset-two-sources", "this asset gives both a path and an uploaded sha256. They can name different files, so pick one.")
	}
	if d != "" {
		p, ok, e := s.cfg.Store.Stored(d)
		if e != nil {
			return "", "", e
		}
		if !ok {
			return "", "", &Error{Status: 422, Code: "asset-not-uploaded", Message: "the upload was dropped between checks. Send the bytes to PUT /v1/assets/{sha256} and ask again.", Missing: []string{d}}
		}
		if e = s.cfg.Store.Touch(p); e != nil {
			return "", "", e
		}
		return p, d, nil
	}
	if p == "" {
		return "", "", refusal(422, "asset-no-path", "each asset needs either a `path` this service can open or the `sha256` of an attachment already uploaded to it. URL fetching is not enabled on this deployment.")
	}
	if !s.cfg.AllowPaths {
		return "", "", refusal(422, "asset-paths-disabled", "this service does not open files from its own filesystem. Upload the attachment with PUT /v1/assets/{sha256} and name it by that sha256.")
	}
	sha, e := hashPath(p)
	return p, sha, e
}

// PrepareBrief is the exact conversion used by POST /v1/briefs. An in-process
// caller receives the same unknown-field, upload and filesystem refusals.
func (s *Service) PrepareBrief(payload []byte) (plan.Brief, error) {
	var out plan.Brief
	if int64(len(payload)) > s.cfg.MaxRequestBytes {
		return out, malformed("the request", "Request exceeds the configured byte limit")
	}
	b, e := ParseBrief(payload)
	if e != nil {
		return out, e
	}
	var wanted []string
	for _, a := range b.Assets {
		for _, d := range []string{value(a.SHA256), value(a.PairedVideoSHA256)} {
			if d != "" {
				if !uploads.IsDigest(d) {
					return out, refusal(422, "asset-name-not-a-digest", "An upload is named by the SHA-256 of its own bytes, as 64 lowercase hexadecimal characters.")
				}
				wanted = append(wanted, d)
			}
		}
	}
	if len(wanted) > 0 && s.cfg.Store == nil {
		return out, &Error{Status: 422, Code: "asset-not-uploaded", Message: "No upload store was configured. Supply the store holding these digests, or use explicitly authorized local paths.", Missing: wanted}
	}
	missing := []string{}
	if len(wanted) > 0 {
		missing, e = s.cfg.Store.Missing(wanted)
	}
	if e != nil {
		return out, e
	}
	if len(missing) > 0 {
		return out, &Error{Status: 422, Code: "asset-not-uploaded", Message: fmt.Sprintf("this service does not hold %d of the attachments this request names. Send the bytes to PUT /v1/assets/{sha256} and ask again. An upload is kept for a while and then dropped, so one that worked an hour ago can need sending a second time.", len(missing)), Missing: missing}
	}
	out = plan.Brief{Intent: b.Intent, Seconds: b.Seconds, Aspect: b.Aspect, Megapixels: b.Megapixels, Assets: []plan.AssetRef{}, Dialogue: []plan.DialogueLine{}, OnscreenText: b.OnscreenText, Shots: b.Shots, LoRAs: b.LoRAs, Constraints: b.Constraints, Silent: b.Silent, Creativity: b.Creativity, Director: b.Director, DirectorProfile: b.DirectorProfile, Effort: b.Effort, Seed: b.Seed, Transcripts: b.Transcripts, AudioCopyModes: b.AudioCopyModes, TranscriptUses: b.TranscriptUses, SubjectProjections: b.SubjectProjections}
	for _, a := range b.Assets {
		p, sha, er := s.source(a)
		if er != nil {
			return out, er
		}
		role := value(a.Role)
		stated := false
		if role != "" {
			for _, rs := range rolesByKind {
				if enum(role, rs) {
					stated = true
				}
			}
		}
		if !stated && strings.TrimSpace(role) != "" {
			return out, refusal(422, "unknown-role", fmt.Sprintf("%q is not a role. For %s the roles are: %s. Omit `role` to have it inferred from the request", role, a.Kind, strings.Join(rolesByKind[a.Kind], ", ")))
		}
		if !stated {
			role = map[string]string{"image": "subject", "video": "edit_source", "audio": "bgm"}[a.Kind]
		}
		paired := a.PairedVideoSHA256
		if value(paired) == "" {
			paired = nil
			if path := value(a.PairedVideoPath); path != "" {
				if !s.cfg.AllowPaths {
					return out, refusal(422, "asset-paths-disabled", "this service does not open files from its own filesystem, and `paired_video_path` names one. Upload the video and use `paired_video_sha256` instead.")
				}
				digest, er := hashPath(path)
				if er != nil {
					if detail, ok := er.(*Error); ok && detail.Code == "asset-missing" {
						detail.Message += " (given as the video this soundtrack is paired with)"
					}
					return out, er
				}
				paired = &digest
			}
		}
		out.Assets = append(out.Assets, plan.AssetRef{Kind: a.Kind, Role: role, SHA256: sha, Path: &p, Note: a.Note, Seconds: a.Seconds, Frames: a.Frames, Sizing: a.Sizing, Composition: "unknown", Provenance: a.Provenance, PairedVideoSHA256: paired, Replaces: strings.TrimSpace(value(a.Replaces)), RoleStated: stated})
	}
	for id := range b.AudioCopyModes {
		valid := false
		for i, a := range out.Assets {
			if id == fmt.Sprintf("asset-%d", i+1) && a.Kind == "audio" && a.Role == "bgm" {
				valid = true
			}
		}
		if !valid {
			return out, malformed("audio_copy_modes", "Copy choices must identify an attached BGM audio as asset-1, asset-2, and so on")
		}
	}
	for _, use := range b.TranscriptUses {
		valid := false
		for i, asset := range out.Assets {
			if use.AssetID == fmt.Sprintf("asset-%d", i+1) && asset.Kind == "audio" {
				valid = true
			}
		}
		if !valid {
			return out, malformed("transcript_uses.asset_id", "A transcript use must identify an attached audio as an exact asset-N ID")
		}
	}
	for _, d := range b.Dialogue {
		out.Dialogue = append(out.Dialogue, plan.DialogueLine{Text: d.Text, Language: d.Language, SpeakerHint: d.Speaker, Voiceover: d.Voiceover})
	}
	if len(out.Assets) > 0 {
		switch b.Answer["anchor_or_reference"] {
		case "opening_frame":
			out.Assets[0].Role = "frame_anchor_first"
			out.Assets[0].RoleStated = true
		case "appears_in":
			out.Assets[0].Role = "subject"
			out.Assets[0].RoleStated = true
		}
	}
	return out, nil
}
