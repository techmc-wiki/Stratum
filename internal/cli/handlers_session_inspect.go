package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	observationsvc "github.com/stratummc/stratum/internal/observation/service"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func inspectSession(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions inspect", stderr)
	id := flags.String("id", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	value, err := store.GetSession(ctx, *id)
	if err != nil {
		return reportError(stderr, "inspect session", err)
	}
	observed, err := agentClient.InspectSession(ctx, *id)
	if err != nil {
		return reportError(stderr, "inspect session agent", err)
	}
	fmt.Fprintf(stdout, "id=%s project=%s room=%s type=%s state=%s sessionRuntimeProfile=%s agent=%s agentStatus=%s runtimeMessage=%q endpoint=%s observed=%s process=%s runtimeProfile=%s runtimeType=%s runtimeMode=%s pid=%d running=%t crashed=%t exitCode=%s runtimeError=%q sessionRoot=%s workDir=%s logsDir=%s\n",
		value.ID, value.ProjectID, value.RoomID, value.Type, value.State, value.RuntimeProfileID, value.AssignedAgentID,
		value.LastAgentStatus, value.LastRuntimeMessage, value.RuntimeEndpoint, observed.Status, observed.ProcessID,
		observed.RuntimeProfileID, observed.RuntimeType, observed.RuntimeMode, observed.PID, observed.Running, observed.Crashed, optionalInt(observed.ExitCode), observed.LastError, observed.SessionRoot, observed.WorkDir, observed.LogsDir)
	return 0
}

func observeSession(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions observe", stderr)
	id := flags.String("id", "", "session ID")
	actor := flags.String("actor", "cli", "actor user ID for observation audit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || strings.TrimSpace(*actor) == "" {
		fmt.Fprintln(stderr, "--id and --actor are required")
		return 2
	}
	controller, err := store.GetSession(ctx, *id)
	if err != nil {
		return reportError(stderr, "observe session", err)
	}

	options := observationsvc.Options{ObservedAt: time.Now().UTC()}
	if expected, err := expectedRuntimeProfile(ctx, store, *id); err != nil {
		return reportError(stderr, "load session runtime profile", err)
	} else {
		options.ExpectedRuntimeProfileID = expected
	}
	status, inspectErr := agentClient.InspectSession(ctx, *id)
	var observed *agent.SessionStatus
	if inspectErr == nil {
		observed = &status
	} else {
		options.Metadata = map[string]string{"agentInspectError": inspectErr.Error()}
	}
	result := observationsvc.Observe(controller, observed, options)
	if err := store.CreateRuntimeObservation(ctx, result); err != nil {
		return reportError(stderr, "persist runtime observation", err)
	}
	if err := appendRuntimeObservationAudit(ctx, store, result, *actor); err != nil {
		return reportError(stderr, "audit runtime observation", err)
	}
	fmt.Fprintf(stdout, "id=%s session=%s controllerState=%s agentStatus=%s agent=%s runtimeProfile=%s process=%s pid=%d mismatch=%t mismatchType=%s severity=%s recommendedAction=%s observedAt=%s persisted=true\n",
		result.ID, result.SessionID, result.ControllerSessionState, result.AgentRuntimeStatus, result.ObserverAgentID,
		result.RuntimeProfileID, result.ProcessID, result.PID, result.MismatchDetected, result.MismatchType,
		result.Severity, result.RecommendedAction, result.ObservedAt.Format(time.RFC3339Nano))
	return 0
}
