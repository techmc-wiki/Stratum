package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/artifactapply"
	"github.com/stratummc/stratum/internal/domain/artifactstaging"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/checkpoint"
	"github.com/stratummc/stratum/internal/domain/environment"
	"github.com/stratummc/stratum/internal/domain/operation"
	"github.com/stratummc/stratum/internal/domain/project"
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/room"
	"github.com/stratummc/stratum/internal/domain/runtimeobservation"
	"github.com/stratummc/stratum/internal/domain/session"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/repository/artifactblob"
	"github.com/stratummc/stratum/internal/repository/filesystem"
	"github.com/stratummc/stratum/internal/service/artifactapplysvc"
	"github.com/stratummc/stratum/internal/service/artifactstagingsvc"
	"github.com/stratummc/stratum/internal/service/artifactsvc"
	"github.com/stratummc/stratum/internal/service/observationsvc"
	"github.com/stratummc/stratum/internal/service/reconcilesvc"
	"github.com/stratummc/stratum/internal/service/sessionsvc"
	"github.com/stratummc/stratum/internal/util"
)

const defaultDataDirectory = ".stratum/data"
const defaultArtifactBlobRoot = ".stratum/artifacts"

func Run(args []string, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("stratum", flag.ContinueOnError)
	global.SetOutput(stderr)
	dataDirectory := global.String("data-dir", defaultDataDirectory, "metadata data directory")
	artifactBlobRoot := global.String("artifact-blob-root", defaultArtifactBlobRoot, "artifact blob storage root")
	agentURL := global.String("agent-url", "", "agent HTTP endpoint; empty uses local fake")
	agentToken := global.String("agent-token", "", "agent HTTP bearer token")
	agentTimeout := global.Duration("agent-timeout", 10*time.Second, "agent HTTP request timeout")
	if err := global.Parse(args); err != nil {
		return 2
	}
	command := global.Args()
	if len(command) < 2 {
		usage(stderr)
		return 2
	}
	resource, action := command[0], command[1]
	if resource == "environments" && action == "validate-file" {
		return validateEnvironmentFile(command[2:], stdout, stderr)
	}

	store, err := filesystem.New(*dataDirectory)
	if err != nil {
		fmt.Fprintf(stderr, "open data directory: %v\n", err)
		return 1
	}

	ctx := context.Background()
	agentClient, agentMode, err := buildAgentClient(*agentURL, *agentToken, *agentTimeout)
	if err != nil {
		fmt.Fprintf(stderr, "configure agent client: %v\n", err)
		return 2
	}
	switch resource + " " + action {
	case "projects create":
		return createProject(ctx, store, command[2:], stdout, stderr)
	case "projects list":
		return listProjects(ctx, store, stdout, stderr)
	case "rooms create":
		return createRoom(ctx, store, command[2:], stdout, stderr)
	case "rooms list":
		return listRooms(ctx, store, stdout, stderr)
	case "sessions create":
		return createSession(ctx, store, command[2:], stdout, stderr)
	case "sessions list":
		return listSessions(ctx, store, stdout, stderr)
	case "sessions inspect":
		return inspectSession(ctx, store, agentClient, command[2:], stdout, stderr)
	case "sessions observe":
		return observeSession(ctx, store, agentClient, command[2:], stdout, stderr)
	case "sessions reconcile":
		return reconcileSession(ctx, store, agentClient, agentMode, strings.TrimSpace(*agentURL) != "", command[2:], stdout, stderr)
	case "sessions logs":
		return sessionLogs(ctx, agentClient, command[2:], stdout, stderr)
	case "sessions artifacts":
		return sessionArtifacts(ctx, agentClient, strings.TrimSpace(*agentURL) != "", command[2:], stdout, stderr)
	case "sessions runtime-status":
		return sessionRuntimeStatus(ctx, agentClient, command[2:], stdout, stderr)
	case "sessions prepare", "sessions start", "sessions stop", "sessions restart",
		"sessions freeze", "sessions unfreeze", "sessions mark-crashed",
		"sessions archive", "sessions delete":
		return runSessionLifecycle(ctx, store, *artifactBlobRoot, agentClient, agentMode, strings.TrimSpace(*agentURL) != "", action, command[2:], stdout, stderr)
	case "checkpoints create":
		return createCheckpoint(ctx, store, command[2:], stdout, stderr)
	case "checkpoints list":
		return listCheckpoints(ctx, store, stdout, stderr)
	case "checkpoints get":
		return getCheckpoint(ctx, store, command[2:], stdout, stderr)
	case "artifacts list":
		return listArtifacts(ctx, store, stdout, stderr)
	case "artifacts inspect":
		return inspectArtifact(ctx, store, command[2:], stdout, stderr)
	case "artifacts create":
		return createArtifact(ctx, store, command[2:], stdout, stderr)
	case "artifacts import-file":
		return importArtifactFile(ctx, store, *artifactBlobRoot, command[2:], stdout, stderr)
	case "artifacts blobs":
		return artifactBlobs(ctx, *artifactBlobRoot, command[2:], stdout, stderr)
	case "artifacts approve", "artifacts reject":
		return reviewArtifact(ctx, store, *artifactBlobRoot, action, command[2:], stdout, stderr)
	case "artifacts staging":
		return artifactStaging(ctx, store, *artifactBlobRoot, agentClient, agentMode, strings.TrimSpace(*agentURL) != "", command[2:], stdout, stderr)
	case "artifacts apply":
		return artifactApply(ctx, store, agentClient, command[2:], stdout, stderr)
	case "environments create":
		return createEnvironment(ctx, store, command[2:], stdout, stderr)
	case "environments list":
		return listEnvironments(ctx, store, stdout, stderr)
	case "environments inspect":
		return inspectEnvironment(ctx, store, command[2:], stdout, stderr)
	case "environments materialize":
		return materializeEnvironment(ctx, store, agentClient, command[2:], stdout, stderr)
	case "operations list":
		return listOperations(ctx, store, command[2:], stdout, stderr)
	case "operations inspect":
		return inspectOperation(ctx, store, command[2:], stdout, stderr)
	case "runtime-observations list":
		return listRuntimeObservations(ctx, store, command[2:], stdout, stderr)
	case "runtime-observations inspect":
		return inspectRuntimeObservation(ctx, store, command[2:], stdout, stderr)
	case "agents list":
		return listAgents(ctx, agentClient, stdout, stderr)
	case "agents inspect":
		return inspectAgent(ctx, agentClient, command[2:], stdout, stderr)
	case "agents resources":
		return agentResources(ctx, agentClient, command[2:], stdout, stderr)
	case "agents runtime-profiles":
		return agentRuntimeProfiles(ctx, agentClient, command[2:], stdout, stderr)
	case "sessions applied-artifacts":
		return sessionsAppliedArtifacts(ctx, agentClient, command[2:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", strings.Join(command[:2], " "))
		usage(stderr)
		return 2
	}
}

func createProject(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("projects create", stderr)
	id := flags.String("id", "", "project ID")
	name := flags.String("name", "", "project name")
	description := flags.String("description", "", "project description")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *name == "" {
		fmt.Fprintln(stderr, "--id and --name are required")
		return 2
	}
	value := project.Project{ID: *id, Name: *name, Description: *description, Members: []project.Member{}, CreatedAt: time.Now().UTC()}
	if err := store.CreateProject(ctx, value); err != nil {
		return reportError(stderr, "create project", err)
	}
	fmt.Fprintf(stdout, "Created project %s (%s).\n", value.ID, value.Name)
	return 0
}

func listProjects(ctx context.Context, store *filesystem.Store, stdout, stderr io.Writer) int {
	values, err := store.ListProjects(ctx)
	if err != nil {
		return reportError(stderr, "list projects", err)
	}
	for _, value := range values {
		fmt.Fprintf(stdout, "%s\t%s\n", value.ID, value.Name)
	}
	return 0
}

func createRoom(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("rooms create", stderr)
	id := flags.String("id", "", "room ID")
	projectID := flags.String("project", "", "project ID")
	name := flags.String("name", "", "room name")
	environmentID := flags.String("environment", "", "environment ID")
	baseWorld := flags.String("base-world", "base-world:unconfigured", "immutable base-world reference")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *projectID == "" || *name == "" {
		fmt.Fprintln(stderr, "--id, --project, and --name are required")
		return 2
	}
	if _, err := store.GetProject(ctx, *projectID); err != nil {
		return reportError(stderr, "find project", err)
	}
	if err := ensureEnvironment(ctx, store, *environmentID); err != nil {
		return reportError(stderr, "prepare environment metadata", err)
	}
	value := room.Room{ID: *id, ProjectID: *projectID, Name: *name, EnvironmentID: *environmentID, BaseWorldRef: *baseWorld, CreatedAt: time.Now().UTC()}
	if err := store.CreateRoom(ctx, value); err != nil {
		return reportError(stderr, "create room", err)
	}
	fmt.Fprintf(stdout, "Created room %s in project %s.\n", value.ID, value.ProjectID)
	return 0
}

func listRooms(ctx context.Context, store *filesystem.Store, stdout, stderr io.Writer) int {
	values, err := store.ListRooms(ctx)
	if err != nil {
		return reportError(stderr, "list rooms", err)
	}
	for _, value := range values {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", value.ID, value.ProjectID, value.Name, value.EnvironmentID)
	}
	return 0
}

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
	if err := ensureEnvironment(ctx, store, environmentID); err != nil {
		return reportError(stderr, "prepare environment metadata", err)
	}
	now := time.Now().UTC()
	value := session.Session{ID: *id, ProjectID: *projectID, RoomID: *roomID, OwnerUserID: *ownerID, Type: requestedType, State: session.StateCreated, EnvironmentID: environmentID, CreatedAt: now, LastActiveAt: now}
	if err := store.CreateSession(ctx, value); err != nil {
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
	options := sessionsvc.OperationOptions{IdempotencyKey: *idempotencyKey, RequestID: *requestID, Timeout: *operationTimeout, RuntimeProfileID: *runtimeProfileID}
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

func listOperations(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("operations list", stderr)
	sessionID := flags.String("session", "", "filter by session ID")
	status := flags.String("status", "", "filter by operation status")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	values, err := store.ListOperations(ctx)
	if err != nil {
		return reportError(stderr, "list operations", err)
	}
	for _, value := range values {
		if *sessionID != "" && value.SessionID != *sessionID {
			continue
		}
		if *status != "" && string(value.Status) != *status {
			continue
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", value.ID, value.SessionID, value.Action, value.Status, value.RequestID)
	}
	return 0
}

func inspectOperation(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("operations inspect", stderr)
	id := flags.String("id", "", "operation ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	value, err := store.GetOperation(ctx, *id)
	if err != nil {
		return reportError(stderr, "inspect operation", err)
	}
	fmt.Fprintf(stdout, "id=%s request=%s actor=%s action=%s session=%s status=%s previous=%s intended=%s final=%s result=%s runtimeProfile=%s errorCode=%s error=%q\n", value.ID, value.RequestID, value.ActorID, value.Action, value.SessionID, value.Status, value.PreviousState, value.IntendedState, value.FinalState, value.Result, value.Metadata["runtimeProfileId"], value.ErrorCode, value.ErrorMessage)
	return 0
}

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
	fmt.Fprintf(stdout, "id=%s project=%s room=%s type=%s state=%s agent=%s agentStatus=%s runtimeMessage=%q endpoint=%s observed=%s process=%s runtimeProfile=%s runtimeType=%s runtimeMode=%s pid=%d running=%t crashed=%t exitCode=%s runtimeError=%q sessionRoot=%s workDir=%s logsDir=%s\n",
		value.ID, value.ProjectID, value.RoomID, value.Type, value.State, value.AssignedAgentID,
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

func agentRuntimeProfiles(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("agents runtime-profiles", stderr)
	id := flags.String("id", local.DefaultAgentID, "agent ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id != local.DefaultAgentID {
		fmt.Fprintf(stderr, "agent %q not found\n", *id)
		return 1
	}
	profiles, err := agentClient.RuntimeProfiles(ctx)
	if err != nil {
		return reportError(stderr, "list runtime profiles", err)
	}
	for _, value := range profiles {
		fmt.Fprintf(stdout, "%s\t%s\t%s\tenabled=%t\tstop=%s\t%s\n", value.ID, value.Name, value.RuntimeType, value.Enabled, value.StopStrategy, value.Notes)
	}
	return 0
}

func listAgents(ctx context.Context, agentClient agent.AgentClient, stdout, stderr io.Writer) int {
	info, err := agentClient.Info(ctx)
	if err != nil {
		return reportError(stderr, "list agents", err)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\n", info.ID, info.Status, info.RuntimeEndpoint)
	return 0
}

func inspectAgent(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("agents inspect", stderr)
	id := flags.String("id", local.DefaultAgentID, "agent ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id != local.DefaultAgentID {
		fmt.Fprintf(stderr, "agent %q not found\n", *id)
		return 1
	}
	info, err := agentClient.Info(ctx)
	if err != nil {
		return reportError(stderr, "inspect agent", err)
	}
	report, err := agentClient.ReportResources(ctx)
	if err != nil {
		return reportError(stderr, "report agent resources", err)
	}
	fmt.Fprintf(stdout, "id=%s status=%s endpoint=%s capabilities=%s cpu=%d memory=%d/%dMB disk=%d/%dMB running=%d\n",
		info.ID, info.Status, info.RuntimeEndpoint, strings.Join(info.Capabilities, ","), report.CPUCapacity,
		report.MemoryUsedMB, report.MemoryTotalMB, report.DiskUsedMB, report.DiskTotalMB, report.RunningSessions)
	return 0
}

func agentResources(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("agents resources", stderr)
	id := flags.String("id", local.DefaultAgentID, "agent ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id != local.DefaultAgentID {
		fmt.Fprintf(stderr, "agent %q not found\n", *id)
		return 1
	}
	report, err := agentClient.ReportResources(ctx)
	if err != nil {
		return reportError(stderr, "report agent resources", err)
	}
	fmt.Fprintf(stdout, "agent=%s cpu=%d memory=%d/%dMB disk=%d/%dMB running=%d reported=%s\n",
		report.AgentID, report.CPUCapacity, report.MemoryUsedMB, report.MemoryTotalMB,
		report.DiskUsedMB, report.DiskTotalMB, report.RunningSessions, report.ReportedAt.Format(time.RFC3339))
	return 0
}

func createCheckpoint(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("checkpoints create", stderr)
	id := flags.String("id", "", "checkpoint ID")
	sessionID := flags.String("session", "", "source session ID")
	note := flags.String("note", "", "semantic checkpoint note")
	notes := flags.String("notes", "", "alias for --note")
	creatorID := flags.String("creator", "cli", "creator user ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *sessionID == "" {
		fmt.Fprintln(stderr, "--id and --session are required")
		return 2
	}
	if *note == "" {
		*note = *notes
	}
	sessionValue, err := store.GetSession(ctx, *sessionID)
	if err != nil {
		return reportError(stderr, "find session", err)
	}
	value, err := checkpoint.New(checkpoint.CreateParams{
		ID: *id, ProjectID: sessionValue.ProjectID, RoomID: sessionValue.RoomID,
		SourceSessionID: sessionValue.ID, CreatorID: *creatorID, Kind: checkpoint.KindManual,
		WorldStateRef: "metadata-only://session/" + sessionValue.ID, EnvironmentID: sessionValue.EnvironmentID,
		Notes: *note,
	})
	if err != nil {
		return reportError(stderr, "build checkpoint metadata", err)
	}
	if err := store.CreateCheckpoint(ctx, value); err != nil {
		return reportError(stderr, "create checkpoint", err)
	}
	fmt.Fprintf(stdout, "Created checkpoint metadata %s for session %s. World snapshot backup is TODO.\n", value.ID, value.SourceSessionID)
	return 0
}

func listCheckpoints(ctx context.Context, store *filesystem.Store, stdout, stderr io.Writer) int {
	values, err := store.ListCheckpoints(ctx)
	if err != nil {
		return reportError(stderr, "list checkpoints", err)
	}
	for _, value := range values {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", value.ID, value.SourceSessionID, value.Kind, value.Notes)
	}
	return 0
}

func getCheckpoint(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("checkpoints get", stderr)
	id := flags.String("id", "", "checkpoint ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	value, err := store.GetCheckpoint(ctx, *id)
	if err != nil {
		return reportError(stderr, "get checkpoint", err)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", value.ID, value.SourceSessionID, value.Kind, value.Notes)
	return 0
}

func listArtifacts(ctx context.Context, store *filesystem.Store, stdout, stderr io.Writer) int {
	values, err := store.ListArtifacts(ctx)
	if err != nil {
		return reportError(stderr, "list artifacts", err)
	}
	for _, value := range values {
		reviewedAt := ""
		if value.ReviewedAt != nil {
			reviewedAt = value.ReviewedAt.Format(time.RFC3339Nano)
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\treviewedBy=%s\treviewedAt=%s\treviewReason=%s\tproject=%s\tpayload=%s\n", value.ID, value.Name, value.Type, value.Status, value.ReviewedBy, reviewedAt, value.ReviewReason, value.ProjectID, value.PayloadStatus)
	}
	return 0
}

func inspectArtifact(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts inspect", stderr)
	id := flags.String("id", "", "artifact ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	value, err := store.GetArtifact(ctx, *id)
	if err != nil {
		if stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
			err = fmt.Errorf("artifact %q not found: %w", *id, err)
		}
		return reportError(stderr, "inspect artifact", err)
	}
	fmt.Fprintf(stdout, "id=%s name=%q type=%s status=%s uploadedBy=%s createdAt=%s",
		value.ID, value.Name, value.Type, value.Status, value.UploaderID, value.CreatedAt.Format(time.RFC3339Nano))
	if value.ProjectID != "" {
		fmt.Fprintf(stdout, " project=%s", value.ProjectID)
	}
	if value.ReviewedBy != "" {
		fmt.Fprintf(stdout, " reviewedBy=%s", value.ReviewedBy)
	}
	if value.ReviewedAt != nil {
		fmt.Fprintf(stdout, " reviewedAt=%s", value.ReviewedAt.Format(time.RFC3339Nano))
	}
	if value.ReviewReason != "" {
		fmt.Fprintf(stdout, " reviewReason=%q", value.ReviewReason)
	}
	if value.PayloadStatus != "" {
		fmt.Fprintf(stdout, " payload=%s", value.PayloadStatus)
	}
	if value.PayloadAlgorithm != "" {
		fmt.Fprintf(stdout, " payloadAlgorithm=%s", value.PayloadAlgorithm)
	}
	if value.SHA256 != "" {
		fmt.Fprintf(stdout, " hash=%s size=%d", value.SHA256, value.SizeBytes)
	}
	if value.PayloadReference != "" {
		fmt.Fprintf(stdout, " payloadReference=%s", value.PayloadReference)
	}
	if value.PayloadImportedBy != "" {
		fmt.Fprintf(stdout, " payloadImportedBy=%s", value.PayloadImportedBy)
	}
	if value.PayloadImportedAt != nil {
		fmt.Fprintf(stdout, " payloadImportedAt=%s", value.PayloadImportedAt.Format(time.RFC3339Nano))
	}
	if len(value.TargetMinecraftVersions) > 0 {
		fmt.Fprintf(stdout, " targetVersions=%s", strings.Join(value.TargetMinecraftVersions, ","))
	}
	if len(value.LoaderCompatibility) > 0 {
		fmt.Fprintf(stdout, " loaders=%s", strings.Join(value.LoaderCompatibility, ","))
	}
	fmt.Fprintln(stdout)
	return 0
}

func createArtifact(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts create", stderr)
	id := flags.String("id", "", "artifact ID")
	name := flags.String("name", "", "artifact name")
	typeValue := flags.String("type", "", "artifact type")
	projectID := flags.String("project", "", "project ID")
	actor := flags.String("actor", "", "creator actor ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || strings.TrimSpace(*name) == "" || strings.TrimSpace(*typeValue) == "" || *projectID == "" || strings.TrimSpace(*actor) == "" {
		fmt.Fprintln(stderr, "--id, --name, --type, --project, and --actor are required")
		return 2
	}
	value, err := artifactsvc.New(store).CreateMetadata(ctx, *id, *name, artifact.Type(*typeValue), *projectID, *actor)
	if err != nil {
		return reportError(stderr, "create artifact", err)
	}
	fmt.Fprintf(stdout, "Artifact %s name=%q type=%s status=%s project=%s payload=%s. Metadata only; no payload was uploaded, hashed, copied, mounted, installed, or executed.\n",
		value.ID, value.Name, value.Type, value.Status, value.ProjectID, value.PayloadStatus)
	return 0
}

func importArtifactFile(ctx context.Context, store *filesystem.Store, blobRoot string, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts import-file", stderr)
	id := flags.String("id", "", "artifact ID")
	path := flags.String("file", "", "local artifact payload file")
	actor := flags.String("actor", "", "importing actor ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || strings.TrimSpace(*path) == "" || strings.TrimSpace(*actor) == "" {
		fmt.Fprintln(stderr, "--id, --file, and --actor are required")
		return 2
	}
	blobs, err := artifactblob.New(blobRoot)
	if err != nil {
		return reportError(stderr, "open artifact blob store", err)
	}
	value, err := artifactsvc.NewWithBlobStore(store, blobs).ImportFile(ctx, *id, *path, *actor)
	if err != nil {
		return reportError(stderr, "import artifact payload", err)
	}
	fmt.Fprintf(stdout, "Artifact %s payloadAlgorithm=%s payloadHash=%s payloadSize=%d payloadStatus=%s. The artifact remains %s and was not approved, mounted, installed, or executed.\n",
		value.ID, value.PayloadAlgorithm, value.SHA256, value.SizeBytes, value.PayloadStatus, value.Status)
	return 0
}

func artifactBlobs(ctx context.Context, blobRoot string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "verify" {
		fmt.Fprintln(stderr, "usage: stratum artifacts blobs verify --sha256 HASH")
		return 2
	}
	flags := newFlagSet("artifacts blobs verify", stderr)
	hash := flags.String("sha256", "", "SHA-256 content hash")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *hash == "" {
		fmt.Fprintln(stderr, "--sha256 is required")
		return 2
	}
	blobs, err := artifactblob.Open(blobRoot)
	if err != nil {
		return reportError(stderr, "open artifact blob store", err)
	}
	metadata, err := blobs.Verify(ctx, *hash)
	if err != nil {
		if stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
			fmt.Fprintf(stderr, "algorithm=sha256 hash=%s status=missing\n", *hash)
		} else if stratumerrors.IsKind(err, stratumerrors.KindValidation) {
			return reportError(stderr, "verify artifact blob", err)
		} else if strings.Contains(err.Error(), "hash mismatch") {
			fmt.Fprintf(stderr, "algorithm=sha256 hash=%s status=corrupted\n", *hash)
		}
		return reportError(stderr, "verify artifact blob", err)
	}
	fmt.Fprintf(stdout, "algorithm=%s hash=%s size=%d status=valid reference=%s\n", metadata.Algorithm, metadata.Hash, metadata.Size, metadata.Reference)
	return 0
}

func reviewArtifact(ctx context.Context, store *filesystem.Store, blobRoot, action string, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts "+action, stderr)
	id := flags.String("id", "", "artifact ID")
	actor := flags.String("actor", "", "reviewer actor ID")
	reason := flags.String("reason", "", "review reason")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || strings.TrimSpace(*actor) == "" || strings.TrimSpace(*reason) == "" {
		fmt.Fprintln(stderr, "--id, --actor, and --reason are required")
		return 2
	}
	service := artifactsvc.New(store)
	var value artifact.Artifact
	var err error
	if action == "approve" {
		blobs, openErr := artifactblob.Open(blobRoot)
		if openErr != nil {
			return reportError(stderr, "open artifact blob store", openErr)
		}
		service = artifactsvc.NewWithPayloadVerifier(store, blobs)
		value, err = service.ApproveArtifact(ctx, *id, *actor, *reason)
	} else {
		value, err = service.RejectArtifact(ctx, *id, *actor, *reason)
	}
	if err != nil {
		return reportError(stderr, "artifact "+action, err)
	}
	reviewedAt := ""
	if value.ReviewedAt != nil {
		reviewedAt = value.ReviewedAt.Format(time.RFC3339Nano)
	}
	fmt.Fprintf(stdout, "Artifact %s status=%s reviewedBy=%s reviewedAt=%s reason=%q. No payload was copied, mounted, installed, or executed.\n", value.ID, value.Status, value.ReviewedBy, reviewedAt, value.ReviewReason)
	return 0
}

func artifactStaging(ctx context.Context, store *filesystem.Store, blobRoot string, agentClient agent.AgentClient, agentMode string, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "plan" && args[0] != "list" && args[0] != "inspect" && args[0] != "materialize" && args[0] != "readiness") {
		fmt.Fprintln(stderr, "usage: stratum artifacts staging <plan|list|inspect|materialize|readiness> [flags]")
		return 2
	}
	switch args[0] {
	case "plan":
		return planArtifactStaging(ctx, store, blobRoot, args[1:], stdout, stderr)
	case "list":
		return listArtifactStaging(ctx, store, args[1:], stdout, stderr)
	case "inspect":
		return inspectArtifactStaging(ctx, store, args[1:], stdout, stderr)
	case "materialize":
		return materializeArtifactStaging(ctx, store, blobRoot, agentClient, agentMode, hasAgentURL, args[1:], stdout, stderr)
	case "readiness":
		return artifactStagingReadiness(ctx, store, blobRoot, agentClient, hasAgentURL, args[1:], stdout, stderr)
	default:
		return 2
	}
}

func artifactStagingReadiness(ctx context.Context, store *filesystem.Store, blobRoot string, agentClient agent.AgentClient, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts staging readiness", stderr)
	sessionID := flags.String("session", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" {
		fmt.Fprintln(stderr, "--session is required")
		return 2
	}
	if !hasAgentURL {
		fmt.Fprintln(stderr, "--agent-url is required for full materialization readiness")
		return 2
	}
	blobs, err := artifactblob.Open(blobRoot)
	if err != nil {
		return reportError(stderr, "open artifact blob store", err)
	}
	result, err := artifactstagingsvc.NewReadinessService(store, blobs, agentClient).Check(ctx, *sessionID)
	if err != nil {
		return reportError(stderr, "check artifact materialization readiness", err)
	}
	fmt.Fprintf(stdout, "session=%s status=%s planned=%d materialized=%d valid=%d missing=%d corrupted=%d unknown=%d issues=%d checkedAt=%s\n", result.SessionID, result.Status, result.PlannedCount, result.MaterializedCount, result.ValidMaterializedCount, result.MissingMaterializedCount, result.CorruptedMaterializedCount, result.UnknownMaterializedCount, len(result.Issues), result.CheckedAt.Format(time.RFC3339Nano))
	for _, issue := range result.Issues {
		fmt.Fprintf(stdout, "issue=%s severity=%s plan=%s artifact=%s message=%q\n", issue.Code, issue.Severity, issue.StagingPlanID, issue.ArtifactID, issue.Message)
	}
	for _, entry := range result.Entries {
		fmt.Fprintf(stdout, "plan=%s artifact=%s artifactStatus=%s materialized=%t verification=%s recommendedAction=%q\n", entry.StagingPlanID, entry.ArtifactID, entry.ArtifactStatus, entry.Materialized, entry.VerificationStatus, entry.RecommendedAction)
	}
	if result.Status != "ready" {
		return 1
	}
	return 0
}

func materializeArtifactStaging(ctx context.Context, store *filesystem.Store, blobRoot string, agentClient agent.AgentClient, agentMode string, hasAgentURL bool, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts staging materialize", stderr)
	planID := flags.String("plan", "", "planned artifact staging plan ID")
	actor := flags.String("actor", "", "actor user ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *planID == "" || strings.TrimSpace(*actor) == "" {
		fmt.Fprintln(stderr, "--plan and --actor are required")
		return 2
	}
	if !hasAgentURL {
		fmt.Fprintln(stderr, "--agent-url is required for artifact materialization")
		return 2
	}
	plan, err := store.GetArtifactStagingPlan(ctx, *planID)
	if err != nil {
		return reportError(stderr, "load artifact staging plan", err)
	}
	if plan.Status != artifactstaging.StatusPlanned {
		return reportError(stderr, "materialize artifact staging", fmt.Errorf("staging plan %q is %q, not planned", plan.ID, plan.Status))
	}
	value, err := store.GetArtifact(ctx, plan.ArtifactID)
	if err != nil {
		return reportError(stderr, "load staged artifact", err)
	}
	if value.Status != artifact.StatusApproved {
		return reportError(stderr, "materialize artifact staging", fmt.Errorf("artifact %q is %q, not approved", value.ID, value.Status))
	}
	if value.SHA256 != plan.ArtifactHash || value.PayloadAlgorithm != artifactblob.Algorithm || value.PayloadStatus != artifact.PayloadAvailable {
		return reportError(stderr, "materialize artifact staging", errors.New("artifact payload metadata does not match planned staging payload"))
	}
	blobs, err := artifactblob.Open(blobRoot)
	if err != nil {
		return reportError(stderr, "open artifact blob store", err)
	}
	metadata, err := blobs.Verify(ctx, value.SHA256)
	if err != nil {
		return reportError(stderr, "verify materialization payload", err)
	}
	if metadata.Algorithm != value.PayloadAlgorithm || metadata.Hash != value.SHA256 || metadata.Reference != value.PayloadReference || metadata.Size != value.SizeBytes {
		return reportError(stderr, "verify materialization payload", errors.New("artifact metadata does not match verified blob"))
	}
	if metadata.Size > agent.MaxArtifactPayloadBytes {
		return reportError(stderr, "materialize artifact staging", fmt.Errorf("artifact payload exceeds %d byte transfer limit", agent.MaxArtifactPayloadBytes))
	}
	path, err := blobs.Path(metadata.Hash)
	if err != nil {
		return reportError(stderr, "resolve materialization payload", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return reportError(stderr, "read materialization payload", err)
	}
	result, err := agentClient.MaterializeArtifact(ctx, agent.ArtifactMaterializationRequest{SessionID: plan.SessionID, ArtifactID: value.ID, StagingPlanID: plan.ID, ArtifactName: value.Name, ArtifactType: string(value.Type), TargetName: plan.TargetStagingName, PayloadAlgorithm: value.PayloadAlgorithm, PayloadHash: value.SHA256, PayloadSize: value.SizeBytes, ActorID: *actor, Payload: payload})
	if err != nil {
		return reportError(stderr, "agent materialize artifact", err)
	}
	auditID, err := util.NewID("audit")
	if err != nil {
		return reportError(stderr, "create materialization audit", err)
	}
	event, err := audit.NewEvent(auditID, *actor, "artifact.materialized", "artifact-staging-plan", plan.ID, result.MaterializedAt)
	if err != nil {
		return reportError(stderr, "create materialization audit", err)
	}
	event.ProjectID = plan.ProjectID
	event.Metadata = map[string]string{"sessionId": plan.SessionID, "artifactId": value.ID, "stagingPlanId": plan.ID, "actor": *actor, "payloadHash": value.SHA256, "payloadSize": fmt.Sprintf("%d", value.SizeBytes), "targetName": plan.TargetStagingName, "runtimeRelativePath": result.RuntimeRelativePath, "agentMode": agentMode, "agentId": result.AgentID, "idempotent": fmt.Sprintf("%t", result.Idempotent)}
	if err := store.AppendAuditEvent(ctx, event); err != nil {
		return reportError(stderr, "write materialization audit", err)
	}
	fmt.Fprintf(stdout, "Artifact materialized status=%s session=%s artifact=%s plan=%s target=%s runtimePath=%s hash=%s size=%d agent=%s idempotent=%t. The payload was not installed, mounted, loaded, or executed.\n", result.Status, result.SessionID, result.ArtifactID, result.StagingPlanID, result.TargetName, result.RuntimeRelativePath, result.PayloadHash, result.PayloadSize, result.AgentID, result.Idempotent)
	return 0
}

func planArtifactStaging(ctx context.Context, store *filesystem.Store, blobRoot string, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts staging plan", stderr)
	sessionID := flags.String("session", "", "session ID")
	artifactID := flags.String("artifact", "", "artifact ID")
	actor := flags.String("actor", "", "actor user ID")
	name := flags.String("name", "", "safe staging name")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" || *artifactID == "" || strings.TrimSpace(*actor) == "" || *name == "" {
		fmt.Fprintln(stderr, "--session, --artifact, --actor, and --name are required")
		return 2
	}
	blobs, err := artifactblob.Open(blobRoot)
	if err != nil {
		return reportError(stderr, "open artifact blob store", err)
	}
	plan, err := artifactstagingsvc.NewWithPayloadVerifier(store, blobs).CreatePlan(ctx, artifactstagingsvc.CreateParams{SessionID: *sessionID, ArtifactID: *artifactID, ActorID: *actor, Name: *name})
	if err != nil {
		return reportError(stderr, "plan artifact staging", err)
	}
	fmt.Fprintf(stdout, "Artifact staging plan %s status=%s session=%s artifact=%s kind=%s target=%s rejection=%q. No payload was copied or mounted.\n", plan.ID, plan.Status, plan.SessionID, plan.ArtifactID, plan.StagingKind, plan.TargetStagingName, plan.RejectionReason)
	return 0
}

func listArtifactStaging(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts staging list", stderr)
	sessionID := flags.String("session", "", "filter by session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	service := artifactstagingsvc.New(store)
	var values []artifactstaging.Plan
	var err error
	if *sessionID != "" {
		values, err = service.ListBySession(ctx, *sessionID)
	} else {
		values, err = service.List(ctx)
	}
	if err != nil {
		return reportError(stderr, "list artifact staging plans", err)
	}
	for _, value := range values {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", value.ID, value.SessionID, value.ArtifactID, value.StagingKind, value.TargetStagingName, value.Status, value.RejectionReason)
	}
	return 0
}

func inspectArtifactStaging(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts staging inspect", stderr)
	id := flags.String("id", "", "artifact staging plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	value, err := artifactstagingsvc.New(store).Get(ctx, *id)
	if err != nil {
		return reportError(stderr, "inspect artifact staging plan", err)
	}
	fmt.Fprintf(stdout, "id=%s session=%s project=%s room=%s artifact=%s artifactName=%q artifactType=%s artifactStatus=%s hash=%s kind=%s target=%s actor=%s status=%s rejection=%q createdAt=%s\n", value.ID, value.SessionID, value.ProjectID, value.RoomID, value.ArtifactID, value.ArtifactName, value.ArtifactType, value.ArtifactStatus, value.ArtifactHash, value.StagingKind, value.TargetStagingName, value.ActorID, value.Status, value.RejectionReason, value.CreatedAt.Format(time.RFC3339Nano))
	return 0
}

func ensureEnvironment(ctx context.Context, store *filesystem.Store, id string) error {
	if id == "" {
		return nil
	}
	if _, err := store.GetEnvironment(ctx, id); err == nil {
		return nil
	} else if !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		return err
	}
	return fmt.Errorf("environment %q is not registered", id)
}

func ensureResourcePolicy(ctx context.Context, store *filesystem.Store) (resourcepolicy.Policy, error) {
	value, err := store.GetResourcePolicy(ctx, "default")
	if err == nil {
		return value, nil
	}
	if !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		return resourcepolicy.Policy{}, err
	}
	value = resourcepolicy.MVPDefault()
	if err := store.CreateResourcePolicy(ctx, value); err != nil {
		return resourcepolicy.Policy{}, err
	}
	return value, nil
}

func artifactApply(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: stratum artifacts apply <plan|list|inspect|dry-run|execute> [flags]")
		return 2
	}
	switch args[0] {
	case "plan":
		return artifactApplyPlan(ctx, store, agentClient, args[1:], stdout, stderr)
	case "list":
		return artifactApplyList(ctx, store, args[1:], stdout, stderr)
	case "inspect":
		return artifactApplyInspect(ctx, store, args[1:], stdout, stderr)
	case "dry-run":
		return artifactApplyDryRun(ctx, store, agentClient, args[1:], stdout, stderr)
	case "execute":
		return artifactApplyExecute(ctx, store, agentClient, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown artifacts apply command %q\n", args[0])
		return 2
	}
}

func artifactApplyPlan(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts apply plan", stderr)
	sessionID := flags.String("session", "", "session ID")
	stagingPlanID := flags.String("staging-plan", "", "staging plan ID")
	actor := flags.String("actor", "", "actor ID")
	target := flags.String("target", "", "target relative path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" || *stagingPlanID == "" || *actor == "" || *target == "" {
		fmt.Fprintln(stderr, "--session, --staging-plan, --actor, and --target are required")
		return 2
	}
	service := artifactapplysvc.New(store, agentClient)
	plan, err := service.CreatePlan(ctx, artifactapplysvc.CreateParams{SessionID: *sessionID, StagingPlanID: *stagingPlanID, ActorID: *actor, TargetPath: *target})
	if err != nil {
		return reportError(stderr, "create artifact apply plan", err)
	}
	fmt.Fprintf(stdout, "Artifact apply plan %s status=%s kind=%s root=%s target=%s rejection=%q. No file was copied, mounted, installed, or executed.\n", plan.ID, plan.Status, plan.ApplyKind, plan.TargetRoot, plan.TargetRelativePath, plan.RejectionReason)
	return 0
}

func artifactApplyList(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts apply list", stderr)
	sessionID := flags.String("session", "", "filter by session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	service := artifactapplysvc.New(store, nil)
	var values []artifactapply.Plan
	var err error
	if *sessionID != "" {
		values, err = service.ListBySession(ctx, *sessionID)
	} else {
		values, err = service.List(ctx)
	}
	if err != nil {
		return reportError(stderr, "list artifact apply plans", err)
	}
	for _, value := range values {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", value.ID, value.SessionID, value.ArtifactID, value.Status, value.ApplyKind, value.TargetRelativePath)
	}
	return 0
}

func artifactApplyInspect(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts apply inspect", stderr)
	id := flags.String("id", "", "apply plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	service := artifactapplysvc.New(store, nil)
	plan, err := service.Get(ctx, *id)
	if err != nil {
		return reportError(stderr, "get artifact apply plan", err)
	}
	fmt.Fprintf(stdout, "Apply Plan %s\n", plan.ID)
	fmt.Fprintf(stdout, "Session: %s\n", plan.SessionID)
	fmt.Fprintf(stdout, "Artifact: %s\n", plan.ArtifactID)
	fmt.Fprintf(stdout, "Status: %s\n", plan.Status)
	fmt.Fprintf(stdout, "ApplyKind: %s\n", plan.ApplyKind)
	fmt.Fprintf(stdout, "TargetRoot: %s\n", plan.TargetRoot)
	fmt.Fprintf(stdout, "TargetPath: %s\n", plan.TargetRelativePath)
	fmt.Fprintf(stdout, "Hash: %s\n", plan.MaterializedArtifactHash)
	fmt.Fprintf(stdout, "Name: %s\n", plan.MaterializedArtifactName)
	if plan.RejectionReason != "" {
		fmt.Fprintf(stdout, "Rejection: %s\n", plan.RejectionReason)
	}
	return 0
}

func artifactApplyDryRun(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts apply dry-run", stderr)
	planID := flags.String("plan", "", "apply plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *planID == "" {
		fmt.Fprintln(stderr, "--plan is required")
		return 2
	}
	service := artifactapplysvc.New(store, nil)
	plan, err := service.Get(ctx, *planID)
	if err != nil {
		return reportError(stderr, "get artifact apply plan", err)
	}
	if plan.Status != artifactapply.StatusPlanned {
		fmt.Fprintf(stderr, "apply plan status is %s, not planned\n", plan.Status)
		return 1
	}
	req := agent.ArtifactApplyDryRunRequest{ApplyPlanID: plan.ID, SessionID: plan.SessionID, StagingPlanID: plan.SourceStagingPlanID, ArtifactID: plan.ArtifactID, TargetRoot: string(plan.TargetRoot), TargetRelativePath: plan.TargetRelativePath, ExpectedHash: plan.MaterializedArtifactHash}
	result, err := agentClient.DryRunArtifactApply(ctx, req)
	if err != nil {
		return reportError(stderr, "dry-run artifact apply", err)
	}
	fmt.Fprintf(stdout, "Dry-run result for apply plan %s:\n", result.ApplyPlanID)
	fmt.Fprintf(stdout, "Status: %s\n", result.Status)
	fmt.Fprintf(stdout, "Action: %s\n", result.Action)
	fmt.Fprintf(stdout, "Source: %s\n", result.SourceRuntimeRelativePath)
	fmt.Fprintf(stdout, "Target: %s\n", result.PlannedTargetRuntimeRelativePath)
	if len(result.Issues) > 0 {
		fmt.Fprintf(stdout, "Issues:\n")
		for _, issue := range result.Issues {
			fmt.Fprintf(stdout, "  - %s\n", issue)
		}
	}
	fmt.Fprintln(stdout, "No files were copied, mounted, installed, or executed.")
	return 0
}

func artifactApplyExecute(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("artifacts apply execute", stderr)
	planID := flags.String("plan", "", "apply plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *planID == "" {
		fmt.Fprintln(stderr, "--plan is required")
		return 2
	}
	service := artifactapplysvc.New(store, nil)
	plan, err := service.Get(ctx, *planID)
	if err != nil {
		return reportError(stderr, "get artifact apply plan", err)
	}
	if plan.Status != artifactapply.StatusPlanned {
		fmt.Fprintf(stderr, "apply plan status is %s, not planned\n", plan.Status)
		return 1
	}
	req := agent.ArtifactApplyExecuteRequest{ApplyPlanID: plan.ID, SessionID: plan.SessionID, StagingPlanID: plan.SourceStagingPlanID, ArtifactID: plan.ArtifactID, TargetRoot: string(plan.TargetRoot), TargetRelativePath: plan.TargetRelativePath, ExpectedHash: plan.MaterializedArtifactHash}
	result, err := agentClient.ExecuteArtifactApply(ctx, req)
	if err != nil {
		return reportError(stderr, "execute artifact apply", err)
	}
	fmt.Fprintf(stdout, "Apply execution result for plan %s:\n", result.ApplyPlanID)
	fmt.Fprintf(stdout, "Status: %s\n", result.Status)
	fmt.Fprintf(stdout, "Action: %s\n", result.Action)
	fmt.Fprintf(stdout, "Source: %s\n", result.SourcePath)
	fmt.Fprintf(stdout, "Target: %s\n", result.TargetPath)
	fmt.Fprintf(stdout, "Copied: %d bytes\n", result.CopiedBytes)
	fmt.Fprintf(stdout, "Hash: %s\n", result.VerifiedTargetHash)
	if len(result.Issues) > 0 {
		fmt.Fprintf(stdout, "Issues:\n")
		for _, issue := range result.Issues {
			fmt.Fprintf(stdout, "  - %s\n", issue)
		}
	}
	return 0
}

func buildAgentClient(rawURL, token string, timeout time.Duration) (agent.AgentClient, string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return local.NewFake(), "local", nil
	}
	client, err := httptransport.NewClient(rawURL, token, timeout)
	if err != nil {
		return nil, "", err
	}
	return client, "http", nil
}

func parseSessionType(value string) (session.Type, bool) {
	candidate := session.Type(value)
	switch candidate {
	case session.TypeShared, session.TypeFork, session.TypePrivate, session.TypeReview, session.TypeArchived:
		return candidate, true
	default:
		return "", false
	}
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

func reportError(stderr io.Writer, action string, err error) int {
	fmt.Fprintf(stderr, "%s: %v\n", action, err)
	return 1
}

func sessionsAppliedArtifacts(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "inspect" {
		return sessionsAppliedArtifactsInspect(ctx, agentClient, args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "verify" {
		return sessionsAppliedArtifactsVerify(ctx, agentClient, args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "verify-all" {
		return sessionsAppliedArtifactsVerifyAll(ctx, agentClient, args[1:], stdout, stderr)
	}
	flags := newFlagSet("sessions applied-artifacts", stderr)
	sessionID := flags.String("id", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" {
		fmt.Fprintln(stderr, "session applied-artifacts: --id is required")
		return 2
	}
	result, err := agentClient.ListAppliedArtifacts(ctx, *sessionID)
	if err != nil {
		return reportError(stderr, "list applied artifacts", err)
	}
	if len(result.Records) == 0 {
		fmt.Fprintln(stdout, "No applied artifacts.")
		return 0
	}
	fmt.Fprintf(stdout, "Applied Artifacts (session %s):\n", result.SessionID)
	for _, r := range result.Records {
		fmt.Fprintf(stdout, "  %s: %s -> %s (status=%s, size=%d, action=%s, applied=%s)\n", r.ApplyPlanID, r.SourceRuntimeRelativePath, r.TargetRuntimeRelativePath, r.Status, r.PayloadSize, r.Action, r.AppliedAt.Format(time.RFC3339))
	}
	return 0
}

func sessionsAppliedArtifactsInspect(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions applied-artifacts inspect", stderr)
	sessionID := flags.String("id", "", "session ID")
	applyPlanID := flags.String("plan", "", "apply plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" || *applyPlanID == "" {
		fmt.Fprintln(stderr, "sessions applied-artifacts inspect: --id and --plan are required")
		return 2
	}
	record, err := agentClient.InspectAppliedArtifact(ctx, *sessionID, *applyPlanID)
	if err != nil {
		return reportError(stderr, "inspect applied artifact", err)
	}
	fmt.Fprintf(stdout, "Apply Plan ID:              %s\n", record.ApplyPlanID)
	fmt.Fprintf(stdout, "Session ID:                 %s\n", record.SessionID)
	fmt.Fprintf(stdout, "Artifact ID:                %s\n", record.ArtifactID)
	fmt.Fprintf(stdout, "Staging Plan ID:            %s\n", record.StagingPlanID)
	fmt.Fprintf(stdout, "Source Runtime Path:        %s\n", record.SourceRuntimeRelativePath)
	fmt.Fprintf(stdout, "Target Runtime Path:        %s\n", record.TargetRuntimeRelativePath)
	fmt.Fprintf(stdout, "Target Root:                %s\n", record.TargetRoot)
	fmt.Fprintf(stdout, "Target Relative Path:       %s\n", record.TargetRelativePath)
	fmt.Fprintf(stdout, "Payload Algorithm:          %s\n", record.PayloadAlgorithm)
	fmt.Fprintf(stdout, "Payload Hash:               %s\n", record.PayloadHash)
	fmt.Fprintf(stdout, "Payload Size:               %d\n", record.PayloadSize)
	fmt.Fprintf(stdout, "Action:                     %s\n", record.Action)
	fmt.Fprintf(stdout, "Status:                     %s\n", record.Status)
	if record.ActorID != "" {
		fmt.Fprintf(stdout, "Actor ID:                   %s\n", record.ActorID)
	}
	fmt.Fprintf(stdout, "Applied At:                 %s\n", record.AppliedAt.Format(time.RFC3339))
	return 0
}

func sessionsAppliedArtifactsVerify(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions applied-artifacts verify", stderr)
	sessionID := flags.String("id", "", "session ID")
	applyPlanID := flags.String("plan", "", "apply plan ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" || *applyPlanID == "" {
		fmt.Fprintln(stderr, "sessions applied-artifacts verify: --id and --plan are required")
		return 2
	}
	result, err := agentClient.VerifyAppliedArtifact(ctx, *sessionID, *applyPlanID)
	if err != nil {
		return reportError(stderr, "verify applied artifact", err)
	}
	fmt.Fprintf(stdout, "Session ID:                 %s\n", result.SessionID)
	fmt.Fprintf(stdout, "Apply Plan ID:              %s\n", result.ApplyPlanID)
	fmt.Fprintf(stdout, "Artifact ID:                %s\n", result.ArtifactID)
	fmt.Fprintf(stdout, "Target Runtime Path:        %s\n", result.TargetRuntimeRelativePath)
	fmt.Fprintf(stdout, "Expected Hash:              %s\n", result.ExpectedHash)
	if result.ActualHash != "" {
		fmt.Fprintf(stdout, "Actual Hash:                %s\n", result.ActualHash)
	}
	fmt.Fprintf(stdout, "Payload Size:               %d\n", result.PayloadSize)
	fmt.Fprintf(stdout, "Actual Size:                %d\n", result.ActualSize)
	fmt.Fprintf(stdout, "Status:                     %s\n", result.Status)
	fmt.Fprintf(stdout, "Verified At:                %s\n", result.VerifiedAt.Format(time.RFC3339))
	if result.ErrorMessage != "" {
		fmt.Fprintf(stdout, "Error:                      %s\n", result.ErrorMessage)
	}
	if result.Status != "valid" {
		return 1
	}
	return 0
}

func sessionsAppliedArtifactsVerifyAll(ctx context.Context, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions applied-artifacts verify-all", stderr)
	sessionID := flags.String("id", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *sessionID == "" {
		fmt.Fprintln(stderr, "sessions applied-artifacts verify-all: --id is required")
		return 2
	}
	result, err := agentClient.VerifyAllAppliedArtifacts(ctx, *sessionID)
	if err != nil {
		return reportError(stderr, "verify all applied artifacts", err)
	}
	fmt.Fprintf(stdout, "Session ID:      %s\n", result.SessionID)
	fmt.Fprintf(stdout, "Verified At:     %s\n", result.VerifiedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "Total:           %d\n", result.Total)
	fmt.Fprintf(stdout, "Valid:           %d\n", result.ValidCount)
	fmt.Fprintf(stdout, "Missing:         %d\n", result.MissingCount)
	fmt.Fprintf(stdout, "Corrupted:       %d\n", result.CorruptedCount)
	fmt.Fprintf(stdout, "Error:           %d\n", result.ErrorCount)
	fmt.Fprintln(stdout)
	for _, entry := range result.Entries {
		fmt.Fprintf(stdout, "Apply Plan ID:   %s\n", entry.ApplyPlanID)
		fmt.Fprintf(stdout, "  Artifact ID:   %s\n", entry.ArtifactID)
		fmt.Fprintf(stdout, "  Target Path:   %s\n", entry.TargetRuntimeRelativePath)
		fmt.Fprintf(stdout, "  Status:        %s\n", entry.Status)
		if entry.ErrorMessage != "" {
			fmt.Fprintf(stdout, "  Error:         %s\n", entry.ErrorMessage)
		}
		fmt.Fprintln(stdout)
	}
	if result.ValidCount != result.Total {
		return 1
	}
	return 0
}

func createEnvironment(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("environments create", stderr)
	id := flags.String("id", "", "")
	name := flags.String("name", "", "")
	minecraftVersion := flags.String("minecraft-version", "", "")
	loaderType := flags.String("loader", string(environment.LoaderNone), "")
	serverCore := flags.String("server-core", string(environment.ServerVanilla), "")
	runtimeProfile := flags.String("runtime-profile", "", "")
	runtimeProfileRequired := flags.Bool("runtime-profile-required", false, "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	env := environment.Environment{
		ID:                     *id,
		Name:                   *name,
		MinecraftVersion:       *minecraftVersion,
		LoaderType:             environment.LoaderType(*loaderType),
		ServerCore:             environment.ServerCore(*serverCore),
		RuntimeProfileID:       *runtimeProfile,
		RuntimeProfileRequired: *runtimeProfileRequired,
		CreatedAt:              time.Now().UTC(),
		UpdatedAt:              time.Now().UTC(),
	}
	if err := env.Validate(); err != nil {
		fmt.Fprintf(stderr, "validation error: %v\n", err)
		return 1
	}
	if err := store.CreateEnvironment(ctx, env); err != nil {
		fmt.Fprintf(stderr, "create environment error: %v\n", err)
		return 1
	}
	eventID, _ := util.NewID("audit")
	event, _ := audit.NewEvent(eventID, "cli", "environment.created", "environment", env.ID, time.Now().UTC())
	event.Metadata = map[string]string{"environmentId": env.ID, "name": env.Name, "minecraftVersion": env.MinecraftVersion, "loaderType": string(env.LoaderType), "serverCore": string(env.ServerCore)}
	_ = store.AppendAuditEvent(ctx, event)
	fmt.Fprintf(stdout, "Environment created: %s\n", env.ID)
	return 0
}

func validateEnvironmentFile(args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("environments validate-file", stderr)
	path := flags.String("file", "", "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*path) == "" {
		fmt.Fprintln(stderr, "--file is required")
		return 2
	}

	data, err := os.ReadFile(*path)
	if err != nil {
		fmt.Fprintf(stderr, "read environment file %q: %v\n", *path, err)
		return 1
	}

	var value environment.Environment
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		fmt.Fprintf(stderr, "decode environment file %q: %v\n", *path, err)
		return 1
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values are not allowed")
		}
		fmt.Fprintf(stderr, "decode environment file %q: %v\n", *path, err)
		return 1
	}
	if err := value.Validate(); err != nil {
		fmt.Fprintf(stderr, "validate environment file %q: %v\n", *path, err)
		return 1
	}

	fmt.Fprintln(stdout, "Environment file is valid.")
	fmt.Fprintf(stdout, "id: %s\n", value.ID)
	fmt.Fprintf(stdout, "name: %s\n", value.Name)
	fmt.Fprintf(stdout, "minecraft_version: %s\n", value.MinecraftVersion)
	fmt.Fprintf(stdout, "java_version: %s\n", value.JavaVersion)
	fmt.Fprintf(stdout, "loader_type: %s\n", value.LoaderType)
	fmt.Fprintf(stdout, "server_core: %s\n", value.ServerCore)
	if value.RuntimeProfileID != "" {
		fmt.Fprintf(stdout, "runtime_profile_id: %s\n", value.RuntimeProfileID)
	}
	fmt.Fprintf(stdout, "runtime_profile_required: %t\n", value.RuntimeProfileRequired)
	return 0
}

func listEnvironments(ctx context.Context, store *filesystem.Store, stdout, _ io.Writer) int {
	environments, err := store.ListEnvironments(ctx)
	if err != nil {
		fmt.Fprintf(stdout, "list error: %v\n", err)
		return 1
	}
	for _, env := range environments {
		runtimeProfile := env.RuntimeProfileID
		if runtimeProfile == "" {
			runtimeProfile = "-"
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", env.ID, env.Name, env.MinecraftVersion, env.LoaderType, env.ServerCore, runtimeProfile)
	}
	return 0
}

func inspectEnvironment(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("environments inspect", stderr)
	id := flags.String("id", "", "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	env, err := store.GetEnvironment(ctx, *id)
	if err != nil {
		fmt.Fprintf(stderr, "get environment error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "ID:                  %s\n", env.ID)
	fmt.Fprintf(stdout, "Name:                %s\n", env.Name)
	fmt.Fprintf(stdout, "Minecraft Version:   %s\n", env.MinecraftVersion)
	fmt.Fprintf(stdout, "Java Version:        %s\n", env.JavaVersion)
	fmt.Fprintf(stdout, "Loader Type:         %s\n", env.LoaderType)
	fmt.Fprintf(stdout, "Server Core:         %s\n", env.ServerCore)
	fmt.Fprintf(stdout, "MCDR Required:       %t\n", env.MCDRRequired)
	fmt.Fprintf(stdout, "Carpet Required:     %t\n", env.CarpetRequired)
	fmt.Fprintf(stdout, "Runtime Profile ID:  %s\n", env.RuntimeProfileID)
	if env.RuntimeProfileRequired {
		fmt.Fprintf(stdout, "Runtime Profile:     required\n")
	}
	fmt.Fprintf(stdout, "Created At:          %s\n", env.CreatedAt.Format(time.RFC3339))
	return 0
}

func materializeEnvironment(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("environments materialize", stderr)
	sessionID := flags.String("session", "", "")
	actor := flags.String("actor", "", "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	sess, err := store.GetSession(ctx, *sessionID)
	if err != nil {
		fmt.Fprintf(stderr, "get session error: %v\n", err)
		return 1
	}
	env, err := store.GetEnvironment(ctx, sess.EnvironmentID)
	if err != nil {
		fmt.Fprintf(stderr, "get environment error: %v\n", err)
		return 1
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:              sess.ID,
		EnvironmentID:          env.ID,
		EnvironmentName:        env.Name,
		MinecraftVersion:       env.MinecraftVersion,
		JavaVersion:            env.JavaVersion,
		LoaderType:             string(env.LoaderType),
		LoaderVersion:          env.LoaderVersion,
		ServerCore:             string(env.ServerCore),
		MCDRRequired:           env.MCDRRequired,
		CarpetRequired:         env.CarpetRequired,
		RuntimeProfileID:       env.RuntimeProfileID,
		RuntimeProfileRequired: env.RuntimeProfileRequired,
		ActorID:                *actor,
	}
	result, err := agentClient.MaterializeEnvironment(ctx, request)
	if err != nil {
		fmt.Fprintf(stderr, "materialize environment error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Environment materialized for session %s\n", result.SessionID)
	fmt.Fprintf(stdout, "  Environment:    %s (%s)\n", result.EnvironmentName, result.EnvironmentID)
	fmt.Fprintf(stdout, "  Minecraft:      %s\n", result.MinecraftVersion)
	fmt.Fprintf(stdout, "  Loader:         %s\n", result.LoaderType)
	fmt.Fprintf(stdout, "  Server Core:    %s\n", result.ServerCore)
	fmt.Fprintf(stdout, "  Runtime Profile: %s\n", result.RuntimeProfileID)
	fmt.Fprintf(stdout, "  Status:         %s\n", result.Status)
	fmt.Fprintf(stdout, "  Directories:    %s\n", strings.Join(result.Directories, ", "))
	fmt.Fprintf(stdout, "  Materialized:   %s\n", result.MaterializedAt.Format(time.RFC3339))
	return 0
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: stratum [--data-dir PATH] [--artifact-blob-root PATH] [--agent-url URL] [--agent-token TOKEN] <projects|rooms|sessions|checkpoints|artifacts|environments|operations|runtime-observations|agents> <command> [flags]")
}
