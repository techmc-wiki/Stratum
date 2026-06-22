package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	agentjava "github.com/stratummc/stratum/internal/agent/java"
	agentprocess "github.com/stratummc/stratum/internal/agent/process"
	agentpython "github.com/stratummc/stratum/internal/agent/python"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
	"github.com/stratummc/stratum/internal/agent/serverjar"
	"github.com/stratummc/stratum/internal/agent/serverproperties"
)

type e2ePythonDetector struct{}

func (e2ePythonDetector) SelectForMCDR(context.Context) (agentpython.Installation, error) {
	return agentpython.Installation{Version: "3.11.5", Major: 3, Minor: 11, ExecutablePath: "python3", HasVenv: true, HasPip: true}, nil
}

type e2ePythonManager struct{}

func (e2ePythonManager) CreateVenv(_ context.Context, req agentpython.VenvRequest) (agentpython.VenvResult, error) {
	return agentpython.BuildVenvResult(req.SessionID, req.VenvPath), nil
}

func (e2ePythonManager) InstallMCDR(context.Context, agentpython.InstallMCDRRequest) error {
	return nil
}

func (e2ePythonManager) VerifyMCDR(context.Context, agentpython.VenvResult) (string, error) {
	return "MCDReforged v2.15.7", nil
}

func (e2ePythonManager) VerifyMCDRExecutable(context.Context, string) (string, error) {
	return "", fmt.Errorf("global MCDR is disabled in e2e tests")
}

type e2eJavaDetector struct{}

func (e2eJavaDetector) SelectForMinecraftVersion(_ context.Context, _ string) (agentjava.Installation, error) {
	return agentjava.Installation{Version: "17.0.10", Major: 17, ExecutablePath: "/usr/bin/java17", Home: "/usr/lib/jvm/java17"}, nil
}

type e2eServerJarDeployer struct {
	root string
}

func (d e2eServerJarDeployer) Deploy(_ context.Context, req serverjar.DeployRequest) (serverjar.DeployResult, error) {
	jarName := fmt.Sprintf("fabric-server-%s-fat.jar", req.MinecraftVersion)
	target := filepath.Join(req.TargetDir, jarName)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return serverjar.DeployResult{}, err
	}
	if err := os.WriteFile(target, []byte("e2e-jar-content"), 0o644); err != nil {
		return serverjar.DeployResult{}, err
	}
	return serverjar.DeployResult{DeployedPath: target, JarName: jarName, SHA256: "e2e-abc", SizeBytes: 99, Source: "e2e-test"}, nil
}

