package contextir_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"go/version"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// These replace the two pinned Python grammar guards in
// tests/test_supported_python_versions.py. Parsing with today's parser is only
// the cheap syntax check. The declared-language guard additionally uses Go's
// compiler/type checking and official stdversion analyzer, without executing
// another go test, downloading a toolchain, or requiring a Git checkout.
func TestGoCompatibilityEverySourceParses(t *testing.T) {
	t.Run("all_runtime_and_test_sources", func(t *testing.T) {
		root := compatibilityModuleRoot(t)
		files, excluded, err := compatibilitySourceFiles(root)
		if err != nil {
			t.Fatal(err)
		}
		runtimeCount, testCount := 0, 0
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				testCount++
			} else {
				runtimeCount++
			}
		}
		if runtimeCount == 0 || testCount == 0 {
			t.Fatal("the module's runtime and test source inventory must both be nonempty")
		}
		if broken := compatibilityParseFiles(files); len(broken) != 0 {
			t.Fatalf("Go source cannot be parsed:\n%s", strings.Join(broken, "\n"))
		}
		t.Logf("parsed %d runtime and %d test files; excluded %d independent modules/metadata directories", runtimeCount, testCount, len(excluded))
	})
	t.Run("malformed_source_is_detected_without_git", func(t *testing.T) {
		root := t.TempDir()
		compatibilityWrite(t, filepath.Join(root, "go.mod"), "module compatibility.invalid/syntax\ngo 1.25\n")
		compatibilityWrite(t, filepath.Join(root, "valid.go"), "package syntax\nfunc Good() {}\n")
		compatibilityWrite(t, filepath.Join(root, "broken_test.go"), "package syntax\nfunc Broken( {\n")
		files, _, err := compatibilitySourceFiles(root)
		if err != nil {
			t.Fatal(err)
		}
		broken := compatibilityParseFiles(files)
		if len(files) != 2 || len(broken) != 1 || !strings.Contains(broken[0], "broken_test.go") {
			t.Fatal("the guard did not reject the malformed test source", files, broken)
		}
	})
	t.Run("independent_nested_modules_are_not_runtime_dependencies", func(t *testing.T) {
		root := t.TempDir()
		compatibilityWrite(t, filepath.Join(root, "go.mod"), "module compatibility.invalid/inventory\ngo 1.25\n")
		compatibilityWrite(t, filepath.Join(root, "runtime.go"), "package inventory\n")
		compatibilityWrite(t, filepath.Join(root, "runtime_test.go"), "package inventory\n")
		// The nested module is deliberate: the distributed Go ZIP omits this
		// reports tree entirely. Neither .git nor reports may be prerequisites.
		compatibilityWrite(t, filepath.Join(root, "conformance", "reports", "go.mod"), "module compatibility.invalid/reports\n")
		compatibilityWrite(t, filepath.Join(root, "conformance", "reports", "unrelated.go"), "not Go source")
		files, excluded, err := compatibilitySourceFiles(root)
		if err != nil || len(files) != 2 || len(excluded) != 1 || len(compatibilityParseFiles(files)) != 0 {
			t.Fatal("an independent report module leaked into the runtime inventory", files, excluded, err)
		}
	})
}

