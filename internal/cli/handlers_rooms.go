package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/stratummc/stratum/internal/room"
	roomsvc "github.com/stratummc/stratum/internal/room/service"
	"github.com/stratummc/stratum/internal/storage/filesystem"
	"github.com/stratummc/stratum/internal/worldprofile"
)

func createRoom(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("rooms create", stderr)
	id := flags.String("id", "", "room ID")
	projectID := flags.String("project", "", "project ID")
	name := flags.String("name", "", "room name")
	environmentID := flags.String("environment", "", "environment ID")
	baseWorld := flags.String("base-world", "base-world:unconfigured", "immutable base-world reference")

	worldProfileName := flags.String("world-name", "", "world profile name")
	seed := flags.String("seed", "", "world seed (empty = random)")
	levelType := flags.String("level-type", "default", "level type: default, flat, largeBiomes, amplified")
	generatorSettings := flags.String("generator-settings", "", "generator settings JSON")
	generateStructures := flags.Bool("generate-structures", true, "generate structures")
	spawnRadius := flags.Int("spawn-radius", 10, "spawn radius")
	difficulty := flags.String("difficulty", "normal", "difficulty: peaceful, easy, normal, hard")

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

	var wp *worldprofile.WorldProfile
	if *worldProfileName != "" {
		wpID := fmt.Sprintf("wp_%s", *id)
		created, err := worldprofile.New(worldprofile.CreateParams{
			ID:                 wpID,
			Name:               *worldProfileName,
			Seed:               *seed,
			LevelType:          worldprofile.LevelType(*levelType),
			GeneratorSettings:  *generatorSettings,
			GenerateStructures: *generateStructures,
			SpawnRadius:        *spawnRadius,
			Difficulty:         worldprofile.Difficulty(*difficulty),
		})
		if err != nil {
			return reportError(stderr, "create world profile", err)
		}
		wp = &created
	}

	svc := roomsvc.New(store, store)
	value := room.Room{
		ID:                  *id,
		ProjectID:           *projectID,
		Name:                *name,
		EnvironmentID:       *environmentID,
		BaseWorldRef:        *baseWorld,
		DefaultWorldProfile: wp,
		CreatedAt:           time.Now().UTC(),
	}
	if err := svc.CreateRoom(ctx, value, "cli"); err != nil {
		return reportError(stderr, "create room", err)
	}
	fmt.Fprintf(stdout, "Created room %s in project %s.\n", value.ID, value.ProjectID)
	if wp != nil {
		fmt.Fprintf(stdout, "  World: %s (level-type: %s, difficulty: %s)\n", wp.Name, wp.LevelType, wp.Difficulty)
	}
	return 0
}

func listRooms(ctx context.Context, store *filesystem.Store, stdout, stderr io.Writer) int {
	values, err := store.ListRooms(ctx)
	if err != nil {
		return reportError(stderr, "list rooms", err)
	}
	for _, value := range values {
		worldInfo := "no world profile"
		if value.DefaultWorldProfile != nil {
			worldInfo = fmt.Sprintf("world: %s", value.DefaultWorldProfile.Name)
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", value.ID, value.ProjectID, value.Name, value.EnvironmentID, worldInfo)
	}
	return 0
}
