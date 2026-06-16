package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	observationsvc "github.com/stratummc/stratum/internal/observation/service"
	"github.com/stratummc/stratum/internal/operation"
	reconcilesvc "github.com/stratummc/stratum/internal/reconcile/service"
	runtimeobservation "github.com/stratummc/stratum/internal/runtime/observation"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func reconcileSession(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, agentMode string, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "mark-stopped" && args[0] != "mark-crashed" && args[0] != "stop-runtime") {
		fmt.Fprintln(stderr, "usage: stratum sessions reconcile <mark-stopped|mark-crashed|stop-runtime> --id ID --actor ACTOR --reason REASON")
		return 2
	}
	action := args[0]
	flags := newFlagSet("sessions reconcile "+action, stderr)
	id := flags.String("id", "", "session ID")
	actor := flags.String("actor", "", "actor user ID")
	reason := flags.String("reason", "", "manual reconciliation reason")
	requestID := flags.String("request-id", "", "request correlation ID")
	idempotencyKey := flags.String("idempotency-key", "", "deduplicate this reconciliation request")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *id == "" || strings.TrimSpace(*actor) == "" || strings.TrimSpace(*reason) == "" {
		fmt.Fprintln(stderr, "--id, --actor, and --reason are required")
		return 2
	}
	if action == "stop-runtime" && !hasAgentURL {
		fmt.Fprintln(stderr, "sessions reconcile stop-runtime requires --agent-url")
		return 2
	}

	if action == "stop-runtime" {
		expected, err := expectedRuntimeProfile(ctx, store, *id)
		if err != nil {
			return reportError(stderr, "load session runtime profile", err)
		}
		value, replay, err := reconcilesvc.New(store).StopRuntime(ctx, agentClient, *id, *actor, *reason, reconcilesvc.StopRuntimeOptions{
			RequestID: *requestID, IdempotencyKey: *idempotencyKey, AgentMode: agentMode, ExpectedRuntimeProfileID: expected,
		})
		if err != nil {
			return reportError(stderr, "session reconcile stop-runtime", err)
		}
		fmt.Fprintf(stdout, "Session %s reconciliation stop-runtime status=%s operation=%s request=%s replay=%t agentResult=%s.\n",
			*id, value.Status, value.ID, value.RequestID, replay, value.Metadata["agentResult"])
		return 0
	}

	options := reconcilesvc.Options{RequestID: *requestID, IdempotencyKey: *idempotencyKey}
	if hasAgentURL {
		observation, inspectErr, err := persistReconcileObservation(ctx, store, agentClient, *id, *actor)
		if err != nil {
			return reportError(stderr, "persist reconciliation observation", err)
		}
		if inspectErr != nil {
			options.InspectError = inspectErr.Error()
		} else {
			options.Observation = observation
		}
	}

	service := reconcilesvc.New(store)
	var value operation.Operation
	var replay bool
	var err error
	if action == "mark-crashed" {
		value, replay, err = service.MarkCrashed(ctx, *id, *actor, *reason, options)
	} else {
		value, replay, err = service.MarkStopped(ctx, *id, *actor, *reason, options)
	}
	if err != nil {
		return reportError(stderr, "session reconcile "+action, err)
	}
	fmt.Fprintf(stdout, "Session %s reconciliation %s status=%s operation=%s request=%s replay=%t observationAvailable=%s.\n",
		*id, action, value.Status, value.ID, value.RequestID, replay, value.Metadata["observationAvailable"])
	return 0
}

func persistReconcileObservation(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, sessionID, actor string) (*runtimeobservation.Observation, error, error) {
	controller, err := store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	observationOptions := observationsvc.Options{ObservedAt: time.Now().UTC()}
	if expected, err := expectedRuntimeProfile(ctx, store, sessionID); err != nil {
		return nil, nil, err
	} else {
		observationOptions.ExpectedRuntimeProfileID = expected
	}
	status, inspectErr := agentClient.InspectSession(ctx, sessionID)
	if inspectErr != nil {
		return nil, inspectErr, nil
	}
	observation := observationsvc.Observe(controller, &status, observationOptions)
	if err := store.CreateRuntimeObservation(ctx, observation); err != nil {
		return nil, nil, err
	}
	if err := appendRuntimeObservationAudit(ctx, store, observation, actor); err != nil {
		return nil, nil, err
	}
	return &observation, nil, nil
}

