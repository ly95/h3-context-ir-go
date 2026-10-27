package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/media"
	"github.com/ly95/h3-context-ir-go/planner"
)

const liveNativeVideoSHA256 = "4f04793f4ab19a618cf47bd8981496ad7df330c5beb148374ba78ccfceda2fbe"
const liveNativeAudioSHA256 = "63a4b1e4c1dc655ac70961ffbf518acd249df237e5a0152faae9a4a836949715"
const liveNativeExpectedSpeech = "And so my fellow Americans, ask not what your country can do for you, ask what you can do for your country."
const liveNativeObservationVersion = "contextir-native-original-media-acceptance/v1"

type liveNativePerceiver struct {
	model      *liveMediaContractModel
	tools      *media.Tools
	asset      contextir.Asset
	path       string
	mime       string
	data       []byte
	evidence   contextir.Evidence
	calls      []contextir.CallUsage
	observeErr error
	observed   bool
	delivered  int
}

func (p *liveNativePerceiver) InspectAsset(ctx context.Context, asset contextir.Asset) (contextir.MediaMetadata, error) {
	if asset.ID != p.asset.ID || asset.Kind != p.asset.Kind || asset.SHA256 != p.asset.SHA256 || asset.URI != p.asset.URI {
		return contextir.MediaMetadata{}, liveMediaContractError("asset", "The native inspector received a different source.")
	}
	data, err := os.ReadFile(p.path)
	if err != nil || liveReferenceHash(data) != p.asset.SHA256 {
		return contextir.MediaMetadata{}, liveMediaContractError("source_bytes", "The fixed source bytes changed before inspection.")
	}
	info, err := p.tools.Inspect(ctx, p.path)
	if err != nil {
		return contextir.MediaMetadata{}, err
	}
	if info.Kind != asset.Kind {
		return contextir.MediaMetadata{}, liveMediaContractError("source_kind", "The measured source kind differs from its declared modality.")
	}
	return info.Metadata, nil
}

func (p *liveNativePerceiver) Perceive(ctx context.Context, asset contextir.Asset) (contextir.Evidence, error) {
	evidence, _, err := p.PerceiveWithCalls(ctx, asset)
	return evidence, err
}

func (p *liveNativePerceiver) PerceiveWithCalls(ctx context.Context, asset contextir.Asset) (contextir.Evidence, []contextir.CallUsage, error) {
	if asset.ID != p.asset.ID || asset.Kind != p.asset.Kind || asset.SHA256 != p.asset.SHA256 || asset.URI != p.asset.URI {
		return contextir.Evidence{}, nil, liveMediaContractError("asset", "The native perceiver received a different source.")
	}
	p.delivered++
	if p.delivered != 1 {
		return contextir.Evidence{}, nil, liveMediaContractError("perception_ledger", "The same-run native observation and its paid call must be delivered exactly once.")
	}
	if !p.observed {
		p.observe(ctx)
	}
	return p.evidence, append([]contextir.CallUsage(nil), p.calls...), p.observeErr
}

