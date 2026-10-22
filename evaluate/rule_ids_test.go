package evaluate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestProductionRuleNumbersHaveSingleMeanings(t *testing.T) {
	pattern := regexp.MustCompile(`^([A-Z]+[0-9]+[a-z]?)-([a-z0-9-]+)$`)
	ids := map[string]map[string][]string{}
	set := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "third_party" || d.Name() == "conformance" || strings.HasPrefix(d.Name(), ".") && path != ".." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			v, ok := n.(*ast.BasicLit)
			if !ok || v.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(v.Value)
			if err != nil {
				return true
			}
			m := pattern.FindStringSubmatch(s)
			if m == nil {
				return true
			}
			if ids[m[1]] == nil {
				ids[m[1]] = map[string][]string{}
			}
			ids[m[1]][m[2]] = append(ids[m[1]][m[2]], set.Position(v.Pos()).String())
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for key := range ids {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(ids[key]) > 1 {
			t.Errorf("rule number %s has multiple meanings: %v", key, ids[key])
		}
	}
	if len(ids) < 50 {
		t.Fatal("guard did not scan production rule namespaces")
	}
}
