package worldcheckpoint

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerCreatesSnapshotOfWorldDir(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	worldDir := filepath.Join(sessionRoot, "work", "world")
	for _, dir := range []string{worldDir, filepath.Join(worldDir, "region"), filepath.Join(worldDir, "data")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(worldDir, "level.dat"), []byte("level-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "region", "r.0.0.mca"), []byte("region-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := worker.Create(context.Background(), CreateParams{
		SessionRoot: sessionRoot,
		WorldDir:    worldDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SnapshotRef == "" || result.SizeBytes <= 0 || result.SHA256 == "" {
		t.Fatalf("result=%+v", result)
	}
	if !strings.HasSuffix(result.SnapshotRef, ".zip") {
		t.Fatalf("snapshot ref = %q", result.SnapshotRef)
	}
	if _, err := os.Stat(result.SnapshotRef); err != nil {
		t.Fatalf("snapshot file not found: %v", err)
	}
	reader, openErr := zip.OpenReader(result.SnapshotRef)
	if openErr != nil {
		t.Fatalf("open snapshot zip: %v", openErr)
	}
	defer reader.Close()
	names := map[string]bool{}
	for _, f := range reader.File {
		names[f.Name] = true
	}
	if !names["level.dat"] || !names["region/r.0.0.mca"] {
		t.Fatalf("zip entries = %v", names)
	}
}

func TestWorkerRejectsWorldDirOutsideSessionRoot(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	escapeDir := filepath.Join(t.TempDir(), "escape")
	if err := os.MkdirAll(escapeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = worker.Create(context.Background(), CreateParams{
		SessionRoot: sessionRoot,
		WorldDir:    escapeDir,
	})
	if err == nil || !strings.Contains(err.Error(), "escapes session root") {
		t.Fatalf("expected escape rejection: %v", err)
	}
}

func TestWorkerRejectsSessionRootOutsideRuntimeRoot(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = worker.Create(context.Background(), CreateParams{
		SessionRoot: filepath.Join(t.TempDir(), "other"),
		WorldDir:    filepath.Join(t.TempDir(), "other", "world"),
	})
	if err == nil || !strings.Contains(err.Error(), "escapes runtime root") {
		t.Fatalf("expected escape rejection: %v", err)
	}
}

func TestWorkerEmptyWorldDirProducesSnapshot(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	worldDir := filepath.Join(sessionRoot, "work", "world")
	if err := os.MkdirAll(worldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := worker.Create(context.Background(), CreateParams{
		SessionRoot: sessionRoot,
		WorldDir:    worldDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SizeBytes <= 0 || result.SHA256 == "" {
		t.Fatalf("empty world should produce valid snapshot: %+v", result)
	}
	if _, err := os.Stat(result.SnapshotRef); err != nil {
		t.Fatalf("snapshot file not found: %v", err)
	}
	reader, openErr := zip.OpenReader(result.SnapshotRef)
	if openErr != nil {
		t.Fatalf("open snapshot zip: %v", openErr)
	}
	defer reader.Close()
	if len(reader.File) != 0 {
		t.Fatalf("empty dir zip should have no entries: %d", len(reader.File))
	}
}

func TestWorkerRejectsWorldDirNotADirectory(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	filePath := filepath.Join(sessionRoot, "work")
	if err := os.MkdirAll(filePath, 0o755); err != nil {
		t.Fatal(err)
	}
	filePath = filepath.Join(filePath, "notadir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = worker.Create(context.Background(), CreateParams{
		SessionRoot: sessionRoot,
		WorldDir:    filePath,
	})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected not-a-directory rejection: %v", err)
	}
}

func TestPathWithin(t *testing.T) {
	tests := []struct {
		root, candidate string
		want            bool
	}{
		{"/a", "/a/b", true},
		{"/a", "/a", true},
		{"/a", "/b", false},
		{"/a", "/a/b/../../b", false},
		{"/a/b/c", "/a/b/c/d", true},
		{"/a", "/aa", false},
	}
	for _, tt := range tests {
		got := pathWithin(tt.root, tt.candidate)
		if got != tt.want {
			t.Errorf("pathWithin(%q, %q) = %t, want %t", tt.root, tt.candidate, got, tt.want)
		}
	}
}
