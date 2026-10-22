package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

func TestAudioPropertyTargetsMechanicallyBindDistinctRecipients(t *testing.T) {
	for _, test := range []struct {
		role Role
		kind audioRecipientKind
	}{{RoleVoiceTimbre, audioRecipientVocal}, {RoleMusicStyle, audioRecipientScore}, {RoleBeatReference, audioRecipientTiming}, {RoleSFX, audioRecipientSFX}} {
		t.Run(string(test.role), func(t *testing.T) {
			contract, _, req := referenceTestContract(t, test.role)
			if len(contract.AudioTargets) != 1 {
				t.Fatal("property reference omitted its host recipient", contract)
			}
			target := contract.AudioTargets[0]
			if target.ReferenceID != contract.References[0].ID || target.Source != contract.References[0].Source || target.Recipient != test.kind || target.Role != rootRole(test.role) {
				t.Fatal("role/source/reference did not settle its exact recipient", target)
			}
			if test.role == RoleVoiceTimbre && (target.SpeakerID == "" || target.SpeakerID != contract.Speakers[0].ID || !target.NoFixedDialogue) {
				t.Fatal("dormant voice lost its reciprocal vocal binding", target)
			}
			input := map[string]json.RawMessage{}
			input["request"], _ = json.Marshal(req)
			if err := contract.projectWriterInput(input); err != nil {
				t.Fatal(err)
			}
			var controls struct {
				Targets []audioPropertyTarget `json:"audio_property_targets"`
			}
			if err := json.Unmarshal(input["compiler_controls"], &controls); err != nil || len(controls.Targets) != 1 || controls.Targets[0].Recipient != test.kind {
				t.Fatal("writer never received its host-owned recipient", err, controls)
			}
			for _, incompatible := range []audioRecipientKind{audioRecipientVocal, audioRecipientScore, audioRecipientTiming, audioRecipientSFX} {
				finding := CompositionFinding{Code: "audio_property_target_mismatch", ReferenceID: target.ReferenceID, RecipientKind: incompatible}
				if incompatible == audioRecipientVocal {
					finding.SpeakerID = target.SpeakerID
					if finding.SpeakerID == "" {
						finding.SpeakerID = "other-vocal"
					}
				}
				knownSpeakers := append(append([]contextir.Speaker(nil), contract.Speakers...), contextir.Speaker{ID: "other-vocal"})
				err := validateAudioPropertyFinding(finding, contract.AudioTargets, knownSpeakers)
				if (incompatible == target.Recipient) != (err != nil) {
					t.Fatal("recipient findings accepted a matching target or erased an incompatible target", incompatible, target, err)
				}
			}
		})
	}
	copyContract, _, _ := referenceTestContract(t, RoleBGM)
	if len(copyContract.AudioTargets) != 0 || copyContract.References[0].Retention != contextir.PartiallyCopy {
		t.Fatal("signal-copy authority was replaced with a property relationship")
	}
}

func TestAudioPropertyVoiceBindingAndWriterPolicyStayAuthoritative(t *testing.T) {
	contract, _, _ := referenceTestContract(t, RoleVoiceTimbre)
	if !CompositionReviewRequired(Brief{}, Structure{}, contextir.Document{References: contract.References}) || CompositionReviewRequired(Brief{Intent: "voice engine score timing"}, Structure{}) {
		t.Fatal("actual typed property references must trigger review without keyword routing")
	}
	broken := append([]contextir.Speaker(nil), contract.Speakers...)
	broken[0].VoiceReferenceID = "missing"
	if _, err := audioPropertyTargets(contract.References, broken, nil); err == nil {
		t.Fatal("nonreciprocal voice target accepted")
	}
	wrongSpeaker := CompositionFinding{Code: "audio_property_target_mismatch", ReferenceID: contract.References[0].ID, RecipientKind: audioRecipientVocal, SpeakerID: "invented"}
	if err := validateAudioPropertyFinding(wrongSpeaker, contract.AudioTargets, contract.Speakers); err == nil {
		t.Fatal("unknown vocal speaker bypassed the host finding boundary")
	}
	wrongSpeaker.SpeakerID = "other-vocal"
	if err := validateAudioPropertyFinding(wrongSpeaker, contract.AudioTargets, append(contract.Speakers, contextir.Speaker{ID: "other-vocal"})); err != nil {
		t.Fatal("an actual different bound vocal recipient was erased", err)
	}
	_, request, _ := remainingMustWrite(t, remainingAudioBrief(RoleVoiceTimbre))
	if !strings.Contains(request.System, audioPropertyTargetInstruction) || strings.Contains(request.System, "references condition a new sound") {
		t.Fatal("writer still received ambiguous permission to redirect any audio property")
	}
	var packet struct {
		Controls struct {
			Targets []audioPropertyTarget `json:"audio_property_targets"`
		} `json:"compiler_controls"`
		Facts compositionBoundary `json:"composition_facts"`
	}
	if err := json.Unmarshal(request.Input, &packet); err != nil || len(packet.Controls.Targets) != 1 || packet.Controls.Targets[0].Recipient != audioRecipientVocal || len(packet.Facts.Audio.Attachments) != 1 || packet.Facts.Audio.Attachments[0].Recipient != audioRecipientVocal {
		t.Fatal("actual writer boundary did not preserve the vocal recipient", err, packet)
	}
}

