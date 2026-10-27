package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/examples/playground/internal/native"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/media"
	"github.com/ly95/h3-context-ir-go/planner"
)

const playgroundNativeAnalyzer = "contextir-playground-native/v1"

type nativeMediaModel interface {
	Generate(context.Context, contextir.ModelRequest) (contextir.ModelResponse, error)
	GenerateMedia(context.Context, contextir.ModelRequest, string, []byte) (contextir.ModelResponse, error)
}

type nativeImageModel interface {
	GenerateWithImages(context.Context, contextir.ModelRequest, []contextir.ImageInput) (contextir.ModelResponse, error)
}

type observedNativeModel struct {
	inner             nativeMediaModel
	notify            func(stageEvent)
	mu                sync.Mutex
	attempts          map[string]int
	audioRefOnly      bool
	transcriptAllowed map[string]bool
}

func (m *observedNativeModel) Generate(ctx context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
	return m.generate(ctx, request, "", nil, false)
}

func (m *observedNativeModel) GenerateMedia(ctx context.Context, request contextir.ModelRequest, mime string, data []byte) (contextir.ModelResponse, error) {
	return m.generate(ctx, request, mime, data, true)
}

func (m *observedNativeModel) generate(ctx context.Context, request contextir.ModelRequest, mime string, data []byte, mediaCall bool) (contextir.ModelResponse, error) {
	guardTranscript := (m.audioRefOnly || len(m.transcriptAllowed) > 0) && (request.Stage == "scope" || request.Stage == "structure")
	if guardTranscript {
		permissions, _ := json.Marshal(m.transcriptAllowed)
		request.System += "\nHost caller contract: source transcript observations never authorize speech. Per-asset transcript selection permissions (true permits the caller-selected transcript_voiceover, false is reference_only): " + string(permissions) + ". transcript_uses may name only an asset with permission true. The observed source transcript remains evidence for reference_only attachments, but cannot be imported into target dialogue. Caller-supplied independent dialogue remains permitted under its own explicit contract."
		if m.audioRefOnly {
			request.System += " All source attachments are reference_only; transcript_uses must be an empty array."
		}
	}
	assetID := perceptionRequestAssetID(request)
	key := request.Stage + "/" + assetID
	m.mu.Lock()
	m.attempts[key]++
	attempt := m.attempts[key]
	m.mu.Unlock()
	m.emit(stageEvent{Stage: request.Stage, Attempt: attempt, Phase: "started", AssetID: assetID})
	var response contextir.ModelResponse
	var err error
	if !mediaCall {
		response, err = m.inner.Generate(ctx, request)
	} else if strings.HasPrefix(mime, "image/") {
		imageModel, ok := m.inner.(nativeImageModel)
		if !ok {
			err = nativePipelineError(contextir.CodeUnsupported, "native_perception", "image", "The configured native model does not accept original images.")
		} else {
			response, err = imageModel.GenerateWithImages(ctx, request, []contextir.ImageInput{{URI: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), Detail: "original"}})
		}
	} else {
		response, err = m.inner.GenerateMedia(ctx, request, mime, data)
	}
	if err == nil && guardTranscript {
		var selected struct {
			TranscriptUses []compiler.TranscriptUse `json:"transcript_uses"`
		}
		if json.Unmarshal(response.JSON, &selected) == nil {
			for _, use := range selected.TranscriptUses {
				if m.audioRefOnly || !m.transcriptAllowed[use.AssetID] {
					err = nativePipelineError(contextir.CodeModelOutput, request.Stage, "transcript_uses", "Selected source words require transcript_voiceover permission for that exact attachment.")
					break
				}
			}
		}
	}
	m.emit(stageEvent{Stage: request.Stage, Attempt: attempt, Phase: "completed", Model: response.Model, InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, Valid: nativeUsageValid(response), Failed: err != nil, AssetID: assetID})
	return response, err
}

func (m *observedNativeModel) emit(event stageEvent) {
	if m.notify != nil {
		m.notify(event)
	}
}

type playgroundNativeAsset struct {
	asset   contextir.Asset
	info    media.Info
	bytes   []byte
	mime    string
	cleanup func()
}

type nativeFrameProgress struct {
	line     []byte
	frames   int64
	complete bool
}

