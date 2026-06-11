package lucy

import "context"

// Manager handles declarative environment files and consistency checks only.
// It intentionally has no process start, stop, or command methods.
type Manager interface {
	Resolve(context.Context, string) error
	Verify(context.Context, string) (string, error)
	LockHash(context.Context, string) (string, error)
}

// TODO: implement real Lucy manifest and lock-file integration.
