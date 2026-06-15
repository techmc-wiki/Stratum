package lucy

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	ErrorCodeInvalidRequest        ErrorCode = "invalid_request"
	ErrorCodeValidationFailed      ErrorCode = "validation_failed"
	ErrorCodeUnsupportedCapability ErrorCode = "unsupported_capability"
	ErrorCodeProviderUnavailable   ErrorCode = "provider_unavailable"
	ErrorCodePackageNotFound       ErrorCode = "package_not_found"
	ErrorCodeVersionConflict       ErrorCode = "version_conflict"
	ErrorCodeLockConflict          ErrorCode = "lock_conflict"
	ErrorCodeChecksumMismatch      ErrorCode = "checksum_mismatch"
	ErrorCodeIOError               ErrorCode = "io_error"
	ErrorCodeTimeout               ErrorCode = "timeout"
	ErrorCodeCancelled             ErrorCode = "cancelled"
	ErrorCodeExternalCommandFailed ErrorCode = "external_command_failed"
	ErrorCodeInternalError         ErrorCode = "internal_error"
)

var knownErrorCodes = map[ErrorCode]bool{
	ErrorCodeInvalidRequest:        true,
	ErrorCodeValidationFailed:      true,
	ErrorCodeUnsupportedCapability: true,
	ErrorCodeProviderUnavailable:   true,
	ErrorCodePackageNotFound:       true,
	ErrorCodeVersionConflict:       true,
	ErrorCodeLockConflict:          true,
	ErrorCodeChecksumMismatch:      true,
	ErrorCodeIOError:               true,
	ErrorCodeTimeout:               true,
	ErrorCodeCancelled:             true,
	ErrorCodeExternalCommandFailed: true,
	ErrorCodeInternalError:         true,
}

type AdapterError struct {
	Code      ErrorCode
	Message   string
	Cause     error
	Retryable bool
	Metadata  map[string]string
}

func (e AdapterError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("lucy adapter [%s]: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("lucy adapter [%s]: %s", e.Code, e.Message)
}

func (e AdapterError) Unwrap() error {
	return e.Cause
}

func (e AdapterError) IsRetryable() bool {
	return e.Retryable
}

func NewAdapterError(code ErrorCode, message string, cause error, retryable bool) AdapterError {
	if !knownErrorCodes[code] {
		code = ErrorCodeInternalError
	}
	if message == "" {
		message = "adapter error"
	}
	return AdapterError{Code: code, Message: message, Cause: cause, Retryable: retryable}
}

func IsCode(err error, code ErrorCode) bool {
	var adapterErr AdapterError
	if errors.As(err, &adapterErr) {
		return adapterErr.Code == code
	}
	return false
}

func ClassifyError(err error) ErrorCode {
	var adapterErr AdapterError
	if errors.As(err, &adapterErr) {
		return adapterErr.Code
	}
	return ErrorCodeInternalError
}
