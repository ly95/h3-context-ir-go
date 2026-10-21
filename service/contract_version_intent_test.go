package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// This is the native counterpart of upstream's pinned contract ceremony. The
// independently recorded v3 hashes cover the complete Go boundary, including
// native extensions; projecting back to Python v2 would miss later changes to
// those extensions. Change the version and review/update this table together.
const nativePinnedContractVersion = 3

func nativePinnedContractDigests() map[string]string {
	return map[string]string{
		"asset_fields":    "38190799fcfb6cf1",
		"brief_fields":    "48d4a080ffdf5ce2",
		"dialogue_fields": "a63047779f907e58",
		"error_codes":     "62e393e774f5f23f",
		"roles":           "4b85814fed5254ab",
		"directors":       "8e50d53980fd8777",
		"camera_moves":    "e0ebdca0a4dcaea8",
		"limits":          "78554cef21934947",
	}
}

// Hash the actual section values independently of production Digest/Digests.
// Reading every body key also catches new sections omitted from SectionNames.
func nativeBoundaryDigests(t *testing.T, snapshot map[string]any) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, value := range snapshot {
		switch name {
		case "contract_version", "field_lists_describe", "digests":
			continue
		}
		var wire bytes.Buffer
		encoder := json.NewEncoder(&wire)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(value); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(bytes.TrimSuffix(wire.Bytes(), []byte("\n")))
		out[name] = hex.EncodeToString(sum[:8])
	}
	return out
}

func nativeContractVersionGate(version int, live map[string]string) error {
	if reflect.DeepEqual(live, nativePinnedContractDigests()) {
		if version != nativePinnedContractVersion {
			return fmt.Errorf("unchanged contract has version %d, pinned %d", version, nativePinnedContractVersion)
		}
		return nil
	}
	if version <= nativePinnedContractVersion {
		return fmt.Errorf("published contract changed without advancing version %d: %v", version, live)
	}
	return nil
}

func TestGoContractVersionTracksPublishedBoundary(t *testing.T) {
	snapshot := Snapshot()
	live := nativeBoundaryDigests(t, snapshot)
	if err := nativeContractVersionGate(ContractVersion, live); err != nil {
		t.Fatal(err)
	}
	advertised := snapshot["digests"].(map[string]any)
	if len(advertised) != len(live) || len(SectionNames()) != len(live) {
		t.Fatal("published body, section names, and digests have different coverage")
	}
	for _, name := range SectionNames() {
		if hash, ok := live[name]; !ok || advertised[name] != hash {
			t.Fatalf("section %q is missing or advertises a digest different from its real body", name)
		}
	}

	// A driver must see the same vocabulary, timing and version wherever it
	// reads them. Exercise the actual HTTP surfaces, not only shared constants.
	s := testService(t, nil, nil)
	status, caps := call(t, s, "GET", "/v1/capabilities", nil)
	if status != 200 {
		t.Fatal(status, caps)
	}
	status, served := call(t, s, "GET", "/v1/contract", nil)
	if status != 200 {
		t.Fatal(status, served)
	}
	delete(served, "package_version")
	if !reflect.DeepEqual(served, snapshot) {
		t.Fatal("HTTP and imported contract disagree")
	}
	limits := snapshot["limits"].(map[string]any)
	for _, name := range []string{"aspects", "dialogue_languages"} {
		if !reflect.DeepEqual(caps[name], limits[name]) {
			t.Fatalf("capabilities and contract disagree about %s", name)
		}
	}
	if caps["output"].(map[string]any)["fps"] != limits["fps"] || caps["contract"].(map[string]any)["version"] != snapshot["contract_version"] {
		t.Fatal("capabilities and contract disagree about timing or version")
	}
}

func TestGoContractVersionGateRejectsUnversionedChanges(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"native_field_added", func(snapshot map[string]any) {
			snapshot["brief_fields"] = append(snapshot["brief_fields"].([]any), "future_native_choice")
		}},
		{"error_status_changed", func(snapshot map[string]any) {
			snapshot["error_codes"].(map[string]any)["internal-error"].(map[string]any)["status"] = float64(503)
		}},
		{"error_route_changed", func(snapshot map[string]any) {
			snapshot["error_codes"].(map[string]any)["malformed-request"].(map[string]any)["on"] = []any{"briefs"}
		}},
		{"section_added", func(snapshot map[string]any) {
			snapshot["future_native_section"] = []any{"new boundary"}
		}},
		{"section_removed", func(snapshot map[string]any) {
			delete(snapshot, "roles")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := Snapshot()
			test.mutate(snapshot)
			live := nativeBoundaryDigests(t, snapshot)
			// Even if a changed implementation honestly recomputes its own
			// hashes, it cannot pass by comparing those hashes with itself.
			if err := nativeContractVersionGate(nativePinnedContractVersion, live); err == nil {
				t.Fatal("changed public boundary passed without a version increase")
			}
			if err := nativeContractVersionGate(nativePinnedContractVersion+1, live); err != nil {
				t.Fatal("versioned contract evolution was refused", err)
			}
		})
	}
	t.Run("unchanged_boundary_keeps_its_version", func(t *testing.T) {
		if err := nativeContractVersionGate(nativePinnedContractVersion, nativePinnedContractDigests()); err != nil {
			t.Fatal(err)
		}
		if err := nativeContractVersionGate(nativePinnedContractVersion+1, nativePinnedContractDigests()); err == nil {
			t.Fatal("version moved while every boundary section stayed unchanged")
		}
	})
}
