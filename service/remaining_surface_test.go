package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	contextir "github.com/ly95/contextir"
	"github.com/ly95/contextir/compiler"
	"github.com/ly95/contextir/plan"
)

func remainingContractFields(t *testing.T, section string, model any) {
	t.Helper()
	declared := Snapshot()[section].([]any)
	typ := reflect.TypeOf(model)
	actual := []string{}
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name != "-" {
			actual = append(actual, name)
		}
	}
	got := []string{}
	for _, f := range declared {
		got = append(got, f.(string))
	}
	if !reflect.DeepEqual(got, actual) {
		t.Fatal("published ordered names differ from actual wire model", got, actual)
	}
	data, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]any
	_ = json.Unmarshal(data, &encoded)
	if len(encoded) != len(got) {
		t.Fatal("the encoded wire model and contract disagree", encoded, got)
	}
	for _, name := range got {
		if _, ok := encoded[name]; !ok {
			t.Fatal("unpublished actual wire field", name)
		}
	}
}

// Source test_contract.py::test_the_contract_names_exactly_the_fields_the_wire_models_take[asset_fields-AssetIn].
func TestRemainingSurfaceContractAssetFields(t *testing.T) {
	remainingContractFields(t, "asset_fields", AssetIn{})
}

// Source test_contract.py::test_the_contract_names_exactly_the_fields_the_wire_models_take[brief_fields-BriefIn].
func TestRemainingSurfaceContractBriefFields(t *testing.T) {
	remainingContractFields(t, "brief_fields", BriefIn{})
}

// Source test_contract.py::test_the_contract_names_exactly_the_fields_the_wire_models_take[dialogue_fields-DialogueIn].
func TestRemainingSurfaceContractDialogueFields(t *testing.T) {
	remainingContractFields(t, "dialogue_fields", DialogueIn{})
}

// Source test_contract.py::test_every_role_belongs_to_at_least_one_kind.
func TestRemainingSurfaceEveryRoleHasKind(t *testing.T) {
	placed := map[string]bool{}
	for _, roles := range RolesByKind() {
		for _, r := range roles {
			placed[r] = true
		}
	}
	for _, role := range compiler.Roles {
		if !placed[string(role)] {
			t.Error("native enum role missing from all published kinds", role)
		}
	}
	if len(compiler.Roles) != 16 {
		t.Fatal("review the independent role cases when the enum changes")
	}
}

// Source test_contract.py::test_no_kind_claims_a_role_that_is_not_a_role.
func TestRemainingSurfaceKindsOnlyClaimKnownRoles(t *testing.T) {
	known := map[string]bool{}
	for _, r := range compiler.Roles {
		known[string(r)] = true
	}
	for kind, roles := range RolesByKind() {
		seen := map[string]bool{}
		for _, role := range roles {
			if !known[role] || seen[role] {
				t.Error(kind, role)
			}
			seen[role] = true
		}
	}
}

