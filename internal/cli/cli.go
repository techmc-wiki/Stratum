package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/domain/checkpoint"
	"github.com/stratummc/stratum/internal/domain/environment"
	"github.com/stratummc/stratum/internal/domain/operation"
	"github.com/stratummc/stratum/internal/domain/project"
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/room"
	"github.com/stratummc/stratum/internal/domain/session"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/repository/filesystem"
	"github.com/stratummc/stratum/internal/service/observationsvc"
	"github.com/stratummc/stratum/internal/service/sessionsvc"
)

const defaultDataDirectory = ".stratum/data"

func Run(args []string, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("stratum", flag.ContinueOnError)
	global.SetOutput(stderr)
	dataDirectory := global.String("data-dir", defaultDataDirectory, "metadata data directory")
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
	resource, action := command[0], command[1]
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
	case "sessions logs":
		return sessionLogs(ctx, agentClient, command[2:], stdout, stderr)
	case "sessions prepare", "sessions start", "sessions stop", "sessions restart",
		"sessions freeze", "sessions unfreeze", "sessions mark-crashed",
		"sessions archive", "sessions delete":
		return runSessionLifecycle(ctx, store, agentClient, agentMode, action, command[2:], stdout, stderr)
	case "checkpoints create":
		return createCheckpoint(ctx, store, command[2:], stdout, stderr)
	case "checkpoints list":
		return listCheckpoints(ctx, store, stdout, stderr)
	case "checkpoints get":
		return getCheckpoint(ctx, store, command[2:], stdout, stderr)
	case "artifacts list":
		return listArtifacts(ctx, store, stdout, stderr)
	case "operations list":
		return listOperations(ctx, store, command[2:], stdout, stderr)
	case "operations inspect":
		return inspectOperation(ctx, store, command[2:], stdout, stderr)
	case "agents list":
		return listAgents(ctx, agentClient, stdout, stderr)
	case "agents inspect":
		return inspectAgent(ctx, agentClient, command[2:], stdout, stderr)
	case "agents resources":
		return agentResources(ctx, agentClient, command[2:], stdout, stderr)
	case "agents runtime-profiles":
		return agentRuntimeProfiles(ctx, agentClient, command[2:], stdout, stderr)
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
	environmentID := flags.String("environment", environment.MVP117Fabric().ID, "environment ID")
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
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", value.ID, value.ProjectID, value.Name)
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
	environmentID := environment.MVP117Fabric().ID
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
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", value.ID, value.ProjectID, value.Type, value.State)
	}
	return 0
}

func runSessionLifecycle(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, agentMode, action string, args []string, stdout, stderr io.Writer) int {
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
	fmt.Fprintf(stdout, "id=%s project=%s room=%s type=%s state=%s agent=%s agentStatus=%s runtimeMessage=%q endpoint=%s observed=%s process=%s runtimeProfile=%s runtimeType=%s runtimeMode=%s pid=%d running=%t crashed=%t exitCode=%s runtimeError=%q\n",
		value.ID, value.ProjectID, value.RoomID, value.Type, value.State, value.AssignedAgentID,
		value.LastAgentStatus, value.LastRuntimeMessage, value.RuntimeEndpoint, observed.Status, observed.ProcessID,
		observed.RuntimeProfileID, observed.RuntimeType, observed.RuntimeMode, observed.PID, observed.Running, observed.Crashed, optionalInt(observed.ExitCode), observed.LastError)
	return 0
}

func observeSession(ctx context.Context, store *filesystem.Store, agentClient agent.AgentClient, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions observe", stderr)
	id := flags.String("id", "", "session ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
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
	fmt.Fprintf(stdout, "id=%s session=%s controllerState=%s agentStatus=%s agent=%s runtimeProfile=%s process=%s pid=%d mismatch=%t mismatchType=%s severity=%s recommendedAction=%s observedAt=%s\n",
		result.ID, result.SessionID, result.ControllerSessionState, result.AgentRuntimeStatus, result.ObserverAgentID,
		result.RuntimeProfileID, result.ProcessID, result.PID, result.MismatchDetected, result.MismatchType,
		result.Severity, result.RecommendedAction, result.ObservedAt.Format(time.RFC3339Nano))
	return 0
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
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", value.ID, value.Name, value.Type, value.Status)
	}
	return 0
}

func ensureEnvironment(ctx context.Context, store *filesystem.Store, id string) error {
	if _, err := store.GetEnvironment(ctx, id); err == nil {
		return nil
	} else if !stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
		return err
	}
	defaultEnvironment := environment.MVP117Fabric()
	if id != defaultEnvironment.ID {
		return fmt.Errorf("environment %q is not registered", id)
	}
	return store.CreateEnvironment(ctx, defaultEnvironment)
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

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: stratum [--data-dir PATH] [--agent-url URL] [--agent-token TOKEN] <projects|rooms|sessions|checkpoints|artifacts|operations|agents> <command> [flags]")
}
