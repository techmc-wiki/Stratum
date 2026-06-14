package lucy

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestNoopAdapterReturnsDeterministicEmptyValues(t *testing.T) {
	adapter := NoopAdapter{}
	ctx := context.Background()

	firstCapabilities, err := adapter.Capabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	secondCapabilities, err := adapter.Capabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstCapabilities, secondCapabilities) {
		t.Fatalf("capabilities are not deterministic: first=%+v second=%+v", firstCapabilities, secondCapabilities)
	}
	if firstCapabilities.SupportsPlan || firstCapabilities.SupportsLock || firstCapabilities.SupportsStatus || len(firstCapabilities.SupportedSources) != 0 || len(firstCapabilities.SupportedLoaders) != 0 {
		t.Fatalf("capabilities = %+v", firstCapabilities)
	}

	plan, err := adapter.PlanEnvironment(ctx, PlanEnvironmentRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 0 || len(plan.Warnings) != 0 || len(plan.Errors) != 0 || plan.RequiresLockUpdate {
		t.Fatalf("plan = %+v", plan)
	}

	lock, err := adapter.LockEnvironment(ctx, LockEnvironmentRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if lock.LockID != "" || lock.LockHash != "" || !lock.GeneratedAt.IsZero() || len(lock.Packages) != 0 || len(lock.Artifacts) != 0 {
		t.Fatalf("lock = %+v", lock)
	}

	status, err := adapter.CheckStatus(ctx, StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if status.OK || len(status.Missing) != 0 || len(status.Drifted) != 0 || len(status.Warnings) != 0 || len(status.Errors) != 0 {
		t.Fatalf("status = %+v", status)
	}
}

func TestBoundaryDTOsJSONRoundTrip(t *testing.T) {
	spec := EnvironmentSpec{
		EnvironmentID: "gtmc-1.17", MinecraftVersion: "1.17.1", JavaVersion: "16",
		LoaderType: "fabric", LoaderVersion: "0.11.7", ServerCore: "carpet",
		CarpetRequired: true, MCDRRequired: true, RuntimeProfileID: "mcdr-managed",
		Packages:       []PackageRef{{ID: "fabric-loader", Source: "modrinth", Name: "Fabric Loader", VersionConstraint: "0.16.x", MinecraftVersion: "1.17.1", Loader: "fabric", Required: true, Metadata: map[string]string{"channel": "stable"}}},
		LocalArtifacts: []LocalArtifactRef{{ArtifactID: "carpet", PayloadAlgorithm: "sha256", PayloadHash: "def", PayloadSize: 34, ArtifactType: "jar", RuntimeName: "carpet.jar", Metadata: map[string]string{"kind": "jar"}}},
		Metadata:       map[string]string{"community": "gtmc"},
	}
	lock := EnvironmentLock{
		LockID: "lock-1", LockHash: "sha256:abc", GeneratedAt: time.Date(2026, 6, 15, 1, 2, 3, 0, time.UTC),
		Packages:         []LockedPackage{{ID: "fabric-loader", Source: "modrinth", Name: "Fabric Loader", Version: "0.16.0", Hash: "abc", Size: 12, Metadata: map[string]string{"channel": "stable"}}},
		Artifacts:        []LockedArtifact{{ArtifactID: "carpet", PayloadAlgorithm: "sha256", PayloadHash: "def", PayloadSize: 34, RuntimeName: "carpet.jar", Metadata: map[string]string{"kind": "jar"}}},
		ProviderMetadata: map[string]string{"adapter": "test"},
	}
	assertJSONRoundTrip(t, spec)
	assertJSONRoundTrip(t, Capabilities{SupportsPlan: true, SupportsLock: true, SupportsStatus: true, SupportedSources: []string{"modrinth"}, SupportedLoaders: []string{"fabric"}, Metadata: map[string]string{"adapter": "test"}})
	assertJSONRoundTrip(t, EnvironmentPlan{Actions: []PlanAction{{ActionType: ActionDownload, PackageID: "fabric-loader", Source: "modrinth", Target: "mods/fabric-loader.jar", Hash: "abc", Size: 12, Metadata: map[string]string{}}}, Warnings: []string{}, Errors: []string{}, RequiresLockUpdate: true, Metadata: map[string]string{"plan": "test"}})
	assertJSONRoundTrip(t, lock)
	assertJSONRoundTrip(t, EnvironmentStatus{Missing: []string{"fabric-loader"}, Drifted: []string{}, Warnings: []string{"lock update required"}, Errors: []string{}, Metadata: map[string]string{"status": "test"}})
}

func assertJSONRoundTrip[T any](t *testing.T, value T) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded T
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, value) {
		t.Fatalf("decoded=%+v want=%+v", decoded, value)
	}
}

func TestLocalArtifactRefRepresentsStratumPayload(t *testing.T) {
	ref := LocalArtifactRef{
		ArtifactID: "artifact-carpet", PayloadAlgorithm: "sha256", PayloadHash: "0123456789abcdef",
		PayloadSize: 4096, ArtifactType: "jar", RuntimeName: "carpet.jar", Metadata: map[string]string{"project_id": "gtmc"},
	}
	if ref.ArtifactID == "" || ref.PayloadAlgorithm != "sha256" || ref.PayloadHash == "" || ref.PayloadSize != 4096 || ref.ArtifactType != "jar" || ref.RuntimeName != "carpet.jar" {
		t.Fatalf("artifact ref = %+v", ref)
	}
}

func TestEnvironmentSpecSupportsGTMC117Fields(t *testing.T) {
	spec := EnvironmentSpec{
		EnvironmentID: "gtmc-1.17", MinecraftVersion: "1.17.1", JavaVersion: "16",
		LoaderType: "fabric", LoaderVersion: "0.11.7", ServerCore: "carpet",
		CarpetRequired: true, MCDRRequired: true, RuntimeProfileID: "mcdr-managed",
		Packages:       []PackageRef{{ID: "fabric-api", Source: "modrinth", Name: "Fabric API", VersionConstraint: "*", MinecraftVersion: "1.17.1", Loader: "fabric", Required: true, Metadata: map[string]string{}}},
		LocalArtifacts: []LocalArtifactRef{}, Metadata: map[string]string{"community": "gtmc"},
	}
	if spec.EnvironmentID != "gtmc-1.17" || spec.MinecraftVersion != "1.17.1" || spec.LoaderType != "fabric" || spec.ServerCore != "carpet" || !spec.CarpetRequired || !spec.MCDRRequired || len(spec.Packages) != 1 {
		t.Fatalf("environment spec = %+v", spec)
	}
}
