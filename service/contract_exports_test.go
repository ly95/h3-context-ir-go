package service_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ly95/h3-context-ir-go/service"
)

func TestContractExportsUseAuthoritativeSnapshot(t *testing.T) {
	jsonExport, err := service.AsJSON()
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(jsonExport), &snapshot); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, service.Snapshot()) {
		t.Fatal("JSON export differs from the authoritative public contract")
	}
	for _, test := range []struct {
		name   string
		export func() (string, error)
	}{
		{"json", service.AsJSON},
		{"javascript", func() (string, error) { return service.AsJS(), nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			first, err := test.export()
			if err != nil {
				t.Fatal(err)
			}
			second, err := test.export()
			if err != nil || first != second || first == "" || !strings.HasSuffix(first, "\n") {
				t.Fatalf("public contract export is not complete and byte-stable: %v", err)
			}
		})
	}
}