func (p *liveNativePerceiver) observe(ctx context.Context) {
	if p.observed {
		return
	}
	p.observed = true
	track, stage, scope := contextir.TrackVisual, "video_perception", "visual_only"
	const common = `Observe only the original attached media bytes. Return one Evidence object matching the complete response schema, with a reusable AssetCard in card and at least one grounded Fact. The anonymous asset_id identifies this attachment only. No target brief, expected answer, filename or reference-role instruction is provided. Do not infer any of them. Analyzer/model/coverage metadata, source intervals, frame counts and sample times are host-owned: leave them unspecified or use empty schema-required placeholders. Facts must identify this asset and the requested track. Do not invent exact timestamps, frame numbers, full temporal coverage or absent tracks. State uncertainty in limitations. Return observations, not a proposed target scene.`
	system := common + ` Inspect the visual track only. Describe visible actions and changes in their observed order, and copy every clearly legible inscription verbatim into card.visible_text. Keep uncertain camera technique, unseen motion and unseen intervals unknown. Do not report any transcript, language, timbre, music or acoustic characterisation; this call does not establish audio observations.`
	if p.asset.Kind == contextir.MediaAudio {
		track, stage, scope = contextir.TrackAudio, "audio_perception", "transcript_and_language_only"
		system = common + ` Inspect the audio track. Transcribe all intelligible speech exactly in card.transcript and state its original language in card.language. Preserve the recognized words and punctuation; do not translate, paraphrase or provide a target performance. This acceptance call requests words and language only: leave timbre, music, acoustic characterisation, speaker identity and all visual fields empty. Facts may contain the actual transcript and original language only. Do not infer a scene from the speech.`
	}
	input, _ := json.Marshal(map[string]any{"asset_id": p.asset.ID, "media_kind": p.asset.Kind, "observation_scope": scope})
	response, err := p.model.GenerateMedia(ctx, contextir.ModelRequest{Stage: stage, System: system, Input: input, ResponseSchema: contextir.EvidenceSchema()}, p.mime, p.data)
	valid := !response.UsageInvalid && response.Usage.InputTokens >= 0 && response.Usage.OutputTokens >= 0
	p.calls = []contextir.CallUsage{{AssetID: p.asset.ID, Stage: stage, Attempt: 1, Model: response.Model, Usage: response.Usage, Valid: valid}}
	if err != nil {
		p.observeErr = err
		return
	}
	if !valid {
		p.observeErr = liveMediaContractError("perception_usage", "Native perception must preserve valid actual usage.")
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(response.JSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p.evidence); err != nil || decoder.Decode(new(any)) != io.EOF {
		p.observeErr = liveMediaContractError("perception_json", "Native perception must return the exact Evidence object.")
		return
	}
	if p.evidence.Card == nil || len(p.evidence.Facts) == 0 {
		p.observeErr = liveMediaContractError("perception_card", "Native perception must provide actual grounded facts and an observation card.")
		return
	}
	card := p.evidence.Card
	if p.asset.Kind == contextir.MediaVideo {
		if card.Transcript != "" || card.Language != "" || card.Timbre != "" || card.Music != "" || card.Characterisation != "" {
			p.observeErr = liveMediaContractError("perception_track", "Visual-only native perception must not claim audio observations.")
			return
		}
	} else if card.Timbre != "" || card.Music != "" || card.Characterisation != "" || len(card.Subjects) != 0 || len(card.VisibleText) != 0 || card.Motion != "" || card.Camera != "" || card.Environment != "" || card.Lighting != "" || card.Framing != "" || card.Style != "" || len(card.Palette) != 0 {
		p.observeErr = liveMediaContractError("perception_track", "Transcript-only audio perception must not invent acoustic or visual observations.")
		return
	}
	p.evidence.AssetID, p.evidence.Model, p.evidence.AnalyzerVersion = p.asset.ID, response.Model, liveNativeObservationVersion
	p.evidence.Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{track}}
	card.AssetID, card.Model, card.AnalyzerVersion, card.FramesSeen = p.asset.ID, response.Model, liveNativeObservationVersion, 0
	card.Coverage = &contextir.PerceptionCoverage{Status: contextir.CoveragePartial, Tracks: []contextir.Track{track}}
	for i := range p.evidence.Facts {
		fact := &p.evidence.Facts[i]
		if fact.Source.Track != track || fact.Source.Interval != nil {
			p.observeErr = liveMediaContractError("perception_fact", "Observed facts must preserve the requested track without invented temporal bounds.")
			return
		}
		fact.ID = fmt.Sprintf("%s.native-fact-%d", p.asset.ID, i+1)
		fact.Source = contextir.SourceRef{AssetID: p.asset.ID, Track: track}
		fact.AnalyzerVersion = liveNativeObservationVersion
	}
	limitation := "Original media bytes were sent to the native model. Native frame selection and exact temporal coverage are unknown; no extracted frames or invented sample times are claimed. This video acceptance inspects visuals only and establishes no audio observations."
	if p.asset.Kind == contextir.MediaAudio {
		limitation = "Original audio bytes were sent to the native model. This acceptance establishes recognized words and language only, not speaker identity, timbre, music, rhythm or acoustic texture."
	}
	p.evidence.Limitations = append(p.evidence.Limitations, limitation)
	p.observeErr = contextir.ValidateEvidence([]contextir.Asset{p.asset}, []contextir.Evidence{p.evidence})
}

