package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newOperationsCommand() *cobra.Command {
	cmd := groupCommand("operations", "Inspect operations")
	cmd.AddCommand(storeCommand("list", "List operations", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return listOperations(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("inspect", "Inspect an operation", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return inspectOperation(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	return cmd
}
