package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newEnvironmentsCommand() *cobra.Command {
	cmd := groupCommand("environments", "Manage environments")
	cmd.AddCommand(storeCommand("create", "Create an environment", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return createEnvironment(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(rawCommand("validate-file", "Validate an environment file", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return validateEnvironmentFile(args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("import-file", "Import an environment file", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return importEnvironmentFile(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("list", "List environments", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return listEnvironments(ctx, runtime.store, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("inspect", "Inspect an environment", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return inspectEnvironment(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("update", "Update an environment", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return updateEnvironment(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeAgentCommand("materialize", "Materialize an environment", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return materializeEnvironment(ctx, runtime.store, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(rawCommand("latest-version", "Show the latest Minecraft release version", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return showLatestVersion(ctx, runtime.stdout, runtime.stderr)
	}))
	return cmd
}