type liveNativeCaseRecord struct {
	Status                string                `json:"status"`
	Phase                 string                `json:"phase"`
	Scope                 string                `json:"scope"`
	SourceSHA256          string                `json:"source_sha256"`
	SourceBytes           int                   `json:"source_bytes"`
	SourceKind            contextir.MediaKind   `json:"source_kind"`
	MIME                  string                `json:"mime_type"`
	Measured              media.Info            `json:"measured"`
	Brief                 compiler.Brief        `json:"brief"`
	Evidence              contextir.Evidence    `json:"evidence"`
	PerceptionCalls       []contextir.CallUsage `json:"perception_calls"`
	Result                compiler.Result       `json:"result"`
	Failures              []string              `json:"failures,omitempty"`
	ErrorCodes            []string              `json:"error_codes,omitempty"`
	ErrorPaths            []string              `json:"error_paths,omitempty"`
	FreshNativeMediaCalls int                   `json:"fresh_native_media_calls"`
	LedgerDeliveries      int                   `json:"ledger_deliveries"`
	RecognitionWordsMatch bool                  `json:"recognition_words_match"`
	ExactPunctuationMatch bool                  `json:"exact_punctuation_match"`
	ArtifactRecompiled    bool                  `json:"artifact_recompiled"`
	SemanticReviewPassed  bool                  `json:"semantic_review_passed"`
}