func (p *nativeFrameProgress) Write(data []byte) (int, error) {
	for _, value := range data {
		if value != '\n' {
			if len(p.line) >= 4096 {
				return 0, fmt.Errorf("media progress line exceeds its explicit limit")
			}
			p.line = append(p.line, value)
			continue
		}
		line := string(p.line)
		p.line = p.line[:0]
		if raw, ok := strings.CutPrefix(line, "frame="); ok {
			frames, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			if err != nil || frames < 0 {
				return 0, fmt.Errorf("media progress returned an invalid frame count")
			}
			p.frames = frames
		}
		if line == "progress=end" {
			p.complete = true
		}
	}
	return len(data), nil
}

func nativePipelineError(code contextir.Code, stage, path, message string) error {
	return &contextir.Error{Code: code, Stage: stage, Issues: []contextir.Issue{{Code: code, Path: path, Message: message}}}
}

func nativeEBMLWidth(first byte, limit int) int {
	for width := 1; width <= limit; width++ {
		if first&(0x80>>uint(width-1)) != 0 {
			return width
		}
	}
	return 0
}

func nativeEBMLSize(data []byte) (uint64, int, bool) {
	if len(data) == 0 {
		return 0, 0, false
	}
	width := nativeEBMLWidth(data[0], 8)
	if width == 0 || width > len(data) {
		return 0, 0, false
	}
	value := uint64(data[0] & byte(0xff>>uint(width)))
	for _, part := range data[1:width] {
		value = value<<8 | uint64(part)
	}
	if value == uint64(1)<<uint(7*width)-1 {
		return 0, 0, false
	}
	return value, width, true
}

func nativeWebMHeader(data []byte) bool {
	if !bytes.HasPrefix(data, []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return false
	}
	size, width, ok := nativeEBMLSize(data[4:])
	if !ok || size > uint64(len(data)-4-width) {
		return false
	}
	offset := 4 + width
	end := offset + int(size)
	found := false
	for offset < end {
		idWidth := nativeEBMLWidth(data[offset], 4)
		if idWidth == 0 || idWidth > end-offset {
			return false
		}
		id := data[offset : offset+idWidth]
		offset += idWidth
		size, width, ok := nativeEBMLSize(data[offset:end])
		if !ok || size > uint64(end-offset-width) {
			return false
		}
		offset += width
		if bytes.Equal(id, []byte{0x42, 0x82}) {
			if found || !bytes.Equal(data[offset:offset+int(size)], []byte("webm")) {
				return false
			}
			found = true
		}
		offset += int(size)
	}
	return found
}

