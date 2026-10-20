package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ly95/contextir"
)

func TestCLIValidateCompileAndSchema(t *testing.T) {
	doc := contextir.Document{Version: contextir.Version, Mode: contextir.ModeText, Target: contextir.Target{Duration: 8000, AspectRatio: "16:9", FPS: contextir.Rational{Numerator: 24, Denominator: 1}}, Shots: []contextir.Shot{{ID: "shot", Start: 0, End: 8000, Description: contextir.Text("A clock ticks on a wooden desk.")}}, Soundscape: "A clock ticks.", Music: "N/A"}
	data, _ := json.Marshal(doc)
	for _, args := range [][]string{{"validate"}, {"compile"}, {"compile", "-profile", "comfyui"}, {"schema"}, {"schema", "-type", "evidence"}} {
		var out, errOut bytes.Buffer
		if code := run(args, bytes.NewReader(data), &out, &errOut); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, errOut.String())
		}
		if !json.Valid(out.Bytes()) {
			t.Fatalf("%v did not produce JSON: %s", args, out.String())
		}
	}
}

func TestCLIFailureIsExplicitAndSafe(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"schema", "-type", "missing"}, {"compile", "-profile", "missing"}, {"compile"}, {"validate"}} {
		var out, errOut bytes.Buffer
		if code := run(args, strings.NewReader(`{"private":"do-not-leak"}`), &out, &errOut); code == 0 {
			t.Fatalf("%v succeeded", args)
		}
		if out.Len() != 0 {
			t.Fatalf("%v emitted partial artifact", args)
		}
		if strings.Contains(errOut.String(), "do-not-leak") {
			t.Fatal("input leaked to diagnostics")
		}
	}
}
