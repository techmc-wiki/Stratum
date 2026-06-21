package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
	"github.com/stratummc/stratum/internal/integration/lucy"
)

func TestGetSessionRuntimeStatusBeforeMaterialization(t *testing.T) {
	tmp := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", tmp, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	status, err := supervisor.GetSessionRuntimeStatus(context.Background(), "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.SessionID != "test-session" {
		t.Errorf("wrong session ID: got %q", status.SessionID)
	}
	if status.RuntimeRootExists != true {
		t.Errorf("runtime root should exist")
	}
	if status.SessionRootExists {
		t.Errorf("session root should not exist before materialization")
	}
	if status.EnvironmentManifest != nil {
		t.Errorf("environment manifest should be nil before materialization")
	}
}

func TestGetSessionRuntimeStatusAfterMaterialization(t *testing.T) {
	tmp := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", tmp, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:              "test-session",
		EnvironmentID:          "env-1-17",
		EnvironmentName:        "1.17 Fabric",
		MinecraftVersion:       "1.17.1",
		JavaVersion:            "17",
		LoaderType:             "fabric",
		ServerCore:             "carpet",
		RuntimeProfileID:       "dummy-process",
		RuntimeProfileRequired: true,
		ActorID:                "alice",
	}
	_, err = supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize failed: %v", err)
	}
	status, err := supervisor.GetSessionRuntimeStatus(context.Background(), "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.SessionRootExists {
		t.Errorf("session root should exist")
	}
	if !status.ConfigDirExists {
		t.Errorf("config dir should exist")
	}
	if !status.LogsDirExists {
		t.Errorf("logs dir should exist")
	}
	if status.EnvironmentManifest == nil {
		t.Fatalf("environment manifest should exist")
	}
	if !status.EnvironmentManifest.Exists {
		t.Errorf("environment manifest exists should be true")
	}
	if status.EnvironmentManifest.Status != "prepared" {
		t.Errorf("wrong status: got %q", status.EnvironmentManifest.Status)
	}
	if status.EnvironmentManifest.EnvironmentID != "env-1-17" {
		t.Errorf("wrong environment ID: got %q", status.EnvironmentManifest.EnvironmentID)
	}
	if status.EnvironmentManifest.MinecraftVersion != "1.17.1" {
		t.Errorf("wrong minecraft version: got %q", status.EnvironmentManifest.MinecraftVersion)
	}
	if status.EnvironmentManifest.RuntimeProfileID != "dummy-process" {
		t.Errorf("wrong runtime profile: got %q", status.EnvironmentManifest.RuntimeProfileID)
	}
}

func TestGetSessionRuntimeStatusIncludesLucyLockHash(t *testing.T) {
	tmp := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", tmp, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := lucy.NewEmbeddedAdapter(&fakeBackend{
		lock: lucy.EnvironmentLock{LockID: "lock-1", LockHash: "hash123"},
	})
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)
	result, err := supervisor.MaterializeEnvironment(context.Background(), agent.EnvironmentMaterializationRequest{
		SessionID:        "test-session",
		EnvironmentID:    "env-1-17",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		ServerCore:       "carpet",
		ActorID:          "alice",
	})
	if err != nil {
		t.Fatalf("materialize failed: %v", err)
	}
	if result.LucyLockHash == "" {
		t.Fatal("materialization result LucyLockHash should be non-empty")
	}
	status, err := supervisor.GetSessionRuntimeStatus(context.Background(), "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.EnvironmentManifest == nil {
		t.Fatal("environment manifest should exist")
	}
	if status.EnvironmentManifest.LucyLockHash != result.LucyLockHash {
		t.Fatalf("LucyLockHash = %q, want %q", status.EnvironmentManifest.LucyLockHash, result.LucyLockHash)
	}
}

func TestGetSessionRuntimeStatusWithMaterializedArtifacts(t *testing.T) {
	tmp := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", tmp, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(tmp, "sessions", "test-session")
	artifactsDir := filepath.Join(sessionRoot, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("create artifacts dir: %v", err)
	}
	manifest := StagingManifest{SessionID: "test-session", Kind: "artifacts", Items: []StagedRuntimeItem{{ID: "art-1", Name: "mod.jar"}, {ID: "art-2", Name: "config.toml"}}}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(artifactsDir, "staged-artifacts.json")
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	status, err := supervisor.GetSessionRuntimeStatus(context.Background(), "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.MaterializedArtifacts == nil {
		t.Fatalf("materialized artifacts should exist")
	}
	if !status.MaterializedArtifacts.ManifestExists {
		t.Errorf("manifest exists should be true")
	}
	if status.MaterializedArtifacts.Count != 2 {
		t.Errorf("wrong count: got %d", status.MaterializedArtifacts.Count)
	}
}

func TestGetSessionRuntimeStatusWithProcessRunning(t *testing.T) {
	tmp := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", tmp, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := supervisor.StartProcess(context.Background(), "test-session", runtimeprofile.DummyProcess()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = supervisor.StopProcess(context.Background(), "test-session") })
	status, err := supervisor.GetSessionRuntimeStatus(context.Background(), "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.ProcessStatus == nil {
		t.Fatalf("process status should exist")
	}
	if status.ProcessStatus.Status != string(StatusRunning) {
		t.Errorf("wrong status: got %q", status.ProcessStatus.Status)
	}
	if status.ProcessStatus.RuntimeProfileID != runtimeprofile.DefaultProfileID {
		t.Errorf("wrong runtime profile: got %q", status.ProcessStatus.RuntimeProfileID)
	}
}

func TestGetSessionRuntimeStatusWithWorldProfile(t *testing.T) {
	tmp := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", tmp, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sessionRoot := filepath.Join(tmp, "sessions", "test-session")
	workDir := filepath.Join(sessionRoot, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	serverProps := `level-seed=12345
level-type=flat
difficulty=hard
view-distance=10
generate-structures=false
spawn-protection=16
`
	if err := os.WriteFile(filepath.Join(workDir, "server.properties"), []byte(serverProps), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := supervisor.GetSessionRuntimeStatus(ctx, "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.WorldProfile == nil {
		t.Fatalf("world profile should be populated")
	}
	if status.WorldProfile.Seed != "12345" {
		t.Errorf("wrong seed: got %q", status.WorldProfile.Seed)
	}
	if status.WorldProfile.LevelType != "flat" {
		t.Errorf("wrong level-type: got %q", status.WorldProfile.LevelType)
	}
	if status.WorldProfile.Difficulty != "hard" {
		t.Errorf("wrong difficulty: got %q", status.WorldProfile.Difficulty)
	}
	if status.WorldProfile.ViewDistance != 10 {
		t.Errorf("wrong view-distance: got %d", status.WorldProfile.ViewDistance)
	}
	if status.WorldProfile.GenerateStructures != false {
		t.Errorf("wrong generate-structures: got %v", status.WorldProfile.GenerateStructures)
	}
	if status.WorldProfile.SpawnRadius != 16 {
		t.Errorf("wrong spawn-radius: got %d", status.WorldProfile.SpawnRadius)
	}
}
