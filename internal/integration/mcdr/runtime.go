package mcdr

import (
	"context"
	"io"
)

// Runtime is the only control-plane boundary allowed to supervise Minecraft
// through MCDR. Lucy implementations must never implement this interface.
type Runtime interface {
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Restart(context.Context, string) error
	SendCommand(context.Context, string, string) error
	Logs(context.Context, string) (io.ReadCloser, error)
}

// TODO: implement the real MCDR runtime bridge.
