package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"

	contextir "github.com/ly95/contextir"
	"github.com/ly95/contextir/compiler"
	"github.com/ly95/contextir/h3"
	"github.com/ly95/contextir/uploads"
)

func round(v float64, places int) float64 { p := math.Pow10(places); return math.RoundToEven(v*p) / p }
func writeJSON(w http.ResponseWriter, status int, v any) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		status = 500
		b.Reset()
		b.WriteString("{\"detail\":{\"code\":\"internal-error\",\"message\":\"response could not be encoded\"}}\n")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b.Bytes())
}
func publicError(err error) *Error {
	var se *Error
	if errors.As(err, &se) {
		return se
	}
	var ue *uploads.Error
	if errors.As(err, &ue) {
		status := 422
		switch ue.Code {
		case uploads.TooLarge:
			status = 413
		case uploads.StoreFull:
			status = 507
		}
		return refusal(status, string(ue.Code), ue.Message)
	}
	var ce *contextir.Error
	if errors.As(err, &ce) {
		messages := []string{}
		for _, issue := range ce.Issues {
			messages = append(messages, issue.Message)
		}
		message := strings.Join(messages, "\n")
		if message == "" {
			message = ce.Error()
		}
		if ce.Code == contextir.CodeLimit && ce.Stage == "capacity" {
			return refusal(422, "over-capacity", message)
		}
		switch ce.Code {
		case contextir.CodeModel, contextir.CodeModelOutput:
			return refusal(502, "llm-error", ce.Error())
		case contextir.CodePerception:
			return refusal(422, "asset-unreadable", message)
		case contextir.CodeCanceled:
			return refusal(503, "llm-unavailable", "compilation was canceled")
		case contextir.CodeInvalid, contextir.CodeLimit, contextir.CodeUnsupported:
			code := "malformed-request"
			if len(ce.Issues) > 0 {
				p := strings.Split(ce.Issues[0].Path, ".")[0]
				if c, ok := map[string]string{"intent": "intent-empty", "seconds": "duration-invalid", "target": "duration-invalid", "aspect": "aspect-invalid", "shots": "shots-invalid", "director": "director-profile-invalid", "director_profile": "director-profile-invalid"}[p]; ok {
					code = c
				}
				return refusal(422, code, ce.Issues[0].Message)
			}
			return refusal(422, code, ce.Error())
		}
	}
	return refusal(500, "internal-error", "the service could not complete this request")
}
func writeError(w http.ResponseWriter, err error) {
	e := publicError(err)
	writeJSON(w, e.Status, map[string]any{"detail": e})
}
func writeCompileError(w http.ResponseWriter, err error, result compiler.Result) {
	e := publicError(err)
	writeJSON(w, e.Status, map[string]any{"detail": e, "calls": result.Calls, "usage_complete": result.UsageComplete, "unmetered_asset_ids": result.UnmeteredAssetIDs})
}
func (s *Service) body(r *http.Request) ([]byte, error) {
	if r.ContentLength > s.cfg.MaxRequestBytes {
		return nil, malformed("the request", "Request exceeds the configured byte limit")
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, s.cfg.MaxRequestBytes+1))
	if e != nil {
		return nil, malformed("the request", "Request body could not be read")
	}
	if int64(len(b)) > s.cfg.MaxRequestBytes {
		return nil, malformed("the request", "Request exceeds the configured byte limit")
	}
	return b, nil
}
func method(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	for _, m := range allowed {
		if m == r.Method {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeJSON(w, 405, map[string]any{"detail": "Method Not Allowed"})
	return false
}

// ServeHTTP exposes no read route for uploaded assets. Public media URIs remain
// input locators; this adapter never fetches them or serves their bytes.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if r.URL.Path == "/health" {
		if !method(w, r, "GET") {
			return
		}
		if s.cfg.Health == nil {
			writeError(w, refusal(503, "llm-unavailable", "compile dependency readiness has not been configured"))
			return
		}
		var probe bool
		if s.cfg.Health != nil {
			var e error
			probe, e = s.cfg.Health(r.Context())
			if e != nil {
				writeError(w, refusal(503, "llm-unavailable", "compile dependencies are unavailable"))
				return
			}
		}
		if !probe {
			writeError(w, refusal(503, "llm-unavailable", "compile dependencies are unavailable"))
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "llm": probe, "profile": s.cfg.Profile})
		return
	}
	if len(p) == 2 && p[0] == "v1" {
		switch p[1] {
		case "contract":
			if method(w, r, "GET") {
				writeJSON(w, 200, Contract(s.cfg.PackageVersion))
			}
			return
		case "capabilities":
			if method(w, r, "GET") {
				writeJSON(w, 200, s.Capabilities())
			}
			return
		case "directors":
			if method(w, r, "GET") {
				writeJSON(w, 200, map[string]any{"governs": "whatever the request and the references leave open. An explicit instruction in the request outranks a profile. Shot count and cut times are never a profile's: pin `shots` and the count is yours, leave it unset and the writer decides the edit.", "camera_moves": compiler.CameraTypes, "directors": directorMappings()})
			}
			return
		case "loras":
			if method(w, r, "GET") {
				s.loras(w)
			}
			return
		case "briefs":
			if method(w, r, "POST") {
				s.create(w, r)
			}
			return
		}
	}
	if len(p) == 3 && p[0] == "v1" && p[1] == "assets" && p[2] != "" {
		if method(w, r, "PUT") {
			s.upload(w, r, p[2])
		}
		return
	}
	if len(p) == 3 && p[0] == "v1" && p[1] == "directors" {
		if !method(w, r, "GET") {
			return
		}
		name := strings.ToLower(strings.TrimSpace(p[2]))
		for _, d := range directorMappings() {
			if d["id"] == name {
				writeJSON(w, 200, d)
				return
			}
		}
		writeJSON(w, 404, map[string]any{"detail": fmt.Sprintf("no director called %q. The list is at GET /v1/directors.", p[2])})
		return
	}
	if (len(p) == 3 || len(p) == 4 && p[3] == "prompt") && p[0] == "v1" && p[1] == "briefs" && p[2] != "" {
		if len(p) == 4 {
			if !method(w, r, "GET") {
				return
			}
		} else if !method(w, r, "GET", "PATCH") {
			return
		}
		rec, ok, e := s.cfg.Records.Get(r.Context(), p[2])
		if e != nil {
			writeError(w, e)
			return
		}
		if !ok {
			writeError(w, refusal(404, "unknown-brief", p[2]))
			return
		}
		if len(p) == 4 {
			if rec.Result.Status == "needs_input" {
				writeError(w, refusal(422, "malformed-request", "An image role answer is required before a renderable prompt exists. POST the brief again with `answer.anchor_or_reference` set to an offered option."))
				return
			}
			writeJSON(w, 200, promptLayer(rec.Result))
			return
		}
		if r.Method == "PATCH" {
			if rec.Result.Status == "needs_input" {
				writeError(w, refusal(422, "malformed-request", "Answer the pending image role question before refining. POST the original brief with `answer.anchor_or_reference`."))
				return
			}
			s.refine(w, r, p[2], rec)
			return
		}
		writeJSON(w, 200, envelope(p[2], rec))
		return
	}
	writeJSON(w, 404, map[string]any{"detail": "Not Found"})
}
func (s *Service) drainBudget() int64 {
	m := s.cfg.Store.Config().MaxBytes
	if m > math.MaxInt64-(1<<20) {
		return math.MaxInt64
	}
	return max(8<<20, m+(1<<20))
}
func (s *Service) drainRefused(body io.Reader) {
	if e := Drain(body, s.drainBudget()); e != nil {
		// FALLBACK: the bounded drain is best effort after a specific refusal has
		// already been established. A broken peer connection must not replace that
		// refusal with an unrelated internal error; no uploaded bytes are published.
		return
	}
}
func (s *Service) upload(w http.ResponseWriter, r *http.Request, digest string) {
	p, have, e := s.cfg.Store.Stored(digest)
	if e != nil {
		s.drainRefused(r.Body)
		writeError(w, e)
		return
	}
	if have {
		if e = Drain(r.Body, s.drainBudget()); e != nil {
			writeError(w, e)
			return
		}
		if e = s.cfg.Store.Touch(p); e != nil {
			writeError(w, e)
			return
		}
		st, e := os.Stat(p)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"sha256": digest, "stored": true, "bytes": st.Size(), "note": "already held; the bytes sent were not needed"})
		return
	}
	var declared *int64
	if raw := r.Header.Get("Content-Length"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			s.drainRefused(r.Body)
			writeError(w, malformed("Content-Length", "Input should be a nonnegative integer"))
			return
		}
		declared = &v
	} else if r.ContentLength >= 0 {
		v := r.ContentLength
		declared = &v
	}
	_, size, e := s.cfg.Store.Put(digest, declared, r.Body)
	if e != nil {
		var ue *uploads.Error
		if errors.As(e, &ue) && (ue.Code == uploads.TooLarge || ue.Code == uploads.StoreFull) {
			s.drainRefused(r.Body)
		}
		writeError(w, e)
		return
	}
	writeJSON(w, 201, map[string]any{"sha256": digest, "stored": true, "bytes": size})
}
func (s *Service) loras(w http.ResponseWriter) {
	rs := []any{}
	fs := []map[string]string{}
	revision := ""
	if registry := s.cfg.Registry; registry != nil {
		revision = registry.Revision
		keys := make([]string, 0, len(registry.Records))
		for k := range registry.Records {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			rs = append(rs, registry.Records[k].Public())
		}
		for _, f := range registry.Findings {
			fs = append(fs, map[string]string{"rule": f.Rule, "severity": f.Severity, "message": f.Msg})
		}
	}
	writeJSON(w, 200, map[string]any{"revision": revision, "loras": rs, "problems": fs})
}
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
func validateResult(r compiler.Result) error {
	if r.ModeDecision != nil {
		d := r.ModeDecision
		if d.NeedsClarification != (len(d.Questions) > 0) {
			return refusal(500, "internal-error", "mode clarification flag and typed questions disagree")
		}
		for _, q := range d.Questions {
			if q.ID == "" || strings.TrimSpace(q.Ask) == "" || len(q.Options) < 2 {
				return refusal(500, "internal-error", "mode clarification is incomplete")
			}
			seen := map[string]bool{}
			for _, o := range q.Options {
				if o.ID == "" || o.Label == "" || seen[o.ID] {
					return refusal(500, "internal-error", "mode clarification options are invalid")
				}
				seen[o.ID] = true
			}
			if !seen[q.DefaultIfUnanswered] || !seen[q.Recommended] {
				return refusal(500, "internal-error", "mode clarification default is not one of its options")
			}
		}
	}
	if r.Status == "needs_input" {
		if r.ModeDecision == nil || !r.ModeDecision.NeedsClarification {
			return refusal(500, "internal-error", "pending compile has no typed clarification")
		}
		if r.Artifact.Prompt != "" || r.Artifact.SHA256 != "" {
			return refusal(500, "internal-error", "unresolved compile must not publish a renderable artifact")
		}
		return nil
	}
	if r.ModeDecision != nil && r.ModeDecision.NeedsClarification && r.ClarificationPolicy != compiler.ClarificationContinue {
		return refusal(500, "internal-error", "unresolved compile was marked ready for publication")
	}
	if r.ModeDecision != nil && r.ModeDecision.NeedsClarification && r.ModeDecision.ClarificationPolicy != compiler.ClarificationContinue {
		return refusal(500, "internal-error", "continued clarification policy was not recorded in the mode decision")
	}
	if r.ModeDecision != nil && r.ModeDecision.Mode != "" && r.ModeDecision.Mode != r.Document.Mode {
		return refusal(500, "internal-error", "published mode differs from the recorded mode decision")
	}
	if r.Status != "compiled" && r.Status != "draft" {
		return refusal(500, "internal-error", "compile returned no published document")
	}
	artifact, e := h3.Compile(r.Document, r.Artifact.Profile)
	if e != nil {
		return &Error{Status: 500, Code: "internal-error", Message: "compiled document did not pass publication checks", Cause: e}
	}
	if artifact.SHA256 != r.Artifact.SHA256 {
		return refusal(500, "internal-error", "compiled prompt and authoritative document do not agree")
	}
	return nil
}
func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	b, e := s.body(r)
	if e != nil {
		writeError(w, e)
		return
	}
	brief, e := s.PrepareBrief(b)
	if e != nil {
		writeError(w, e)
		return
	}
	result, e := s.withUploadedDependencies(r.Context(), brief, func(ctx context.Context) (compiler.Result, error) {
		return s.cfg.Compile(ctx, brief)
	})
	if e != nil {
		writeCompileError(w, e, result)
		return
	}
	if e = validateResult(result); e != nil {
		writeCompileError(w, e, result)
		return
	}
	var raw [8]byte
	if _, e = rand.Read(raw[:]); e != nil {
		writeCompileError(w, e, result)
		return
	}
	id := hex.EncodeToString(raw[:])
	rec, e := s.cfg.Records.Save(r.Context(), id, Record{Brief: brief, Result: result})
	if e != nil {
		writeCompileError(w, e, result)
		return
	}
	status, out := resultEnvelope(id, rec)
	if status == 200 {
		status = 201
	}
	writeJSON(w, status, out)
}
func (s *Service) refine(w http.ResponseWriter, r *http.Request, id string, rec Record) {
	raw, e := s.body(r)
	if e != nil {
		writeError(w, e)
		return
	}
	obj, e := knownKeys(raw, []string{"change"}, "")
	if e != nil {
		writeError(w, e)
		return
	}
	if e = required(obj, "change", ""); e != nil {
		writeError(w, e)
		return
	}
	var body struct {
		Change string `json:"change"`
	}
	if e = json.Unmarshal(raw, &body); e != nil {
		writeError(w, malformed("change", e.Error()))
		return
	}
	if strings.TrimSpace(body.Change) == "" {
		writeError(w, refusal(422, "change-empty", "`change` is empty, so there is nothing to apply. Say what to change in plain language."))
		return
	}
	result, e := s.withUploadedDependencies(r.Context(), rec.Brief, func(ctx context.Context) (compiler.Result, error) {
		return s.cfg.Refine(ctx, rec.Result, body.Change)
	})
	if e != nil {
		writeCompileError(w, e, result)
		return
	}
	if e = validateResult(result); e != nil {
		writeCompileError(w, e, result)
		return
	}
	previous := rec.Result
	rec.Result = result
	rec.Brief, e = amendedBrief(rec.Brief, previous.Brief, result.Brief, result.Decisions)
	if e != nil {
		writeCompileError(w, e, result)
		return
	}
	rec, e = s.cfg.Records.Save(r.Context(), id, rec)
	if e != nil {
		writeCompileError(w, e, result)
		return
	}
	status, out := resultEnvelope(id, rec)
	out["version"] = rec.Version
	out["changed"] = changedFields(previous.Brief, result.Brief)
	writeJSON(w, status, out)
}
func changedFields(a, b compiler.Brief) []string {
	out := []string{}
	ta, tb := reflect.ValueOf(a), reflect.ValueOf(b)
	t := ta.Type()
	for i := 0; i < ta.NumField(); i++ {
		if !reflect.DeepEqual(ta.Field(i).Interface(), tb.Field(i).Interface()) {
			out = append(out, strings.Split(t.Field(i).Tag.Get("json"), ",")[0])
		}
	}
	return out
}
func resultEnvelope(id string, rec Record) (int, map[string]any) {
	out := envelope(id, rec)
	errs := []map[string]string{}
	caller := false
	rules := map[string]bool{"T6": true, "T7": true, "M5": true, "M6": true, "M7": true, "L3": true, "X10": true, "X11": true, "X12": true, "X13": true, "X15": true}
	for _, f := range rec.Result.Findings {
		if f.Severity == "ERROR" {
			errs = append(errs, map[string]string{"rule": f.Rule, "message": f.Msg})
			if rules[strings.Split(f.Rule, "-")[0]] {
				caller = true
			}
		}
	}
	if len(errs) > 0 {
		out["status"] = "invalid"
		out["errors"] = errs
		if caller {
			return 422, out
		}
		return 500, out
	}
	return 200, out
}