func prepareNativeAsset(ctx context.Context, input compileInput) (*playgroundNativeAsset, error) {
	if input.image != nil && input.media != nil {
		return nil, nativePipelineError(contextir.CodeInvalid, "media_validation", "media", "Supply exactly one image, video or audio source.")
	}
	if input.image == nil && input.media == nil {
		return nil, nil
	}
	var data []byte
	var mime string
	var kind contextir.MediaKind
	var metadata contextir.MediaMetadata
	if input.image != nil {
		decoded, err := decodeImage(input.image.bytes)
		if err != nil || decoded.mime != input.image.mime || decoded.width != input.image.width || decoded.height != input.image.height {
			return nil, nativePipelineError(contextir.CodeInvalid, "media_validation", "image", "The uploaded image does not match its complete decoded bytes and metadata.")
		}
		data, mime, kind = input.image.bytes, decoded.mime, contextir.MediaImage
		metadata = contextir.MediaMetadata{Width: decoded.width, Height: decoded.height}
	} else {
		data, mime, kind = input.media.bytes, input.media.mime, input.media.kind
		if len(data) == 0 || kind != contextir.MediaVideo && kind != contextir.MediaAudio {
			return nil, nativePipelineError(contextir.CodeInvalid, "media_validation", "media", "A nonempty video or audio source is required.")
		}
		if mime == "video/webm" && !nativeWebMHeader(data) {
			return nil, nativePipelineError(contextir.CodeInvalid, "media_validation", "media.container", "WebM media requires its actual WebM EBML DocType; other Matroska containers are unsupported.")
		}
	}
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	source := &playgroundNativeAsset{asset: contextir.Asset{ID: "reference-media", Kind: kind, SHA256: hash, URI: "upload://" + hash, Metadata: metadata}, bytes: append([]byte(nil), data...), mime: mime, cleanup: func() {}}
	if kind == contextir.MediaImage {
		source.info = media.Info{Kind: kind, Metadata: metadata}
		return source, nil
	}
	tools, err := media.NewTools(media.Options{FFprobe: input.ffprobe, FFmpeg: input.ffmpeg, MaxInputBytes: int64(len(data))})
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "contextir-media-*")
	if err != nil {
		return nil, nativePipelineError(contextir.CodeUnsupported, "media_validation", "temporary_file", "A private temporary media file could not be created.")
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	source.cleanup = cleanup
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		cleanup()
		return nil, nativePipelineError(contextir.CodeUnsupported, "media_validation", "temporary_file", "The private temporary media file could not be written.")
	}
	validationCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	info, inspectErr := tools.Inspect(validationCtx, path)
	if inspectErr != nil || info.Kind != kind || info.Metadata.Duration <= 0 || kind == contextir.MediaAudio && !info.HasAudio {
		cleanup()
		return nil, nativeValidationFailure(validationCtx, "The complete uploaded file must contain the declared video or audio stream and a measured duration.")
	}
	args := []string{"-nostdin", "-v", "error", "-xerror", "-err_detect", "explode", "-protocol_whitelist", "file,pipe", "-i", path, "-map", "0:V?", "-map", "0:a?"}
	progress := &nativeFrameProgress{}
	if kind == contextir.MediaVideo {
		args = append(args, "-fps_mode", "passthrough", "-progress", "pipe:1", "-nostats")
	}
	args = append(args, "-f", "null", "-")
	command := exec.CommandContext(validationCtx, input.ffmpeg, args...)
	command.Stdout, command.Stderr = progress, io.Discard
	if command.Run() != nil {
		cleanup()
		return nil, nativeValidationFailure(validationCtx, "The complete uploaded media could not be decoded without errors.")
	}
	if kind == contextir.MediaVideo {
		if !progress.complete || progress.frames <= 0 {
			cleanup()
			return nil, nativeValidationFailure(validationCtx, "The complete uploaded video must establish its actual decoded frame count.")
		}
		info.Frames = progress.frames
		if info.Frames < 5 {
			cleanup()
			return nil, nativePipelineError(contextir.CodeLimit, "media_validation", "media.frames", "H3 reference videos require at least five measured source frames.")
		}
	}
	source.info, source.asset.Metadata = info, info.Metadata
	return source, nil
}

func nativeValidationFailure(ctx context.Context, message string) error {
	if ctx.Err() != nil {
		return nativePipelineError(contextir.CodeCanceled, "media_validation", "media", "Media validation was canceled or exceeded its time limit.")
	}
	return nativePipelineError(contextir.CodeInvalid, "media_validation", "media", message)
}

type nativeObservationPerceiver struct {
	model     nativeMediaModel
	source    *playgroundNativeAsset
	evidence  contextir.Evidence
	calls     []contextir.CallUsage
	observed  bool
	delivered bool
	err       error
}

func (p *nativeObservationPerceiver) InspectAsset(_ context.Context, asset contextir.Asset) (contextir.MediaMetadata, error) {
	if !p.matches(asset) {
		return contextir.MediaMetadata{}, nativePipelineError(contextir.CodeInvalid, "media_validation", "asset", "The inspector received a different source.")
	}
	return p.source.asset.Metadata, nil
}

func (p *nativeObservationPerceiver) matches(asset contextir.Asset) bool {
	return p.source != nil && asset.ID == p.source.asset.ID && asset.SHA256 == p.source.asset.SHA256 && asset.URI == p.source.asset.URI && asset.Kind == p.source.asset.Kind
}

func (p *nativeObservationPerceiver) Perceive(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
	evidence, _, err := p.PerceiveWithCalls(ctx, asset)
	return evidence, err
}

func (p *nativeObservationPerceiver) PerceiveWithCalls(ctx context.Context, asset contextir.Asset) (contextir.Evidence, []contextir.CallUsage, error) {
	if !p.matches(asset) || p.delivered {
		return contextir.Evidence{}, nil, nativePipelineError(contextir.CodeInvalid, "native_perception", "asset", "The same-run observation and its actual call ledger must be delivered once for the original source.")
	}
	p.delivered = true
	if !p.observed {
		p.observe(ctx)
	}
	return p.evidence, append([]contextir.CallUsage(nil), p.calls...), p.err
}

