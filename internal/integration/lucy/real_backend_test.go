package lucy

import (
	"context"
	"testing"

	lucystate "github.com/mclucy/lucy/state"
)

func TestLucyProjectBackendCapabilities(t *testing.T) {
	backend := NewLucyProjectBackend(t.TempDir())
	caps, err := backend.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !caps.SupportsPlan || !caps.SupportsLock || !caps.SupportsStatus {
		t.Fatalf("capabilities = %+v", caps)
	}
	if len(caps.SupportedSources) == 0 || len(caps.SupportedLoaders) == 0 {
		t.Fatalf("expected supported sources and loaders: %+v", caps)
	}
}

func TestManifestToLucyManifest(t *testing.T) {
	manifest := ManifestToLucyManifest(EnvironmentSpec{
		EnvironmentID:    "env-1",
		MinecraftVersion: "1.17.1",
		LoaderType:       "fabric",
		LoaderVersion:    "0.12.0",
		ServerCore:       "carpet",
		MCDRRequired:     true,
		Packages: []PackageRef{{
			ID:                "fabric/carpet",
			Source:            "modrinth",
			Name:              "carpet",
			VersionConstraint: "1.4.83",
			Required:          true,
		}},
	})
	if manifest.Environment.GameVersion != "1.17.1" || manifest.Environment.ModdingPlatform != "fabric" || !manifest.Environment.Mcdr {
		t.Fatalf("environment = %+v", manifest.Environment)
	}
	if len(manifest.Packages) != 1 || manifest.Packages[0].ID != "fabric/carpet" || manifest.Packages[0].Source != "modrinth" || manifest.Packages[0].Role != lucystate.RoleRequired {
		t.Fatalf("packages = %+v", manifest.Packages)
	}
}

func TestLucyLockToEnvironmentLock(t *testing.T) {
	lock := lucystate.NewLock()
	lock.ManifestFingerprint = "sha256:manifest"
	lock.GameVersion = "1.17.1"
	lock.Platform = "fabric"
	lock.PlatformVersion = "0.12.0"
	lock.Packages = []lucystate.LockedPackage{{ID: "fabric/carpet", Source: "modrinth", Version: "1.4.83", URL: "https://example.invalid/carpet.jar", Hash: "abc", Filename: "carpet.jar", HashAlgorithm: "sha512", InstallPath: "mods/carpet.jar", Side: "server", Provenance: []string{"root"}, Requester: "test"}}
	converted := LucyLockToEnvironmentLock(&lock)
	if converted.LockHash == "" || converted.ProviderMetadata["provider"] != "lucy" {
		t.Fatalf("converted = %+v", converted)
	}
	if len(converted.Packages) != 1 || converted.Packages[0].ID != "fabric/carpet" || converted.Packages[0].Name != "carpet" || converted.Packages[0].Version != "1.4.83" {
		t.Fatalf("packages = %+v", converted.Packages)
	}
}

func TestLucyProjectBackendPlanReturnsNoActionsForEmptySpec(t *testing.T) {
	backend := NewLucyProjectBackend(t.TempDir())
	plan, err := backend.Plan(context.Background(), EnvironmentSpec{EnvironmentID: "env-1", MinecraftVersion: "1.17.1", LoaderType: "fabric", ServerCore: "carpet"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 0 || len(plan.Errors) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
}