// Source test_contract.py::test_the_contract_lists_every_refusal_the_service_can_raise.
// Go AST scans actual production constructors (not this contract table). Source
// compiler refusal codes also remain legal on the explicitly injected Compile
// port, and every published code is exercised through its real JSON writer.
func TestRemainingSurfacePublishedRefusalsCoverNativeEmitters(t *testing.T) {
	found := map[string]bool{}
	set := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") || path == "contract.go" {
			continue
		}
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		// publicError's native typed-error mapping carries codes through
		// variables/map values rather than direct constructor string arguments.
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "publicError" {
				continue
			}
			pattern := regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+$`)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					value, _ := strconv.Unquote(lit.Value)
					if pattern.MatchString(value) {
						found[value] = true
					}
				}
				return true
			})
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				fn, ok := x.Fun.(*ast.Ident)
				if ok && fn.Name == "refusal" && len(x.Args) > 1 {
					if lit, ok := x.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						code, _ := strconv.Unquote(lit.Value)
						found[code] = true
					}
				}
			case *ast.CompositeLit:
				if name, ok := x.Type.(*ast.Ident); ok && name.Name == "Error" {
					for _, el := range x.Elts {
						kv, ok := el.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						key, ok := kv.Key.(*ast.Ident)
						if ok && key.Name == "Code" {
							if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
								code, _ := strconv.Unquote(lit.Value)
								found[code] = true
							}
						}
					}
				}
			}
			return true
		})
	}
	if len(found) < 10 || !found["internal-error"] || !found["asset-not-uploaded"] {
		t.Fatal("native AST refusal scan was vacuous", found)
	}
	for code := range found {
		if _, ok := errorSpecs[code]; !ok {
			t.Error("actual native refusal is not published", code)
		}
	}
	for code, spec := range errorSpecs {
		w := httptest.NewRecorder()
		writeError(w, &Error{Code: code, Status: spec["status"].(int), Message: "explicit adapter refusal"})
		var body struct {
			Detail Error `json:"detail"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Detail.Code != code || w.Code != spec["status"].(int) {
			t.Fatal("published typed adapter refusal cannot cross the real boundary", code, w.Code)
		}
	}
	// Native extensions have actual routes, not merely entries in a new table.
	s := testService(t, func(context.Context, plan.Brief) (compiler.Result, error) {
		return compiler.Result{}, errors.New("private cause")
	}, nil)
	status, body := call(t, s, "POST", "/v1/briefs", []byte(`{"intent":"x"}`))
	if status != 500 || body["detail"].(map[string]any)["code"] != "internal-error" {
		t.Fatal(status, body)
	}
	r := httptest.NewRequest("PUT", "/v1/assets/"+strings.Repeat("a", 64), strings.NewReader("x"))
	r.Header.Set("Content-Length", "not-a-length")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var b map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	if w.Code != 422 || b["detail"].(map[string]any)["code"] != "malformed-request" {
		t.Fatal(w.Code, b)
	}
	if !reflect.DeepEqual(errorSpecs["malformed-request"]["on"], []string{"assets", "briefs"}) {
		t.Fatal("actual upload refusal route is hidden")
	}
}

// Source test_contract.py::test_every_published_refusal_states_a_status_and_a_route.
func TestRemainingSurfacePublishedRefusalStatusAndRoute(t *testing.T) {
	for code, spec := range errorSpecs {
		status, ok := spec["status"].(int)
		if !ok || status < 400 || status >= 600 {
			t.Error(code, spec)
		}
		routes, ok := spec["on"].([]string)
		if !ok || len(routes) == 0 {
			t.Fatal(code, spec)
		}
		for _, r := range routes {
			if r != "briefs" && r != "assets" {
				t.Error(code, r)
			}
		}
	}
}