func (p *nativeObservationPerceiver) observe(ctx context.Context) error {
	if p.observed {
		return p.err
	}
	p.observed = true
	asset := p.source.asset
	stage := string(asset.Kind) + "_perception"
	scope := "visual"
	system := `Observe only the original attached media bytes. Return one Evidence object matching the complete schema, including card and at least one grounded Fact. No target intent, filename, expected answer or reference role is supplied. Describe observations, not a target performance. Copy clearly readable visible words and intelligible speech exactly, preserving original language. Distinguish observed facts from uncertainty in limitations. Analyzer/model/coverage metadata, source intervals, sample times and frame counts are host-owned: leave them unspecified or use empty required placeholders. Do not invent timestamps or exact coverage. Every fact must name the anonymous attachment asset_id and the actually observed visual or audio track. Do not infer unseen content, identity or provenance.`
	switch asset.Kind {
	case contextir.MediaImage:
		system += ` Inspect this single image only. Report visual appearance, subjects, setting, layout, legible words and uncertainty. Do not claim motion across time, transcript, spoken language, timbre, music or acoustic characterisation.`
	case contextir.MediaVideo:
		system += ` Inspect the original video. Describe actually observed action order, transitions, visible words and camera changes without guessing unseen intervals or exact camera technique.`
		if p.source.info.HasAudio {
			scope = "visual_and_audio"
			system += ` The host measured an audio stream in this container. Inspect that stream too when supported; report intelligible original speech and language, voice/timbre, music, rhythm and acoustic texture only when actually audible. If any audio property cannot be established, leave it empty and state the limitation. An audio stream's existence is not evidence of music, speech or silence.`
		} else {
			system += ` The host measured no audio stream. Do not report transcript, spoken language, timbre, music or acoustic characterisation.`
		}
	case contextir.MediaAudio:
		scope = "audio"
		system += ` Inspect this original audio. Transcribe intelligible speech exactly into card.transcript and its original language into card.language. Describe actually audible voice/timbre, delivery, music, rhythm and acoustic texture in the corresponding audio fields; leave unknown or absent properties empty and explain uncertainty. Do not infer speaker identity, visual subjects, scene, lighting, framing, motion or visible words from speech.`
	}
	input, _ := json.Marshal(map[string]any{"asset_id": asset.ID, "media_kind": asset.Kind, "observation_scope": scope})
	response, err := p.model.GenerateMedia(ctx, contextir.ModelRequest{Stage: stage, System: system, Input: input, ResponseSchema: contextir.EvidenceSchema()}, p.source.mime, p.source.bytes)
	p.calls = []contextir.CallUsage{{AssetID: asset.ID, Stage: stage, Attempt: 1, Model: response.Model, Usage: response.Usage, Valid: nativeUsageValid(response)}}
	if err != nil {
		p.err = err
		return err
	}
	if !nativeUsageValid(response) {
		p.err = nativePipelineError(contextir.CodeModelOutput, "native_perception", "usage", "Native perception must preserve valid actual usage.")
		return p.err
	}
	decoder := json.NewDecoder(bytes.NewReader(response.JSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p.evidence); err != nil || decoder.Decode(new(any)) != io.EOF || p.evidence.Card == nil || len(p.evidence.Facts) == 0 {
		p.err = nativePipelineError(contextir.CodeModelOutput, "native_perception", "evidence", "Native perception must return one complete Evidence object with an observation card and grounded facts.")
		return p.err
	}
	card := p.evidence.Card
	if p.evidence.AssetID != asset.ID {
		p.err = nativePipelineError(contextir.CodeModelOutput, "native_perception", "evidence.asset_id", "The observation must identify the exact uploaded source.")
		return p.err
	}
	if card.AssetID != asset.ID {
		p.err = nativePipelineError(contextir.CodeModelOutput, "native_perception", "evidence.card.asset_id", "The observation card must identify the exact uploaded source.")
		return p.err
	}
	visualOnly := asset.Kind == contextir.MediaImage || asset.Kind == contextir.MediaVideo && !p.source.info.HasAudio
	if visualOnly && (card.Transcript != "" || card.Language != "" || card.Timbre != "" || card.Music != "" || card.Characterisation != "") || asset.Kind == contextir.MediaAudio && nativeCardHasVisuals(*card) {
		p.err = nativePipelineError(contextir.CodeModelOutput, "native_perception", "evidence.card", "Native observations must remain within the source's measured tracks.")
		return p.err
	}
	tracks := []contextir.Track{}
	for i := range p.evidence.Facts {
		fact := &p.evidence.Facts[i]
		if fact.Source.AssetID != asset.ID {
			p.err = nativePipelineError(contextir.CodeModelOutput, "native_perception", fmt.Sprintf("evidence.facts[%d].source.asset_id", i), "Each fact must identify the exact uploaded source.")
			return p.err
		}
		track := fact.Source.Track
		if fact.Source.Interval != nil || track != contextir.TrackVisual && track != contextir.TrackAudio || asset.Kind == contextir.MediaAudio && track != contextir.TrackAudio || visualOnly && track != contextir.TrackVisual {
			p.err = nativePipelineError(contextir.CodeModelOutput, "native_perception", "evidence.facts.source", "Facts must identify an available observed track without invented temporal bounds.")
			return p.err
		}
		if !slices.Contains(tracks, track) {
			tracks = append(tracks, track)
		}
		fact.ID, fact.AnalyzerVersion = fmt.Sprintf("%s.native-%d", asset.ID, i+1), playgroundNativeAnalyzer
		fact.Source = contextir.SourceRef{AssetID: asset.ID, Track: track}
	}
	if asset.Kind != contextir.MediaAudio && !slices.Contains(tracks, contextir.TrackVisual) || (card.Transcript != "" || card.Language != "" || card.Timbre != "" || card.Music != "" || card.Characterisation != "") && !slices.Contains(tracks, contextir.TrackAudio) {
		p.err = nativePipelineError(contextir.CodeModelOutput, "native_perception", "evidence.coverage", "Each observed modality must have grounded facts in its actual track.")
		return p.err
	}
	coverage := &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: tracks}
	if asset.Kind == contextir.MediaVideo && !p.source.info.HasAudio {
		coverage.AbsentTracks = []contextir.Track{contextir.TrackAudio}
	}
	p.evidence.AssetID, p.evidence.Model, p.evidence.AnalyzerVersion, p.evidence.Coverage = asset.ID, response.Model, playgroundNativeAnalyzer, coverage
	card.AssetID, card.Model, card.AnalyzerVersion, card.Coverage, card.FramesSeen = asset.ID, response.Model, playgroundNativeAnalyzer, coverage, 0
	p.evidence.Limitations = append(p.evidence.Limitations, "The original uploaded bytes were sent to the native model. Native sampling and exact temporal coverage are unknown; no extracted frames, observed frame count or invented sample times are claimed. Empty acoustic properties do not establish silence.")
	if asset.Kind == contextir.MediaVideo && p.source.info.HasAudio && !slices.Contains(tracks, contextir.TrackAudio) {
		p.evidence.Limitations = append(p.evidence.Limitations, "The host measured an audio stream, but this native response established no grounded audio observations. Its speech, music and acoustic properties remain unknown.")
	}
	p.err = contextir.ValidateEvidence([]contextir.Asset{asset}, []contextir.Evidence{p.evidence})
	return p.err
}

