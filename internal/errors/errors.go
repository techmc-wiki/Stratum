package errors

import "fmt"

type Kind string

const (
	KindValidation Kind = "validation"
	KindNotFound   Kind = "not_found"
	KindConflict   Kind = "conflict"
	KindForbidden  Kind = "forbidden"
)

type Error struct {
	Kind      Kind
	Operation string
	Message   string
	Cause     error
}

func (e Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %s: %v", e.Kind, e.Operation, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s: %s", e.Kind, e.Operation, e.Message)
}

func (e Error) Unwrap() error { return e.Cause }

func IsKind(err error, kind Kind) bool {
	for err != nil {
		if typed, ok := err.(Error); ok && typed.Kind == kind {
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}
