// Command h3-context-ir-go-score scores archived final output and compares stored
// runs offline. It has no provider configuration, credential lookup or HTTP IO.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/evaluate"
	"github.com/ly95/h3-context-ir-go/h3"
)

func main() {
	code, err := run(os.Args[1:], os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}
func read(path string, stdin io.Reader) ([]byte, error) {
	var r io.Reader = stdin
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	const max = 32 << 20
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if len(b) > max {
		return nil, errors.New("input exceeds32MiB limit")
	}
	return b, nil
}
func decode(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}
func emit(path string, value any, stdout io.Writer) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "" || path == "-" {
		_, err = stdout.Write(data)
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	ce := f.Close()
	if err != nil {
		return err
	}
	return ce
}

func run(args []string, stdin io.Reader, stdout io.Writer) (int, error) {
	if len(args) == 0 {
		return 2, errors.New("usage: h3-context-ir-go-score score|compare|baseline [flags]")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	switch args[0] {
	case "score":
		input := fs.String("in", "-", "JSON input path or -")
		kind := fs.String("kind", "artifact", "artifact, document, result or case")
		name := fs.String("name", "", "case label")
		out := fs.String("out", "-", "new JSON path or -")
		asRun := fs.Bool("as-run", false, "emit Run containing score+aggregate+provenance")
		planPath := fs.String("plan", "", "optional original Plan JSON (shots/body/word_target)")
		commit := fs.String("commit", "", "source revision provenance")
		note := fs.String("note", "", "run note")
		if err := fs.Parse(args[1:]); err != nil {
			return 2, err
		}
		if fs.NArg() != 0 {
			return 2, errors.New("unexpected positional arguments")
		}
		data, err := read(*input, stdin)
		if err != nil {
			return 2, err
		}
		options := evaluate.ArtifactOptions{Name: *name}
		if *planPath != "" {
			b, e := read(*planPath, stdin)
			if e != nil {
				return 2, e
			}
			if e = decode(b, &options.Plan); e != nil {
				return 2, e
			}
		}
		var score evaluate.Score
		model := ""
		profile := ""
		switch *kind {
		case "document":
			var doc evaluate.DocumentInput
			if err = decode(data, &doc); err != nil {
				return 2, err
			}
			score = evaluate.ScoreDocument(doc, *name)
		case "artifact":
			var a h3.Artifact
			if err = decode(data, &a); err != nil {
				return 2, err
			}
			profile = a.Profile.Name
			score, err = evaluate.ScoreArtifact(a, options)
		case "result", "case":
			var result compiler.Result
			elapsed := 0.0
			if *kind == "case" {
				var wrapper struct {
					CaseID  string          `json:"case_id"`
					Result  compiler.Result `json:"result"`
					Status  string          `json:"status"`
					Elapsed float64         `json:"elapsed_ms"`
				}
				if e := json.Unmarshal(data, &wrapper); e != nil {
					return 2, e
				}
				if wrapper.Status != "compiled" {
					return 1, errors.New("case did not compile; preserve it as a Run.errors entry rather than scoring an absent artifact")
				}
				result = wrapper.Result
				elapsed = wrapper.Elapsed / 1000
				if options.Name == "" {
					options.Name = wrapper.CaseID
				}
			} else if e := decode(data, &result); e != nil {
				return 2, e
			}
			if result.Status != "compiled" {
				return 1, errors.New("result did not compile")
			}
			options.Findings = result.Findings
			if len(options.Plan.Shots) == 0 {
				for range result.Structure.Shots {
					options.Plan.Shots = append(options.Plan.Shots, evaluate.Shot{})
				}
			}
			attempts := 0
			models := map[string]bool{}
			for _, call := range result.Calls {
				if call.Stage == "plan" {
					attempts++
				}
				if call.Model != "" {
					models[call.Model] = true
				}
			}
			if attempts > 0 {
				options.Provenance.FixRounds = attempts - 1
			}
			names := []string{}
			for m := range models {
				names = append(names, m)
			}
			sort.Strings(names)
			model = strings.Join(names, ",")
			profile = result.Artifact.Profile.Name
			score, err = evaluate.ScoreArtifact(result.Artifact, options)
			score.WallSeconds = elapsed
		default:
			return 2, errors.New("unsupported input kind")
		}
		if err != nil {
			return 2, err
		}
		var value any = score
		if *asRun {
			value = evaluate.Run{Config: evaluate.Config{Label: score.Name, Note: *note, Profile: profile, Omit: []string{}, Only: []string{}}, Scores: []evaluate.Score{score}, Aggregate: evaluate.AggregateScores([]evaluate.Score{score}), Commit: *commit, Model: model, Errors: []string{}, WallSeconds: score.WallSeconds}
		}
		if err = emit(*out, value, stdout); err != nil {
			return 2, err
		}
		if !score.OK {
			return 1, nil
		}
		return 0, nil
	case "compare":
		base := fs.String("baseline", "", "stored baseline Run JSON")
		cand := fs.String("candidate", "", "candidate Run JSON")
		out := fs.String("out", "-", "new report path or -")
		if err := fs.Parse(args[1:]); err != nil {
			return 2, err
		}
		if fs.NArg() != 0 || *base == "" || *cand == "" {
			return 2, errors.New("explicit -baseline and -candidate paths required")
		}
		baseline, err := evaluate.LoadBaseline(*base)
		if err != nil {
			return 2, err
		}
		data, err := read(*cand, stdin)
		if err != nil {
			return 2, err
		}
		r, err := evaluate.DecodeRun(data)
		if err != nil {
			return 2, err
		}
		gate, err := evaluate.Gate(r, baseline)
		if err != nil {
			return 2, err
		}
		value := struct {
			evaluate.GateResult
			Baseline  evaluate.Run `json:"baseline_provenance"`
			Candidate evaluate.Run `json:"candidate_provenance"`
		}{gate, baseline.Run, r}
		if err = emit(*out, value, stdout); err != nil {
			return 2, err
		}
		if !gate.OK {
			return 1, nil
		}
		return 0, nil
	case "baseline":
		input := fs.String("in", "-", "Run JSON path")
		out := fs.String("out", "", "new baseline path")
		if err := fs.Parse(args[1:]); err != nil {
			return 2, err
		}
		if fs.NArg() != 0 || *out == "" {
			return 2, errors.New("a new -out baseline path is required")
		}
		data, err := read(*input, stdin)
		if err != nil {
			return 2, err
		}
		r, err := evaluate.DecodeRun(data)
		if err != nil {
			return 2, err
		}
		if err = evaluate.SaveBaseline(*out, r); err != nil {
			return 2, err
		}
		return 0, nil
	default:
		return 2, errors.New("unknown offline score command")
	}
}