func nativeCardHasVisuals(card contextir.AssetCard) bool {
	return len(card.Subjects) > 0 || card.Environment != "" || card.Lighting != "" || len(card.Palette) > 0 || card.Framing != "" || card.Style != "" || len(card.VisibleText) > 0 || card.Motion != "" || card.Camera != "" || card.IsReferenceSheet || card.Composition != "" && card.Composition != "unknown"
}

func nativeUsageValid(response contextir.ModelResponse) bool {
	return !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
}

func compileNativeBrief(input compileInput, source *playgroundNativeAsset, evidence contextir.Evidence) (compiler.Brief, error) {
	brief := baseBrief(input)
	if source == nil {
		if input.audioUse != "" && input.audioUse != "reference_only" {
			return brief, nativePipelineError(contextir.CodeInvalid, "native_input", "audio_use", "Transcript voiceover requires an uploaded audio source.")
		}
		return brief, nil
	}
	brief.Mode = contextir.ModeReference
	if source.asset.Kind == contextir.MediaImage && input.role == compiler.RoleFirstFrame {
		brief.Mode = contextir.ModeFirstFrame
	}
	brief.Assets = []compiler.Asset{{Asset: source.asset, Role: input.role, Frames: source.info.Frames}}
	if input.audioUse != "" && input.audioUse != "reference_only" && input.audioUse != "transcript_voiceover" {
		return brief, nativePipelineError(contextir.CodeInvalid, "native_input", "audio_use", "Choose reference_only or transcript_voiceover explicitly.")
	}
	if input.audioUse != "transcript_voiceover" {
		return brief, nil
	}
	if source.asset.Kind != contextir.MediaAudio || input.role != compiler.RoleVoiceTimbre || input.noDialogue || input.silence {
		return brief, nativePipelineError(contextir.CodeInvalid, "native_input", "audio_use", "Transcript voiceover requires a voice_timbre audio source with dialogue enabled and full silence disabled.")
	}
	if evidence.Card == nil || strings.TrimSpace(evidence.Card.Transcript) == "" || strings.TrimSpace(evidence.Card.Language) == "" {
		return brief, nativePipelineError(contextir.CodePerception, "native_perception", "evidence.card.transcript", "Transcript voiceover requires actual recognized speech and its original language.")
	}
	brief.Transcripts = map[string]string{source.asset.ID: evidence.Card.Transcript}
	brief.TranscriptUses = []compiler.TranscriptUse{{AssetID: source.asset.ID, Text: evidence.Card.Transcript, Language: evidence.Card.Language, SpeakerHint: "the off-screen narrator", Voiceover: true}}
	return brief, nil
}

