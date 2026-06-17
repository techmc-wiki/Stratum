package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newSessionsCommand() *cobra.Command {
	cmd := groupCommand("sessions", "Manage sessions")
	cmd.AddCommand(storeCommand("create", "Create a session", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return createSession(ctx, runtime.store, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeCommand("list", "List sessions", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return listSessions(ctx, runtime.store, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeAgentCommand("inspect", "Inspect a session", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return inspectSession(ctx, runtime.store, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeAgentCommand("observe", "Observe session runtime state", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return observeSession(ctx, runtime.store, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(storeAgentCommand("reconcile", "Reconcile session state", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return reconcileSession(ctx, runtime.store, runtime.agentClient, runtime.agentMode, runtime.hasAgentURL(), args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(agentCommand("logs", "Collect session logs", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return sessionLogs(ctx, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(agentCommand("runtime-status", "Inspect session runtime layout", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return sessionRuntimeStatus(ctx, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(agentCommand("artifacts", "Inspect materialized session artifacts", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return sessionArtifacts(ctx, runtime.agentClient, runtime.hasAgentURL(), args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(agentCommand("applied-artifacts", "Inspect applied session artifacts", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return sessionsAppliedArtifacts(ctx, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(agentCommand("mcdr-config-stub", "Inspect MCDR config stubs", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return sessionsMCDRConfigStub(ctx, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	cmd.AddCommand(agentCommand("send-command", "Send a command to session runtime stdin", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return sessionSendCommand(ctx, runtime.agentClient, args, runtime.stdout, runtime.stderr)
	}))
	for _, lifecycleAction := range []string{"prepare", "start", "stop", "restart", "freeze", "unfreeze", "mark-crashed", "archive", "delete"} {
		cmd.AddCommand(sessionLifecycleCommand(lifecycleAction))
	}
	return cmd
}

func sessionLifecycleCommand(action string) *cobra.Command {
	return storeAgentCommand(action, "Run session lifecycle operation", func(ctx context.Context, runtime *commandRuntime, args []string) int {
		return runSessionLifecycle(ctx, runtime.store, runtime.artifactBlobRoot, runtime.agentClient, runtime.agentMode, runtime.hasAgentURL(), action, args, runtime.stdout, runtime.stderr)
	})
}
