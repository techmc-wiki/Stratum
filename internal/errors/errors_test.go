package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsKindMatchesWrappedAndJoinedErrors(t *testing.T) {
	validationErr := Error{Kind: KindValidation, Operation: "test", Message: "invalid"}
	wrapped := fmt.Errorf("wrap: %w", validationErr)
	joined := errors.Join(errors.New("other"), wrapped)

	if !IsKind(joined, KindValidation) {
		t.Fatalf("expected joined wrapped validation error to match")
	}
	if IsKind(joined, KindConflict) {
		t.Fatalf("did not expect joined wrapped validation error to match conflict")
	}
}
