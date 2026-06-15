package lucy

import (
	"errors"
	"testing"
)

func TestAdapterErrorMessage(t *testing.T) {
	err := NewAdapterError(ErrorCodeValidationFailed, "invalid spec", nil, false)
	expected := "lucy adapter [validation_failed]: invalid spec"
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}

func TestAdapterErrorMessageWithCause(t *testing.T) {
	cause := errors.New("root cause")
	err := NewAdapterError(ErrorCodeIOError, "failed to read", cause, true)
	msg := err.Error()
	if msg != "lucy adapter [io_error]: failed to read: root cause" {
		t.Fatalf("expected cause in message, got %q", msg)
	}
}

func TestAdapterErrorUnwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := NewAdapterError(ErrorCodeTimeout, "timeout occurred", cause, true)
	if errors.Unwrap(err) != cause {
		t.Fatal("Unwrap should return cause")
	}
}

func TestAdapterErrorUnwrapNil(t *testing.T) {
	err := NewAdapterError(ErrorCodeInvalidRequest, "bad request", nil, false)
	if errors.Unwrap(err) != nil {
		t.Fatal("Unwrap should return nil when no cause")
	}
}

func TestIsCode(t *testing.T) {
	err := NewAdapterError(ErrorCodePackageNotFound, "package missing", nil, false)
	if !IsCode(err, ErrorCodePackageNotFound) {
		t.Fatal("IsCode should return true for matching code")
	}
	if IsCode(err, ErrorCodeTimeout) {
		t.Fatal("IsCode should return false for non-matching code")
	}
}

func TestIsCodeWrapped(t *testing.T) {
	adapterErr := NewAdapterError(ErrorCodeChecksumMismatch, "checksum failed", nil, false)
	wrapped := errors.Join(adapterErr, errors.New("other"))
	if !IsCode(wrapped, ErrorCodeChecksumMismatch) {
		t.Fatal("IsCode should work with wrapped errors")
	}
}

func TestIsCodeOrdinaryError(t *testing.T) {
	err := errors.New("ordinary error")
	if IsCode(err, ErrorCodeInternalError) {
		t.Fatal("IsCode should return false for non-AdapterError")
	}
}

func TestClassifyError(t *testing.T) {
	err := NewAdapterError(ErrorCodeVersionConflict, "conflict", nil, false)
	if ClassifyError(err) != ErrorCodeVersionConflict {
		t.Fatalf("ClassifyError should return code from AdapterError")
	}
}

func TestClassifyErrorOrdinary(t *testing.T) {
	err := errors.New("ordinary error")
	if ClassifyError(err) != ErrorCodeInternalError {
		t.Fatal("ClassifyError should return internal_error for ordinary error")
	}
}

func TestClassifyErrorWrapped(t *testing.T) {
	adapterErr := NewAdapterError(ErrorCodeLockConflict, "lock conflict", nil, false)
	wrapped := errors.Join(adapterErr, errors.New("other"))
	if ClassifyError(wrapped) != ErrorCodeLockConflict {
		t.Fatal("ClassifyError should work with wrapped errors")
	}
}

func TestRetryableFlag(t *testing.T) {
	retryable := NewAdapterError(ErrorCodeProviderUnavailable, "unavailable", nil, true)
	if !retryable.IsRetryable() {
		t.Fatal("retryable error should return true")
	}
	nonRetryable := NewAdapterError(ErrorCodeInvalidRequest, "invalid", nil, false)
	if nonRetryable.IsRetryable() {
		t.Fatal("non-retryable error should return false")
	}
}

func TestNewAdapterErrorUnknownCode(t *testing.T) {
	err := NewAdapterError(ErrorCode("unknown_code"), "message", nil, false)
	if err.Code != ErrorCodeInternalError {
		t.Fatalf("unknown code should default to internal_error, got %s", err.Code)
	}
}

func TestNewAdapterErrorEmptyMessage(t *testing.T) {
	err := NewAdapterError(ErrorCodeTimeout, "", nil, true)
	if err.Message != "adapter error" {
		t.Fatalf("empty message should default to 'adapter error', got %q", err.Message)
	}
}

func TestKnownErrorCodes(t *testing.T) {
	codes := []ErrorCode{
		ErrorCodeInvalidRequest,
		ErrorCodeValidationFailed,
		ErrorCodeUnsupportedCapability,
		ErrorCodeProviderUnavailable,
		ErrorCodePackageNotFound,
		ErrorCodeVersionConflict,
		ErrorCodeLockConflict,
		ErrorCodeChecksumMismatch,
		ErrorCodeIOError,
		ErrorCodeTimeout,
		ErrorCodeCancelled,
		ErrorCodeExternalCommandFailed,
		ErrorCodeInternalError,
	}
	for _, code := range codes {
		if !knownErrorCodes[code] {
			t.Fatalf("code %s should be known", code)
		}
		err := NewAdapterError(code, "test", nil, false)
		if err.Code != code {
			t.Fatalf("NewAdapterError should preserve known code %s", code)
		}
	}
}
