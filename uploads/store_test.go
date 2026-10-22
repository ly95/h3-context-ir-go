package uploads_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/ly95/h3-context-ir-go/uploads"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func store(t *testing.T, max, total int64) *uploads.Store {
	t.Helper()
	s, e := uploads.New(uploads.Config{Directory: filepath.Join(t.TempDir(), "uploads"), MaxBytes: max, StoreBytes: total, TTL: 48 * time.Hour, Grace: 15 * time.Minute})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func code(t *testing.T, e error, c uploads.Code) {
	t.Helper()
	var x *uploads.Error
	if !errors.As(e, &x) || x.Code != c {
		t.Fatalf("wanted %s: %v", c, e)
	}
}
func TestStreamingIntegrityAndAbort(t *testing.T) {
	s := store(t, 1000, 2500)
	b := bytes.Repeat([]byte("x"), 900)
	n := int64(len(b))
	r, e := s.Receiver(digest(b), &n)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Write(b[:400]); e != nil {
		t.Fatal(e)
	}
	if e = r.Abort(); e != nil {
		t.Fatal(e)
	}
	if e = r.Abort(); e != nil {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(s.Config().Directory)
	if len(entries) != 0 {
		t.Fatal("aborted bytes were retained")
	}
	_, _, e = s.Put(digest([]byte("different")), &n, bytes.NewReader(b))
	code(t, e, uploads.DigestMismatch)
	if _, have, e := s.Stored(digest(b)); e != nil || have {
		t.Fatal("mismatched bytes were published")
	}
	p, size, e := s.Put(digest(b), &n, bytes.NewReader(b))
	if e != nil || size != 900 {
		t.Fatal(size, e)
	}
	got, e := os.ReadFile(p)
	if e != nil || !bytes.Equal(b, got) {
		t.Fatal("stored data differs", e)
	}
}
func TestConcurrentReservationsAndActualByteAllowance(t *testing.T) {
	s := store(t, 1000, 2500)
	var mu sync.Mutex
	var accepted []*uploads.Receiver
	var rejected int
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			n := int64(900)
			r, e := s.Receiver(digest([]byte{byte(i)}), &n)
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				accepted = append(accepted, r)
			} else {
				code(t, e, uploads.StoreFull)
				rejected++
			}
		}(i)
	}
	close(gate)
	wg.Wait()
	if len(accepted) != 2 || rejected != 18 {
		t.Fatal("store overcommitted", len(accepted), rejected)
	}
	for _, r := range accepted {
		if e := r.Abort(); e != nil {
			t.Fatal(e)
		}
	}
	n := int64(10)
	r, e := s.Receiver(digest([]byte("x")), &n)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Abort()
	_, e = r.Write(bytes.Repeat([]byte("x"), 11))
	code(t, e, uploads.StoreFull)
}
func TestEvictionPreservesGraceAndUsesLastUse(t *testing.T) {
	s := store(t, 1000, 2500)
	var paths []string
	for _, v := range []byte{'a', 'b'} {
		b := bytes.Repeat([]byte{v}, 900)
		n := int64(len(b))
		p, _, e := s.Put(digest(b), &n, bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		paths = append(paths, p)
	}
	b := bytes.Repeat([]byte{'c'}, 900)
	n := int64(900)
	_, _, e := s.Put(digest(b), &n, bytes.NewReader(b))
	code(t, e, uploads.StoreFull)
	past := time.Now().Add(-3 * time.Hour)
	for _, p := range paths {
		if e = os.Chtimes(p, past, past); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Touch(paths[0]); e != nil {
		t.Fatal(e)
	}
	stats, e := s.Sweep(900, time.Now().Add(time.Hour))
	if e != nil || stats.Dropped != 1 {
		t.Fatal(stats, e)
	}
	if _, e = os.Stat(paths[0]); e != nil {
		t.Fatal("recently used attachment evicted", e)
	}
	if _, e = os.Stat(paths[1]); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("older attachment not evicted", e)
	}
}
func TestInvalidNamesDoNotCreateStore(t *testing.T) {
	s := store(t, 1000, 2500)
	for _, d := range []string{"../file", "A" + string(bytes.Repeat([]byte{'a'}, 63)), string(bytes.Repeat([]byte{'a'}, 65)), ""} {
		_, e := s.Receiver(d, nil)
		code(t, e, uploads.NotADigest)
	}
	if _, e := os.Stat(s.Config().Directory); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("invalid digest created storage", e)
	}
}
