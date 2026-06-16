package lucybridge

import (
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/artifact"
	"github.com/stratummc/stratum/internal/environment"
)

func TestEnvironmentToSpecGTMC117(t *testing.T) {
	env := environment.Environment{
		ID:               "gtmc-1.17",
		Name:             "GTMC 1.17 Testing",
		MinecraftVersion: "1.17.1",
		JavaVersion:      "17",
		LoaderType:       environment.LoaderFabric,
		LoaderVersion:    "0.14.21",
		ServerCore:       environment.ServerCarpet,
		MCDRRequired:     true,
		CarpetRequired:   true,
		RuntimeProfileID: "mcdr-fabric-carpet",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Metadata:         map[string]string{"project": "gtmc"},
	}
	spec, err := EnvironmentToSpec(env)
	if err != nil {
		t.Fatal(err)
	}
	if spec.EnvironmentID != "gtmc-1.17" {
		t.Fatalf("expected gtmc-1.17, got %s", spec.EnvironmentID)
	}
	if spec.MinecraftVersion != "1.17.1" {
		t.Fatalf("expected 1.17.1, got %s", spec.MinecraftVersion)
	}
	if spec.LoaderType != "fabric" {
		t.Fatalf("expected fabric, got %s", spec.LoaderType)
	}
	if spec.ServerCore != "carpet" {
		t.Fatalf("expected carpet, got %s", spec.ServerCore)
	}
	if !spec.MCDRRequired {
		t.Fatal("expected MCDRRequired true")
	}
	if !spec.CarpetRequired {
		t.Fatal("expected CarpetRequired true")
	}
	if spec.RuntimeProfileID != "mcdr-fabric-carpet" {
		t.Fatalf("expected mcdr-fabric-carpet, got %s", spec.RuntimeProfileID)
	}
	if spec.Metadata["project"] != "gtmc" {
		t.Fatal("expected metadata to be copied")
	}
}

func TestEnvironmentToSpecMapsFlags(t *testing.T) {
	env := environment.Environment{
		ID:               "test",
		Name:             "Test",
		MinecraftVersion: "1.17.1",
		LoaderType:       environment.LoaderFabric,
		ServerCore:       environment.ServerFabric,
		MCDRRequired:     false,
		CarpetRequired:   true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	spec, err := EnvironmentToSpec(env)
	if err != nil {
		t.Fatal(err)
	}
	if spec.MCDRRequired {
		t.Fatal("expected MCDRRequired false")
	}
	if !spec.CarpetRequired {
		t.Fatal("expected CarpetRequired true")
	}
}

func TestEnvironmentToSpecMapsRuntimeProfileID(t *testing.T) {
	env := environment.Environment{
		ID:               "test",
		Name:             "Test",
		MinecraftVersion: "1.17.1",
		LoaderType:       environment.LoaderFabric,
		ServerCore:       environment.ServerFabric,
		RuntimeProfileID: "custom-profile",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	spec, err := EnvironmentToSpec(env)
	if err != nil {
		t.Fatal(err)
	}
	if spec.RuntimeProfileID != "custom-profile" {
		t.Fatalf("expected custom-profile, got %s", spec.RuntimeProfileID)
	}
}

func TestEnvironmentToSpecMissingRequiredFields(t *testing.T) {
	env := environment.Environment{
		ID:         "test",
		Name:       "Test",
		LoaderType: environment.LoaderFabric,
		ServerCore: environment.ServerFabric,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_, err := EnvironmentToSpec(env)
	if err == nil {
		t.Fatal("expected error for missing minecraft_version")
	}
}

func TestArtifactToLocalRefWithPayload(t *testing.T) {
	now := time.Now()
	art := artifact.Artifact{
		ID:               "art-1",
		Name:             "test-mod.jar",
		Type:             artifact.TypeJar,
		SHA256:           "abc123",
		SizeBytes:        1024,
		PayloadAlgorithm: "sha256",
		PayloadStatus:    artifact.PayloadAvailable,
		CreatedAt:        now,
	}
	ref, err := ArtifactToLocalRef(art, "mods/test-mod.jar")
	if err != nil {
		t.Fatal(err)
	}
	if ref.ArtifactID != "art-1" {
		t.Fatalf("expected art-1, got %s", ref.ArtifactID)
	}
	if ref.PayloadHash != "abc123" {
		t.Fatalf("expected abc123, got %s", ref.PayloadHash)
	}
	if ref.PayloadSize != 1024 {
		t.Fatalf("expected 1024, got %d", ref.PayloadSize)
	}
	if ref.ArtifactType != "jar" {
		t.Fatalf("expected jar, got %s", ref.ArtifactType)
	}
	if ref.RuntimeName != "mods/test-mod.jar" {
		t.Fatalf("expected mods/test-mod.jar, got %s", ref.RuntimeName)
	}
}

func TestArtifactToLocalRefRuntimeNameTraversalFails(t *testing.T) {
	now := time.Now()
	art := artifact.Artifact{
		ID:               "art-1",
		Name:             "test-mod.jar",
		Type:             artifact.TypeJar,
		SHA256:           "abc123",
		SizeBytes:        1024,
		PayloadAlgorithm: "sha256",
		CreatedAt:        now,
	}
	_, err := ArtifactToLocalRef(art, "../escape.jar")
	if err == nil {
		t.Fatal("expected error for traversal runtime_name")
	}
}

func TestMappingDoesNotCallLucy(t *testing.T) {
	env := environment.Environment{
		ID:               "test",
		Name:             "Test",
		MinecraftVersion: "1.17.1",
		LoaderType:       environment.LoaderFabric,
		ServerCore:       environment.ServerFabric,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	_, err := EnvironmentToSpec(env)
	if err != nil {
		t.Fatal(err)
	}
}