func expectedRuntimeProfile(ctx context.Context, store *filesystem.Store, sessionID string) (string, error) {
	values, err := store.ListOperationsBySession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	var latest operation.Operation
	var latestAt time.Time
	for _, value := range values {
		if value.Status != operation.StatusSucceeded || (value.Action != "start" && value.Action != "restart") {
			continue
		}
		profileID := value.Metadata["runtimeProfileId"]
		if profileID == "" {
			continue
		}
		completedAt := value.CreatedAt
		if value.CompletedAt != nil {
			completedAt = *value.CompletedAt
		}
		if latestAt.IsZero() || completedAt.After(latestAt) {
			latest, latestAt = value, completedAt
		}
	}
	return latest.Metadata["runtimeProfileId"], nil
}

func optionalInt(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}

func sessionLogs(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions logs", stderr)
	id := flags.String("id", "", "session ID")
	maxBytes := flags.Int("max-bytes", 0, "maximum output bytes; zero returns all available logs")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	batch, err := agentClient.CollectLogs(agent.WithLogMaxBytes(ctx, *maxBytes), *id)
	if err != nil {
		return reportError(stderr, "collect session logs", err)
	}
	remaining := *maxBytes
	for _, line := range batch.Lines {
		output := line + "\n"
		if remaining > 0 && len(output) > remaining {
			_, _ = io.WriteString(stdout, output[:remaining])
			break
		}
		_, _ = io.WriteString(stdout, output)
		if remaining > 0 {
			remaining -= len(output)
			if remaining == 0 {
				break
			}
		}
	}
	return 0
}

