package chatcompat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// Source test_endpoint_portability.py::test_the_vision_check_failure_is_a_backend_error_callers_already_catch.
// Python exception inheritance maps to Go errors.As on the same native backend
// error type, including after the caller wraps the actual VisionCheck failure.
func TestRemainingSurfaceVisionRefusalIsBackendError(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		_, _ = w.Write([]byte(`{"error":{"message":"this model does not accept pictures"}}`))
	}, "explicit-local-model", ExplicitModel)
	_, _, err := c.VisionCheck(context.Background())
	if err == nil {
		t.Fatal("picture refusal disappeared")
	}
	var backend *Error
	if !errors.As(fmt.Errorf("caller context: %w", err), &backend) || backend.Code != "endpoint_refused" || backend.StatusCode != 415 {
		t.Fatalf("existing backend catch cannot catch vision failure: %T %v", err, err)
	}
}