func TestE2EReadSessionFileServerProperties(t *testing.T) {
	root := t.TempDir()
	sessionID := "e2e-readfile-1"
	sessionDir := filepath.Join(root, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	serverProps := `# Minecraft server properties
level-seed=777888
level-type=amplified
difficulty=hard
generate-structures=true
spawn-protection=12
view-distance=16
`
	if err := os.WriteFile(filepath.Join(sessionDir, "server.properties"), []byte(serverProps), 0o644); err != nil {
		t.Fatal(err)
	}

	sup, err := agentprocess.NewSupervisorWithRoot("e2e-agent", root, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	pa := &ProcessAgent{
		id:         "e2e-agent",
		supervisor: sup,
	}

	data, err := pa.ReadSessionFile(context.Background(), sessionID, "server.properties")
	if err != nil {
		t.Fatalf("ReadSessionFile: %v", err)
	}

	if string(data) != serverProps {
		t.Errorf("ReadSessionFile returned unexpected content:\n%s", string(data))
	}

	_, err = pa.ReadSessionFile(context.Background(), sessionID, "../etc/passwd")
	if err == nil {
		t.Fatal("Expected error for path traversal, got nil")
	}
}

func TestE2EMCDRSessionMaterializeAndStart(t *testing.T) {
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	mcdrProfile := runtimeprofile.Profile{
		ID:                  "mcdr-e2e",
		Name:                "MCDR E2E",
		RuntimeType:         runtimeprofile.TypeMCDRPython,
		CommandArgv:         []string{executable, "-test.run=TestE2EMCDRHelperProcess", "--"},
		WorkingDir:          ".",
		Env:                 map[string]string{"STRATUM_E2E_MCDR_HELPER": "1"},
		StopStrategy:        runtimeprofile.StopStdin,
		StopStdinCommand:    "stop",
		GracefulStopTimeout: time.Second,
		ForceKillTimeout:    time.Second,
		LogMode:             runtimeprofile.LogMemory,
		Enabled:             true,
	}
	registry, err := runtimeprofile.NewRegistry(runtimeprofile.DummyProcess(), mcdrProfile)
	if err != nil {
		t.Fatal(err)
	}
	pa, err := NewProcessAgentWithRegistryAndRoot(DefaultAgentID, registry, root)
	if err != nil {
		t.Fatal(err)
	}
	pa.supervisor.SetPythonRuntime(e2ePythonDetector{}, e2ePythonManager{})
	pa.supervisor.SetJavaAndServerJarRuntime(e2eJavaDetector{}, e2eServerJarDeployer{root: root})

	result, err := pa.MaterializeEnvironment(context.Background(), agent.EnvironmentMaterializationRequest{
		SessionID:        "e2e-mcdr-1",
		EnvironmentID:    "e2e-env",
		EnvironmentName:  "E2E Environment",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "16",
		LoaderType:       "fabric",
		LoaderVersion:    "0.14.0",
		ServerCore:       "fabric",
		MCDRRequired:     true,
		CarpetRequired:   true,
		ActorID:          "e2e-actor",
	})
	if err != nil {
		t.Fatalf("MaterializeEnvironment: %v", err)
	}
	if result.Status != "prepared" {
		t.Fatalf("expected prepared, got %s", result.Status)
	}
	if result.Metadata["mcdrMaterializationStatus"] != "ready" {
		t.Fatalf("MCDR not ready: %+v", result.Metadata)
	}
	if result.Metadata["javaDetectionStatus"] != "ok" {
		t.Fatalf("Java not detected: %+v", result.Metadata)
	}
	if result.Metadata["serverJarDeployStatus"] != "ok" {
		t.Fatalf("server jar not deployed: %+v", result.Metadata)
	}
	t.Logf("materialization metadata: %+v", result.Metadata)

	state, err := pa.StartSession(context.Background(), agent.SessionRequest{SessionID: "e2e-mcdr-1", RuntimeProfileID: mcdrProfile.ID})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	t.Logf("started: status=%s pid=%d", state.Message, 0)

	status, err := pa.InspectSession(context.Background(), "e2e-mcdr-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running {
		t.Fatalf("session not running: %+v", status)
	}
	if status.RuntimeType != string(runtimeprofile.TypeMCDRPython) {
		t.Fatalf("unexpected runtime type: %s", status.RuntimeType)
	}

	sessionLayout, _ := agentprocess.NewSessionRuntimeLayout(root, "e2e-mcdr-1")
	mcdrLayout, _ := sessionLayout.MCDR()
	configPath := filepath.Join(mcdrLayout.MCDRConfigDir, "config.yml")
	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	cfgContent := string(cfgData)
	t.Logf("generated config.yml:\n%s", cfgContent)
	if !strings.Contains(cfgContent, "start_command") {
		t.Fatal("config.yml missing start_command")
	}
	if !strings.Contains(cfgContent, "fabric-server-1.17.1") {
		t.Fatalf("config.yml missing server jar name:\n%s", cfgContent)
	}
	if !strings.Contains(cfgContent, "/usr/bin/java17") {
		t.Fatalf("config.yml missing java executable:\n%s", cfgContent)
	}

	logs := pa.supervisor.CollectLogs("e2e-mcdr-1", 0)
	if len(logs) < 1 {
		t.Fatal("no process logs")
	}
	t.Logf("process logs: %v", logs[:min(len(logs), 3)])

	if _, err := pa.StopSession(context.Background(), agent.SessionRequest{SessionID: "e2e-mcdr-1"}); err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	status, _ = pa.InspectSession(context.Background(), "e2e-mcdr-1")
	if status.Running {
		t.Fatal("session still running after stop")
	}
}

func TestE2EMCDRSessionConfigYMLInStartUp(t *testing.T) {
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	profile := runtimeprofile.Profile{
		ID:                  "mcdr-e2e-config",
		Name:                "MCDR E2E Config",
		RuntimeType:         runtimeprofile.TypeMCDRPython,
		CommandArgv:         []string{executable, "-test.run=TestE2EMCDRHelperProcess", "--"},
		WorkingDir:          ".",
		Env:                 map[string]string{"STRATUM_E2E_MCDR_HELPER": "1"},
		StopStrategy:        runtimeprofile.StopStdin,
		StopStdinCommand:    "stop",
		GracefulStopTimeout: time.Second,
		ForceKillTimeout:    time.Second,
		LogMode:             runtimeprofile.LogMemory,
		Enabled:             true,
	}
	registry, err := runtimeprofile.NewRegistry(runtimeprofile.DummyProcess(), profile)
	if err != nil {
		t.Fatal(err)
	}
	pa, err := NewProcessAgentWithRegistryAndRoot(DefaultAgentID, registry, root)
	if err != nil {
		t.Fatal(err)
	}
	pa.supervisor.SetPythonRuntime(e2ePythonDetector{}, e2ePythonManager{})
	pa.supervisor.SetJavaAndServerJarRuntime(e2eJavaDetector{}, e2eServerJarDeployer{root: root})

	_, err = pa.MaterializeEnvironment(context.Background(), agent.EnvironmentMaterializationRequest{
		SessionID:        "e2e-config-1",
		EnvironmentID:    "e2e-env-config",
		EnvironmentName:  "E2E Config Environment",
		MinecraftVersion: "1.20.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		LoaderVersion:    "latest",
		ServerCore:       "fabric",
		MCDRRequired:     true,
		ActorID:          "e2e-actor",
	})
	if err != nil {
		t.Fatalf("MaterializeEnvironment: %v", err)
	}

	_, err = pa.StartSession(context.Background(), agent.SessionRequest{SessionID: "e2e-config-1", RuntimeProfileID: profile.ID})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	sessionLayout2, _ := agentprocess.NewSessionRuntimeLayout(root, "e2e-config-1")
	mcdrLayout, _ := sessionLayout2.MCDR()
	cfgPath := filepath.Join(mcdrLayout.MCDRConfigDir, "config.yml")
	cfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config.yml not found at %s: %v", cfgPath, err)
	}
	cfg := string(cfgBytes)
	t.Logf("config.yml for 1.20.1:\n%s", cfg)

	requiredFields := []string{
		"working_directory",
		"plugin_directories",
		"handler",
		"vanilla_handler",
		"start_command",
		"config_directory",
	}
	for _, field := range requiredFields {
		if !strings.Contains(cfg, field) {
			t.Fatalf("config.yml missing %q", field)
		}
	}

	if _, err := pa.StopSession(context.Background(), agent.SessionRequest{SessionID: "e2e-config-1"}); err != nil {
		t.Fatalf("StopSession: %v", err)
	}
}

func TestE2EMCDRHelperProcess(t *testing.T) {
	if os.Getenv("STRATUM_E2E_MCDR_HELPER") != "1" {
		return
	}
	os.Stdout.WriteString("e2e-mcdr-helper-ready\n")
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			os.Exit(0)
		}
		if strings.TrimSpace(string(buf[:n])) == "stop" {
			os.Stdout.WriteString("e2e-mcdr-helper-stopped\n")
			os.Exit(0)
		}
	}
}

