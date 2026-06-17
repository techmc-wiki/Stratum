package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newCheckpointsCommand() *cobra.Command {
	cmd := groupCommand("checkpoints", "Manage checkpoints")
	cmd.AddCommand(storeAgentCommand("create", "Create a checkpoint", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return createCheckpoint(ctx, runtime.store, runtime.agentClient, runtime.hasAgentURL(), args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("list", "List checkpoints", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return listCheckpoints(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("inspect", "Inspect a checkpoint", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return inspectCheckpoint(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeAgentCommand("restore", "Restore world state from a checkpoint to a target session", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return restoreCheckpoint(ctx, runtime.store, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	return cmd
}
