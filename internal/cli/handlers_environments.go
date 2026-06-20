package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/audit"
	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/idgen"
	"github.com/stratummc/stratum/internal/storage/filesystem"
	stratumerrors "github.com/stratummc/stratum/internal/stratumerr"
)

func createEnvironment(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("environments create", stderr)
	id := flags.String("id", "", "")
	name := flags.String("name", "", "")
	minecraftVersion := flags.String("minecraft-version", "", "")
	javaVersion := flags.String("java-version", "", "")
	loaderType := flags.String("loader", string(environment.LoaderNone), "")
	serverCore := flags.String("server-core", string(environment.ServerVanilla), "")
	mcdrRequired := flags.Bool("mcdr-required", false, "")
	runtimeProfile := flags.String("runtime-profile", "", "")
	runtimeProfileRequired := flags.Bool("runtime-profile-required", false, "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	env := environment.Environment{
		ID:                     *id,
		Name:                   *name,
		MinecraftVersion:       *minecraftVersion,
		JavaVersion:            *javaVersion,
		LoaderType:             environment.LoaderType(*loaderType),
		ServerCore:             environment.ServerCore(*serverCore),
		MCDRRequired:           *mcdrRequired,
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
	eventID, _ := idgen.NewID("audit")
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

	value, err := readEnvironmentFile(*path)
	if err != nil {
		fmt.Fprintln(stderr, err)
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

func importEnvironmentFile(
	ctx context.Context,
	store *filesystem.Store,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
) int {
	flags := newFlagSet("environments import-file", stderr)
	path := flags.String("file", "", "")
	actor := flags.String("actor", "", "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*path) == "" {
		fmt.Fprintln(stderr, "--file is required")
		return 2
	}
	actorID := strings.TrimSpace(*actor)
	if actorID == "" {
		fmt.Fprintln(stderr, "--actor is required")
		return 2
	}

	value, err := readEnvironmentFile(*path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	auditID, err := idgen.NewID("audit")
	if err != nil {
		return reportError(stderr, "create environment import audit id", err)
	}
	event, err := audit.NewEvent(
		auditID,
		actorID,
		"environment.imported",
		"environment",
		value.ID,
		time.Now().UTC(),
	)
	if err != nil {
		return reportError(stderr, "create environment import audit", err)
	}
	event.Metadata = map[string]string{
		"environmentId":    value.ID,
		"name":             value.Name,
		"minecraftVersion": value.MinecraftVersion,
		"loaderType":       string(value.LoaderType),
		"serverCore":       string(value.ServerCore),
		"actor":            actorID,
		"sourceFile":       filepath.Base(filepath.Clean(*path)),
	}
	if value.RuntimeProfileID != "" {
		event.Metadata["runtimeProfileId"] = value.RuntimeProfileID
	}
	if err := store.CreateEnvironment(ctx, value); err != nil {
		return reportError(stderr, "import environment", err)
	}
	if err := store.AppendAuditEvent(ctx, event); err != nil {
		return reportError(stderr, "write environment import audit", err)
	}

	fmt.Fprintf(stdout, "Imported Environment: %s\n", value.ID)
	fmt.Fprintf(stdout, "name: %s\n", value.Name)
	fmt.Fprintf(stdout, "minecraft_version: %s\n", value.MinecraftVersion)
	fmt.Fprintf(stdout, "loader_type: %s\n", value.LoaderType)
	fmt.Fprintf(stdout, "server_core: %s\n", value.ServerCore)
	if value.RuntimeProfileID != "" {
		fmt.Fprintf(stdout, "runtime_profile_id: %s\n", value.RuntimeProfileID)
	}
	return 0
}

func readEnvironmentFile(path string) (environment.Environment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return environment.Environment{}, fmt.Errorf("read environment file %q: %w", path, err)
	}

	var value environment.Environment
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return environment.Environment{}, fmt.Errorf(
			"decode environment file %q: %w",
			path,
			err,
		)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values are not allowed")
		}
		return environment.Environment{}, fmt.Errorf(
			"decode environment file %q: %w",
			path,
			err,
		)
	}
	if err := value.Validate(); err != nil {
		return environment.Environment{}, fmt.Errorf(
			"validate environment file %q: %w",
			path,
			err,
		)
	}
	return value, nil
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
	fmt.Fprintf(stdout, "Updated At:          %s\n", env.UpdatedAt.Format(time.RFC3339))
	return 0
}

func updateEnvironment(ctx context.Context, store *filesystem.Store, args []string, stdout, stderr io.Writer) int {
	flags := newFlagSet("environments update", stderr)
	id := flags.String("id", "", "")
	actor := flags.String("actor", "", "")
	expectedUpdatedAt := flags.String("expected-updated-at", "", "")
	name := flags.String("name", "", "")
	description := flags.String("description", "", "")
	minecraftVersion := flags.String("minecraft-version", "", "")
	javaVersion := flags.String("java-version", "", "")
	loader := flags.String("loader", "", "")
	loaderVersion := flags.String("loader-version", "", "")
	serverCore := flags.String("server-core", "", "")
	mcdrRequired := flags.String("mcdr-required", "", "")
	carpetRequired := flags.String("carpet-required", "", "")
	runtimeProfile := flags.String("runtime-profile", "", "")
	runtimeProfileRequired := flags.String("runtime-profile-required", "", "")
	notes := flags.String("notes", "", "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(stderr, "--id is required")
		return 2
	}
	if *actor == "" {
		fmt.Fprintln(stderr, "--actor is required")
		return 2
	}
	if *expectedUpdatedAt == "" {
		fmt.Fprintln(stderr, "--expected-updated-at is required")
		return 2
	}
	expected, err := time.Parse(time.RFC3339, *expectedUpdatedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --expected-updated-at: %v\n", err)
		return 2
	}
	env, err := store.GetEnvironment(ctx, *id)
	if err != nil {
		fmt.Fprintf(stderr, "get environment error: %v\n", err)
		return 1
	}
	changed := []string{}
	if *name != "" && *name != env.Name {
		env.Name = *name
		changed = append(changed, "name")
	}
	if *description != "" && *description != env.Description {
		env.Description = *description
		changed = append(changed, "description")
	}
	if *minecraftVersion != "" && *minecraftVersion != env.MinecraftVersion {
		env.MinecraftVersion = *minecraftVersion
		changed = append(changed, "minecraftVersion")
	}
	if *javaVersion != "" && *javaVersion != env.JavaVersion {
		env.JavaVersion = *javaVersion
		changed = append(changed, "javaVersion")
	}
	if *loader != "" && *loader != string(env.LoaderType) {
		env.LoaderType = environment.LoaderType(*loader)
		changed = append(changed, "loaderType")
	}
	if *loaderVersion != "" && *loaderVersion != env.LoaderVersion {
		env.LoaderVersion = *loaderVersion
		changed = append(changed, "loaderVersion")
	}
	if *serverCore != "" && *serverCore != string(env.ServerCore) {
		env.ServerCore = environment.ServerCore(*serverCore)
		changed = append(changed, "serverCore")
	}
	if *mcdrRequired != "" {
		val := *mcdrRequired == "true"
		if val != env.MCDRRequired {
			env.MCDRRequired = val
			changed = append(changed, "mcdrRequired")
		}
	}
	if *carpetRequired != "" {
		val := *carpetRequired == "true"
		if val != env.CarpetRequired {
			env.CarpetRequired = val
			changed = append(changed, "carpetRequired")
		}
	}
	if *runtimeProfile != "" && *runtimeProfile != env.RuntimeProfileID {
		env.RuntimeProfileID = *runtimeProfile
		changed = append(changed, "runtimeProfileId")
	}
	if *runtimeProfileRequired != "" {
		val := *runtimeProfileRequired == "true"
		if val != env.RuntimeProfileRequired {
			env.RuntimeProfileRequired = val
			changed = append(changed, "runtimeProfileRequired")
		}
	}
	if *notes != "" && *notes != env.Notes {
		env.Notes = *notes
		changed = append(changed, "notes")
	}
	env.UpdatedAt = time.Now().UTC()
	if err := env.Validate(); err != nil {
		fmt.Fprintf(stderr, "validation error: %v\n", err)
		return 1
	}
	if err := store.UpdateEnvironment(ctx, env, expected); err != nil {
		if stratumerrors.IsKind(err, stratumerrors.KindConflict) {
			fmt.Fprintf(stderr, "conflict: %v\n", err)
		} else {
			fmt.Fprintf(stderr, "update environment error: %v\n", err)
		}
		return 1
	}
	eventID, _ := idgen.NewID("audit")
	event, _ := audit.NewEvent(eventID, *actor, "environment.updated", "environment", env.ID, time.Now().UTC())
	event.Metadata = map[string]string{"environmentId": env.ID, "changedFields": strings.Join(changed, ","), "previousUpdatedAt": expected.Format(time.RFC3339), "newUpdatedAt": env.UpdatedAt.Format(time.RFC3339)}
	_ = store.AppendAuditEvent(ctx, event)
	fmt.Fprintf(stdout, "Environment updated: %s\n", env.ID)
	fmt.Fprintf(stdout, "Updated At: %s\n", env.UpdatedAt.Format(time.RFC3339))
	fmt.Fprintf(stdout, "Changed fields: %s\n", strings.Join(changed, ", "))
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
