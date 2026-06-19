package process

import (
	"context"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
	agentpython "github.com/stratummc/stratum/internal/agent/python"
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
