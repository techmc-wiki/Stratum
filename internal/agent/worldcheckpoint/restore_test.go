package worldcheckpoint

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerRestoreCreatesWorldDirWithFiles(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	snapshotPath := filepath.Join(root, "world.zip")
	golden := map[string]string{
		"level.dat":        "level-data",
		"region/r.0.0.mca": "region-data",
		"data/stats.dat":   "stats-data",
	}
	createTestZip(t, snapshotPath, golden)

	result, err := worker.Restore(context.Background(), RestoreParams{
		SessionRoot:  sessionRoot,
		WorldDirRel:  "world",
		SnapshotPath: snapshotPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EntryCount != len(golden) {
		t.Fatalf("entry count: got %d, want %d", result.EntryCount, len(golden))
	}
	for rel, expected := range golden {
		fullPath := filepath.Join(result.RestoredDir, filepath.FromSlash(rel))
		data, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if string(data) != expected {
			t.Fatalf("%s: got %q, want %q", rel, string(data), expected)
		}
	}
}

func TestWorkerRestoreRejectsWorldDirRelWithDots(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	snapshotPath := filepath.Join(root, "world.zip")
	createTestZip(t, snapshotPath, map[string]string{"file.txt": "data"})
	_, err = worker.Restore(context.Background(), RestoreParams{
		SessionRoot:  sessionRoot,
		WorldDirRel:  "../escape",
		SnapshotPath: snapshotPath,
	})
	if err == nil || !strings.Contains(err.Error(), "safe") {
		t.Fatalf("expected safe rejection: %v", err)
	}
}

func TestWorkerRestoreDefaultWorldDirRel(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	snapshotPath := filepath.Join(root, "world.zip")
	createTestZip(t, snapshotPath, map[string]string{"level.dat": "data"})

	result, err := worker.Restore(context.Background(), RestoreParams{
		SessionRoot:  sessionRoot,
		SnapshotPath: snapshotPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(result.RestoredDir, "world") {
		t.Fatalf("restored dir = %q", result.RestoredDir)
	}
}

func TestWorkerRestoreRejectsZipSlip(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	snapshotPath := filepath.Join(root, "slip.zip")
	createTestZipWithName(t, snapshotPath, "../escape/file.txt", "evil")

	_, err = worker.Restore(context.Background(), RestoreParams{
		SessionRoot:  sessionRoot,
		WorldDirRel:  "world",
		SnapshotPath: snapshotPath,
	})
	if err == nil || !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("expected traversal rejection: %v", err)
	}
}

func TestWorkerRestoreRejectsSymlinkEntry(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	snapshotPath := filepath.Join(root, "symlink.zip")
	createTestZipWithSymlink(t, snapshotPath, "link.txt")

	_, err = worker.Restore(context.Background(), RestoreParams{
		SessionRoot:  sessionRoot,
		WorldDirRel:  "world",
		SnapshotPath: snapshotPath,
	})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection: %v", err)
	}
}

func TestWorkerRestoreRejectsSessionRootOutsideRuntimeRoot(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = worker.Restore(context.Background(), RestoreParams{
		SessionRoot:  filepath.Join(t.TempDir(), "other"),
		WorldDirRel:  "world",
		SnapshotPath: filepath.Join(root, "world.zip"),
	})
	if err == nil || !strings.Contains(err.Error(), "escapes runtime root") {
		t.Fatalf("expected escape rejection: %v", err)
	}
}

func TestWorkerRestoreEmptyZip(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	snapshotPath := filepath.Join(root, "empty.zip")
	createEmptyTestZip(t, snapshotPath)

	result, err := worker.Restore(context.Background(), RestoreParams{
		SessionRoot:  sessionRoot,
		WorldDirRel:  "world",
		SnapshotPath: snapshotPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EntryCount != 0 || result.SizeBytes != 0 {
		t.Fatalf("empty restore: %+v", result)
	}
}

func TestWorkerRestoreWithDirs(t *testing.T) {
	root := t.TempDir()
	worker, err := NewWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	snapshotPath := filepath.Join(root, "dirs.zip")
	createTestZip(t, snapshotPath, map[string]string{
		"level.dat":         "level",
		"region/r.0.0.mca":  "r00",
		"data/sub/file.txt": "sub",
	})

	result, err := worker.Restore(context.Background(), RestoreParams{
		SessionRoot:  sessionRoot,
		WorldDirRel:  "world",
		SnapshotPath: snapshotPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EntryCount != 3 {
		t.Fatalf("entry count: %d", result.EntryCount)
	}
	if _, err := os.Stat(filepath.Join(result.RestoredDir, "region", "r.0.0.mca")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(result.RestoredDir, "data", "sub", "file.txt")); err != nil {
		t.Fatal(err)
	}
}

func createTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	w, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	zw := zip.NewWriter(w)
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func createTestZipWithName(t *testing.T, path, name, content string) {
	t.Helper()
	w, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	zw := zip.NewWriter(w)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func createTestZipWithSymlink(t *testing.T, path, name string) {
	t.Helper()
	w, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	zw := zip.NewWriter(w)
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(os.ModeSymlink | 0o777)
	f, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("/etc/passwd")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func createEmptyTestZip(t *testing.T, path string) {
	t.Helper()
	w, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(w)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	w.Close()
}