func sessionRuntimeStatus(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions runtime-status", stderr)
	id := flags.String("id", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	status, err := agentClient.GetSessionRuntimeStatus(ctx, *id)
	if err != nil {
		return reportError(stderr, "get session runtime status", err)
	}
	fmt.Fprintf(stdout, "Session: %s\n", status.SessionID)
	fmt.Fprintf(stdout, "Checked at: %s\n", status.CheckedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "Runtime root exists: %t\n", status.RuntimeRootExists)
	fmt.Fprintf(stdout, "Session root exists: %t\n", status.SessionRootExists)
	fmt.Fprintf(stdout, "Directories:\n")
	fmt.Fprintf(stdout, "  work: %t\n", status.WorkDirExists)
	fmt.Fprintf(stdout, "  config: %t\n", status.ConfigDirExists)
	fmt.Fprintf(stdout, "  logs: %t\n", status.LogsDirExists)
	fmt.Fprintf(stdout, "  artifacts: %t\n", status.ArtifactsDirExists)
	fmt.Fprintf(stdout, "  checkpoints: %t\n", status.CheckpointsDirExists)
	fmt.Fprintf(stdout, "  tmp: %t\n", status.TmpDirExists)
	if status.EnvironmentManifest != nil {
		fmt.Fprintf(stdout, "Environment manifest:\n")
		fmt.Fprintf(stdout, "  exists: %t\n", status.EnvironmentManifest.Exists)
		if status.EnvironmentManifest.Exists {
			fmt.Fprintf(stdout, "  path: %s\n", status.EnvironmentManifest.Path)
			fmt.Fprintf(stdout, "  status: %s\n", status.EnvironmentManifest.Status)
			if status.EnvironmentManifest.EnvironmentID != "" {
				fmt.Fprintf(stdout, "  environment: %s\n", status.EnvironmentManifest.EnvironmentID)
			}
			if status.EnvironmentManifest.MinecraftVersion != "" {
				fmt.Fprintf(stdout, "  minecraft: %s\n", status.EnvironmentManifest.MinecraftVersion)
			}
			if status.EnvironmentManifest.LoaderType != "" {
				fmt.Fprintf(stdout, "  loader: %s\n", status.EnvironmentManifest.LoaderType)
			}
			if status.EnvironmentManifest.ServerCore != "" {
				fmt.Fprintf(stdout, "  server-core: %s\n", status.EnvironmentManifest.ServerCore)
			}
			if status.EnvironmentManifest.RuntimeProfileID != "" {
				fmt.Fprintf(stdout, "  runtime-profile: %s\n", status.EnvironmentManifest.RuntimeProfileID)
			}
		}
	}
	if status.MCDRLayout != nil {
		fmt.Fprintf(stdout, "MCDR layout:\n")
		fmt.Fprintf(stdout, "  root exists: %t\n", status.MCDRLayout.MCDRRootExists)
		fmt.Fprintf(stdout, "  manifest exists: %t\n", status.MCDRLayout.ManifestExists)
		if status.MCDRLayout.ManifestExists {
			fmt.Fprintf(stdout, "  manifest path: %s\n", status.MCDRLayout.ManifestPath)
		}
	}
	if status.MaterializedArtifacts != nil {
		fmt.Fprintf(stdout, "Materialized artifacts:\n")
		fmt.Fprintf(stdout, "  manifest exists: %t\n", status.MaterializedArtifacts.ManifestExists)
		if status.MaterializedArtifacts.ManifestExists {
			fmt.Fprintf(stdout, "  manifest path: %s\n", status.MaterializedArtifacts.ManifestPath)
			fmt.Fprintf(stdout, "  count: %d\n", status.MaterializedArtifacts.Count)
		}
	}
	if status.AppliedArtifacts != nil {
		fmt.Fprintf(stdout, "Applied artifacts:\n")
		fmt.Fprintf(stdout, "  manifest exists: %t\n", status.AppliedArtifacts.ManifestExists)
		if status.AppliedArtifacts.ManifestExists {
			fmt.Fprintf(stdout, "  manifest path: %s\n", status.AppliedArtifacts.ManifestPath)
			fmt.Fprintf(stdout, "  count: %d\n", status.AppliedArtifacts.Count)
		}
	}
	if status.ProcessStatus != nil {
		fmt.Fprintf(stdout, "Process:\n")
		fmt.Fprintf(stdout, "  status: %s\n", status.ProcessStatus.Status)
		if status.ProcessStatus.RuntimeProfileID != "" {
			fmt.Fprintf(stdout, "  runtime-profile: %s\n", status.ProcessStatus.RuntimeProfileID)
		}
		if status.ProcessStatus.PID > 0 {
			fmt.Fprintf(stdout, "  pid: %d\n", status.ProcessStatus.PID)
		}
		fmt.Fprintf(stdout, "  crashed: %t\n", status.ProcessStatus.Crashed)
		if status.ProcessStatus.StartedAt != nil {
			fmt.Fprintf(stdout, "  started-at: %s\n", status.ProcessStatus.StartedAt.Format(time.RFC3339))
		}
		if status.ProcessStatus.StoppedAt != nil {
			fmt.Fprintf(stdout, "  stopped-at: %s\n", status.ProcessStatus.StoppedAt.Format(time.RFC3339))
		}
	}
	return 0
}

func sessionArtifacts(ctx context.Context, agentClient agent.AgentClient, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "inspect" {
		return inspectSessionArtifact(ctx, agentClient, hasAgentURL, args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "verify" {
		return verifySessionArtifact(ctx, agentClient, hasAgentURL, args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "verify-all" {
		return verifySessionArtifacts(ctx, agentClient, hasAgentURL, args[1:], stdout, stderr)
	}
	flags := newFlagSet("sessions artifacts", stderr)
	id := flags.String("id", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	if !hasAgentURL {
		fmt.Fprintln(stderr, "--agent-url is required for materialized artifact inspection")
		return 2
	}
	result, err := agentClient.InspectMaterializedArtifacts(ctx, *id)
	if err != nil {
		return reportError(stderr, "inspect materialized artifacts", err)
	}
	if len(result.Items) == 0 {
		fmt.Fprintf(stdout, "session=%s agent=%s status=%s materializedArtifacts=0\n", result.SessionID, result.AgentID, result.Status)
		return 0
	}
	for _, item := range result.Items {
		fmt.Fprintf(stdout, "session=%s artifact=%s plan=%s name=%q type=%s target=%s algorithm=%s hash=%s size=%d runtimePath=%s actor=%s status=%s materializedAt=%s\n", result.SessionID, item.ArtifactID, item.StagingPlanID, item.ArtifactName, item.ArtifactType, item.TargetName, item.PayloadAlgorithm, item.PayloadHash, item.PayloadSize, item.RuntimeRelativePath, item.ActorID, item.Status, item.MaterializedAt.Format(time.RFC3339Nano))
	}
	return 0
}

func verifySessionArtifacts(ctx context.Context, agentClient agent.AgentClient, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions artifacts verify-all", stderr)
	id := flags.String("id", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	if !hasAgentURL {
		fmt.Fprintln(stderr, "--agent-url is required for materialized artifact verification")
		return 2
	}
	result, err := agentClient.VerifyMaterializedArtifacts(ctx, *id)
	if err != nil {
		return reportError(stderr, "verify materialized artifacts", err)
	}
	fmt.Fprintf(stdout, "session=%s agent=%s total=%d valid=%d missing=%d corrupted=%d errors=%d verifiedAt=%s\n", result.SessionID, result.AgentID, result.Total, result.ValidCount, result.MissingCount, result.CorruptedCount, result.ErrorCount, result.VerifiedAt.Format(time.RFC3339Nano))
	for _, entry := range result.Entries {
		fmt.Fprintf(stdout, "artifact=%s plan=%s target=%s runtimePath=%s algorithm=%s expectedHash=%s actualHash=%s payloadSize=%d actualSize=%d status=%s error=%q\n", entry.ArtifactID, entry.StagingPlanID, entry.TargetName, entry.RuntimeRelativePath, entry.PayloadAlgorithm, entry.ExpectedHash, entry.ActualHash, entry.PayloadSize, entry.ActualSize, entry.Status, entry.ErrorMessage)
	}
	if result.MissingCount > 0 || result.CorruptedCount > 0 || result.ErrorCount > 0 {
		return 1
	}
	return 0
}

func verifySessionArtifact(ctx context.Context, agentClient agent.AgentClient, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions artifacts verify", stderr)
	id := flags.String("id", "", "session ID")
	planID := flags.String("plan", "", "staging plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *planID == "" {
		fmt.Fprintln(stderr, "--id and --plan are required")
		return 2
	}
	if !hasAgentURL {
		fmt.Fprintln(stderr, "--agent-url is required for materialized artifact verification")
		return 2
	}
	result, err := agentClient.VerifyMaterializedArtifact(ctx, *id, *planID)
	if err != nil {
		return reportError(stderr, "verify materialized artifact", err)
	}
	fmt.Fprintf(stdout, "session=%s agent=%s artifact=%s plan=%s target=%s runtimePath=%s algorithm=%s expectedHash=%s actualHash=%s payloadSize=%d actualSize=%d status=%s verifiedAt=%s\n", result.SessionID, result.AgentID, result.ArtifactID, result.StagingPlanID, result.TargetName, result.RuntimeRelativePath, result.PayloadAlgorithm, result.ExpectedHash, result.ActualHash, result.PayloadSize, result.ActualSize, result.Status, result.VerifiedAt.Format(time.RFC3339Nano))
	return 0
}

func inspectSessionArtifact(ctx context.Context, agentClient agent.AgentClient, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions artifacts inspect", stderr)
	id := flags.String("id", "", "session ID")
	planID := flags.String("plan", "", "staging plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *planID == "" {
		fmt.Fprintln(stderr, "--id and --plan are required")
		return 2
	}
	if !hasAgentURL {
		fmt.Fprintln(stderr, "--agent-url is required for materialized artifact inspection")
		return 2
	}
	item, err := agentClient.InspectMaterializedArtifact(ctx, *id, *planID)
	if err != nil {
		return reportError(stderr, "inspect materialized artifact", err)
	}
	fmt.Fprintf(stdout, "session=%s agent=%s artifact=%s plan=%s name=%q type=%s target=%s algorithm=%s hash=%s size=%d runtimePath=%s actor=%s status=%s materializedAt=%s\n", item.SessionID, item.AgentID, item.ArtifactID, item.StagingPlanID, item.ArtifactName, item.ArtifactType, item.TargetName, item.PayloadAlgorithm, item.PayloadHash, item.PayloadSize, item.RuntimeRelativePath, item.ActorID, item.Status, item.MaterializedAt.Format(time.RFC3339Nano))
	return 0
}