func TestLiveNativeMediaCompile(t *testing.T) {
	if os.Getenv("CONTEXTIR_NATIVE_MEDIA_LIVE") != "1" {
		t.Skip("set CONTEXTIR_NATIVE_MEDIA_LIVE=1 and explicit native model configuration to run paid native media acceptance")
	}
	key, baseURL, modelID := os.Getenv("CONTEXTIR_NATIVE_MEDIA_API_KEY"), os.Getenv("CONTEXTIR_NATIVE_MEDIA_BASE_URL"), os.Getenv("CONTEXTIR_NATIVE_MEDIA_MODEL")
	if strings.TrimSpace(key) == "" || strings.TrimSpace(baseURL) == "" || strings.TrimSpace(modelID) == "" {
		t.Fatal("CONTEXTIR_NATIVE_MEDIA_API_KEY, CONTEXTIR_NATIVE_MEDIA_BASE_URL and CONTEXTIR_NATIVE_MEDIA_MODEL must all be explicitly supplied")
	}
	directory := os.Getenv("CONTEXTIR_NATIVE_MEDIA_OUTPUT_DIR")
	if directory == "" {
		directory = filepath.Join("..", ".ci", "native-media-live", time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	tools, err := media.NewTools(media.Options{FFprobe: "ffprobe", FFmpeg: "ffmpeg"})
	if err != nil {
		t.Fatal("explicit native media acceptance requires ffprobe and ffmpeg")
	}
	for _, name := range []string{"video", "audio"} {
		t.Run(name, func(t *testing.T) {
			caseDir := filepath.Join(directory, name)
			if os.MkdirAll(caseDir, 0700) != nil {
				t.Fatal("the native acceptance output directory could not be created")
			}
			record := liveNativeCaseRecord{Status: "failed", Phase: "setup", Scope: "One fresh native original-media observation immediately before a complete real-model Compile; the same-run evidence and its actual paid call enter the compiler ledger once. No stored observation replay or fabricated billing."}
			defer func() { liveNativeSaveCase(t, caseDir, record) }()
			client, err := newLiveNativeClient(&http.Client{Timeout: 10 * time.Minute}, baseURL, modelID, key, "contextir-native-media-"+name+"-"+time.Now().UTC().Format("20060102T150405.000000000"))
			if err != nil {
				record.ErrorCodes, record.ErrorPaths = liveReferenceErrorSummary(err)
				t.Fatal("explicit native model configuration rejected; inspect the saved typed summary")
			}
			wireAttempt := 0
			client.onReceipt = func(receipt liveNativeReceipt) {
				wireAttempt++
				data, err := json.MarshalIndent(receipt, "", "  ")
				if err != nil || os.WriteFile(filepath.Join(caseDir, fmt.Sprintf("wire-%02d.json", wireAttempt)), append(data, '\n'), 0600) != nil {
					t.Error("the native wire receipt could not be persisted")
				}
			}
			model := &liveMediaContractModel{t: t, client: client, dir: caseDir}
			kind, mime, expectedHash := contextir.MediaVideo, "video/mp4", liveNativeVideoSHA256
			path := filepath.Join("..", "compiler", "testdata", "remaining_media", "off-vs-on.mp4")
			if name == "audio" {
				kind, mime, expectedHash = contextir.MediaAudio, "audio/flac", liveNativeAudioSHA256
				path = filepath.Join("testdata", "audio", "jfk.flac")
			}
			path, err = filepath.Abs(path)
			if err != nil {
				t.Fatal("the original media fixture path could not be resolved")
			}
			data, err := os.ReadFile(path)
			if err != nil || liveReferenceHash(data) != expectedHash {
				t.Fatal("the unmodified pinned native media fixture is required")
			}
			info, err := tools.Inspect(t.Context(), path)
			if err != nil || info.Kind != kind {
				t.Fatal("the original source could not be measured as its declared modality")
			}
			asset := contextir.Asset{ID: "source-1", Kind: kind, SHA256: expectedHash, URI: "fixture://native-media/source-1", Metadata: info.Metadata}
			perceiver := &liveNativePerceiver{model: model, tools: tools, asset: asset, path: path, mime: mime, data: data}
			record.SourceSHA256, record.SourceBytes, record.SourceKind, record.MIME, record.Measured = expectedHash, len(data), kind, mime, info
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Minute)
			defer cancel()
			record.Phase = "native_perception"
			perceiver.observe(ctx)
			record.FreshNativeMediaCalls = len(perceiver.calls)
			record.Evidence, record.PerceptionCalls = perceiver.evidence, perceiver.calls
			if perceiver.observeErr != nil {
				record.ErrorCodes, record.ErrorPaths = liveReferenceErrorSummary(perceiver.observeErr)
				t.Fatal("fresh native media observation failed; its actual call remains in the saved receipt")
			}
			shots := 1
			brief := compiler.Brief{Mode: contextir.ModeReference, Seconds: 15, SecondsExplicit: true, FrameCount: 360, Aspect: "16:9", AspectExplicit: true, Shots: &shots, Creativity: compiler.CreativityRestrained, Silent: true}
			card := perceiver.evidence.Card
			if kind == contextir.MediaVideo {
				record.Phase = "independent_video_recognition"
				observed, _ := json.Marshal(perceiver.evidence)
				if len(card.VisibleText) != 2 {
					record.Failures = append(record.Failures, "native_video_label_cardinality")
				}
				for _, label := range []string{"your sentence, sent raw", "compiled by open-h3-ir"} {
					if !strings.Contains(string(observed), label) || !slices.Contains(card.VisibleText, label) {
						record.Failures = append(record.Failures, "native_video_label_missing")
					}
				}
				if len(record.Failures) > 0 {
					t.Fatal("native video did not independently recognize the held-out visible labels")
				}
				brief.Intent = "Create a 15-second silent edit of the supplied source video. Preserve its observed action order, transitions and already visible words, while extending only a natural final hold if needed. Keep the source scene and people within their observed scope. Add no dialogue, music, sound or new lettering."
				brief.Shots = nil
				brief.SilenceAll = true
				brief.OnscreenText = append([]string(nil), card.VisibleText...)
				brief.Assets = []compiler.Asset{{Asset: asset, Role: compiler.RoleEditSource}}
			} else {
				record.Phase = "independent_audio_recognition"
				record.RecognitionWordsMatch = reflect.DeepEqual(liveNativeWords(card.Transcript), liveNativeWords(liveNativeExpectedSpeech))
				record.ExactPunctuationMatch = card.Transcript == liveNativeExpectedSpeech
				if !record.RecognitionWordsMatch || !(strings.EqualFold(card.Language, "en") || strings.EqualFold(card.Language, "English")) {
					record.Failures = append(record.Failures, "native_audio_recognition_mismatch")
					t.Fatal("native audio did not independently recognize the held-out speech words and language")
				}
				brief.Intent = "Create a 15-second single-shot view of an empty room with the complete recognized words from the attached audio performed exactly once as an off-screen voiceover in their original language. Preserve those observed words verbatim without translation or extra speech. The source is a voice reference, not permission to copy its recorded signal. Add no music or visible text."
				brief.Transcripts = map[string]string{asset.ID: card.Transcript}
				brief.TranscriptUses = []compiler.TranscriptUse{{AssetID: asset.ID, Text: card.Transcript, Language: card.Language, SpeakerHint: "the off-screen narrator", Voiceover: true}}
				brief.Assets = []compiler.Asset{{Asset: asset, Role: compiler.RoleVoiceTimbre}}
			}
			record.Brief = brief
			record.Phase = "compile"
			instance, err := compiler.New(model, perceiver, compiler.Options{Profile: h3.OfficialProfile(), FramePolicy: compiler.FramePolicyExact, FailurePolicy: compiler.FailurePolicyFail, ClarificationPolicy: compiler.ClarificationRequire, MaxStructureRetries: 1, Planner: planner.Options{MaxRetries: 2, AllowPartialPerception: true}})
			if err != nil {
				record.ErrorCodes, record.ErrorPaths = liveReferenceErrorSummary(err)
				t.Fatal("the explicit native media compiler configuration was rejected")
			}
			result, compileErr := instance.Compile(ctx, brief)
			record.Result, record.LedgerDeliveries = result, perceiver.delivered
			record.ErrorCodes, record.ErrorPaths = liveReferenceErrorSummary(compileErr)
			if compileErr != nil || result.Status != "compiled" {
				t.Fatal("the complete native media Compile failed; actual responses, usage and result were retained")
			}
			record.Phase = "assert_complete_contract"
			liveNativeCheckResult(&record, perceiver, kind)
			if len(record.Failures) != 0 {
				t.Fatal("native media compilation did not meet its final evidence, ledger, binding or artifact assertions")
			}
			record.Status, record.Phase = "passed", "complete"
		})
	}
}

func liveNativeWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

func liveNativeCheckResult(record *liveNativeCaseRecord, perceiver *liveNativePerceiver, kind contextir.MediaKind) {
	result := record.Result
	check := func(ok bool, failure string) {
		if !ok {
			record.Failures = append(record.Failures, failure)
		}
	}
	check(result.UsageComplete && len(result.Calls) > 1, "incomplete_actual_usage")
	check(perceiver.delivered == 1 && len(perceiver.calls) == 1, "native_observation_not_delivered_once")
	perceptionCount, semanticCalls := 0, 0
	for _, call := range result.Calls {
		check(call.Valid && !call.Unmetered, "invalid_actual_call_usage")
		if call.Stage == perceiver.calls[0].Stage {
			perceptionCount++
			check(reflect.DeepEqual(call, perceiver.calls[0]), "native_perception_ledger_changed")
		}
		if call.Stage == "semantic_review" {
			semanticCalls++
		}
	}
	check(perceptionCount == 1, "native_perception_ledger_not_unique")
	check(len(result.Document.Assets) == 1 && result.Document.Assets[0].SHA256 == perceiver.asset.SHA256 && result.Document.Assets[0].Kind == kind, "original_source_not_preserved")
	check(len(result.Evidence) == 1 && reflect.DeepEqual(result.Evidence[0], perceiver.evidence), "observed_evidence_changed")
	check(result.Document.Target.Duration == 15000 && result.Artifact.Frames == 360, "explicit_target_changed")
	if len(result.CompositionReviews) > 0 {
		last := result.CompositionReviews[len(result.CompositionReviews)-1]
		record.SemanticReviewPassed = last.Required && last.Passed && len(last.Findings) == 0
	}
	check(semanticCalls > 0 && record.SemanticReviewPassed, "real_semantic_review_missing_or_failed")
	artifact, err := h3.Compile(result.Document, h3.OfficialProfile())
	record.ArtifactRecompiled = err == nil && reflect.DeepEqual(artifact, result.Artifact)
	check(record.ArtifactRecompiled, "artifact_not_derived_from_authoritative_document")
	if kind == contextir.MediaVideo {
		check(result.Document.Soundscape == "N/A" && result.Document.Music == "N/A" && len(result.Document.Dialogue) == 0, "video_visual_only_target_gained_audio")
		bound := false
		for _, ref := range result.Document.References {
			bound = bound || ref.Kind == contextir.ReferenceVideo && ref.Source.AssetID == perceiver.asset.ID && ref.Source.Track == contextir.TrackVisual && slices.Contains(ref.Roles, contextir.RoleEditSource)
		}
		check(bound && strings.Contains(result.Artifact.Prompt, "[video editing]"), "native_video_source_relationship_missing")
		for _, label := range []string{"your sentence, sent raw", "compiled by open-h3-ir"} {
			check(strings.Contains(result.Artifact.Prompt, label), "observed_video_label_not_preserved")
		}
		return
	}
	actual := perceiver.evidence.Card.Transcript
	check(len(result.Document.Dialogue) == 1 && result.Document.Dialogue[0].Text == actual && result.Document.Dialogue[0].Voiceover, "actual_audio_words_not_preserved_as_voiceover")
	check(strings.Count(result.Artifact.Prompt, actual) == 1 && result.Document.Music == "N/A", "actual_audio_words_duplicated_or_music_added")
	if len(result.Document.Dialogue) != 1 {
		return
	}
	line := result.Document.Dialogue[0]
	voiceBindings, dialogueEvents := 0, 0
	for _, speaker := range result.Document.Speakers {
		if !slices.Contains(line.SpeakerIDs, speaker.ID) {
			continue
		}
		for _, ref := range result.Document.References {
			if ref.ID == speaker.VoiceReferenceID && ref.SpeakerID == speaker.ID && ref.Kind == contextir.ReferenceAudio && ref.Source.AssetID == perceiver.asset.ID && ref.Source.Track == contextir.TrackAudio && slices.Contains(ref.Roles, contextir.RoleVoice) {
				voiceBindings++
			}
		}
	}
	for _, shot := range result.Document.Shots {
		for _, event := range shot.Events {
			if event.Dialogue != nil && event.Dialogue.LineID == line.ID {
				dialogueEvents++
				check(event.Dialogue.Text == actual, "host_dialogue_event_changed_actual_audio_words")
			}
		}
	}
	check(voiceBindings == 1 && dialogueEvents == 1, "actual_audio_voice_binding_or_event_not_unique")
}

func liveNativeSaveCase(t *testing.T, directory string, record liveNativeCaseRecord) {
	t.Helper()
	for _, file := range []struct {
		name  string
		value any
	}{{"summary.json", record}, {"brief.json", record.Brief}, {"evidence.json", record.Evidence}, {"perception-calls.json", record.PerceptionCalls}, {"result.json", record.Result}} {
		data, err := json.MarshalIndent(file.value, "", "  ")
		if err != nil || os.WriteFile(filepath.Join(directory, file.name), append(data, '\n'), 0600) != nil {
			t.Error("the native acceptance receipt could not be persisted")
		}
	}
	if os.WriteFile(filepath.Join(directory, "prompt.txt"), []byte(record.Result.Artifact.Prompt), 0600) != nil {
		t.Error("the native acceptance prompt could not be persisted")
	}
	t.Logf("native_acceptance status=%s phase=%s kind=%s source_bytes=%d source_sha256=%s native_calls=%d compiler_calls=%d usage_complete=%t artifact_match=%t semantic_passed=%t failures=%v error_codes=%v error_paths=%v", record.Status, record.Phase, record.SourceKind, record.SourceBytes, record.SourceSHA256, record.FreshNativeMediaCalls, len(record.Result.Calls), record.Result.UsageComplete, record.ArtifactRecompiled, record.SemanticReviewPassed, record.Failures, record.ErrorCodes, record.ErrorPaths)
}
