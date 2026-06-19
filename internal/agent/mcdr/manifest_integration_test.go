package mcdr

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentprocess "github.com/stratummc/stratum/internal/agent/process"
)

func TestStartCommandUsesJavaExecutable(t *testing.T) {
	layout := testMCDRRuntimeLayout(t)
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	cfg := NewRuntimeConfig(layout)
	cfg.ServerJarName = "fabric-server-launch.jar"
	cfg.JavaExecutable = "/usr/lib/jvm/java-16/bin/java"
	path, err := WriteRuntimeConfig(layout, cfg)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(payload)
	want := `start_command: "/usr/lib/jvm/java-16/bin/java" -jar "fabric-server-launch.jar" nogui`
	if !strings.Contains(content, want) {
		t.Fatalf("config.yml missing expected start_command:\nwant: %s\ngot:\n%s", want, content)
	}
}

func TestStartCommandFallsBackToSystemJava(t *testing.T) {
	layout := testMCDRRuntimeLayout(t)
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	cfg := NewRuntimeConfig(layout)
	cfg.ServerJarName = "server.jar"
	path, err := WriteRuntimeConfig(layout, cfg)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(payload)
	if !strings.Contains(content, `"java" -jar "server.jar" nogui`) {
		t.Fatalf("config.yml should fall back to quoted java:\n%s", content)
	}
}

func TestStartOmitsStartCommandWithoutServerJar(t *testing.T) {
	layout := testMCDRRuntimeLayout(t)
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	cfg := NewRuntimeConfig(layout)
	cfg.JavaExecutable = "/usr/bin/java21"
	path, err := WriteRuntimeConfig(layout, cfg)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(payload)
	if strings.Contains(content, "start_command") {
		t.Fatalf("config.yml should not contain start_command without ServerJarName:\n%s", content)
	}
}

func TestReadMaterializationManifest(t *testing.T) {
	configDir := t.TempDir()
	manifestPath := filepath.Join(configDir, "environment-materialization.json")
	manifest := map[string]interface{}{
		"mcdrExecutable": "/venv/bin/mcdreforged",
		"javaExecutable": "/usr/bin/java17",
		"serverJarName":  "fabric-server-launch.jar",
		"otherField":     42,
	}
	data, _ := json.Marshal(manifest)
	os.WriteFile(manifestPath, data, 0o644)

	result := readMaterializationManifest(configDir)
	if result["mcdrExecutable"] != "/venv/bin/mcdreforged" {
		t.Fatalf("mcdrExecutable=%q", result["mcdrExecutable"])
	}
	if result["javaExecutable"] != "/usr/bin/java17" {
		t.Fatalf("javaExecutable=%q", result["javaExecutable"])
	}
	if result["serverJarName"] != "fabric-server-launch.jar" {
		t.Fatalf("serverJarName=%q", result["serverJarName"])
	}
	if _, exists := result["otherField"]; exists {
		t.Fatal("non-string field 42 should not appear in string-only result")
	}
}

func TestReadMaterializationManifestMissingFileReturnsEmpty(t *testing.T) {
	result := readMaterializationManifest(filepath.Join(t.TempDir(), "nonexistent"))
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %+v", result)
	}
}

func TestStartWithManifestData(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "stdin")

	exe, _ := os.Executable()
	sessionLayout, _ := agentprocess.NewSessionRuntimeLayout(root, "mcdr-manifest")
	sessionLayout.Create()
	manifest := map[string]interface{}{
		"mcdrExecutable": exe,
		"javaExecutable": "/path/to/java17",
		"serverJarName":  "test-server.jar",
	}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(sessionLayout.ConfigDir, "environment-materialization.json")
	os.MkdirAll(sessionLayout.ConfigDir, 0o755)
	os.WriteFile(manifestPath, data, 0o644)

	state, err := ms.Start(context.Background(), "mcdr-manifest", profile)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusRunning {
		t.Fatalf("expected running, got %s", state.Status)
	}

	mcdrLayout, _ := sessionLayout.MCDR()
	configPath := filepath.Join(mcdrLayout.MCDRConfigDir, configYMLName)
	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config.yml not written: %v", err)
	}
	content := string(payload)
	if !strings.Contains(content, `"/path/to/java17" -jar "test-server.jar"`) {
		t.Fatalf("config.yml missing expected start_command:\n%s", content)
	}

	ms.Stop(context.Background(), "mcdr-manifest")
}

func TestStartUsesVenvMCDRExecutable(t *testing.T) {
	root := t.TempDir()
	ps, err := agentprocess.NewSupervisorWithRoot("agent-test", root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewSupervisor(ps)
	profile := mcdrTestProfile(t, "stdin")

	exe, _ := os.Executable()
	sessionLayout, _ := agentprocess.NewSessionRuntimeLayout(root, "mcdr-venv-mcdr")
	sessionLayout.Create()
	manifest := map[string]interface{}{
		"mcdrExecutable": exe,
	}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(sessionLayout.ConfigDir, "environment-materialization.json")
	os.MkdirAll(sessionLayout.ConfigDir, 0o755)
	os.WriteFile(manifestPath, data, 0o644)

	state, err := ms.Start(context.Background(), "mcdr-venv-mcdr", profile)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusRunning {
		t.Fatalf("expected running, got %s", state.Status)
	}
	ms.Stop(context.Background(), "mcdr-venv-mcdr")
}
