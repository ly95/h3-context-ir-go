package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/h3"
	"github.com/ly95/h3-context-ir-go/media"
	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/service"
	"github.com/ly95/h3-context-ir-go/uploads"
)

var serviceRecords = map[string]service.RecordStore{}

type sourceBodyReader struct {
	pending []byte
	ended   bool
}

func (r *sourceBodyReader) Read(p []byte) (int, error) {
	for len(r.pending) == 0 && !r.ended {
		reply, e := invokePythonCallback("service.body_next", nil)
		if e != nil {
			return 0, e
		}
		if reply.Error != nil {
			return 0, fmt.Errorf("source body callback: %s", reply.Error.Message)
		}
		var value struct {
			Data       []byte `json:"data"`
			EOF        bool   `json:"eof"`
			Disconnect bool   `json:"disconnect"`
		}
		if e := json.Unmarshal(reply.Value, &value); e != nil {
			return 0, e
		}
		r.pending = value.Data
		r.ended = value.EOF || value.Disconnect
	}
	if len(r.pending) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func serviceCallback(name string, payload any) (compiler.Result, error) {
	reply, e := invokePythonCallback(name, payload)
	if e != nil {
		return compiler.Result{}, e
	}
	if reply.Error != nil {
		return compiler.Result{}, &service.Error{Status: reply.Error.Status, Code: reply.Error.Code, Message: reply.Error.Message}
	}
	var result compiler.Result
	if e := json.Unmarshal(reply.Value, &result); e != nil {
		return result, e
	}
	return result, nil
}

func serviceOperation(name string, b json.RawMessage) (any, error) {
	if name == "h3ir.compile.check_request" || name == "h3ir.compile.check_capacity" {
		var a struct {
			Brief plan.Brief `json:"brief"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		input, e := compiler.FromOriginal(a.Brief, nil)
		if e != nil {
			return nil, e
		}
		adapter, e := compiler.New(nil, nil, compiler.Options{Profile: h3.ComfyUIProfile(), FramePolicy: compiler.FramePolicySnapUp})
		if e != nil {
			return nil, e
		}
		if name == "h3ir.compile.check_capacity" {
			e = adapter.CheckCapacity(input)
		} else {
			e = adapter.CheckRequest(input)
		}
		if e == nil {
			return map[string]any{"ok": true}, nil
		}
		var typed *contextir.Error
		if !errors.As(e, &typed) {
			return nil, e
		}
		code := "malformed-request"
		messages := []string{}
		if typed.Stage == "capacity" {
			code = "over-capacity"
		}
		for _, issue := range typed.Issues {
			messages = append(messages, issue.Message)
			if code != "over-capacity" {
				switch strings.Split(issue.Path, ".")[0] {
				case "intent":
					code = "intent-empty"
				case "seconds", "frame_count":
					code = "duration-invalid"
				case "aspect":
					code = "aspect-invalid"
				case "shots":
					code = "shots-invalid"
				}
			}
		}
		return map[string]any{"ok": false, "code": code, "message": strings.Join(messages, "; ")}, nil
	}
	if strings.HasPrefix(name, "h3ir.contract.") {
		var a struct {
			PackageVersion string `json:"package_version"`
			Value          any    `json:"value"`
		}
		if e := json.Unmarshal(b, &a); e != nil {
			return nil, e
		}
		switch name {
		case "h3ir.contract.snapshot":
			return service.Snapshot(), nil
		case "h3ir.contract.contract":
			return service.Contract(a.PackageVersion), nil
		case "h3ir.contract.digests":
			return service.Digests(), nil
		case "h3ir.contract.digest":
			return service.Digest(a.Value)
		case "h3ir.contract.as_json":
			return service.AsJSON()
		case "h3ir.contract.as_js":
			return service.AsJS(), nil
		}
		return nil, fmt.Errorf("unsupported contract conformance operation %q", name)
	}
	var a struct {
		Config struct {
			Root           string  `json:"root"`
			Max            int64   `json:"max_bytes"`
			Store          int64   `json:"store_bytes"`
			TTL            float64 `json:"ttl_hours"`
			AllowPaths     bool    `json:"allow_paths"`
			Profile        string  `json:"profile"`
			PackageVersion string  `json:"package_version"`
		} `json:"config"`
		Client         string                     `json:"client"`
		Method         string                     `json:"method"`
		URL            string                     `json:"url"`
		Headers        map[string][]string        `json:"headers"`
		Body           []byte                     `json:"body"`
		Budget         int64                      `json:"budget"`
		Seeds          map[string]compiler.Result `json:"seeds"`
		NativeCompiler bool                       `json:"native_compiler"`
	}
	if e := json.Unmarshal(b, &a); e != nil {
		return nil, e
	}
	if name == "h3ir.service._drain" {
		return nil, service.Drain(&sourceBodyReader{}, a.Budget)
	}
	store, e := uploads.New(uploads.Config{Directory: a.Config.Root, MaxBytes: a.Config.Max, StoreBytes: a.Config.Store, TTL: time.Duration(a.Config.TTL * float64(time.Hour)), Grace: 900 * time.Second})
	if e != nil {
		return nil, e
	}
	records := serviceRecords[a.Client]
	if records == nil {
		records = service.NewMemoryRecords()
		serviceRecords[a.Client] = records
	}
	for id, result := range a.Seeds {
		if _, ok, e := records.Get(context.Background(), id); e != nil {
			return nil, e
		} else if !ok {
			if _, e := records.Save(context.Background(), id, service.Record{Result: result}); e != nil {
				return nil, e
			}
		}
	}
	svc, e := service.New(service.Config{Store: store, AllowPaths: a.Config.AllowPaths, Profile: a.Config.Profile, PackageVersion: a.Config.PackageVersion, Records: records, MaxRequestBytes: 16 << 20,
		Compile: func(ctx context.Context, brief plan.Brief) (compiler.Result, error) {
			if a.NativeCompiler {
				input, err := compiler.FromOriginal(brief, nil)
				if err != nil {
					return compiler.Result{}, err
				}
				adapter, err := compiler.New(nil, nil, compiler.Options{Profile: h3.ComfyUIProfile(), FramePolicy: compiler.FramePolicySnapUp})
				if err != nil {
					return compiler.Result{}, err
				}
				if _, err = adapter.ValidateIntake(ctx, input); err != nil {
					return compiler.Result{}, err
				}
				for _, asset := range input.Assets {
					if _, err = media.ValidateDeclaration(asset.URI, asset.Kind); err != nil {
						return compiler.Result{}, err
					}
				}
				return compiler.Result{}, &service.Error{Status: 503, Code: "llm-unavailable", Message: "The offline conformance harness provides no live model; all deterministic intake checks passed."}
			}
			return serviceCallback("service.compile", brief)
		},
		Refine: func(ctx context.Context, result compiler.Result, change string) (compiler.Result, error) {
			return serviceCallback("service.refine", map[string]any{"result": result, "change": change})
		},
	})
	if e != nil {
		return nil, e
	}
	if name == "h3ir.service._to_brief" {
		brief, e := svc.PrepareBrief(a.Body)
		if e != nil {
			var detail *service.Error
			if errors.As(e, &detail) {
				return map[string]any{"ok": false, "status": detail.Status, "detail": detail}, nil
			}
			return nil, e
		}
		return map[string]any{"ok": true, "brief": brief}, nil
	}
	if name != "h3ir.service.http" {
		return nil, fmt.Errorf("unsupported service test operation %q", name)
	}
	req := httptest.NewRequest(a.Method, a.URL, bytes.NewReader(a.Body))
	for key, values := range a.Headers {
		req.Header[key] = values
	}
	if lengths := req.Header.Values("Content-Length"); len(lengths) > 0 { // httptest already owns the exact bytes supplied by the original request.
		req.ContentLength = int64(len(a.Body))
	}
	recorder := httptest.NewRecorder()
	svc.ServeHTTP(recorder, req)
	result := recorder.Result()
	defer result.Body.Close()
	body, e := io.ReadAll(result.Body)
	if e != nil {
		return nil, e
	}
	return map[string]any{"status": result.StatusCode, "headers": result.Header, "body": body}, nil
}