func TestGoCompatibilityDeclaredLanguageVersion(t *testing.T) {
	root := compatibilityModuleRoot(t)
	declared, err := compatibilityDeclaredVersion(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("official_compiler_and_stdversion_guard", func(t *testing.T) {
		// go vet loads the current target's production and test variants. The
		// compiler enforces their language version; stdversion separately checks
		// standard-library references against each file's effective Go version.
		// Config.GoVersion alone does not check when a stdlib API was added.
		out, err := compatibilityVet(root)
		if err != nil {
			t.Fatalf("module declaring %s failed local offline go vet -stdversion:\n%s\n%v", declared, out, err)
		}
		t.Logf("declared language %s checked by local compiler and stdversion; a real minimum-toolchain suite remains a separate, stronger check", declared)
	})
	t.Run("current_parser_is_not_a_minimum_language_proof", func(t *testing.T) {
		// Today's parser accepts this construct, but range-over-integers needs
		// Go 1.22. This is the same class as an old-version-incompatible f-string
		// accepted by a newer Python parser in the original regression.
		const source = "package control\nfunc f() { for range 3 {} }\n"
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "language_control.go", source, parser.AllErrors)
		if err != nil {
			t.Fatal("the current parser must admit the language control", err)
		}
		old := types.Config{GoVersion: "go1.21"}
		if _, err = old.Check("compatibility.invalid/control", fset, []*ast.File{file}, nil); err == nil || !strings.Contains(err.Error(), "go1.22") {
			t.Fatal("the declared-language checker failed to reject the too-new construct", err)
		}
		allowed := types.Config{GoVersion: "go1.22"}
		if _, err = allowed.Check("compatibility.invalid/control", fset, []*ast.File{file}, nil); err != nil {
			t.Fatal("the checker rejected a construct supported by its explicit language version", err)
		}
	})
	t.Run("too_new_standard_library_api_is_rejected", func(t *testing.T) {
		root := t.TempDir()
		compatibilityWrite(t, filepath.Join(root, "go.mod"), "module compatibility.invalid/newapi\ngo 1.23\n")
		compatibilityWrite(t, filepath.Join(root, "api.go"), "package newapi\nimport \"os\"\nfunc open(path string) { _, _ = os.OpenRoot(path) }\n")
		out, err := compatibilityVet(root)
		if err == nil || !strings.Contains(out, "os.OpenRoot") || !strings.Contains(out, "go1.24") || !strings.Contains(out, "requires") {
			t.Fatal("stdversion did not reject an API newer than the fixture's declared Go 1.23", out, err)
		}
	})
	t.Run("supported_standard_library_api_is_accepted", func(t *testing.T) {
		root := t.TempDir()
		compatibilityWrite(t, filepath.Join(root, "go.mod"), "module compatibility.invalid/oldapi\ngo 1.23\n")
		compatibilityWrite(t, filepath.Join(root, "api.go"), "package oldapi\nimport \"os\"\nfunc open(path string) { _, _ = os.Open(path) }\n")
		if out, err := compatibilityVet(root); err != nil {
			t.Fatal("a supported standard-library API was rejected", out, err)
		}
	})
}

func compatibilityModuleRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = compatibilityDeclaredVersion(filepath.Join(root, "go.mod")); err != nil {
		t.Fatal("run this root-package test in its module directory; no Git discovery is used", err)
	}
	return root
}

func compatibilityDeclaredVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	declared := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.SplitN(line, "//", 2)[0])
		if len(fields) == 0 || fields[0] != "go" {
			continue
		}
		if len(fields) != 2 || declared != "" || !version.IsValid("go"+fields[1]) {
			return "", fmt.Errorf("invalid or repeated go language directive in %s", path)
		}
		declared = "go" + fields[1]
	}
	if declared == "" {
		return "", fmt.Errorf("go.mod must explicitly declare the supported Go language version")
	}
	return declared, nil
}

func compatibilitySourceFiles(root string) ([]string, []string, error) {
	var files, excluded []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && path != root {
			if entry.Name() == ".git" || entry.Name() == ".codegraph" {
				excluded = append(excluded, path)
				return filepath.SkipDir
			}
			_, err := os.Stat(filepath.Join(path, "go.mod"))
			if err == nil {
				excluded = append(excluded, path)
				return filepath.SkipDir
			}
			if !os.IsNotExist(err) {
				return err
			}
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	sort.Strings(excluded)
	return files, excluded, err
}

func compatibilityParseFiles(files []string) []string {
	var broken []string
	for _, path := range files {
		if _, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors); err != nil {
			broken = append(broken, err.Error())
		}
	}
	return broken
}

func compatibilityWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func compatibilityVet(root string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	goName := "go"
	if runtime.GOOS == "windows" {
		goName += ".exe"
	}
	// Use the toolchain that built this test, including an actual minimum-version
	// run. Looking up "go" on PATH could silently select a newer installation.
	cmd := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", goName), "vet", "-stdversion", "./...")
	cmd.Dir = root
	// Keep only OS/tool/cache locations. No provider, Git authentication,
	// workspace, overlay, toolchain-download, or parent Go-env override leaks in.
	for _, name := range []string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "WINDIR", "TMPDIR", "TMP", "TEMP", "GOCACHE", "GOMODCACHE", "GOPATH"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	cmd.Env = append(cmd.Env, "GOWORK=off", "GOENV=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "GOAUTH=off", "GOFLAGS=-mod=readonly", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
