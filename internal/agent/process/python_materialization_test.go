package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
	agentjava "github.com/stratummc/stratum/internal/agent/java"
	agentpython "github.com/stratummc/stratum/internal/agent/python"
	"github.com/stratummc/stratum/internal/agent/serverjar"
)

type fakePythonDetector struct{}

func (fakePythonDetector) SelectForMCDR(context.Context) (agentpython.Installation, error) {
	return agentpython.Installation{Version: "3.11.5", Major: 3, Minor: 11, Patch: 5, ExecutablePath: "python3", HasVenv: true, HasPip: true}, nil
}

type fakePythonManager struct{}

func (fakePythonManager) CreateVenv(_ context.Context, req agentpython.VenvRequest) (agentpython.VenvResult, error) {
	return agentpython.BuildVenvResult(req.SessionID, req.VenvPath), nil
}

func (fakePythonManager) InstallMCDR(context.Context, agentpython.InstallMCDRRequest) error {
	return nil
}

func (fakePythonManager) VerifyMCDR(context.Context, agentpython.VenvResult) (string, error) {
	return "MCDReforged v2.15.7", nil
}

type fakeJavaDetector struct{}

func (fakeJavaDetector) SelectForMinecraftVersion(_ context.Context, _ string) (agentjava.Installation, error) {
	return agentjava.Installation{Version: "17.0.10", Major: 17, ExecutablePath: "/usr/bin/java17", Home: "/usr/lib/jvm/java17"}, nil
}

type fakeServerJarDeployer struct {
	root string
}

func (d fakeServerJarDeployer) Deploy(_ context.Context, req serverjar.DeployRequest) (serverjar.DeployResult, error) {
	if req.ServerCore == "error" {
		return serverjar.DeployResult{}, fmt.Errorf("download failed")
	}
	jarName := "test-server.jar"
	if req.ServerCore == "fabric" {
		jarName = "fabric-server-" + req.MinecraftVersion + "-fat.jar"
	}
	target := filepath.Join(req.TargetDir, jarName)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return serverjar.DeployResult{}, err
	}
	if err := os.WriteFile(target, []byte("jar-content"), 0o644); err != nil {
		return serverjar.DeployResult{}, err
	}
	return serverjar.DeployResult{DeployedPath: target, JarName: jarName, SHA256: "abc123", SizeBytes: 42, Source: "test"}, nil
}

func TestMaterializeEnvironmentPreparesMCDRPythonRuntime(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetPythonRuntime(fakePythonDetector{}, fakePythonManager{})

	result, err := supervisor.MaterializeEnvironment(context.Background(), agent.EnvironmentMaterializationRequest{
		SessionID:        "session-mcdr-python",
		EnvironmentID:    "env-117-fabric",
		EnvironmentName:  "1.17 Fabric Carpet",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "16",
		LoaderType:       "fabric",
		LoaderVersion:    "0.12.0",
		ServerCore:       "fabric",
		MCDRRequired:     true,
		CarpetRequired:   true,
		ActorID:          "alice",
	})
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Metadata["mcdrMaterializationStatus"] != "ready" {
		t.Fatalf("metadata=%+v", result.Metadata)
	}
	if result.Metadata["mcdrVersion"] != "MCDReforged v2.15.7" {
		t.Fatalf("mcdrVersion=%q", result.Metadata["mcdrVersion"])
	}
	if result.Metadata["mcdrExecutable"] == "" || result.Metadata["mcdrVenvPath"] == "" {
		t.Fatalf("missing MCDR runtime metadata: %+v", result.Metadata)
	}
}

func TestMaterializeEnvironmentJavaAndServerJar(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetJavaAndServerJarRuntime(fakeJavaDetector{}, fakeServerJarDeployer{root: root})

	result, err := supervisor.MaterializeEnvironment(context.Background(), agent.EnvironmentMaterializationRequest{
		SessionID:        "session-java-jar",
		EnvironmentID:    "env-fabric",
		EnvironmentName:  "Fabric",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "16",
		LoaderType:       "fabric",
		LoaderVersion:    "0.12.0",
		ServerCore:       "fabric",
		ActorID:          "alice",
	})
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Metadata["javaDetectionStatus"] != "ok" {
		t.Fatalf("java metadata=%+v", result.Metadata)
	}
	if result.Metadata["javaExecutable"] != "/usr/bin/java17" {
		t.Fatalf("java executable: %q", result.Metadata["javaExecutable"])
	}
	if result.Metadata["serverJarDeployStatus"] != "ok" {
		t.Fatalf("server jar metadata=%+v", result.Metadata)
	}
	if result.Metadata["serverJarName"] == "" {
		t.Fatalf("missing server jar name")
	}
}

func TestMaterializeEnvironmentServerJarFailureNonFatal(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetJavaAndServerJarRuntime(fakeJavaDetector{}, fakeServerJarDeployer{root: root})

	result, err := supervisor.MaterializeEnvironment(context.Background(), agent.EnvironmentMaterializationRequest{
		SessionID:        "session-fail",
		EnvironmentID:    "env-fail",
		EnvironmentName:  "Fail",
		MinecraftVersion: "1.17.1",
		ServerCore:       "error",
		ActorID:          "alice",
	})
	if err != nil {
		t.Fatalf("materialize should not fail on server jar error: %v", err)
	}
	if result.Metadata["serverJarDeployStatus"] != "failed" {
		t.Fatalf("expected server jar deploy failure: %+v", result.Metadata)
	}
}

func TestMaterializeEnvironmentSkipsServerJarForCustom(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetJavaAndServerJarRuntime(fakeJavaDetector{}, fakeServerJarDeployer{root: root})

	result, err := supervisor.MaterializeEnvironment(context.Background(), agent.EnvironmentMaterializationRequest{
		SessionID:        "session-custom",
		EnvironmentID:    "env-custom",
		EnvironmentName:  "Custom",
		MinecraftVersion: "1.17.1",
		ServerCore:       "custom",
		ActorID:          "alice",
	})
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Metadata["serverJarDeployStatus"] == "ok" {
		t.Fatalf("should skip server jar for custom core: %+v", result.Metadata)
	}
}
