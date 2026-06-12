package process

import (
	"os"
	"path/filepath"
	"testing"
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
