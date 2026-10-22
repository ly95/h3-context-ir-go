// A complete manual IR → H3 compilation. Run: go run ./examples/manual
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func main() {
	doc := contextir.Document{
		Version: contextir.Version, Mode: contextir.ModeText,
		Target:   contextir.Target{Duration: 8000, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}},
		Style:    "Live-action, warm morning light.",
		Speakers: []contextir.Speaker{{ID: "baker", Description: "A middle-aged baker with a gentle, low voice"}},
		Dialogue: []contextir.DialogueLine{{ID: "greeting", SpeakerIDs: []string{"baker"}, Language: "Chinese", Text: "早上好，面包刚出炉。"}},
		Shots: []contextir.Shot{{ID: "opening", Start: 0, End: 8000,
			Description: contextir.Text("A medium shot frames a baker opening the shutters of a small bakery. The camera slowly pushes in."),
			Events:      []contextir.Event{{Kind: contextir.EventDialogue, Dialogue: &contextir.DialoguePart{LineID: "greeting", Text: "早上好，面包刚出炉。"}}}}},
		Soundscape: "Wooden shutters scrape open, followed by trays clinking softly.", Music: "N/A",
	}
	artifact, err := h3.Compile(doc, h3.OfficialProfile())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(artifact); err != nil {
		os.Exit(1)
	}
}
