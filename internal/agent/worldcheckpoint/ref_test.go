package worldcheckpoint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAgentLocalSnapshotRef(t *testing.T) {
	t.Run("normal path generates agent-local ref", func(t *testing.T) {
		root := t.TempDir()
		sessionsDir := filepath.Join(root, "sessions", "sess-1", "checkpoints")
		mustMkdirAll(t, sessionsDir)
		snapshotPath := filepath.Join(sessionsDir, "world-20260614T120000Z.zip")
		mustWriteFile(t, snapshotPath, []byte("dummy"))

		ref, err := BuildAgentLocalSnapshotRef("agent-1", "sess-1", snapshotPath, root)
		if err != nil {
			t.Fatal(err)
		}
		if ref != "agent-local://agent-1/sessions/sess-1/checkpoints/world-20260614T120000Z.zip" {
			t.Fatalf("ref = %q", ref)
		}
	})

	t.Run("agent ID empty is rejected", func(t *testing.T) {
		_, err := BuildAgentLocalSnapshotRef("", "sess-1", "/a/b.zip", "/a")
		if err == nil || !strings.Contains(err.Error(), "agent ID") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("agent ID with slash is rejected", func(t *testing.T) {
		_, err := BuildAgentLocalSnapshotRef("a/b", "sess-1", "/a/b.zip", "/a")
		if err == nil || !strings.Contains(err.Error(), "path separator") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("agent ID with backslash is rejected", func(t *testing.T) {
		_, err := BuildAgentLocalSnapshotRef("a\\b", "sess-1", "/a/b.zip", "/a")
		if err == nil || !strings.Contains(err.Error(), "path separator") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("session ID empty is rejected", func(t *testing.T) {
		_, err := BuildAgentLocalSnapshotRef("agent-1", "", "/a/b.zip", "/a")
		if err == nil || !strings.Contains(err.Error(), "session ID") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("session ID with slash is rejected", func(t *testing.T) {
		_, err := BuildAgentLocalSnapshotRef("agent-1", "sess/1", "/a/b.zip", "/a")
		if err == nil || !strings.Contains(err.Error(), "path separator") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("snapshotPath outside runtimeRoot is rejected", func(t *testing.T) {
		root := t.TempDir()
		other := t.TempDir()
		escapePath := filepath.Join(other, "outside.zip")
		mustWriteFile(t, escapePath, []byte("x"))

		_, err := BuildAgentLocalSnapshotRef("agent-1", "sess-1", escapePath, root)
		if err == nil || !strings.Contains(err.Error(), "escapes runtime root") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("snapshotPath with .. traversal is rejected", func(t *testing.T) {
		root := t.TempDir()
		sessionsDir := filepath.Join(root, "sessions", "sess-1", "checkpoints")
		mustMkdirAll(t, sessionsDir)
		snapshotPath := filepath.Join(sessionsDir, "world.zip")
		mustWriteFile(t, snapshotPath, []byte("x"))

		nestedRoot := filepath.Join(root, "sessions", "sess-1", "checkpoints", "inner")
		mustMkdirAll(t, nestedRoot)
		_, err := BuildAgentLocalSnapshotRef("agent-1", "sess-1", snapshotPath, nestedRoot)
		if err == nil || !strings.Contains(err.Error(), "escapes runtime root") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("snapshotPath equals runtimeRoot is rejected", func(t *testing.T) {
		root := t.TempDir()
		_, err := BuildAgentLocalSnapshotRef("agent-1", "sess-1", root, root)
		if err == nil || !strings.Contains(err.Error(), "runtime root") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestParseAgentLocalSnapshotRef(t *testing.T) {
	t.Run("parses valid ref with session", func(t *testing.T) {
		ref := "agent-local://agent-1/sessions/sess-1/checkpoints/world.zip"
		agentID, sessionID, relPath, err := ParseAgentLocalSnapshotRef(ref)
		if err != nil {
			t.Fatal(err)
		}
		if agentID != "agent-1" || sessionID != "sess-1" || relPath != "sessions/sess-1/checkpoints/world.zip" {
			t.Fatalf("parsed: agentID=%q sessionID=%q relPath=%q", agentID, sessionID, relPath)
		}
	})

	t.Run("parses ref without sessions prefix", func(t *testing.T) {
		ref := "agent-local://agent-1/other/stuff/world.zip"
		agentID, sessionID, relPath, err := ParseAgentLocalSnapshotRef(ref)
		if err != nil {
			t.Fatal(err)
		}
		if agentID != "agent-1" || sessionID != "" || relPath != "other/stuff/world.zip" {
			t.Fatalf("parsed: agentID=%q sessionID=%q relPath=%q", agentID, sessionID, relPath)
		}
	})

	t.Run("rejects ref without agent-local prefix", func(t *testing.T) {
		_, _, _, err := ParseAgentLocalSnapshotRef("https://example.com/sessions/sess-1/checkpoints/world.zip")
		if err == nil || !strings.HasPrefix(err.Error(), "not an agent-local") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("rejects ref without enough slashes", func(t *testing.T) {
		_, _, _, err := ParseAgentLocalSnapshotRef("agent-local://agentonly")
		if err == nil || !strings.Contains(err.Error(), "invalid agent-local ref format") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("rejects empty path", func(t *testing.T) {
		_, _, _, err := ParseAgentLocalSnapshotRef("agent-local://agent-1/")
		if err == nil || !strings.Contains(err.Error(), "invalid agent-local ref format") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestBuildAndParseRoundTrip(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions", "sess-1", "checkpoints")
	mustMkdirAll(t, sessionsDir)
	snapshotPath := filepath.Join(sessionsDir, "world-20260614T120000Z.zip")
	mustWriteFile(t, snapshotPath, []byte("dummy"))

	ref, err := BuildAgentLocalSnapshotRef("agent-1", "sess-1", snapshotPath, root)
	if err != nil {
		t.Fatal(err)
	}
	agentID, sessionID, relPath, err := ParseAgentLocalSnapshotRef(ref)
	if err != nil {
		t.Fatal(err)
	}
	if agentID != "agent-1" || sessionID != "sess-1" {
		t.Fatalf("roundtrip: agentID=%q sessionID=%q relPath=%q", agentID, sessionID, relPath)
	}
}

func TestBuildAgentLocalSnapshotRefWindowsPath(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions", "sess-1", "checkpoints")
	mustMkdirAll(t, sessionsDir)
	snapshotPath := filepath.Join(sessionsDir, "world.zip")
	mustWriteFile(t, snapshotPath, []byte("dummy"))

	ref, err := BuildAgentLocalSnapshotRef("agent-1", "sess-1", snapshotPath, root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref, "agent-local://") {
		t.Fatalf("ref should start with agent-local://: %q", ref)
	}
	if strings.Contains(ref, "\\") {
		t.Fatalf("ref must not contain backslashes: %q", ref)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
