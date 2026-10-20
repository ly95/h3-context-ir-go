// Package uploads implements a bounded, caller-owned content-addressed store.
// It never fetches a URL, serves stored bytes over HTTP, or reads environment.
package uploads

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Code string

const (
	NotADigest     Code = "asset-name-not-a-digest"
	TooLarge       Code = "asset-too-large"
	StoreFull      Code = "upload-store-full"
	DigestMismatch Code = "asset-digest-mismatch"
)

type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

type Config struct {
	Directory            string
	MaxBytes, StoreBytes int64
	TTL, Grace           time.Duration
}
type Stats struct {
	Dropped int   `json:"dropped"`
	Freed   int64 `json:"freed"`
	Bytes   int64 `json:"bytes"`
}
type Store struct {
	cfg      Config
	mu       sync.Mutex
	promised int64
}

func New(cfg Config) (*Store, error) {
	if !filepath.IsAbs(cfg.Directory) || cfg.MaxBytes < 0 || cfg.StoreBytes < 0 || cfg.TTL < 0 || cfg.Grace < 0 {
		return nil, fmt.Errorf("upload store requires an absolute directory and nonnegative explicit limits")
	}
	if cfg.Grace == 0 {
		cfg.Grace = 15 * time.Minute
	}
	cfg.Directory = filepath.Clean(cfg.Directory)
	return &Store{cfg: cfg}, nil
}
func (s *Store) Config() Config { return s.cfg }
func IsDigest(name string) bool {
	if len(name) != 64 {
		return false
	}
	for _, b := range []byte(name) {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}
func (s *Store) PathFor(digest string) (string, error) {
	if !IsDigest(digest) {
		return "", &Error{NotADigest, "An uploaded attachment must be named by its own SHA-256, as 64 lowercase hexadecimal characters."}
	}
	return filepath.Join(s.cfg.Directory, digest[:2], digest), nil
}
func (s *Store) Stored(digest string) (string, bool, error) {
	p, e := s.PathFor(digest)
	if e != nil {
		return "", false, e
	}
	st, e := os.Lstat(p)
	if errors.Is(e, os.ErrNotExist) {
		return "", false, nil
	}
	if e != nil {
		return "", false, e
	}
	if !st.Mode().IsRegular() {
		return "", false, fmt.Errorf("stored attachment is not a regular file")
	}
	return p, true, nil
}
func (s *Store) Missing(digests []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, d := range digests {
		_, have, e := s.Stored(d)
		if e != nil {
			return nil, e
		}
		if !have && !seen[d] {
			out = append(out, d)
			seen[d] = true
		}
	}
	return out, nil
}
func (s *Store) Touch(path string) error {
	rel, e := filepath.Rel(s.cfg.Directory, path)
	if e != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("touch path is outside the upload store")
	}
	now := time.Now()
	return os.Chtimes(path, now, now)
}

type entry struct {
	path     string
	size     int64
	modified time.Time
}

func (s *Store) entries() ([]entry, error) {
	shards, e := os.ReadDir(s.cfg.Directory)
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var out []entry
	for _, shard := range shards {
		if !shard.IsDir() {
			continue
		}
		if len(shard.Name()) != 2 {
			return nil, fmt.Errorf("invalid directory in upload store")
		}
		items, e := os.ReadDir(filepath.Join(s.cfg.Directory, shard.Name()))
		if e != nil {
			return nil, e
		}
		for _, f := range items {
			p := filepath.Join(s.cfg.Directory, shard.Name(), f.Name())
			st, e := f.Info()
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e != nil {
				return nil, e
			}
			if !st.Mode().IsRegular() {
				return nil, fmt.Errorf("non-regular entry in upload store")
			}
			out = append(out, entry{p, st.Size(), st.ModTime()})
		}
	}
	return out, nil
}
func (s *Store) TotalBytes() (int64, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.total() }
func (s *Store) total() (int64, error) {
	items, e := s.entries()
	if e != nil {
		return 0, e
	}
	var n int64
	for _, it := range items {
		n += it.size
	}
	return n, nil
}
func (s *Store) Sweep(reserve int64, now time.Time) (Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reserve < 0 {
		return Stats{}, fmt.Errorf("negative reservation")
	}
	return s.sweep(reserve, now)
}
func (s *Store) sweep(reserve int64, now time.Time) (Stats, error) {
	items, e := s.entries()
	if e != nil {
		return Stats{}, e
	}
	sort.Slice(items, func(i, j int) bool { return items[i].modified.Before(items[j].modified) })
	out := Stats{}
	for _, it := range items {
		out.Bytes += it.size
	}
	kept := make([]entry, 0, len(items))
	remove := func(it entry) error {
		e := os.Remove(it.path)
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		out.Dropped++
		out.Freed += it.size
		out.Bytes -= it.size
		return nil
	}
	for _, it := range items {
		age := now.Sub(it.modified)
		if s.cfg.TTL > 0 && age > s.cfg.TTL && age > s.cfg.Grace {
			if e = remove(it); e != nil {
				return out, e
			}
		} else {
			kept = append(kept, it)
		}
	}
	for _, it := range kept {
		if out.Bytes <= s.cfg.StoreBytes-reserve {
			break
		}
		if now.Sub(it.modified) <= s.cfg.Grace {
			continue
		}
		if e = remove(it); e != nil {
			return out, e
		}
	}
	parts, e := filepath.Glob(filepath.Join(s.cfg.Directory, "incoming-*.part"))
	if e != nil {
		return out, e
	}
	for _, p := range parts {
		st, e := os.Lstat(p)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return out, e
		}
		if now.Sub(st.ModTime()) > s.cfg.Grace {
			if e = os.Remove(p); e != nil && !errors.Is(e, os.ErrNotExist) {
				return out, e
			}
		}
	}
	return out, nil
}