func nativeEarlyResult(evidence contextir.Evidence, calls []contextir.CallUsage, err error) compiler.Result {
	result := compiler.Result{Status: "failed", Calls: append([]contextir.CallUsage(nil), calls...), UsageComplete: true, ModelError: err}
	if evidence.AssetID != "" {
		result.Evidence = []contextir.Evidence{evidence}
	}
	for _, call := range calls {
		result.UsageComplete = result.UsageComplete && call.Valid && !call.Unmetered
	}
	return result
}

func prepareNativeReferences(ctx context.Context, input compileInput, notify func(stageEvent)) ([]*playgroundNativeAsset, error) {
	refs, err := validatedInputReferences(input)
	if err != nil {
		return nil, err
	}
	sources := make([]*playgroundNativeAsset, 0, len(refs))
	seen := map[string]bool{}
	cleanup := func() {
		for _, source := range sources {
			source.cleanup()
		}
	}
	for _, ref := range refs {
		if ref.id == "" || seen[ref.id] {
			cleanup()
			return nil, nativePipelineError(contextir.CodeInvalid, "media_validation", "references.id", "Every reference requires a distinct identifier.")
		}
		seen[ref.id] = true
		if ref.audioUse != "" && ref.audioUse != "reference_only" && ref.audioUse != "transcript_voiceover" {
			cleanup()
			return nil, nativePipelineError(contextir.CodeInvalid, "native_input", "references.audio_use", "Choose reference_only or transcript_voiceover for each source.")
		}
		if ref.audioUse == "transcript_voiceover" && (ref.media == nil || ref.media.kind != contextir.MediaAudio || ref.role != compiler.RoleVoiceTimbre || input.noDialogue || input.silence) {
			cleanup()
			return nil, nativePipelineError(contextir.CodeInvalid, "native_input", "references.audio_use", "Each transcript voiceover requires its own voice_timbre audio source with dialogue enabled and full silence disabled.")
		}
		one := input
		one.image, one.media, one.role, one.audioUse = ref.image, ref.media, ref.role, ref.audioUse
		if notify != nil {
			notify(stageEvent{Stage: "media_validation", Attempt: 1, Phase: "started", Local: true, AssetID: ref.id})
		}
		source, err := prepareNativeAsset(ctx, one)
		if notify != nil {
			notify(stageEvent{Stage: "media_validation", Attempt: 1, Phase: "completed", Local: true, Valid: err == nil, Failed: err != nil, AssetID: ref.id})
		}
		if err != nil {
			cleanup()
			return nil, err
		}
		if source == nil {
			cleanup()
			return nil, nativePipelineError(contextir.CodeInvalid, "media_validation", "references", "Each declared reference requires its own uploaded media.")
		}
		source.asset.ID = ref.id
		source.asset.URI = "upload://" + ref.id + "/" + source.asset.SHA256
		sources = append(sources, source)
	}
	assets := make([]compiler.Asset, 0, len(sources))
	for i, source := range sources {
		assets = append(assets, compiler.Asset{Asset: source.asset, Role: refs[i].role, Frames: source.info.Frames})
	}
	if _, err := buildReferenceBrief(input, assets); err != nil {
		cleanup()
		return nil, err
	}
	return sources, nil
}

