package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newRuntimeObservationsCommand() *cobra.Command {
	cmd := groupCommand("runtime-observations", "Inspect runtime observations")
	cmd.AddCommand(storeCommand("list", "List runtime observations", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return listRuntimeObservations(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("inspect", "Inspect a runtime observation", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return inspectRuntimeObservation(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	return cmd
}
