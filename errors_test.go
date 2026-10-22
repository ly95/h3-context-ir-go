package contextir_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ly95/h3-context-ir-go"
)

func TestNilTypedErrorCannotPanicDuringInspection(t *testing.T) {
	var typed *contextir.Error
	for _, err := range []error{typed, fmt.Errorf("adapter: %w", typed)} {
		if errors.Is(err, context.Canceled) || contextir.HasCode(err, contextir.CodeCanceled) {
			t.Fatal("nil error pointer acquired a cancellation classification")
		}
		if err.Error() == "" {
			t.Fatal("nil error pointer has no safe diagnostic")
		}
	}
	if typed.Unwrap() != nil {
		t.Fatal("nil error pointer has a cause")
	}
}
