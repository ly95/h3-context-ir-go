package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ly95/h3-context-ir-go"
)

type CardCacheKey struct {
	SourceSHA256    string              `json:"source_sha256"`
	Kind            contextir.MediaKind `json:"kind"`
	AssetID         string              `json:"asset_id"`
	ModelIdentity   string              `json:"model_identity"`
	AnalyzerVersion string              `json:"analyzer_version"`
	ContractSHA256  string              `json:"contract_sha256"`
}
type CardCacheOptions struct {
	Directory     string
	MaxEntryBytes int64
}
type CachedObservation struct {
	Card     contextir.AssetCard `json:"card"`
	Evidence *contextir.Evidence `json:"evidence,omitempty"`
}
type CardCacheResult struct {
	Observation CachedObservation `json:"observation"`
	Hit         bool              `json:"hit"`
	Bypassed    bool              `json:"bypassed"`
	Reason      string            `json:"reason,omitempty"`
}
type CardCacheEvent struct {
	AssetID       string
	Kind          contextir.MediaKind
	Hit, Bypassed bool
	Reason        string
}

type AssetCardCache struct {
	directory string
	max       int64
	mu        sync.Mutex
}

const cardCacheVersion = "contextir-observation-cache/v1"

type cardCacheRecord struct {
	Version     string            `json:"version"`
	Key         CardCacheKey      `json:"key"`
	Observation CachedObservation `json:"observation"`
	SHA256      string            `json:"sha256"`
}

func NewAssetCardCache(options CardCacheOptions) (*AssetCardCache, error) {
	if !filepath.IsAbs(options.Directory) || options.MaxEntryBytes <= 0 {
		return nil, mediaError(contextir.CodeInvalid, "card_cache", "Choose an absolute cache directory and a positive explicit entry-byte limit.", nil)
	}
	if err := os.MkdirAll(options.Directory, 0700); err != nil {
		return nil, err
	}
	return &AssetCardCache{directory: options.Directory, max: options.MaxEntryBytes}, nil
}
func cacheDigest(text string) bool {
	b, err := hex.DecodeString(text)
	return err == nil && len(b) == 32 && text == strings.ToLower(text)
}
func (k CardCacheKey) Digest() (string, error) {
	if !cacheDigest(k.SourceSHA256) || !cacheDigest(k.ContractSHA256) || k.AssetID == "" || strings.TrimSpace(k.ModelIdentity) == "" || strings.TrimSpace(k.AnalyzerVersion) == "" {
		return "", mediaError(contextir.CodeInvalid, "card_cache_key", "Cache identity needs full content and contract SHA-256, asset ID, model identity and analyzer version.", nil)
	}
	if k.Kind != contextir.MediaImage && k.Kind != contextir.MediaVideo && k.Kind != contextir.MediaAudio {
		return "", mediaError(contextir.CodeInvalid, "kind", "Unsupported observation-cache modality.", nil)
	}
	return k.IdentityDigest(), nil
}

