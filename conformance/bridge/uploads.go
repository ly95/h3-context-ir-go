package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ly95/h3-context-ir-go/uploads"
)

var uploadStores = map[string]*uploads.Store{}
var uploadReceivers = map[string]*uploads.Receiver{}
var uploadSequence int

type uploadBridgeError struct{ cause *uploads.Error }

func (e *uploadBridgeError) Error() string         { return e.cause.Message }
func (e *uploadBridgeError) Unwrap() error         { return e.cause }
func (e *uploadBridgeError) OperationCode() string { return string(e.cause.Code) }
func uploadError(err error) error {
	if typed, ok := err.(*uploads.Error); ok {
		return &uploadBridgeError{typed}
	}
	return err
}
func uploadOperation(name string, b json.RawMessage) (any, error) {
	var a struct {
		Config struct {
			Root  string  `json:"root"`
			Max   int64   `json:"max_bytes"`
			Store int64   `json:"store_bytes"`
			TTL   float64 `json:"ttl_hours"`
		} `json:"config"`
		Digest   string   `json:"digest"`
		Digests  []string `json:"digests"`
		Declared *int64   `json:"declared"`
		ID       string   `json:"id"`
		Data     []byte   `json:"data"`
		Path     string   `json:"path"`
		Reserve  int64    `json:"reserve"`
		Now      *float64 `json:"now"`
	}
	if e := json.Unmarshal(b, &a); e != nil {
		return nil, e
	}
	if strings.HasPrefix(name, "h3ir.uploads.Receiver.") {
		rx := uploadReceivers[a.ID]
		if rx == nil {
			return nil, fmt.Errorf("unknown upload receiver handle")
		}
		switch name {
		case "h3ir.uploads.Receiver.write":
			n, e := rx.Write(a.Data)
			return n, uploadError(e)
		case "h3ir.uploads.Receiver.finish":
			p, n, e := rx.Finish()
			return []any{p, n}, uploadError(e)
		case "h3ir.uploads.Receiver.abort":
			return nil, uploadError(rx.Abort())
		}
		return nil, fmt.Errorf("unknown receiver operation")
	}
	keyBytes, e := json.Marshal(a.Config)
	if e != nil {
		return nil, e
	}
	key := string(keyBytes)
	store := uploadStores[key]
	if store == nil {
		store, e = uploads.New(uploads.Config{Directory: a.Config.Root, MaxBytes: a.Config.Max, StoreBytes: a.Config.Store, TTL: time.Duration(a.Config.TTL * float64(time.Hour)), Grace: 900 * time.Second})
		if e != nil {
			return nil, uploadError(e)
		}
		uploadStores[key] = store
	}
	switch name {
	case "h3ir.uploads.path_for":
		p, e := store.PathFor(a.Digest)
		return p, uploadError(e)
	case "h3ir.uploads.stored":
		p, ok, e := store.Stored(a.Digest)
		if e != nil {
			return nil, uploadError(e)
		}
		if !ok {
			return nil, nil
		}
		return p, nil
	case "h3ir.uploads.missing":
		m, e := store.Missing(a.Digests)
		return m, uploadError(e)
	case "h3ir.uploads.touch":
		return nil, uploadError(store.Touch(a.Path))
	case "h3ir.uploads.total_bytes":
		n, e := store.TotalBytes()
		return n, uploadError(e)
	case "h3ir.uploads.sweep":
		now := time.Now()
		if a.Now != nil {
			now = time.Unix(0, int64(*a.Now*1e9))
		}
		stats, e := store.Sweep(a.Reserve, now)
		return stats, uploadError(e)
	case "h3ir.uploads.receiver":
		rx, e := store.Receiver(a.Digest, a.Declared)
		if e != nil {
			return nil, uploadError(e)
		}
		uploadSequence++
		id := fmt.Sprintf("receiver-%d", uploadSequence)
		uploadReceivers[id] = rx
		return id, nil
	}
	return nil, fmt.Errorf("unsupported upload conformance operation %q", name)
}
