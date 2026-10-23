package service

import (
	"errors"
	"io"
)

var ErrClientDisconnected = errors.New("client disconnected")

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
