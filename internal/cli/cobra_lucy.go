package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newLucyCommand() *cobra.Command {
	cmd := groupCommand("lucy", "Manage Lucy environment operations")
	cmd.AddCommand(storeAgentCommand("plan <environment-id>", "Plan Lucy environment changes", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return lucyPlan(ctx, runtime.store, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeAgentCommand("lock <session-id>", "Resolve and print Lucy lock metadata", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return lucyLock(ctx, runtime.store, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(agentCommand("status <session-id>", "Inspect Lucy runtime status", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return lucyStatus(ctx, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeAgentCommand("verify <session-id>", "Verify Lucy environment integrity", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return lucyVerify(ctx, runtime.store, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeAgentCommand("install <session-id>", "Install missing Lucy packages", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return lucyInstall(ctx, runtime.store, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	return cmd
}
