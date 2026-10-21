package service

import (
	"errors"
	"io"
)

// ErrClientDisconnected lets a hosting transport distinguish a normal peer
// disconnect during a refused-body drain from an unrelated read failure.
var ErrClientDisconnected = errors.New("client disconnected")

// Drain reads through at most budget+4096 bytes, with bounded individual reads.
// The small overread observes the same terminal budget condition as the source
// chunk stream without allocating or consuming an unbounded rejected body.
func Drain(r io.Reader, budget int64) error {
	if budget <= 0 {
		return errors.New("drain budget must be positive")
	}
	buf := make([]byte, 4096)
	var seen int64
	for {
		n, e := r.Read(buf)
		seen += int64(n)
		if seen > budget {
			return nil
		}
		if e != nil {
			if errors.Is(e, io.EOF) || errors.Is(e, ErrClientDisconnected) {
				return nil
			}
			return e
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
}
