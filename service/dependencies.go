package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ly95/h3-context-ir-go/compiler"
	"github.com/ly95/h3-context-ir-go/plan"
	"github.com/ly95/h3-context-ir-go/uploads"
)

func missingDependencies(digests []string) *Error {
	return &Error{Status: 422, Code: "asset-not-uploaded", Missing: digests, Message: "This service no longer holds an attachment required by the brief. Upload the missing bytes again with PUT /v1/assets/{sha256}, then retry the request. Uploads are kept for a while and then dropped."}
}

func (s *Service) uploadedDependencies(b plan.Brief) ([]string, error) {
	digests := []string{}
	seen := map[string]bool{}
	for _, a := range b.Assets {
		if !uploads.IsDigest(a.SHA256) || a.Path == nil {
			continue
		}
		path, err := s.cfg.Store.PathFor(a.SHA256)
		if err != nil {
			return nil, err
		}
		if filepath.Clean(*a.Path) == path && !seen[a.SHA256] {
			digests = append(digests, a.SHA256)
			seen[a.SHA256] = true
		}
	}
	return digests, nil
}

func (s *Service) touchDependencies(digests []string) error {
	missing, err := s.cfg.Store.Missing(digests)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return missingDependencies(missing)
	}
	for _, digest := range digests {
		path, have, err := s.cfg.Store.Stored(digest)
		if err != nil {
			return err
		}
		if !have {
			return missingDependencies([]string{digest})
		}
		if err = s.cfg.Store.Touch(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return missingDependencies([]string{digest})
			}
			return err
		}
	}
	return nil
}

func (s *Service) withUploadedDependencies(ctx context.Context, b plan.Brief, operation func(context.Context) (compiler.Result, error)) (compiler.Result, error) {
	digests, err := s.uploadedDependencies(b)
	if err != nil {
		return compiler.Result{Status: "failed", UsageComplete: true}, err
	}
	if len(digests) == 0 {
		return operation(ctx)
	}
	if err = s.touchDependencies(digests); err != nil {
		return compiler.Result{Status: "failed", UsageComplete: true}, err
	}
	config := s.cfg.Store.Config()
	interval := config.Grace
	if config.TTL > 0 && (interval <= 0 || config.TTL < interval) {
		interval = config.TTL
	}
	interval /= 3
	if interval <= 0 {
		interval = time.Nanosecond
	}
	if interval > time.Minute {
		interval = time.Minute
	}
	active, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				done <- nil
				return
			case <-active.Done():
				done <- nil
				return
			case <-ticker.C:
				if err := s.touchDependencies(digests); err != nil {
					done <- err
					cancel()
					return
				}
			}
		}
	}()
	result, operationErr := operation(active)
	close(stop)
	if err := <-done; err != nil {
		return result, err
	}
	if operationErr != nil {
		return result, operationErr
	}
	if err := s.touchDependencies(digests); err != nil {
		return result, err
	}
	return result, nil
}
