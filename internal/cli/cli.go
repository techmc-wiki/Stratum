package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/domain/checkpoint"
	"github.com/stratummc/stratum/internal/domain/environment"
	"github.com/stratummc/stratum/internal/domain/project"
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/room"
	"github.com/stratummc/stratum/internal/domain/session"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/repository/filesystem"
	"github.com/stratummc/stratum/internal/service/sessionsvc"
)

const defaultDataDirectory = ".stratum/data"

func Run(args []string, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("stratum", flag.ContinueOnError)
	global.SetOutput(stderr)
	dataDirectory := global.String("data-dir", defaultDataDirectory, "metadata data directory")
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
	case "sessions prepare", "sessions start", "sessions stop", "sessions restart",
		"sessions freeze", "sessions unfreeze", "sessions mark-crashed",
		"sessions archive", "sessions delete":
		return runSessionLifecycle(ctx, store, action, command[2:], stdout, stderr)
	case "checkpoints create":
		return createCheckpoint(ctx, store, command[2:], stdout, stderr)
	case "checkpoints list":
		return listCheckpoints(ctx, store, stdout, stderr)
	case "checkpoints get":
		return getCheckpoint(ctx, store, command[2:], stdout, stderr)
	case "artifacts list":
		return listArtifacts(ctx, store, stdout, stderr)
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
	fmt.Fprintf(stdout, "Created %s session %s in state %s. Runtime start is TODO.\n", value.Type, value.ID, value.State)
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

func runSessionLifecycle(ctx context.Context, store *filesystem.Store, action string, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("sessions "+action, stderr)
	id := flags.String("id", "", "session ID")
	actor := flags.String("actor", "", "actor user ID")
	reason := flags.String("reason", "", "operation reason")
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
	service := sessionsvc.New(store, policy)
	switch action {
	case "prepare":
		err = service.Prepare(ctx, *id, *actor)
	case "start":
		err = service.Start(ctx, *id, *actor)
	case "stop":
		err = service.Stop(ctx, *id, *actor)
	case "restart":
		err = service.Restart(ctx, *id, *actor)
	case "freeze":
		err = service.Freeze(ctx, *id, *actor)
	case "unfreeze":
		err = service.Unfreeze(ctx, *id, *actor)
	case "mark-crashed":
		err = service.MarkCrashed(ctx, *id, *actor, *reason)
	case "archive":
		err = service.Archive(ctx, *id, *actor)
	case "delete":
		err = service.Delete(ctx, *id, *actor)
	}
	if err != nil {
		return reportError(stderr, "session "+action, err)
	}
	fmt.Fprintf(stdout, "Session %s operation %s completed.\n", *id, action)
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
	fmt.Fprintln(writer, "usage: stratum [--data-dir PATH] <projects|rooms|sessions|checkpoints|artifacts> <command> [flags]")
}
