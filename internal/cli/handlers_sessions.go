package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	artifactstagingsvc "github.com/stratummc/stratum/internal/artifact/stagingservice"
	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/checkpoint/consistency"
	"github.com/stratummc/stratum/internal/idgen"
	"github.com/stratummc/stratum/internal/operation"
	"github.com/stratummc/stratum/internal/resourcepolicy"
	"github.com/stratummc/stratum/internal/session"
	sessionsvc "github.com/stratummc/stratum/internal/session/service"
	"github.com/stratummc/stratum/internal/storage/artifactblob"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func createSession(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions create", stderr)
	id := flags.String("id", "", "session ID")
	projectID := flags.String("project", "", "project ID")
	roomID := flags.String("room", "", "room ID")
	typeValue := flags.String("type", string(session.TypeShared), "session type")
	ownerID := flags.String("owner", "cli", "owner user ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *projectID == "" {
		fmt.Fprintln(stderr, "--id and --project are required")
		return 2
	}
	requestedType, ok := parseSessionType(*typeValue)
	if !ok {
		fmt.Fprintf(stderr, "unsupported session type %q\n", *typeValue)
		return 2
	}
	if requestedType == session.TypeShared && *roomID == "" {
		fmt.Fprintln(stderr, "shared sessions require --room")
		return 2
	}
	if _, err := store.GetProject(ctx, *projectID); err != nil {
		return reportError(stderr, "find project", err)
	}
	environmentID := ""
	if *roomID != "" {
		roomValue, err := store.GetRoom(ctx, *roomID)
		if err != nil {
			return reportError(stderr, "find room", err)
		}
		if roomValue.ProjectID != *projectID {
			fmt.Fprintln(stderr, "room belongs to a different project")
			return 2
		}
		environmentID = roomValue.EnvironmentID
	}
	now := time.Now().UTC()
	value := session.Session{ID: *id, ProjectID: *projectID, RoomID: *roomID, OwnerUserID: *ownerID, Type: requestedType, State: session.StateCreated, EnvironmentID: environmentID, CreatedAt: now, LastActiveAt: now}
	svc := sessionsvc.New(store, resourcepolicy.MVPDefault())
	if err := svc.Create(ctx, value); err != nil {
		return reportError(stderr, "create session", err)
	}
	fmt.Fprintf(stdout, "Created %s session %s in state %s. Runtime is not started.\n", value.Type, value.ID, value.State)
	return 0
}

func listSessions(ctx context.Context, store *filesystem.Store, stdout, stderr io.Writer) int {
	values, err := store.ListSessions(ctx)
	if err != nil {
		return reportError(stderr, "list sessions", err)
	}
	for _, value := range values {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", value.ID, value.ProjectID, value.Type, value.State, value.EnvironmentID)
	}
	return 0
}