type nativeBatchPerceiver struct {
	order []string
	byID  map[string]*nativeObservationPerceiver
}

func newNativeBatch(model nativeMediaModel, sources []*playgroundNativeAsset) *nativeBatchPerceiver {
	batch := &nativeBatchPerceiver{byID: map[string]*nativeObservationPerceiver{}}
	for _, source := range sources {
		batch.order = append(batch.order, source.asset.ID)
		batch.byID[source.asset.ID] = &nativeObservationPerceiver{model: model, source: source}
	}
	return batch
}

func (p *nativeBatchPerceiver) InspectAsset(ctx context.Context, asset contextir.Asset) (contextir.MediaMetadata, error) {
	if child := p.byID[asset.ID]; child != nil {
		return child.InspectAsset(ctx, asset)
	}
	return contextir.MediaMetadata{}, nativePipelineError(contextir.CodeInvalid, "media_validation", "asset", "The inspector received an unknown reference.")
}

func (p *nativeBatchPerceiver) Perceive(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
	evidence, _, err := p.PerceiveWithCalls(ctx, asset)
	return evidence, err
}

func (p *nativeBatchPerceiver) PerceiveWithCalls(ctx context.Context, asset contextir.Asset) (contextir.Evidence, []contextir.CallUsage, error) {
	if child := p.byID[asset.ID]; child != nil {
		return child.PerceiveWithCalls(ctx, asset)
	}
	return contextir.Evidence{}, nil, nativePipelineError(contextir.CodeInvalid, "native_perception", "asset", "The perceiver received an unknown reference.")
}

