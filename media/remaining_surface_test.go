package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contextir "github.com/ly95/h3-context-ir-go"
	"github.com/ly95/h3-context-ir-go/media"
	"github.com/ly95/h3-context-ir-go/perception"
)

func remainingSurfaceManifest(t *testing.T) struct {
	Source       string            `json:"source_commit"`
	Files        map[string]string `json:"source_sha256"`
	Declarations []struct {
		Brief, Filename, SHA256 string
		PX                      [2]int `json:"px"`
	} `json:"suite_declarations"`
} {
	t.Helper()
	var result struct {
		Source       string            `json:"source_commit"`
		Files        map[string]string `json:"source_sha256"`
		Declarations []struct {
			Brief, Filename, SHA256 string
			PX                      [2]int `json:"px"`
		} `json:"suite_declarations"`
	}
	data, err := os.ReadFile("../third_party/open-h3-ir/testdata/surface_manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(data)
	if hex.EncodeToString(h[:]) != "9e4519f4c9f1d246c0fac4a19db21de30fb9a00638ceb8b17a9de29ab03d0195" {
		t.Fatal("independently extracted source fixture changed")
	}
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.Source != "fd031e136ae3d89147324c0d1b2aa65e838c21f5" || len(result.Declarations) != 3 {
		t.Fatal("source declarations missing", result.Source, len(result.Declarations))
	}
	return result
}

// Source test_golden_asset_geometry.py::test_the_reference_assets_are_pngs_and_present.
func TestRemainingSurfaceGoldenAssetsAreRealPNGs(t *testing.T) {
	manifest := remainingSurfaceManifest(t)
	paths, err := filepath.Glob("../third_party/open-h3-ir/testdata/golden/assets/*.png")
	if err != nil || len(paths) != 2 {
		t.Fatal("the original assets must be present, never skipped", paths, err)
	}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) < 33 || !bytes.Equal(b[:8], []byte("\x89PNG\r\n\x1a\n")) || string(b[12:16]) != "IHDR" {
			t.Fatal("not a PNG opening with IHDR", path)
		}
		digest := sha256.Sum256(b)
		if hex.EncodeToString(digest[:]) != manifest.Files["h3ir/golden/assets/"+filepath.Base(path)] {
			t.Fatal("original bytes changed", path)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		h, err := media.ReadHeader(absolute)
		if err != nil || h.Kind != contextir.MediaImage || h.MIME != "image/png" || h.Width <= 0 || h.Height <= 0 || h.Width != int(binary.BigEndian.Uint32(b[16:20])) || h.Height != int(binary.BigEndian.Uint32(b[20:24])) {
			t.Fatal("native header disagrees with original IHDR", h, err)
		}
	}
}

// Source test_golden_asset_geometry.py::test_declared_dimensions_match_the_file_on_disk.
// Every original declaration is kept as a list, so repeated ref1/ref2 entries
// cannot mask a wrong literal through a dictionary keyed by filename.
func TestRemainingSurfaceEveryDeclaredGeometryMatchesBytes(t *testing.T) {
	fixture := remainingSurfaceManifest(t)
	seen := map[string]int{}
	for _, decl := range fixture.Declarations {
		path, err := filepath.Abs(filepath.Join("../third_party/open-h3-ir/testdata/golden/assets", decl.Filename))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(b)
		if hex.EncodeToString(hash[:]) != decl.SHA256 {
			t.Fatal("declared source SHA does not identify the real file", decl.Brief, decl.Filename)
		}
		h, err := media.ReadHeader(path)
		if err != nil || [2]int{h.Width, h.Height} != decl.PX {
			t.Error(decl.Brief, decl.Filename, decl.PX, h, err)
		}
		seen[decl.Filename]++
	}
	if seen["ref1.png"] < 2 || seen["ref2.png"] < 1 {
		t.Fatal("the list was collapsed and lost a declaration", seen)
	}
}

// Source test_video_reference.py::test_a_video_with_no_path_raises_instead_of_returning_an_empty_card.
func TestRemainingSurfaceVideoWithoutPathFailsBeforeObservation(t *testing.T) {
	calls := 0
	p, err := media.NewPerceiver(media.PerceiverOptions{Resolve: func(context.Context, contextir.Asset) (string, error) { return "", nil }, Model: vision(func(context.Context, contextir.ModelRequest, []perception.Image) (contextir.ModelResponse, error) {
		calls++
		t.Error("missing video reached vision model")
		return contextir.ModelResponse{}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	evidence, ledger, err := p.PerceiveWithCalls(context.Background(), contextir.Asset{ID: "video", Kind: contextir.MediaVideo, SHA256: strings.Repeat("a", 64)})
	if err == nil || !contextir.HasCode(err, contextir.CodeInvalid) || evidence.Card != nil || len(evidence.Facts) != 0 || len(ledger) != 0 || calls != 0 {
		t.Fatal("missing media became an empty successful observation", evidence, ledger, err, calls)
	}
}

// Source test_video_reference.py::test_the_analyzer_version_moves_when_the_card_contract_does.
// Go has its own version namespace rather than reusing Python's integer. Cache
// identity includes BOTH analyzer version and prompt/schema contract digest;
// demonstrate a real old-card hit and invalidation by each changed contract.
func TestRemainingSurfaceAnalyzerContractInvalidatesOldCards(t *testing.T) {
	var revision int
	if _, err := fmt.Sscanf(media.AnalyzerVersion, "contextir-media/v%d", &revision); err != nil || revision < 1 {
		t.Fatal("native analyzer contract has no monotonic version", media.AnalyzerVersion)
	}
	c := newCardCache(t)
	old := cacheIdentity(contextir.MediaVideo)
	old.AnalyzerVersion = "legacy/no-pose-no-frames-no-characterisation"
	card := contextir.AssetCard{AssetID: old.AssetID, Summary: "one clip", Characterisation: "source audio character", FramesSeen: 3, Subjects: []contextir.SubjectCard{{Kind: "person", Descriptor: "the person", Attributes: []string{"black jacket"}, Pose: []string{"walking"}}}}
	if _, err := c.Put(context.Background(), old, media.CachedObservation{Card: card}); err != nil {
		t.Fatal(err)
	}
	if hit, err := c.Get(context.Background(), old); err != nil || !hit.Hit {
		t.Fatal("negative cache control did not really contain an old observation", hit, err)
	}
	current := old
	current.AnalyzerVersion = media.AnalyzerVersion
	if hit, err := c.Get(context.Background(), current); err != nil || hit.Hit {
		t.Fatal("a changed analyzer reused the old contract", hit, err)
	}
	if _, err := c.Put(context.Background(), current, media.CachedObservation{Card: card}); err != nil {
		t.Fatal(err)
	}
	next := current
	next.ContractSHA256 = cacheHash("changed attributes/pose/sampled-frame/characterisation schema")
	if hit, err := c.Get(context.Background(), next); err != nil || hit.Hit {
		t.Fatal("changed card schema reused stale observation", hit, err)
	}
	hit, err := c.Get(context.Background(), current)
	if err != nil || !hit.Hit || hit.Observation.Card.FramesSeen != 3 || len(hit.Observation.Card.Subjects[0].Pose) != 1 || hit.Observation.Card.Characterisation != "source audio character" {
		t.Fatal("current native card lost required contract fields", hit, err)
	}
}
