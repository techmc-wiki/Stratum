package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newAgentsCommand() *cobra.Command {
	cmd := groupCommand("agents", "Inspect agents")
	cmd.AddCommand(agentCommand("list", "List agents", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return listAgents(ctx, runtime.agentClient, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(agentCommand("inspect", "Inspect an agent", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return inspectAgent(ctx, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(agentCommand("resources", "Inspect agent resources", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return agentResources(ctx, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(agentCommand("runtime-profiles", "List agent runtime profiles", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return agentRuntimeProfiles(ctx, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	return cmd
}