func (k CardCacheKey) IdentityDigest() string {
	b, _ := json.Marshal(struct {
		Version string
		Key     CardCacheKey
	}{cardCacheVersion, k})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (c *AssetCardCache) PathForDigest(digest string) (string, error) {
	if c == nil || !cacheDigest(digest) {
		return "", mediaError(contextir.CodeInvalid, "card_cache_key", "Use an initialized cache and a complete lowercase SHA-256 key.", nil)
	}
	return filepath.Join(c.directory, digest+".json"), nil
}
func (c *AssetCardCache) Path(key CardCacheKey) (string, error) {
	d, err := key.Digest()
	if err != nil {
		return "", err
	}
	return c.PathForDigest(d)
}
func observationHash(value CachedObservation) (string, error) {
	b, e := json.Marshal(value)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func validateCachedObservation(key CardCacheKey, value CachedObservation) error {
	if value.Card.AssetID != key.AssetID {
		return mediaError(contextir.CodeInvalid, "card_cache", "Cached observation identifies a different asset.", nil)
	}
	if value.Evidence != nil {
		e := value.Evidence
		if e.AssetID != key.AssetID || e.Card == nil {
			return mediaError(contextir.CodeInvalid, "card_cache", "Cached evidence must identify this asset and include its card.", nil)
		}
		a, _ := json.Marshal(e.Card)
		b, _ := json.Marshal(value.Card)
		if !bytes.Equal(a, b) {
			return mediaError(contextir.CodeInvalid, "card_cache", "Cached card and evidence disagree.", nil)
		}
		if err := contextir.ValidateEvidence([]contextir.Asset{{ID: key.AssetID, Kind: key.Kind}}, []contextir.Evidence{*e}); err != nil {
			return err
		}
	}
	return nil
}
func (c *AssetCardCache) Get(ctx context.Context, key CardCacheKey) (CardCacheResult, error) {
	if c == nil {
		return CardCacheResult{}, mediaError(contextir.CodeInvalid, "card_cache", "Initialize an explicit observation cache first.", nil)
	}
	if err := ctx.Err(); err != nil {
		return CardCacheResult{}, err
	}
	if key.Kind == contextir.MediaAudio {
		return CardCacheResult{Bypassed: true, Reason: "audio_metadata_is_request_specific"}, nil
	}
	path, err := c.Path(key)
	if err != nil {
		return CardCacheResult{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return CardCacheResult{}, nil
	}
	if err != nil {
		return CardCacheResult{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > c.max {
		return CardCacheResult{}, mediaError(contextir.CodeLimit, "card_cache", "Cache entry is not a regular file within the configured byte limit.", nil)
	}
	file, err := os.Open(path)
	if err != nil {
		return CardCacheResult{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, c.max+1))
	if err != nil {
		return CardCacheResult{}, err
	}
	if int64(len(raw)) > c.max {
		return CardCacheResult{}, mediaError(contextir.CodeLimit, "card_cache", "Cache entry exceeds the configured byte limit.", nil)
	}
	var record cardCacheRecord
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&record); err != nil {
		return CardCacheResult{}, fmt.Errorf("invalid observation cache: %w", err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return CardCacheResult{}, fmt.Errorf("observation cache contains trailing data")
	}
	hash, err := observationHash(record.Observation)
	if err != nil {
		return CardCacheResult{}, err
	}
	if record.Version != cardCacheVersion || record.Key != key || record.SHA256 != hash {
		return CardCacheResult{}, mediaError(contextir.CodeInvalid, "card_cache", "Observation cache contract or content hash differs.", nil)
	}
	if err = validateCachedObservation(key, record.Observation); err != nil {
		return CardCacheResult{}, err
	}
	return CardCacheResult{Hit: true, Observation: record.Observation}, nil
}
func (c *AssetCardCache) Put(ctx context.Context, key CardCacheKey, value CachedObservation) (CardCacheResult, error) {
	if c == nil {
		return CardCacheResult{}, mediaError(contextir.CodeInvalid, "card_cache", "Initialize an explicit observation cache first.", nil)
	}
	if err := ctx.Err(); err != nil {
		return CardCacheResult{}, err
	}
	if key.Kind == contextir.MediaAudio {
		return CardCacheResult{Bypassed: true, Reason: "audio_metadata_is_request_specific"}, nil
	}
	path, err := c.Path(key)
	if err != nil {
		return CardCacheResult{}, err
	}
	if err = validateCachedObservation(key, value); err != nil {
		return CardCacheResult{}, err
	}
	hash, err := observationHash(value)
	if err != nil {
		return CardCacheResult{}, err
	}
	data, err := json.Marshal(cardCacheRecord{Version: cardCacheVersion, Key: key, Observation: value, SHA256: hash})
	if err != nil {
		return CardCacheResult{}, err
	}
	if int64(len(data)) > c.max {
		return CardCacheResult{}, mediaError(contextir.CodeLimit, "card_cache", "Observation exceeds the explicit cache-entry byte limit.", nil)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	temp, err := os.CreateTemp(c.directory, ".observation-*")
	if err != nil {
		return CardCacheResult{}, err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return CardCacheResult{}, err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return CardCacheResult{}, err
	}
	if err = temp.Close(); err != nil {
		return CardCacheResult{}, err
	}
	if err = ctx.Err(); err != nil {
		return CardCacheResult{}, err
	}
	if err = os.Rename(name, path); err != nil {
		return CardCacheResult{}, err
	}
	return CardCacheResult{Observation: value}, nil
}

func (c *AssetCardCache) Analyze(ctx context.Context, key CardCacheKey, audio *AudioMetadataRequest, observe func(context.Context) (CachedObservation, error)) (CardCacheResult, error) {
	if c == nil {
		return CardCacheResult{}, mediaError(contextir.CodeInvalid, "card_cache", "Initialize an explicit observation cache first.", nil)
	}
	if err := ctx.Err(); err != nil {
		return CardCacheResult{}, err
	}
	if key.Kind == contextir.MediaAudio {
		if audio == nil || audio.AssetID != key.AssetID {
			return CardCacheResult{}, mediaError(contextir.CodeInvalid, "audio_metadata", "Explicit per-request audio metadata is required.", nil)
		}
		card, err := AudioMetadataCard(*audio)
		return CardCacheResult{Observation: CachedObservation{Card: card}, Bypassed: true, Reason: "audio_metadata_is_request_specific"}, err
	}
	hit, err := c.Get(ctx, key)
	if err != nil || hit.Hit {
		return hit, err
	}
	if observe == nil {
		return CardCacheResult{}, mediaError(contextir.CodeUnsupported, "observation", "A cache miss requires an explicit visual observation adapter.", nil)
	}
	value, err := observe(ctx)
	if err != nil {
		return CardCacheResult{}, err
	}
	return c.Put(ctx, key, value)
}
