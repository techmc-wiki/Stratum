package storage

import (
	"context"
	"io"
)

type Backend interface {
	Put(context.Context, string, io.Reader) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
	CreateWorldSnapshot(context.Context, string, string) (string, error)
	RestoreWorldSnapshot(context.Context, string, string) error
}

// TODO: implement immutable base-world and checkpoint storage backends.
