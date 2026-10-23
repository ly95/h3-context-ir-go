package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ly95/h3-context-ir-go/service"
)

func TestRemainingSurfaceContractCLIPrintsPublishedBytes(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "h3-context-ir-go")
	build := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-o", binary, ".")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("native CLI build: %v\n%s", err, out)
	}
	expectedJSON, err := service.AsJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name     string
		args     []string
		expected string
	}{{"json", []string{"contract"}, expectedJSON}, {"javascript", []string{"contract", "-js"}, service.AsJS()}} {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(binary, c.args...)
			var out, errOut bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &errOut
			if err := cmd.Run(); err != nil {
				t.Fatalf("real CLI exit: %v\n%s", err, errOut.String())
			}
			if out.String() != c.expected || errOut.Len() != 0 {
				t.Fatal("CLI changed the published bytes, trailing newline, or stderr")
			}
		})
	}
}