func (p *nativeBatchPerceiver) observe(ctx context.Context) error {
	for _, id := range p.order {
		if err := p.byID[id].observe(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (p *nativeBatchPerceiver) retain(result compiler.Result) compiler.Result {
	paid := []contextir.CallUsage{}
	known := map[contextir.CallUsage]int{}
	evidenceByID := map[string]bool{}
	for _, evidence := range result.Evidence {
		evidenceByID[evidence.AssetID] = true
	}
	for _, id := range p.order {
		child := p.byID[id]
		for _, call := range child.calls {
			paid = append(paid, call)
			known[call]++
		}
		if child.evidence.AssetID != "" && !evidenceByID[child.evidence.AssetID] {
			result.Evidence = append(result.Evidence, child.evidence)
			evidenceByID[child.evidence.AssetID] = true
		}
	}
	for _, call := range result.Calls {
		if known[call] > 0 {
			known[call]--
			continue
		}
		paid = append(paid, call)
	}
	result.Calls = paid
	result.UsageComplete = true
	for _, call := range paid {
		result.UsageComplete = result.UsageComplete && call.Valid && !call.Unmetered
	}
	return result
}

func compileNativeBatchBrief(input compileInput, sources []*playgroundNativeAsset, batch *nativeBatchPerceiver) (compiler.Brief, error) {
	refs := inputReferences(input)
	assets := make([]compiler.Asset, 0, len(sources))
	for i, source := range sources {
		assets = append(assets, compiler.Asset{Asset: source.asset, Role: refs[i].role, Frames: source.info.Frames})
	}
	brief, err := buildReferenceBrief(input, assets)
	if err != nil {
		return brief, err
	}
	voiceCount := 0
	for _, ref := range refs {
		if ref.role == compiler.RoleVoiceTimbre {
			voiceCount++
		}
	}
	for i, ref := range refs {
		if ref.audioUse != "transcript_voiceover" {
			continue
		}
		child := batch.byID[sources[i].asset.ID]
		if child == nil || child.evidence.Card == nil || strings.TrimSpace(child.evidence.Card.Transcript) == "" || strings.TrimSpace(child.evidence.Card.Language) == "" {
			return brief, nativePipelineError(contextir.CodePerception, "native_perception", "evidence.card.transcript", "Each selected voiceover requires actual recognized words and their original language from that exact audio source.")
		}
		card := child.evidence.Card
		if brief.Transcripts == nil {
			brief.Transcripts = map[string]string{}
		}
		brief.Transcripts[ref.id] = card.Transcript
		brief.TranscriptUses = append(brief.TranscriptUses, compiler.TranscriptUse{AssetID: ref.id, Text: card.Transcript, Language: card.Language, SpeakerHint: "the off-screen narrator for " + ref.id, Voiceover: true})
		if brief.VoiceBindings == nil {
			brief.VoiceBindings = map[string]string{}
		}
		brief.VoiceBindings[ref.id] = fmt.Sprintf("speaker-%d", len(brief.TranscriptUses))
	}
	if voiceCount > 1 {
		for _, ref := range refs {
			if ref.role != compiler.RoleVoiceTimbre || ref.audioUse == "transcript_voiceover" {
				continue
			}
			if len(brief.TranscriptUses) > 0 {
				return brief, nativePipelineError(contextir.CodeUnsupported, "native_input", "voice_bindings", "Multiple active and dormant voice references require an explicit target speaker mapping; this playground does not select a speaker on the caller's behalf.")
			}
			if brief.VoiceBindings == nil {
				brief.VoiceBindings = map[string]string{}
			}
			brief.VoiceBindings[ref.id] = "voice-" + ref.id
		}
	}
	return brief, nil
}

func compileNativePrompt(ctx context.Context, input compileInput, notify func(stageEvent)) (compiler.Result, error) {
	sources, err := prepareNativeReferences(ctx, input, notify)
	if err != nil {
		return nativeEarlyResult(contextir.Evidence{}, nil, err), err
	}
	for _, source := range sources {
		defer source.cleanup()
	}
	var session [16]byte
	if _, err := rand.Read(session[:]); err != nil {
		return compiler.Result{}, nativePipelineError(contextir.CodeUnsupported, "native_input", "session", "A private request session could not be created.")
	}
	clientHTTP := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: time.Minute}, Timeout: 180 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer clientHTTP.CloseIdleConnections()
	adapter, err := native.New(native.Options{Client: clientHTTP, BaseURL: input.baseURL, Model: input.model, APIKey: input.key, Session: "contextir-playground-" + hex.EncodeToString(session[:]), AllowedResponseModels: input.allowedModels})
	if err != nil {
		return nativeEarlyResult(contextir.Evidence{}, nil, err), err
	}
	permissions := map[string]bool{}
	for _, ref := range inputReferences(input) {
		if ref.media != nil && ref.media.kind == contextir.MediaAudio {
			permissions[ref.id] = ref.audioUse == "transcript_voiceover"
		}
	}
	model := &observedNativeModel{inner: adapter, notify: notify, attempts: map[string]int{}, transcriptAllowed: permissions}
	batch := newNativeBatch(model, sources)
	var plannerPerceiver planner.Perceiver
	if len(sources) > 0 {
		if err := batch.observe(ctx); err != nil {
			return batch.retain(nativeEarlyResult(contextir.Evidence{}, nil, err)), err
		}
		plannerPerceiver = batch
	}
	brief, err := compileNativeBatchBrief(input, sources, batch)
	if err != nil {
		return batch.retain(nativeEarlyResult(contextir.Evidence{}, nil, err)), err
	}
	instance, err := compiler.New(model, plannerPerceiver, compiler.Options{Profile: h3.OfficialProfile(), FramePolicy: compiler.FramePolicyExact, FailurePolicy: compiler.FailurePolicyFail, ClarificationPolicy: compiler.ClarificationRequire, MaxStructureRetries: 1, Planner: planner.Options{MaxRetries: 2, AllowPartialPerception: true}})
	if err != nil {
		return batch.retain(nativeEarlyResult(contextir.Evidence{}, nil, err)), err
	}
	result, err := instance.Compile(ctx, brief)
	return batch.retain(result), err
}
