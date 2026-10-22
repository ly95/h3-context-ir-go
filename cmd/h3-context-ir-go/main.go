package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/h3"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: h3-context-ir-go schema | validate | compile | contract [-js] | directors | budget | tokens | check-h3 | draft | brief | demo (use command -h for flags)")
		return 2
	}
	command := args[0]
	switch command {
	case "contract", "directors", "budget", "tokens", "check-h3", "checkH3":
		return runDeterministic(command, args[1:], in, out, errOut)
	case "brief", "draft":
		return runBrief(command, args[1:], in, out, errOut)
	case "demo":
		return runDemo(args[1:], out, errOut)
	}
	if command == "schema" {
		flags := flag.NewFlagSet(command, flag.ContinueOnError)
		flags.SetOutput(errOut)
		typeName := flags.String("type", "document", "Schema root: document or evidence")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return 2
		}
		var schema json.RawMessage
		switch *typeName {
		case "document":
			schema = contextir.DocumentSchema()
		case "evidence":
			schema = contextir.EvidenceSchema()
		default:
			fmt.Fprintln(errOut, "unsupported schema type")
			return 2
		}
		if _, err := fmt.Fprintln(out, string(schema)); err != nil {
			return 1
		}
		return 0
	}
	if command != "compile" && command != "validate" {
		fmt.Fprintln(errOut, "unknown command")
		return 2
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(errOut)
	profileName := flags.String("profile", "official", "H3 target profile: official or comfyui")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return 2
	}
	var profile h3.Profile
	switch *profileName {
	case "official":
		profile = h3.OfficialProfile()
	case "comfyui":
		profile = h3.ComfyUIProfile()
	default:
		fmt.Fprintln(errOut, "unsupported profile")
		return 2
	}
	doc, err := contextir.Decode(in)
	if err != nil {
		return report(errOut, err)
	}
	if command == "validate" {
		if err := contextir.Validate(doc); err != nil {
			return report(errOut, err)
		}
		if err := json.NewEncoder(out).Encode(map[string]any{"valid": true, "version": doc.Version}); err != nil {
			return 1
		}
		return 0
	}
	artifact, err := h3.Compile(doc, profile)
	if err != nil {
		return report(errOut, err)
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(artifact); err != nil {
		return 1
	}
	return 0
}

func report(out io.Writer, err error) int {
	var typed *contextir.Error
	if errors.As(err, &typed) && typed != nil {
		_ = json.NewEncoder(out).Encode(struct {
			Code   contextir.Code    `json:"code"`
			Stage  string            `json:"stage"`
			Issues []contextir.Issue `json:"issues,omitempty"`
		}{typed.Code, typed.Stage, typed.Issues})
	} else {
		fmt.Fprintln(out, "h3-context-ir-go operation failed")
	}
	return 1
}
