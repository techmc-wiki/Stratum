package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/stratummc/stratum/internal/audit"
	runtimeobservation "github.com/stratummc/stratum/internal/runtime/observation"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func listRuntimeObservations(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("runtime-observations list", stderr)
	sessionID := flags.String("session", "", "filter by session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	var values []runtimeobservation.Observation
	var err error
	if *sessionID != "" {
		values, err = store.ListRuntimeObservationsBySession(ctx, *sessionID)
	} else {
		values, err = store.ListRuntimeObservations(ctx)
	}
	if err != nil {
		return reportError(stderr, "list runtime observations", err)
	}
	for _, value := range values {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", value.ID, value.SessionID, value.ObservedAt.Format(time.RFC3339Nano), value.MismatchType, value.Severity, value.RecommendedAction, value.ControllerSessionState, value.AgentRuntimeStatus)
	}
	return 0
}

func inspectRuntimeObservation(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("runtime-observations inspect", stderr)
	id := flags.String("id", "", "runtime observation ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	value, err := store.GetRuntimeObservation(ctx, *id)
	if err != nil {
		return reportError(stderr, "inspect runtime observation", err)
	}
	fmt.Fprintf(stdout, "id=%s session=%s project=%s room=%s observedAt=%s agent=%s controllerState=%s agentStatus=%s runtimeProfile=%s process=%s pid=%d exitCode=%s crashed=%t logsAvailable=%t mismatch=%t mismatchType=%s severity=%s recommendedAction=%s lastError=%q\n",
		value.ID, value.SessionID, value.ProjectID, value.RoomID, value.ObservedAt.Format(time.RFC3339Nano), value.ObserverAgentID,
		value.ControllerSessionState, value.AgentRuntimeStatus, value.RuntimeProfileID, value.ProcessID, value.PID,
		optionalInt(value.ExitCode), value.Crashed, value.LogsAvailable, value.MismatchDetected, value.MismatchType, value.Severity, value.RecommendedAction, value.LastError)
	return 0
}

func appendRuntimeObservationAudit(ctx context.Context, store *filesystem.Store, observation runtimeobservation.Observation, actorID string) error {
	event, err := audit.NewEvent("audit-"+observation.ID, actorID, "runtime.observation.created", "runtime-observation", observation.ID, observation.ObservedAt)
	if err != nil {
		return err
	}
	event.ProjectID = observation.ProjectID
	event.Metadata = map[string]string{
		"observationId":          observation.ID,
		"sessionId":              observation.SessionID,
		"mismatchType":           string(observation.MismatchType),
		"severity":               string(observation.Severity),
		"recommendedAction":      string(observation.RecommendedAction),
		"agentRuntimeStatus":     observation.AgentRuntimeStatus,
		"controllerSessionState": observation.ControllerSessionState,
	}
	return store.AppendAuditEvent(ctx, event)
}
