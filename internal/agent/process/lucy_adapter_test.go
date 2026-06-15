package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/integration/lucy"
)

func TestDefaultLucyAdapterIsNoop(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-noop",
		EnvironmentID:    "env-117-fabric",
		EnvironmentName:  "1.17 Fabric Carpet",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		LoaderVersion:    "0.12.0",
		ServerCore:       "carpet",
		MCDRRequired:     true,
		CarpetRequired:   true,
		ActorID:          "alice",
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Metadata["lucyAdapterMode"] != "noop" {
		t.Errorf("lucy adapter mode: got %q, want %q", result.Metadata["lucyAdapterMode"], "noop")
	}
	if result.Metadata["lucyResolutionStatus"] != "not_requested" {
		t.Errorf("lucy resolution status: got %q, want %q", result.Metadata["lucyResolutionStatus"], "not_requested")
	}
	if result.Metadata["lucyAdapterConfigured"] != "false" {
		t.Errorf("lucy adapter configured: got %q, want %q", result.Metadata["lucyAdapterConfigured"], "false")
	}
}

func TestSetLucyAdapterNilDefaultsToNoop(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(nil)
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-nil",
		EnvironmentID:    "env-117-fabric",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		ServerCore:       "carpet",
		ActorID:          "alice",
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Metadata["lucyAdapterMode"] != "noop" {
		t.Errorf("lucy adapter mode: got %q, want %q", result.Metadata["lucyAdapterMode"], "noop")
	}
}

func TestSetLucyAdapterEmbedded(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{}
	adapter, err := lucy.NewEmbeddedAdapter(backend)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.SetLucyAdapter(adapter)
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-embedded",
		EnvironmentID:    "env-117-fabric",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		ServerCore:       "carpet",
		ActorID:          "alice",
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	if result.Metadata["lucyAdapterMode"] != "embedded" {
		t.Errorf("lucy adapter mode: got %q, want %q", result.Metadata["lucyAdapterMode"], "embedded")
	}
	if result.Metadata["lucyAdapterConfigured"] != "true" {
		t.Errorf("lucy adapter configured: got %q, want %q", result.Metadata["lucyAdapterConfigured"], "true")
	}
}

func TestMaterializationWritesManifestWithLucyMetadata(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-manifest",
		EnvironmentID:    "env-117-fabric",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		ServerCore:       "carpet",
		ActorID:          "alice",
	}
	result, err := supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	manifestPath := result.Metadata["manifestPath"]
	if manifestPath == "" {
		t.Fatal("manifest path not in metadata")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest["lucy_adapter_mode"] != "noop" {
		t.Errorf("manifest lucy_adapter_mode: got %v, want noop", manifest["lucy_adapter_mode"])
	}
	if manifest["lucy_resolution_status"] != "not_requested" {
		t.Errorf("manifest lucy_resolution_status: got %v, want not_requested", manifest["lucy_resolution_status"])
	}
}

func TestMaterializationDoesNotWriteLucyManifests(t *testing.T) {
	root := t.TempDir()
	supervisor, err := NewSupervisorWithRoot("test-agent", root, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	request := agent.EnvironmentMaterializationRequest{
		SessionID:        "session-no-lucy",
		EnvironmentID:    "env-117-fabric",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       "fabric",
		ServerCore:       "carpet",
		ActorID:          "alice",
	}
	_, err = supervisor.MaterializeEnvironment(context.Background(), request)
	if err != nil {
		t.Fatalf("materialize environment: %v", err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-no-lucy")
	lucyYaml := filepath.Join(sessionRoot, "lucy.yaml")
	if _, err := os.Stat(lucyYaml); err == nil {
		t.Error("lucy.yaml should not exist")
	}
	lucyLock := filepath.Join(sessionRoot, "lucy-lock.yaml")
	if _, err := os.Stat(lucyLock); err == nil {
		t.Error("lucy-lock.yaml should not exist")
	}
}

type fakeBackend struct {
	caps   lucy.Capabilities
	plan   lucy.EnvironmentPlan
	lock   lucy.EnvironmentLock
	status lucy.EnvironmentStatus
	err    error
}

func (f *fakeBackend) Capabilities(_ context.Context) (lucy.Capabilities, error) {
	return f.caps, f.err
}

func (f *fakeBackend) Plan(_ context.Context, _ lucy.EnvironmentSpec) (lucy.EnvironmentPlan, error) {
	return f.plan, f.err
}

func (f *fakeBackend) Lock(_ context.Context, _ lucy.EnvironmentSpec) (lucy.EnvironmentLock, error) {
	return f.lock, f.err
}

func (f *fakeBackend) Status(_ context.Context, _ lucy.EnvironmentSpec, _ *lucy.EnvironmentLock) (lucy.EnvironmentStatus, error) {
	return f.status, f.err
}
