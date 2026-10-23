package service_test

import (
	"testing"

	"github.com/ly95/h3-context-ir-go/service"
)

func TestParseBriefDefaultsOnlyOmittedFields(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    string
		kind     string
		sizing   string
		language string
	}{
		{"omitted", `{"intent":"x","assets":[{}],"dialogue":[{"text":"Exact words."}]}`, "image", "match", "English"},
		{"explicit", `{"intent":"x","assets":[{"kind":"audio","sizing":"max"}],"dialogue":[{"text":"原话。","language":"Chinese"}]}`, "audio", "max", "Chinese"},
		{"empty language", `{"intent":"x","assets":[{}],"dialogue":[{"text":"Exact words.","language":""}]}`, "image", "match", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			brief, err := service.ParseBrief([]byte(test.input))
			if err != nil {
				t.Fatal(err)
			}
			if len(brief.Assets) != 1 || brief.Assets[0].Kind != test.kind || brief.Assets[0].Sizing != test.sizing || len(brief.Dialogue) != 1 || brief.Dialogue[0].Language != test.language {
				t.Fatalf("wire defaults changed caller choices: %+v", brief)
			}
		})
	}
	for name, input := range map[string]string{
		"null kind":     `{"intent":"x","assets":[{"kind":null}]}`,
		"null sizing":   `{"intent":"x","assets":[{"sizing":null}]}`,
		"null language": `{"intent":"x","dialogue":[{"text":"Exact words.","language":null}]}`,
		"empty kind":    `{"intent":"x","assets":[{"kind":""}]}`,
		"empty sizing":  `{"intent":"x","assets":[{"sizing":""}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.ParseBrief([]byte(input)); err == nil {
				t.Fatal("invalid explicit value was accepted")
			}
		})
	}
}
