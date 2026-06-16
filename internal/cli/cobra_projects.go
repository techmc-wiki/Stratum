package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newProjectsCommand() *cobra.Command {
	cmd := groupCommand("projects", "Manage projects")
	cmd.AddCommand(storeCommand("create", "Create a project", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return createProject(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("list", "List projects", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return listProjects(ctx, runtime.store, runtime.stdout, runtime.stderr)
	}))
	return cmd
}
