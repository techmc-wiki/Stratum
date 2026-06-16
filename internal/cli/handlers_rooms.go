package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/stratummc/stratum/internal/room"
	roomsvc "github.com/stratummc/stratum/internal/room/service"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

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
	svc := roomsvc.New(store, store)
	value := room.Room{ID: *id, ProjectID: *projectID, Name: *name, EnvironmentID: *environmentID, BaseWorldRef: *baseWorld, CreatedAt: time.Now().UTC()}
	if err := svc.CreateRoom(ctx, value, "cli"); err != nil {
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