type Receiver struct {
	store               *Store
	claimed             string
	allowance, received int64
	file                *os.File
	tmp                 string
	digest              hash.Hash
	done                bool
	mu                  sync.Mutex
}

func (s *Store) Receiver(claimed string, declared *int64) (*Receiver, error) {
	if _, e := s.PathFor(claimed); e != nil {
		return nil, e
	}
	if s.cfg.MaxBytes == 0 {
		return nil, &Error{TooLarge, "This service does not accept uploads: H3IR_UPLOAD_MAX_BYTES is set to 0. Configure a positive per-file limit to enable uploads."}
	}
	if declared != nil && *declared < 0 {
		return nil, fmt.Errorf("declared byte length cannot be negative")
	}
	if declared != nil && *declared > s.cfg.MaxBytes {
		return nil, &Error{TooLarge, fmt.Sprintf("The attachment is %d bytes; the per-file ceiling is %d bytes. Configure MaxBytes (H3IR_UPLOAD_MAX_BYTES in an HTTP host) or send a smaller attachment.", *declared, s.cfg.MaxBytes)}
	}
	want := s.cfg.MaxBytes
	if declared != nil {
		want = *declared
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if want > s.cfg.StoreBytes || s.promised > s.cfg.StoreBytes-want {
		return nil, s.full(want)
	}
	if _, e := s.sweep(s.promised+want, time.Now()); e != nil {
		return nil, e
	}
	n, e := s.total()
	if e != nil {
		return nil, e
	}
	if n > s.cfg.StoreBytes-s.promised-want {
		return nil, s.full(want)
	}
	if e = os.MkdirAll(s.cfg.Directory, 0700); e != nil {
		return nil, e
	}
	f, e := os.CreateTemp(s.cfg.Directory, "incoming-*.part")
	if e != nil {
		return nil, e
	}
	s.promised += want
	return &Receiver{store: s, claimed: claimed, allowance: want, file: f, tmp: f.Name(), digest: sha256.New()}, nil
}
func (s *Store) full(want int64) *Error {
	return &Error{StoreFull, fmt.Sprintf("There is no room for another %d bytes: the upload store ceiling is %d bytes and recent attachments are still in use. Wait for eviction or configure StoreBytes (H3IR_UPLOAD_STORE_BYTES in an HTTP host).", want, s.cfg.StoreBytes)}
}
func (r *Receiver) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return 0, fmt.Errorf("upload has already finished or aborted")
	}
	if int64(len(p)) > r.allowance-r.received {
		if r.allowance >= r.store.cfg.MaxBytes {
			return 0, &Error{TooLarge, fmt.Sprintf("The attachment exceeds %d bytes. Configure H3IR_UPLOAD_MAX_BYTES or send a smaller attachment.", r.store.cfg.MaxBytes)}
		}
		return 0, &Error{StoreFull, "The attachment exceeds its declared length and the reserved store allowance. Send it again with its actual length."}
	}
	n, e := r.file.Write(p)
	if n > 0 {
		r.digest.Write(p[:n])
		r.received += int64(n)
	}
	return n, e
}
func (r *Receiver) release() { r.store.mu.Lock(); r.store.promised -= r.allowance; r.store.mu.Unlock() }
func (r *Receiver) Finish() (string, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return "", 0, fmt.Errorf("upload has already finished or aborted")
	}
	r.done = true
	defer r.release()
	defer os.Remove(r.tmp)
	if e := r.file.Close(); e != nil {
		return "", 0, e
	}
	computed := hex.EncodeToString(r.digest.Sum(nil))
	if computed != r.claimed {
		return "", 0, &Error{DigestMismatch, fmt.Sprintf("The received bytes hash to %s, not the claimed digest. The file may have changed while it was being sent or been truncated. Send it again.", computed)}
	}
	p, e := r.store.PathFor(computed)
	if e != nil {
		return "", 0, e
	}
	shard := filepath.Dir(p)
	if e = os.MkdirAll(shard, 0700); e != nil {
		return "", 0, e
	}
	st, e := os.Lstat(shard)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", 0, fmt.Errorf("upload shard is not a private regular directory")
	}
	if e = os.Rename(r.tmp, p); e != nil {
		return "", 0, e
	}
	return p, r.received, nil
}
func (r *Receiver) Abort() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return nil
	}
	r.done = true
	defer r.release()
	closeErr := r.file.Close()
	removeErr := os.Remove(r.tmp)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(closeErr, removeErr)
}

// Put streams bytes without trusting Content-Length; the receiver enforces its
// reserved allowance against every actual chunk and aborts on any read failure.
func (s *Store) Put(claimed string, declared *int64, body io.Reader) (string, int64, error) {
	r, e := s.Receiver(claimed, declared)
	if e != nil {
		return "", 0, e
	}
	defer r.Abort()
	if _, e = io.Copy(r, body); e != nil {
		return "", 0, e
	}
	return r.Finish()
}