func runSessionLifecycle(ctx context.Context, store *filesystem.Store, blobRoot string, agentClient agent.AgentClient, agentMode string, hasAgentURL bool, action string, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions "+action, stderr)
	id := flags.String("id", "", "session ID")
	actor := flags.String("actor", "", "actor user ID")
	reason := flags.String("reason", "", "operation reason")
	idempotencyKey := flags.String("idempotency-key", "", "deduplicate this actor/action/session request")
	requestID := flags.String("request-id", "", "request correlation ID")
	operationTimeout := flags.Duration("operation-timeout", 0, "maximum lifecycle operation duration")
	runtimeProfileID := flags.String("runtime-profile", "", "trusted Agent runtime profile ID (start/restart only)")
	preOpCheckpoint := flags.Bool("pre-op-checkpoint", false, "create world snapshot before dangerous operations (restart)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *actor == "" {
		fmt.Fprintln(stderr, "--id and --actor are required")
		return 2
	}
	policy, err := ensureResourcePolicy(ctx, store)
	if err != nil {
		return reportError(stderr, "prepare resource policy", err)
	}
	service := sessionsvc.New(store, policy, agentClient)
	if action == "start" && hasAgentURL {
		blobs, openErr := artifactblob.Open(blobRoot)
		if openErr != nil {
			return reportError(stderr, "open artifact blob store", openErr)
		}
		service.WithArtifactReadinessGate(sessionArtifactReadinessGate{service: artifactstagingsvc.NewPreStartService(store, blobs, agentClient)})
	}
	if *preOpCheckpoint && hasAgentURL {
		service.WithPreOpCheckpoint(makePreOpCheckpointFunc(store, agentClient))
	}
	options := sessionsvc.OperationOptions{IdempotencyKey: *idempotencyKey, RequestID: *requestID, Timeout: *operationTimeout, RuntimeProfileID: *runtimeProfileID, CreatePreOpCheckpoint: *preOpCheckpoint}
	var operationValue operation.Operation
	var replay bool
	switch action {
	case "prepare":
		operationValue, replay, err = service.PrepareWithOptions(ctx, *id, *actor, options)
	case "start":
		operationValue, replay, err = service.StartWithOptions(ctx, *id, *actor, options)
	case "stop":
		operationValue, replay, err = service.StopWithOptions(ctx, *id, *actor, options)
	case "restart":
		operationValue, replay, err = service.RestartWithOptions(ctx, *id, *actor, options)
	case "freeze":
		operationValue, replay, err = service.FreezeWithOptions(ctx, *id, *actor, options)
	case "unfreeze":
		operationValue, replay, err = service.UnfreezeWithOptions(ctx, *id, *actor, options)
	case "mark-crashed":
		operationValue, replay, err = service.MarkCrashedWithOptions(ctx, *id, *actor, *reason, options)
	case "archive":
		operationValue, replay, err = service.ArchiveWithOptions(ctx, *id, *actor, options)
	case "delete":
		operationValue, replay, err = service.DeleteWithOptions(ctx, *id, *actor, options)
	}
	if err != nil {
		return reportError(stderr, "session "+action, err)
	}
	fmt.Fprintf(stdout, "Session %s operation %s status=%s operation=%s request=%s replay=%t via=%s.\n", *id, action, operationValue.Status, operationValue.ID, operationValue.RequestID, replay, agentMode)
	return 0
}

type sessionArtifactReadinessGate struct {
	service *artifactstagingsvc.PreStartService
}

func (g sessionArtifactReadinessGate) Check(ctx context.Context, sessionID string) (map[string]string, error) {
	result, err := g.service.Check(ctx, sessionID)
	return result.Metadata(), err
}

func makePreOpCheckpointFunc(store *filesystem.Store, agentClient agent.AgentClient) sessionsvc.PreOpCheckpointFunc {
	return func(ctx context.Context, sessionID, actorID string) error {
		sess, err := store.GetSession(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("get session for pre-op checkpoint: %w", err)
		}
		snapResult, err := agentClient.CreateWorldSnapshot(ctx, agent.WorldCheckpointRequest{SessionID: sessionID})
		if err != nil {
			return fmt.Errorf("create world snapshot: %w", err)
		}
		cpID, idErr := idgen.NewID("cp")
		if idErr != nil {
			return fmt.Errorf("generate checkpoint id: %w", idErr)
		}
		cp := checkpoint.Checkpoint{
			ID:               cpID,
			ProjectID:        sess.ProjectID,
			RoomID:           sess.RoomID,
			SourceSessionID:  sessionID,
			CreatorID:        actorID,
			Kind:             checkpoint.KindPreOperation,
			Status:           checkpoint.StatusMetadataOnly,
			ConsistencyLevel: consistency.LevelMetadataOnly,
			EnvironmentID:    sess.EnvironmentID,
			RuntimeProfileID: sess.RuntimeProfileID,
			WorldStateRef:    snapResult.SnapshotRef,
			Notes:            "Pre-operation checkpoint before session restart",
			CreatedAt:        time.Now().UTC(),
		}
		if err := store.CreateCheckpoint(ctx, cp); err != nil {
			return fmt.Errorf("save pre-op checkpoint: %w", err)
		}
		return nil
	}
}
