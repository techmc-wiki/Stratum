package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
)

func TestMaterializeEnvironment(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:              "session-test",
		EnvironmentID:          "env-117-fabric",
		EnvironmentName:        "1.17 Fabric Carpet",
		MinecraftVersion:       "1.17.1",
		JavaVersion:            "17",
		LoaderType:             "fabric",
		LoaderVersion:          "0.12.0",
		ServerCore:             "carpet",
		MCDRRequired:           true,
		CarpetRequired:         true,
		RuntimeProfileID:       "dummy-process",
		RuntimeProfileRequired: true,
		ActorID:                "alice",
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.SessionID != "session-test" {
		t.Errorf("session id: got %q, want %q", result.SessionID, "session-test")
	}
	if result.EnvironmentID != "env-117-fabric" {
		t.Errorf("environment id: got %q, want %q", result.EnvironmentID, "env-117-fabric")
	}
	if result.Status != "prepared" {
		t.Errorf("status: got %q, want %q", result.Status, "prepared")
	}
	expectedDirs := []string{"config", "work", "world", "logs", "mods"}
	if len(result.Directories) != len(expectedDirs) {
		t.Errorf("directories count: got %d, want %d", len(result.Directories), len(expectedDirs))
	}
	sessionRoot := filepath.Join(root, "sessions", "session-test")
	for _, dir := range expectedDirs {
		dirPath := filepath.Join(sessionRoot, dir)
		info, err := os.Stat(dirPath)
		if err != nil {
			t.Errorf("directory %q not created: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", dir)
		}
	}
	configDir := filepath.Join(sessionRoot, "config")
	if _, err := os.Stat(configDir); err != nil {
		t.Fatalf("config directory not created: %v", err)
	}
}
