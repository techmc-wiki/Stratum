package process

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeStagingPathsStayInsideSessionDirs(t *testing.T) {
	layout := testRuntimeLayout(t)
	staging := layout.Staging()
	artifactPath, err := staging.ArtifactPath("mods/test-mod.jar")
	if err != nil {
		t.Fatal(err)
	}
	configPath, err := staging.ConfigPath("server/server.properties")
	if err != nil {
		t.Fatal(err)
	}
	if artifactPath != filepath.Join(layout.ArtifactsDir, "mods", "test-mod.jar") || configPath != filepath.Join(layout.ConfigDir, "server", "server.properties") {
		t.Fatalf("artifact=%q config=%q", artifactPath, configPath)
	}
	if !pathWithin(layout.ArtifactsDir, artifactPath) || !pathWithin(layout.ConfigDir, configPath) {
		t.Fatalf("staging paths escaped layout: artifact=%q config=%q", artifactPath, configPath)
	}
}

func TestRuntimeStagingRejectsUnsafeNames(t *testing.T) {
	staging := testRuntimeLayout(t).Staging()
	unsafe := []string{"", ".", "..", "../escape.jar", "mods/../../escape.jar", filepath.Join("..", "escape.jar"), "bad:name.jar", "bad name.jar"}
	for _, name := range unsafe {
		if _, err := staging.ArtifactPath(name); err == nil {
			t.Fatalf("artifact name %q should fail", name)
		}
		if _, err := staging.ConfigPath(name); err == nil {
			t.Fatalf("config name %q should fail", name)
		}
	}
	absolute := filepath.Join(staging.ArtifactsDir, "file.jar")
	if !filepath.IsAbs(absolute) {
		t.Fatalf("test path should be absolute: %q", absolute)
	}
	if _, err := staging.ArtifactPath(absolute); err == nil {
		t.Fatalf("absolute path %q should fail", absolute)
	}
}

func TestRuntimeStagingManifestWriteRoundTrip(t *testing.T) {
	layout := testRuntimeLayout(t)
	staging := layout.Staging()
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	artifact, err := staging.ArtifactEntry("artifact-1", "mods/test.jar", now)
	if err != nil {
		t.Fatal(err)
	}
	config, err := staging.ConfigEntry("config-1", "server.properties", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := staging.WriteArtifactManifest([]StagedRuntimeItem{artifact}, now); err != nil {
		t.Fatal(err)
	}
	if err := staging.WriteConfigManifest([]StagedRuntimeItem{config}, now); err != nil {
		t.Fatal(err)
	}
	artifactManifest := readManifest(t, staging.StagedArtifactsManifest)
	if artifactManifest.SessionID != layout.SessionID || artifactManifest.Kind != "artifacts" || len(artifactManifest.Items) != 1 || artifactManifest.Items[0].ID != "artifact-1" {
		t.Fatalf("artifact manifest=%+v", artifactManifest)
	}
	configManifest := readManifest(t, staging.StagedConfigManifest)
	if configManifest.SessionID != layout.SessionID || configManifest.Kind != "config" || len(configManifest.Items) != 1 || configManifest.Items[0].ID != "config-1" {
		t.Fatalf("config manifest=%+v", configManifest)
	}
}

func testRuntimeLayout(t *testing.T) SessionRuntimeLayout {
	t.Helper()
	layout, err := NewSessionRuntimeLayout(t.TempDir(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	return layout
}

func readManifest(t *testing.T, path string) StagingManifest {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var manifest StagingManifest
	if err := json.NewDecoder(file).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}
