package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
)

func TestMaterializeEnvironmentWritesManifest(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 1024*1024)
	if err != nil {
		t.Fatalf("NewSupervisorWithRoot: %v", err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:              "session-1",
		EnvironmentID:          "env-1",
		EnvironmentName:        "Test Env",
		MinecraftVersion:       "1.17.1",
		JavaVersion:            "17",
		LoaderType:             "fabric",
		LoaderVersion:          "0.14.0",
		ServerCore:             "carpet",
		MCDRRequired:           true,
		CarpetRequired:         true,
		RuntimeProfileID:       "dummy-process",
		RuntimeProfileRequired: true,
		ActorID:                "actor-1",
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("MaterializeEnvironment: %v", err)
	}
	manifestPath := filepath.Join(root, "sessions", "session-1", "config", "environment-materialization.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest file not found: %v", err)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest["session_id"] != "session-1" {
		t.Errorf("session_id: got %v, want session-1", manifest["session_id"])
	}
	if manifest["environment_id"] != "env-1" {
		t.Errorf("environment_id: got %v, want env-1", manifest["environment_id"])
	}
	if manifest["minecraft_version"] != "1.17.1" {
		t.Errorf("minecraft_version: got %v, want 1.17.1", manifest["minecraft_version"])
	}
	if manifest["runtime_profile_id"] != "dummy-process" {
		t.Errorf("runtime_profile_id: got %v, want dummy-process", manifest["runtime_profile_id"])
	}
	if manifest["status"] != "prepared" {
		t.Errorf("status: got %v, want prepared", manifest["status"])
	}
	notes, ok := manifest["notes"].(string)
	if !ok || notes == "" {
		t.Errorf("notes: missing or not string")
	}
	if manifestPath != result.Metadata["manifestPath"] {
		t.Errorf("result manifestPath: got %v, want %v", result.Metadata["manifestPath"], manifestPath)
	}
}

func TestMaterializationIsIdempotent(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 1024*1024)
	if err != nil {
		t.Fatalf("NewSupervisorWithRoot: %v", err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:              "session-1",
		EnvironmentID:          "env-1",
		EnvironmentName:        "Test Env",
		MinecraftVersion:       "1.17.1",
		JavaVersion:            "17",
		LoaderType:             "fabric",
		RuntimeProfileID:       "dummy-process",
		RuntimeProfileRequired: true,
		ActorID:                "actor-1",
	}
	if _, err := supervisor.MaterializeEnvironment(context.Background(), request); err != nil {
		t.Fatalf("first MaterializeEnvironment: %v", err)
	}
	if _, err := supervisor.MaterializeEnvironment(context.Background(), request); err != nil {
		t.Fatalf("second MaterializeEnvironment: %v", err)
	}
	manifestPath := filepath.Join(root, "sessions", "session-1", "config", "environment-materialization.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest file not found after second materialization: %v", err)
	}
}

func TestManifestContainsNoInstallNotes(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 1024*1024)
	if err != nil {
		t.Fatalf("NewSupervisorWithRoot: %v", err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-1",
		EnvironmentID:    "env-1",
		MinecraftVersion: "1.17.1",
		RuntimeProfileID: "dummy-process",
		ActorID:          "actor-1",
	}
	if _, err := supervisor.MaterializeEnvironment(context.Background(), request); err != nil {
		t.Fatalf("MaterializeEnvironment: %v", err)
	}
	manifestPath := filepath.Join(root, "sessions", "session-1", "config", "environment-materialization.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	notes, ok := manifest["notes"].(string)
	if !ok {
		t.Fatal("notes field missing or not string")
	}
	expected := "Environment materialization prepared directories only; it did not install Java, Minecraft, Fabric, Carpet, Lucy, MCDR, or start any runtime."
	if notes != expected {
		t.Errorf("notes: got %q, want %q", notes, expected)
	}
}
