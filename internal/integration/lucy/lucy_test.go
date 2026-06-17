package lucy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	lucystate "github.com/mclucy/lucy/state"
)

func TestManifestService_ReadWrite(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	svc := NewManifestService(tmpDir)

	// Create a test manifest
	manifest := CreateDefault("1.17.1", "fabric", "0.14.0", true)
	manifest.Packages = []lucystate.ManifestPackage{
		{
			ID:      "fabric/carpet",
			Version: "1.4.83",
			Role:    lucystate.RoleRequired,
			Side:    lucystate.SideServer,
		},
	}

	// Write manifest
	if err := svc.Write(ctx, manifest); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify file exists
	manifestPath := filepath.Join(tmpDir, "lucy.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatal("Manifest file not created")
	}

	// Read manifest
	read, err := svc.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify content
	if read.Environment.GameVersion != "1.17.1" {
		t.Errorf("Expected game version 1.17.1, got %s", read.Environment.GameVersion)
	}
	if read.Environment.ModdingPlatform != "fabric" {
		t.Errorf("Expected platform fabric, got %s", read.Environment.ModdingPlatform)
	}
	if !read.Environment.Mcdr {
		t.Error("Expected MCDR to be enabled")
	}
	if len(read.Packages) != 1 {
		t.Fatalf("Expected 1 package, got %d", len(read.Packages))
	}
	if read.Packages[0].ID != "fabric/carpet" {
		t.Errorf("Expected package fabric/carpet, got %s", read.Packages[0].ID)
	}
}

func TestLockService_Hash(t *testing.T) {
	lock := lucystate.NewLock()
	lock.ManifestFingerprint = "test-fingerprint"
	lock.GameVersion = "1.17.1"
	lock.Platform = "fabric"
	lock.PlatformVersion = "0.14.0"
	lock.Packages = []lucystate.LockedPackage{
		{
			ID:            "fabric/carpet",
			Version:       "1.4.83",
			Source:        "modrinth",
			URL:           "https://example.com/carpet.jar",
			Filename:      "carpet-1.4.83.jar",
			Hash:          "abc123",
			HashAlgorithm: "sha512",
			InstallPath:   "mods/carpet-1.4.83.jar",
			Side:          "server",
			Provenance:    []string{"root"},
			Requester:     "user",
		},
	}

	hash1, err := Hash(&lock)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}
	if hash1 == "" {
		t.Error("Expected non-empty hash")
	}

	// Same lock should produce same hash
	hash2, err := Hash(&lock)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}
	if hash1 != hash2 {
		t.Error("Same lock produced different hashes")
	}

	// Modified lock should produce different hash
	lock.Packages[0].Version = "1.4.84"
	hash3, err := Hash(&lock)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}
	if hash1 == hash3 {
		t.Error("Modified lock produced same hash")
	}

	// Nil lock should return empty hash
	emptyHash, err := Hash(nil)
	if err != nil {
		t.Fatalf("Hash(nil) failed: %v", err)
	}
	if emptyHash != "" {
		t.Errorf("Expected empty hash for nil lock, got %s", emptyHash)
	}
}

func TestCreateDefault(t *testing.T) {
	manifest := CreateDefault("1.17.1", "fabric", "0.14.0", true)

	if manifest.Environment.GameVersion != "1.17.1" {
		t.Errorf("Expected game version 1.17.1, got %s", manifest.Environment.GameVersion)
	}
	if manifest.Environment.ModdingPlatform != "fabric" {
		t.Errorf("Expected platform fabric, got %s", manifest.Environment.ModdingPlatform)
	}
	if manifest.Environment.ModdingPlatformVersion != "0.14.0" {
		t.Errorf("Expected platform version 0.14.0, got %s", manifest.Environment.ModdingPlatformVersion)
	}
	if !manifest.Environment.Mcdr {
		t.Error("Expected MCDR to be enabled")
	}
	if len(manifest.Packages) != 0 {
		t.Errorf("Expected 0 packages, got %d", len(manifest.Packages))
	}
}

func TestProbeService_ServerInfo(t *testing.T) {
	// This test requires a valid Lucy workspace, skip if not available
	t.Skip("Requires Lucy workspace setup")
}

func TestInstallService_PackageRequest(t *testing.T) {
	req := PackageRequest{
		Platform: "fabric",
		Name:     "carpet",
		Scope:    "modrinth",
		Version:  "1.4.83",
	}

	if req.Platform != "fabric" {
		t.Errorf("Expected platform fabric, got %s", req.Platform)
	}
}
