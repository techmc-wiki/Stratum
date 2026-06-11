package agent

import "context"

type Runtime interface {
	Prepare(context.Context, string) error
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Inspect(context.Context, string) (Status, error)
}

type Status struct {
	Running bool
	Message string
}

// TODO: implement authenticated controller-to-agent communication and real process supervision.
