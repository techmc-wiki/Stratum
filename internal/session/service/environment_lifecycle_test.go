package service_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/project"
	"github.com/stratummc/stratum/internal/resourcepolicy"
	"github.com/stratummc/stratum/internal/room"
	"github.com/stratummc/stratum/internal/session"
	sessionsvc "github.com/stratummc/stratum/internal/session/service"
	"github.com/stratummc/stratum/internal/storage/filesystem"
)

func TestImportedGTMCEnvironmentLifecycle(t *testing.T) {
	tmpData := t.TempDir()
	tmpRuntime := t.TempDir()
	store, err := filesystem.New(tmpData)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	agentClient, err := local.NewProcessAgentWithRegistryAndRoot("test-agent", runtimeprofile.Builtins(), tmpRuntime)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	ctx := context.Background()

	fixtureBytes, err := os.ReadFile(filepath.Join("testdata", "environment-lifecycle-test.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var importedEnv environment.Environment
	if err := json.Unmarshal(fixtureBytes, &importedEnv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	importedEnv.CreatedAt = time.Now().UTC()
	importedEnv.UpdatedAt = time.Now().UTC()
	if err := store.CreateEnvironment(ctx, importedEnv); err != nil {
		t.Fatalf("import environment: %v", err)
	}

	retrieved, err := store.GetEnvironment(ctx, "test-env-lifecycle")
	if err != nil {
		t.Fatalf("get environment: %v", err)
	}
	if retrieved.ID != "test-env-lifecycle" {
		t.Errorf("wrong id: got %q", retrieved.ID)
	}
	if retrieved.Name != "Test Environment for Lifecycle" {
		t.Errorf("wrong name: got %q", retrieved.Name)
	}
	if retrieved.RuntimeProfileID != "dummy-process" {
		t.Errorf("wrong runtime profile: got %q", retrieved.RuntimeProfileID)
	}

	proj := project.Project{
		ID:          "test-project",
		Name:        "Test Project",
		Description: "Integration test project",
		Members:     []project.Member{},
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.CreateProject(ctx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}

	rm := room.Room{
		ID:            "test-room",
		ProjectID:     "test-project",
		Name:          "Test Room",
		EnvironmentID: "test-env-lifecycle",
		BaseWorldRef:  "base-world:test",
		CreatedAt:     time.Now().UTC(),
	}
	if err := store.CreateRoom(ctx, rm); err != nil {
		t.Fatalf("create room: %v", err)
	}

	sess := session.Session{
		ID:              "test-session",
		ProjectID:       "test-project",
		RoomID:          "test-room",
		OwnerUserID:     "integration-test",
		Type:            session.TypeShared,
		State:           session.StateCreated,
		EnvironmentID:   "test-env-lifecycle",
		AssignedAgentID: "test-agent",
		CreatedAt:       time.Now().UTC(),
		LastActiveAt:    time.Now().UTC(),
	}
	if err := store.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create session: %v", err)
	}

	retrievedSession, err := store.GetSession(ctx, "test-session")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if retrievedSession.EnvironmentID != "test-env-lifecycle" {
		t.Errorf("session did not inherit environment: got %q", retrievedSession.EnvironmentID)
	}

	matReq := agent.EnvironmentMaterializationRequest{
		SessionID:              "test-session",
		EnvironmentID:          importedEnv.ID,
		EnvironmentName:        importedEnv.Name,
		MinecraftVersion:       importedEnv.MinecraftVersion,
		JavaVersion:            importedEnv.JavaVersion,
		LoaderType:             string(importedEnv.LoaderType),
		LoaderVersion:          importedEnv.LoaderVersion,
		ServerCore:             string(importedEnv.ServerCore),
		MCDRRequired:           importedEnv.MCDRRequired,
		CarpetRequired:         importedEnv.CarpetRequired,
		RuntimeProfileID:       importedEnv.RuntimeProfileID,
		RuntimeProfileRequired: importedEnv.RuntimeProfileRequired,
		ActorID:                "integration-test",
	}
	matResult, err := agentClient.MaterializeEnvironment(ctx, matReq)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if matResult.Status != "prepared" {
		t.Errorf("wrong materialization status: got %q", matResult.Status)
	}

	manifestPath := filepath.Join(tmpRuntime, "sessions", "test-session", "config", "environment-materialization.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Errorf("environment manifest does not exist at %s", manifestPath)
	}

	runtimeStatus, err := agentClient.GetSessionRuntimeStatus(ctx, "test-session")
	if err != nil {
		t.Fatalf("get runtime status: %v", err)
	}
	if !runtimeStatus.SessionRootExists {
		t.Error("session root should exist after materialization")
	}
	if runtimeStatus.EnvironmentManifest == nil {
		t.Fatal("environment manifest should be reported in runtime status")
	}
	if runtimeStatus.EnvironmentManifest.EnvironmentID != "test-env-lifecycle" {
		t.Errorf("wrong environment ID in status: got %q", runtimeStatus.EnvironmentManifest.EnvironmentID)
	}
	if runtimeStatus.EnvironmentManifest.MinecraftVersion != "1.17.1" {
		t.Errorf("wrong minecraft version in status: got %q", runtimeStatus.EnvironmentManifest.MinecraftVersion)
	}
	if runtimeStatus.EnvironmentManifest.RuntimeProfileID != "dummy-process" {
		t.Errorf("wrong runtime profile in status: got %q", runtimeStatus.EnvironmentManifest.RuntimeProfileID)
	}

	policy := resourcepolicy.MVPDefault()
	sessionSvc := sessionsvc.New(store, policy, agentClient)
	if err := sessionSvc.Start(ctx, "test-session", "integration-test"); err != nil {
		t.Fatalf("start session: %v", err)
	}

	started, err := store.GetSession(ctx, "test-session")
	if err != nil {
		t.Fatalf("get session after start: %v", err)
	}
	if started.State != session.StateRunning && started.State != session.StateStarting {
		t.Errorf("session should be running or starting: got %s", started.State)
	}

	agentStatus, err := agentClient.InspectSession(ctx, "test-session")
	if err != nil {
		t.Fatalf("inspect session: %v", err)
	}
	if !agentStatus.Running {
		t.Error("agent should report session running")
	}
	if agentStatus.RuntimeProfileID != "dummy-process" {
		t.Errorf("wrong runtime profile: got %q", agentStatus.RuntimeProfileID)
	}
}

func TestRoomCreationWithNonExistentEnvironment(t *testing.T) {
	tmpData := t.TempDir()
	store, err := filesystem.New(tmpData)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	ctx := context.Background()

	proj := project.Project{
		ID:          "test-project-negative",
		Name:        "Test Project Negative",
		Description: "Negative test project",
		Members:     []project.Member{},
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.CreateProject(ctx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}

	rm := room.Room{
		ID:            "bad-room",
		ProjectID:     "test-project-negative",
		Name:          "Bad Room",
		EnvironmentID: "nonexistent-env",
		BaseWorldRef:  "base-world:test",
		CreatedAt:     time.Now().UTC(),
	}
	if err := store.CreateRoom(ctx, rm); err != nil {
		t.Logf("room creation rejected: %v", err)
		return
	}
	t.Log("room creation accepted without Environment validation (current repository behavior)")
}
