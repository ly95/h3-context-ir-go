package compiler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/h3check"
	"github.com/ly95/h3-context-ir-go/tokens"
)

func remainingRecordedRendererHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func remainingRecordedRendererJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal("recorded renderer comparison could not encode its typed value", err)
	}
	return data
}

func remainingRecordedResultDifferenceFields(t *testing.T, actual, expected []byte) []string {
	t.Helper()
	var left, right map[string]json.RawMessage
	if json.Unmarshal(actual, &left) != nil || json.Unmarshal(expected, &right) != nil {
		t.Fatal("recorded result diagnostics could not inspect the typed comparison")
	}
	fields := map[string]bool{}
	for name := range left {
		fields[name] = true
	}
	for name := range right {
		fields[name] = true
	}
	var changed []string
	for name := range fields {
		if !bytes.Equal(left[name], right[name]) {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

func remainingRecordedSummaryParts(t *testing.T, prompt string) (string, string, string) {
	t.Helper()
	const summaryHead = "\n\nsummary:\n"
	const summaryTail = "\n\nretention_analysis:\n"
	if strings.Count(prompt, summaryHead) != 1 || strings.Count(prompt, summaryTail) != 1 {
		t.Fatal("recorded reference prompt does not have one unambiguous summary section")
	}
	head, rest, _ := strings.Cut(prompt, summaryHead)
	body, tail, found := strings.Cut(rest, summaryTail)
	if !found {
		t.Fatal("recorded reference prompt has a malformed summary boundary")
	}
	return head + summaryHead, body, summaryTail + tail
}

func remainingRecordedEmptySummary(document contextir.Document) bool {
	for _, span := range document.Summary {
		if span.SubjectID != "" || span.ReferenceID != "" || strings.TrimSpace(span.Text) != "" {
			return false
		}
	}
	return true
}

func remainingCurrentRecordedArtifact(t *testing.T, document contextir.Document, historical h3.Artifact) h3.Artifact {
	t.Helper()
	if historical.CompilerVersion != "contextir-h3/v1.2" {
		t.Fatal("the historical renderer migration requires the pinned v1.2 artifact")
	}
	if historical.DocumentSHA256 != remainingRecordedRendererHash(remainingRecordedRendererJSON(t, document)) || historical.ProfileSHA256 != remainingRecordedRendererHash(remainingRecordedRendererJSON(t, historical.Profile)) || historical.ManifestSHA256 != remainingRecordedRendererHash(remainingRecordedRendererJSON(t, historical.Manifest)) || historical.PromptSHA256 != remainingRecordedRendererHash([]byte(historical.Prompt)) {
		t.Fatal("a historical artifact hash does not identify its original document, profile, manifest or prompt")
	}
	historicalIdentity := historical
	historicalIdentity.SHA256 = ""
	if historical.SHA256 != remainingRecordedRendererHash(remainingRecordedRendererJSON(t, historicalIdentity)) {
		t.Fatal("the original historical artifact identity is inconsistent")
	}
	current, err := h3.Compile(document, historical.Profile)
	if err != nil {
		t.Fatal("the original recorded document no longer compiles with its recorded format profile", err)
	}
	expected := historical
	expected.CompilerVersion = h3.CompilerVersion
	if document.Mode == contextir.ModeReference && remainingRecordedEmptySummary(document) {
		oldHead, oldSummary, oldTail := remainingRecordedSummaryParts(t, historical.Prompt)
		newHead, newSummary, newTail := remainingRecordedSummaryParts(t, current.Prompt)
		if oldHead != newHead || oldTail != newTail {
			t.Fatal("renderer migration changed content outside the absent-summary projection")
		}
		prefix, _, found := strings.Cut(strings.TrimPrefix(oldSummary, "["), "]")
		if !strings.HasPrefix(oldSummary, "[") || !found || prefix == "" {
			t.Fatal("the historical summary does not contain its typed task prefix")
		}
		var subjectLabels []string
		subjectLabelsByID := map[string]string{}
		for _, subject := range historical.Manifest.Subjects {
			subjectLabelsByID[subject.ID] = subject.Label
		}
		for _, subject := range document.Subjects {
			if subject.TakenOverBy == "" && len(subject.ShotIDs) > 0 {
				label := subjectLabelsByID[subject.ID]
				if label == "" {
					t.Fatal("a recorded appearing subject lost its original manifest label")
				}
				subjectLabels = append(subjectLabels, label)
			}
		}
		labels := map[string]string{}
		for _, reference := range historical.Manifest.References {
			labels[reference.ID] = reference.Label
		}
		editSource := ""
		for _, reference := range document.References {
			if slices.Contains(reference.Roles, contextir.RoleEditSource) {
				editSource = labels[reference.ID]
				if editSource == "" {
					t.Fatal("the historical edit source lost its original manifest label")
				}
				break
			}
		}
		wantedSummary := h3.FormatSummary(h3.SummarySpec{TaskTypes: strings.Split(prefix, " + "), SubjectLabels: subjectLabels, EditSourceLabel: editSource})
		if newSummary != wantedSummary {
			t.Fatal("renderer migration did not use the default summary of typed appearing subjects")
		}
		expected.Prompt = oldHead + wantedSummary + oldTail
	} else if current.Prompt != historical.Prompt {
		t.Fatal("renderer migration changed an explicit summary or a base-mode prompt")
	}
	expected.PromptSHA256 = remainingRecordedRendererHash([]byte(expected.Prompt))
	expected.SHA256 = ""
	expected.SHA256 = remainingRecordedRendererHash(remainingRecordedRendererJSON(t, expected))
	if !reflect.DeepEqual(current, expected) {
		t.Fatal("current renderer changed more than its version, permitted default summary and derived hashes")
	}
	return expected
}

func remainingCurrentRecordedResult(t *testing.T, actual, historical Result) Result {
	t.Helper()
	historicalMatch := bytes.Equal(remainingRecordedRendererJSON(t, actual), remainingRecordedRendererJSON(t, historical))
	t.Logf("historical-renderer-compatibility compiler=%s complete-result-equal=%t document-equal=%t paid-calls-equal=%t recorded-prompt-sha256=%s", historical.Artifact.CompilerVersion, historicalMatch, reflect.DeepEqual(actual.Document, historical.Document), reflect.DeepEqual(actual.Calls, historical.Calls), historical.Artifact.PromptSHA256)
	if !reflect.DeepEqual(actual.Document, historical.Document) || !reflect.DeepEqual(actual.Calls, historical.Calls) {
		t.Fatal("renderer migration changed the original document or actual paid-call accounting")
	}
	oldTokens, err := tokens.Count(historical.Artifact.Prompt)
	if err != nil || oldTokens != historical.Budget.PromptTokens {
		t.Fatal("the historical prompt budget does not identify the original rendered prompt", err)
	}
	expected := historical
	expected.Artifact = remainingCurrentRecordedArtifact(t, historical.Document, historical.Artifact)
	newTokens, err := tokens.Count(expected.Artifact.Prompt)
	if err != nil {
		t.Fatal("the migrated renderer prompt budget could not be counted", err)
	}
	expected.Budget.PromptTokens = newTokens
	oldBand := remainingRecordedTokenBand(historical.Artifact.Prompt, oldTokens)
	newBand := remainingRecordedTokenBand(expected.Artifact.Prompt, newTokens)
	var recordedBand []h3check.Finding
	for _, finding := range historical.Findings {
		if finding.Rule == "H3-token-band" {
			recordedBand = append(recordedBand, finding)
		}
	}
	if !reflect.DeepEqual(recordedBand, oldBand) {
		t.Fatal("the historical token-band diagnostic does not match its original prompt and token budget")
	}
	if !reflect.DeepEqual(oldBand, newBand) {
		if len(oldBand) != 1 {
			t.Fatal("renderer migration introduced a token-band diagnostic without a historical warning position")
		}
		expected.Findings = nil
		for _, finding := range historical.Findings {
			if finding.Rule == "H3-token-band" {
				expected.Findings = append(expected.Findings, newBand...)
			} else {
				expected.Findings = append(expected.Findings, finding)
			}
		}
		t.Logf("rendered-token-band-diagnostic historical-warnings=%d current-warnings=%d budget=%v", len(oldBand), len(newBand), h3check.NewContext().TokenBudget)
	}
	t.Logf("current-native-renderer compiler=%s default-summary-migration=%t rendered-prompt-tokens-before=%d rendered-prompt-tokens-after=%d prompt-sha256=%s", expected.Artifact.CompilerVersion, historical.Artifact.Prompt != expected.Artifact.Prompt, oldTokens, newTokens, expected.Artifact.PromptSHA256)
	return expected
}

func remainingRecordedTokenBand(prompt string, count int) []h3check.Finding {
	ctx := h3check.NewContext()
	ctx.PromptTokens = &count
	var band []h3check.Finding
	for _, finding := range h3check.Validate(prompt, ctx) {
		if finding.Rule == "H3-token-band" {
			band = append(band, finding)
		}
	}
	return band
}