func TestActualVoiceToEngineCandidateReceivesGroundedRecipientReview(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_audio_property_recipient_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "0c035ec659d50c3d027254074028641779bb53bafbfe2f6df8b56a57313e6e42" {
		t.Fatal("actual fourth-run voice-to-engine candidate changed")
	}
	for _, variant := range []string{"actual_voice_to_engine", "invented_quote", "missing_reference", "unknown_reference", "matching_recipient", "unknown_vocal_speaker", "wrong_bound_vocal", "independent_engine", "authorized_vocal", "authorized_nonverbal_vocal"} {
		t.Run(variant, func(t *testing.T) {
			var recorded struct {
				Brief     Brief              `json:"brief"`
				Structure Structure          `json:"structure"`
				Document  contextir.Document `json:"document"`
			}
			if err := json.Unmarshal(raw, &recorded); err != nil {
				t.Fatal(err)
			}
			d := recorded.Document
			vocal := d.References[0].SpeakerID
			good := variant == "independent_engine" || variant == "authorized_vocal" || variant == "authorized_nonverbal_vocal"
			if good {
				d.Summary = contextir.Text("A car rolls forward from darkness with independently generated engine ambience.")
				d.Soundscape = "Independently generated engine ambience; the dormant voice reference does not guide this mechanical sound."
				for i := range d.Shots {
					d.Shots[i].Description = contextir.Text("The car rolls forward from darkness. Newly generated engine ambience is independent of the voice reference.")
					for j := range d.Shots[i].Events {
						if d.Shots[i].Events[j].Kind == contextir.EventSound {
							d.Shots[i].Events[j].Description = contextir.Text("Independently generated low engine rumble.")
						}
					}
				}
			}
			if variant == "authorized_vocal" {
				d.Constraints = nil
				d.Dialogue = []contextir.DialogueLine{{ID: "fixed-vocal", Text: "Go.", Language: "English", SpeakerIDs: []string{vocal}, Voiceover: true}}
				d.Shots[0].Events = append(d.Shots[0].Events, contextir.Event{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: "fixed-vocal", Text: "Go."}})
				recorded.Brief.Dialogue = []Dialogue{{Text: "Go.", Language: "English", Voiceover: true}}
				d.Soundscape += " The authorized offscreen speaker follows the bound voice reference for the one vocal performance."
			}
			if variant == "authorized_nonverbal_vocal" {
				recorded.Brief.Intent += " The explicitly bound offscreen vocal speaker may breathe softly using this voice reference; no words are requested."
				recorded.Brief.VoiceBindings = map[string]string{"asset-1": vocal}
				d.Soundscape += " Separately, the explicitly authorized bound speaker breathes softly with the vocal quality from the voice reference."
			}
			if variant == "wrong_bound_vocal" {
				d.Speakers = append(d.Speakers, contextir.Speaker{ID: "other-vocal", Description: "Another target voice that does not own this reference."})
				d.Soundscape = "The supplied voice reference shapes the vocal performance of the other target speaker, rather than its reciprocally bound speaker."
			}
			calls := 0
			c, err := New(modelFunc(func(_ context.Context, request contextir.ModelRequest) (contextir.ModelResponse, error) {
				calls++
				var input struct {
					Version   string                `json:"version"`
					Targets   []audioPropertyTarget `json:"audio_property_targets"`
					Fragments map[string]string     `json:"fragments"`
				}
				if err := json.Unmarshal(request.Input, &input); err != nil || len(input.Targets) != 1 || input.Targets[0].Recipient != audioRecipientVocal || input.Targets[0].SpeakerID != vocal || input.Targets[0].ReferenceID != d.References[0].ID {
					t.Fatal("critic lost the host-owned reference/vocal target", err, input)
				}
				if request.Stage != "semantic_review" || !strings.Contains(string(request.ResponseSchema), "audio_property_target_mismatch") || !strings.Contains(request.System, "similar timbre") || !strings.Contains(request.System, "nonverbal vocalization") || input.Version != CompositionReviewVersion {
					t.Fatal("recipient review, published version or legitimate acoustic/vocal cases missing")
				}
				if variant == "authorized_vocal" && input.Targets[0].NoFixedDialogue || variant != "authorized_vocal" && !input.Targets[0].NoFixedDialogue {
					t.Fatal("typed fixed-vocal evidence drifted", input.Targets[0])
				}
				findings := []CompositionFinding{}
				if !good {
					finding := CompositionFinding{Code: "audio_property_target_mismatch", FragmentID: "soundscape", Quote: input.Fragments["soundscape"], ReferenceID: d.References[0].ID, RecipientKind: audioRecipientSFX}
					switch variant {
					case "invented_quote":
						finding.Quote = "A sentence never present in this actual candidate."
					case "missing_reference":
						finding.ReferenceID = ""
					case "unknown_reference":
						finding.ReferenceID = "unknown-reference"
					case "matching_recipient":
						finding.RecipientKind, finding.SpeakerID = audioRecipientVocal, vocal
					case "unknown_vocal_speaker":
						finding.RecipientKind, finding.SpeakerID = audioRecipientVocal, "invented-vocal"
					case "wrong_bound_vocal":
						finding.RecipientKind, finding.SpeakerID = audioRecipientVocal, "other-vocal"
					}
					findings = append(findings, finding)
				}
				wire, _ := json.Marshal(map[string]any{"findings": findings})
				return contextir.ModelResponse{JSON: wire, Model: "explicit-typed-recipient-review-double", Usage: contextir.Usage{InputTokens: 19, OutputTokens: 7}}, nil
			}), nil, Options{})
			if err != nil {
				t.Fatal(err)
			}
			review, err := c.ReviewComposition(context.Background(), recorded.Brief, recorded.Structure, d)
			if calls != 1 || len(review.Calls) != 1 || review.Calls[0].Usage.InputTokens != 19 || !review.Required {
				t.Fatal("recipient audit lost its actual call or usage", review, err)
			}
			if good {
				if err != nil || !review.Passed {
					t.Fatal("legitimate vocal/independent engine target was rejected", review, err)
				}
			} else if err == nil || review.Passed || !contextir.HasCode(err, contextir.CodeModelOutput) {
				t.Fatal("real mismatch or invalid supporting evidence accepted", review, err)
			}
			if variant == "actual_voice_to_engine" && (len(review.Findings) != 1 || review.Findings[0].ReferenceID != d.References[0].ID || review.Findings[0].RecipientKind != audioRecipientSFX) {
				t.Fatal("actual grounded relationship did not reach repair boundary", review, err)
			}
			if variant == "wrong_bound_vocal" && (len(review.Findings) != 1 || review.Findings[0].SpeakerID != "other-vocal") {
				t.Fatal("actual wrong vocal recipient did not reach repair boundary", review, err)
			}
			if !good && variant != "actual_voice_to_engine" && variant != "wrong_bound_vocal" && len(review.Findings) != 0 {
				t.Fatal("invalid quote/reference/recipient was silently trusted", review, err)
			}
		})
	}
}
