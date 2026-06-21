package process

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stratummc/stratum/internal/safepath"
)

func TestSessionRuntimeLayoutPathsAndCreate(t *testing.T) {
	root := t.TempDir()
	layout, err := NewSessionRuntimeLayout(root, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if layout.RuntimeRoot != filepath.Clean(root) || layout.SessionRoot != filepath.Join(root, "sessions", "session-1") || layout.WorkDir != filepath.Join(root, "sessions", "session-1", "work") {
		t.Fatalf("layout=%+v", layout)
	}
	if err := layout.Create(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{layout.SessionRoot, layout.WorkDir, layout.LogsDir, layout.ConfigDir, layout.ArtifactsDir, layout.CheckpointsDir, layout.TmpDir} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("directory %q info=%+v err=%v", path, info, err)
		}
	}
}

func TestSessionRuntimeLayoutRejectsTraversal(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../escape", "session/escape", `session\escape`} {
		if _, err := NewSessionRuntimeLayout(t.TempDir(), id); err == nil {
			t.Fatalf("session id %q should fail", id)
		}
	}
}

func TestMCDRRuntimeLayoutPathsAndCreate(t *testing.T) {
	root := t.TempDir()
	layout, err := NewSessionRuntimeLayout(root, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	mcdr, err := layout.MCDR()
	if err != nil {
		t.Fatal(err)
	}
	if mcdr.MCDRRoot != filepath.Join(layout.WorkDir, "mcdr") {
		t.Fatalf("mcdr root=%q", mcdr.MCDRRoot)
	}
	if mcdr.MCDRConfigDir != filepath.Join(mcdr.MCDRRoot, "config") {
		t.Fatalf("mcdr config=%q", mcdr.MCDRConfigDir)
	}
	if !safepath.Within(root, mcdr.MCDRRoot) || !safepath.Within(root, mcdr.MCDRServerDir) {
		t.Fatal("MCDR paths escape runtime root")
	}
	if err := mcdr.Create(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{mcdr.MCDRRoot, mcdr.MCDRConfigDir, mcdr.MCDRPluginsDir, mcdr.MCDRServerDir, mcdr.MCDRLogsDir, mcdr.MCDRTmpDir} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("directory %q info=%+v err=%v", path, info, err)
		}
	}
}

func TestMCDRRuntimeLayoutIsIdempotent(t *testing.T) {
	root := t.TempDir()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	mcdr, _ := layout.MCDR()
	if err := mcdr.Create(); err != nil {
		t.Fatal(err)
	}
	if err := mcdr.Create(); err != nil {
		t.Fatalf("repeated create: %v", err)
	}
}

func TestMCDRLayoutManifest(t *testing.T) {
	root := t.TempDir()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	mcdr, _ := layout.MCDR()
	if err := mcdr.Create(); err != nil {
		t.Fatal(err)
	}
	if err := mcdr.WriteManifest(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(mcdr.MCDRManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest MCDRLayoutManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if manifest.SessionID != "session-1" {
		t.Fatalf("session_id=%q", manifest.SessionID)
	}
	if manifest.Status != "prepared" {
		t.Fatalf("status=%q", manifest.Status)
	}
	if !strings.Contains(manifest.Notes, "not started") {
		t.Fatalf("notes=%q", manifest.Notes)
	}
	if manifest.MCDRRoot != mcdr.MCDRRoot {
		t.Fatalf("mcdr_root=%q", manifest.MCDRRoot)
	}
	if manifest.ConfigDir != mcdr.MCDRConfigDir {
		t.Fatalf("config_dir=%q", manifest.ConfigDir)
	}
}

func TestMCDRManifestIsIdempotent(t *testing.T) {
	root := t.TempDir()
	layout, _ := NewSessionRuntimeLayout(root, "session-1")
	mcdr, _ := layout.MCDR()
	_ = mcdr.Create()
	if err := mcdr.WriteManifest(); err != nil {
		t.Fatal(err)
	}
	if err := mcdr.WriteManifest(); err != nil {
		t.Fatalf("repeated manifest write: %v", err)
	}
}
