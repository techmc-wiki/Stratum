package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newArtifactsCommand() *cobra.Command {
	cmd := groupCommand("artifacts", "Manage artifacts")
	cmd.AddCommand(storeCommand("list", "List artifacts", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return listArtifacts(ctx, runtime.store, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("inspect", "Inspect an artifact", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return inspectArtifact(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("create", "Create artifact metadata", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return createArtifact(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("import-file", "Import artifact payload", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return importArtifactFile(ctx, runtime.store, runtime.artifactBlobRoot, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(rawCommand("blobs", "Manage artifact blobs", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return artifactBlobs(ctx, runtime.artifactBlobRoot, args, runtime.stdout, runtime.stderr)
	}))
	for _, reviewAction := range []string{"approve", "reject"} {
		cmd.AddCommand(artifactReviewCommand(reviewAction))
	}
	cmd.AddCommand(storeAgentCommand("staging", "Manage artifact staging", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return artifactStaging(ctx, runtime.store, runtime.artifactBlobRoot, runtime.agentClient, runtime.agentMode, runtime.hasAgentURL(), args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeAgentCommand("apply", "Manage artifact apply plans", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return artifactApply(ctx, runtime.store, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	return cmd
}

func artifactReviewCommand(action string) *cobra.Command {
	return storeCommand(action, "Review an artifact", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return reviewArtifact(ctx, runtime.store, runtime.artifactBlobRoot, action, args, runtime.stdout, runtime.stderr)
	})
}