// Source test_every_role_renders.py::test_every_role_in_the_enum_has_a_case_here.
// This independent table is extracted from the original ROLE_CASES, not built
// from the current enum that it checks.
func TestRemainingSurfaceOriginalRoleCaseCompleteness(t *testing.T) {
	var fixture struct {
		Source string `json:"source_commit"`
		Cases  []struct {
			Role string `json:"role"`
			Kind string `json:"kind"`
			Mode string `json:"mode"`
		} `json:"role_cases"`
	}
	data, err := os.ReadFile("../third_party/open-h3-ir/testdata/surface_manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(data)
	if hex.EncodeToString(h[:]) != "9e4519f4c9f1d246c0fac4a19db21de30fb9a00638ceb8b17a9de29ab03d0195" {
		t.Fatal("independent original role table changed")
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Source != compiler.UpstreamRevision || len(fixture.Cases) != 16 {
		t.Fatal("original fixed role table missing")
	}
	got := []string{}
	for _, c := range fixture.Cases {
		if c.Kind == "" || c.Mode == "" {
			t.Fatal(c)
		}
		got = append(got, c.Role)
	}
	want := []string{}
	for _, role := range compiler.Roles {
		want = append(want, string(role))
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("native role is not covered by the independent original role cases", got, want)
	}
}

// Native version3 deliberately adds the BGM-copy field and two refusal routes.
// Covers test_contract.py::test_the_service_publishes_one_statement_of_the_lists_it_shares_with_the_contract
// and ::test_the_version_moves_when_any_part_of_the_contract_moves.
func TestRemainingSurfaceContractVersionAndCapabilities(t *testing.T) {
	s := testService(t, nil, nil)
	_, caps := call(t, s, "GET", "/v1/capabilities", nil)
	if ContractVersion != 3 || caps["contract"].(map[string]any)["version"] != float64(ContractVersion) || Snapshot()["contract_version"] != float64(ContractVersion) {
		t.Fatal("actual native contract version is not shared", caps)
	}
}

func TestRemainingSurfaceAudioCopyModeWireRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.wav")
	if err := os.WriteFile(path, []byte("caller-provided audio bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"fully_copy", "partially_copy"} {
		payload, _ := json.Marshal(map[string]any{"intent": "explicit audio relationship", "assets": []map[string]any{{"path": path, "kind": "audio", "role": "bgm"}}, "audio_copy_modes": map[string]string{"asset-1": mode}})
		var received map[string]string
		s := testService(t, func(_ context.Context, brief plan.Brief) (compiler.Result, error) {
			received = brief.AudioCopyModes
			return compiler.Result{}, refusal(503, "llm-unavailable", "explicit test boundary, no provider")
		}, nil)
		s.cfg.AllowPaths = true
		status, _ := call(t, s, "POST", "/v1/briefs", payload)
		if status != 503 || received["asset-1"] != mode {
			t.Fatal("HTTP decoder dropped the explicit copy relationship", status, received)
		}
		original, err := PrepareRequest(PrepareConfig{AllowPaths: true}, payload)
		if err != nil {
			t.Fatal(err)
		}
		native, err := compiler.FromOriginal(original, nil)
		if err != nil || native.AudioCopyModes["asset-1"] != contextir.Retention(mode) {
			t.Fatal("wire copy choice dropped", native.AudioCopyModes, err)
		}
		stored, err := amendedBrief(original, native, native)
		if err != nil || stored.AudioCopyModes["asset-1"] != mode {
			t.Fatal("refinement storage dropped explicit copy choice", stored.AudioCopyModes, err)
		}
	}
	// The source-compatible DTO has positional IDs. Removing the first
	// attachment must not leave the retained BGM's choice on old asset-2.
	original := plan.Brief{Intent: "x", Seconds: 5, Aspect: "16:9", Assets: []plan.AssetRef{
		{Kind: "image", Role: "subject", Path: &path}, {Kind: "audio", Role: "bgm", Path: &path},
	}, AudioCopyModes: map[string]string{"asset-2": "fully_copy"}}
	before, err := compiler.FromOriginal(original, nil)
	if err != nil {
		t.Fatal(err)
	}
	after := before
	after.Assets = append([]compiler.Asset(nil), before.Assets[1:]...)
	stored, err := amendedBrief(original, before, after)
	if err != nil || len(stored.AudioCopyModes) != 1 || stored.AudioCopyModes["asset-1"] != "fully_copy" {
		t.Fatal("copy choice no longer follows retained source after removal", stored.AudioCopyModes, err)
	}
	replayed, err := compiler.FromOriginal(stored, nil)
	if err != nil || replayed.Assets[0].Kind != contextir.MediaAudio || replayed.AudioCopyModes[replayed.Assets[0].ID] != contextir.FullyCopy {
		t.Fatal("source DTO replay changed the selected audio relationship", replayed.AudioCopyModes, err)
	}
	for _, value := range []any{nil, map[string]string{"asset-1": "reference"}, map[string]string{"unknown": "fully_copy"}} {
		payload, _ := json.Marshal(map[string]any{"intent": "x", "assets": []map[string]any{{"path": path, "kind": "audio", "role": "bgm"}}, "audio_copy_modes": value})
		if _, err := PrepareRequest(PrepareConfig{AllowPaths: true}, payload); err == nil {
			t.Fatal("invalid copy choice was silently admitted", value)
		}
	}
}