func TestE2ECheckpointCaptureServerProperties(t *testing.T) {
	root := t.TempDir()
	sessionID := "e2e-checkpoint-1"
	sessionDir := filepath.Join(root, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	serverProps := `# Minecraft server properties
level-seed=987654321
level-type=flat
difficulty=easy
generate-structures=false
spawn-protection=8
view-distance=10
`
	if err := os.WriteFile(filepath.Join(sessionDir, "server.properties"), []byte(serverProps), 0o644); err != nil {
		t.Fatal(err)
	}

	sup, err := agentprocess.NewSupervisorWithRoot("e2e-agent", root, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	pa := &ProcessAgent{
		id:         "e2e-agent",
		supervisor: sup,
	}

	data, err := pa.ReadSessionFile(context.Background(), sessionID, "server.properties")
	if err != nil {
		t.Fatalf("ReadSessionFile: %v", err)
	}

	cfg, err := serverproperties.Parse(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("Parse server.properties: %v", err)
	}

	if cfg.LevelSeed != "987654321" {
		t.Errorf("LevelSeed = %q, want 987654321", cfg.LevelSeed)
	}
	if cfg.LevelType != "flat" {
		t.Errorf("LevelType = %q, want flat", cfg.LevelType)
	}
	if cfg.Difficulty != "easy" {
		t.Errorf("Difficulty = %q, want easy", cfg.Difficulty)
	}
	if cfg.GenerateStructures != false {
		t.Errorf("GenerateStructures = %v, want false", cfg.GenerateStructures)
	}
	if cfg.SpawnProtection != 8 {
		t.Errorf("SpawnProtection = %d, want 8", cfg.SpawnProtection)
	}
	if cfg.ViewDistance != 10 {
		t.Errorf("ViewDistance = %d, want 10", cfg.ViewDistance)
	}

	snapshot := serverproperties.ToWorldProfileSnapshot(cfg, "1.17.1")
	if snapshot.Seed != "987654321" {
		t.Errorf("Snapshot Seed = %q", snapshot.Seed)
	}
	if snapshot.LevelType != "flat" {
		t.Errorf("Snapshot LevelType = %q", snapshot.LevelType)
	}
	if snapshot.Difficulty != "easy" {
		t.Errorf("Snapshot Difficulty = %q", snapshot.Difficulty)
	}
	if snapshot.SpawnRadius != 8 {
		t.Errorf("Snapshot SpawnRadius = %d", snapshot.SpawnRadius)
	}
	if snapshot.ViewDistance != 10 {
		t.Errorf("Snapshot ViewDistance = %d", snapshot.ViewDistance)
	}
	if snapshot.MinecraftVersion != "1.17.1" {
		t.Errorf("Snapshot MinecraftVersion = %q", snapshot.MinecraftVersion)
	}
	if snapshot.CapturedFrom != "server.properties" {
		t.Errorf("Snapshot CapturedFrom = %q", snapshot.CapturedFrom)
	}
}

func TestE2EWriteSessionFileServerProperties(t *testing.T) {
	root := t.TempDir()
	sessionID := "e2e-write-1"
	sessionDir := filepath.Join(root, "sessions", sessionID)

	sup, err := agentprocess.NewSupervisorWithRoot("e2e-agent", root, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	pa := &ProcessAgent{
		id:         "e2e-agent",
		supervisor: sup,
	}

	propsContent := []byte("level-seed=111\nlevel-type=default\ndifficulty=normal\nview-distance=8\n")
	if err := pa.WriteSessionFile(context.Background(), sessionID, "server.properties", propsContent); err != nil {
		t.Fatalf("WriteSessionFile: %v", err)
	}

	readData, err := pa.ReadSessionFile(context.Background(), sessionID, "server.properties")
	if err != nil {
		t.Fatalf("ReadSessionFile: %v", err)
	}

	if string(readData) != string(propsContent) {
		t.Errorf("ReadSessionFile returned unexpected content:\ngot:  %s\nwant: %s", string(readData), string(propsContent))
	}

	if err := pa.WriteSessionFile(context.Background(), sessionID, "../etc/passwd", []byte("bad")); err == nil {
		t.Fatal("Expected error for path traversal, got nil")
	}

	writtenPath := filepath.Join(sessionDir, "server.properties")
	if _, err := os.Stat(writtenPath); os.IsNotExist(err) {
		t.Fatalf("server.properties was not written to %s", writtenPath)
	}
}
