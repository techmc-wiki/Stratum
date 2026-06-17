package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/process"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

func TestProcessAgentLifecycleLogsAndResources(t *testing.T) {
	ctx := context.Background()
	runtime := NewProcessAgent()
	request := agent.SessionRequest{SessionID: "session-1"}
	if _, err := runtime.PrepareSession(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.StartSession(ctx, request); err != nil {
		t.Fatal(err)
	}
	status, err := runtime.InspectSession(ctx, request.SessionID)
	if err != nil || !status.Running || status.Status != string(process.StatusRunning) || status.ProcessID == "" || status.RuntimeMode != process.RuntimeModeDummy || status.RuntimeProfileID != runtimeprofile.DefaultProfileID {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	logs, err := runtime.CollectLogs(ctx, request.SessionID)
	if err != nil || len(logs.Lines) < 2 || !containsRuntimeLog(logs.Lines, "dummy-runtime") {
		t.Fatalf("logs=%+v err=%v", logs, err)
	}
	report, err := runtime.ReportResources(ctx)
	if err != nil || report.RunningSessions != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if _, err := runtime.StopSession(ctx, request); err != nil {
		t.Fatal(err)
	}
	status, _ = runtime.InspectSession(ctx, request.SessionID)
	if status.Running || status.Status != string(process.StatusStopped) || status.ExitCode == nil {
		t.Fatalf("stopped=%+v", status)
	}
}

func containsRuntimeLog(lines []string, text string) bool {
	for _, line := range lines {
		if strings.Contains(line, text) {
			return true
		}
	}
	return false
}

func TestProcessAgentRejectsUnknownProfile(t *testing.T) {
	runtime := NewProcessAgent()
	if _, err := runtime.StartSession(context.Background(), agent.SessionRequest{SessionID: "session-unknown", RuntimeProfileID: "missing"}); err == nil {
		t.Fatal("unknown runtime profile should fail")
	}
	if _, err := runtime.StartSession(context.Background(), agent.SessionRequest{SessionID: "session-explicit", RuntimeProfileID: runtimeprofile.DefaultProfileID}); err != nil {
		t.Fatalf("explicit dummy profile: %v", err)
	}
}

func TestProcessAgentCreatesRuntimeLayout(t *testing.T) {
	root := t.TempDir()
	runtime, err := NewProcessAgentWithRegistryAndRoot(DefaultAgentID, runtimeprofile.Builtins(), root)
	if err != nil {
		t.Fatal(err)
	}
	request := agent.SessionRequest{SessionID: "session-layout", RuntimeProfileID: runtimeprofile.DefaultProfileID}
	if _, err := runtime.StartSession(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	status, err := runtime.InspectSession(context.Background(), request.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if status.WorkDir != filepath.Join(root, "sessions", "session-layout", "work") || status.LogsDir == "" || status.SessionRoot == "" {
		t.Fatalf("status=%+v", status)
	}
	if info, err := os.Stat(status.WorkDir); err != nil || !info.IsDir() {
		t.Fatalf("work dir info=%+v err=%v", info, err)
	}
}

func TestProcessAgentMCDRStopSessionIsIdempotent(t *testing.T) {
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	profile := runtimeprofile.Profile{
		ID:                  "mcdr-agent-test",
		Name:                "MCDR agent test",
		RuntimeType:         runtimeprofile.TypeMCDRPython,
		CommandArgv:         []string{executable, "-test.run=TestProcessAgentMCDRHelperProcess", "--"},
		WorkingDir:          ".",
		Env:                 map[string]string{"STRATUM_PROCESS_AGENT_MCDR_HELPER": "1"},
		StopStrategy:        runtimeprofile.StopStdin,
		StopStdinCommand:    "stop",
		GracefulStopTimeout: time.Second,
		ForceKillTimeout:    time.Second,
		LogMode:             runtimeprofile.LogMemory,
		Enabled:             true,
	}
	registry, err := runtimeprofile.NewRegistry(runtimeprofile.DummyProcess(), profile)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewProcessAgentWithRegistryAndRoot(DefaultAgentID, registry, root)
	if err != nil {
		t.Fatal(err)
	}
	request := agent.SessionRequest{SessionID: "session-mcdr-stop", RuntimeProfileID: profile.ID}
	if _, err := runtime.StartSession(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	status, err := runtime.InspectSession(context.Background(), request.SessionID)
	if err != nil || status.RuntimeType != string(runtimeprofile.TypeMCDRPython) || !status.Running {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	first, err := runtime.StopSession(context.Background(), request)
	if err != nil || first.Message != "runtime stopped" {
		t.Fatalf("first stop=%+v err=%v", first, err)
	}
	second, err := runtime.StopSession(context.Background(), request)
	if err != nil || second.Message != "runtime stopped" {
		t.Fatalf("second stop=%+v err=%v", second, err)
	}
}

func TestProcessAgentMCDRHelperProcess(t *testing.T) {
	if os.Getenv("STRATUM_PROCESS_AGENT_MCDR_HELPER") != "1" {
		return
	}
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			os.Exit(0)
		}
		if strings.TrimSpace(string(buf[:n])) == "stop" {
			os.Exit(0)
		}
	}
}

func TestProcessAgentCreateWorldSnapshot(t *testing.T) {
	root := t.TempDir()
	runtime, err := NewProcessAgentWithRegistryAndRoot(DefaultAgentID, runtimeprofile.Builtins(), root)
	if err != nil {
		t.Fatal(err)
	}
	worldDir := filepath.Join(root, "sessions", "session-1", "work", "world")
	if err := os.MkdirAll(worldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "level.dat"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.CreateWorldSnapshot(context.Background(), agent.WorldCheckpointRequest{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("CreateWorldSnapshot: %v", err)
	}
	if result.SnapshotRef == "" || result.SizeBytes <= 0 || result.SHA256 == "" {
		t.Fatalf("result=%+v", result)
	}
	if !strings.HasPrefix(result.SnapshotRef, "agent-local://") {
		t.Fatalf("SnapshotRef should be agent-local:// ref, got %q", result.SnapshotRef)
	}
	if filepath.IsAbs(result.SnapshotRef) {
		t.Fatalf("SnapshotRef must not be an absolute path: %q", result.SnapshotRef)
	}
	if result.LocalPath == "" {
		t.Fatalf("LocalPath should be set for diagnostics")
	}
	if !filepath.IsAbs(result.LocalPath) {
		t.Fatalf("LocalPath should be an absolute path: %q", result.LocalPath)
	}
	if _, err := os.Stat(result.LocalPath); err != nil {
		t.Fatalf("LocalPath file not found: %v", err)
	}
}

func TestProcessAgentCreateWorldSnapshotRejectsEscape(t *testing.T) {
	root := t.TempDir()
	runtime, err := NewProcessAgentWithRegistryAndRoot(DefaultAgentID, runtimeprofile.Builtins(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"../escape", "C:/escape"} {
		_, err := runtime.CreateWorldSnapshot(context.Background(), agent.WorldCheckpointRequest{SessionID: "session-1", WorldDirRel: dir})
		if err == nil || !strings.Contains(err.Error(), "safe") {
			t.Fatalf("dir=%q err=%v", dir, err)
		}
	}
}

func TestProcessAgentCreateWorldSnapshotMissingWorldDir(t *testing.T) {
	root := t.TempDir()
	runtime, err := NewProcessAgentWithRegistryAndRoot(DefaultAgentID, runtimeprofile.Builtins(), root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.CreateWorldSnapshot(context.Background(), agent.WorldCheckpointRequest{SessionID: "session-1"})
	if err == nil {
		t.Fatal("expected error for missing world dir")
	}
}

func TestProcessAgentRestoreWorldSnapshot(t *testing.T) {
	root := t.TempDir()
	runtime, err := NewProcessAgentWithRegistryAndRoot(DefaultAgentID, runtimeprofile.Builtins(), root)
	if err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions", "session-1")
	workDir := filepath.Join(sessionRoot, "work")
	worldDir := filepath.Join(workDir, "world")
	if err := os.MkdirAll(worldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "level.dat"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapResult, err := runtime.CreateWorldSnapshot(context.Background(), agent.WorldCheckpointRequest{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("CreateWorldSnapshot: %v", err)
	}

	restoreResult, err := runtime.RestoreWorldSnapshot(context.Background(), agent.WorldCheckpointRestoreRequest{
		SessionID:   "session-1",
		SnapshotRef: snapResult.SnapshotRef,
		WorldDirRel: "world_restored",
	})
	if err != nil {
		t.Fatalf("RestoreWorldSnapshot: %v", err)
	}
	if restoreResult.SessionID != "session-1" {
		t.Fatalf("session id: %q", restoreResult.SessionID)
	}
	if restoreResult.EntryCount != 1 {
		t.Fatalf("entry count: %d", restoreResult.EntryCount)
	}
	if !strings.HasPrefix(restoreResult.RestoredRef, "agent-local://") {
		t.Fatalf("restored ref: %q", restoreResult.RestoredRef)
	}
	if !strings.Contains(restoreResult.RestoredRef, "world_restored") {
		t.Fatalf("restored ref should reference world_restored: %q", restoreResult.RestoredRef)
	}
	restoredPath := filepath.Join(workDir, "world_restored", "level.dat")
	data, err := os.ReadFile(restoredPath)
	if err != nil {
		t.Fatalf("read restored level.dat: %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("restored data: %q", string(data))
	}
}

func TestProcessAgentRestoreRejectsMismatchedAgentID(t *testing.T) {
	root := t.TempDir()
	runtime, err := NewProcessAgentWithRegistryAndRoot("agent-99", runtimeprofile.Builtins(), root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.RestoreWorldSnapshot(context.Background(), agent.WorldCheckpointRestoreRequest{
		SessionID:   "session-1",
		SnapshotRef: "agent-local://agent-other/sessions/session-1/checkpoints/world.zip",
		WorldDirRel: "world_restored",
	})
	if err == nil || !strings.Contains(err.Error(), "belongs to agent") {
		t.Fatalf("expected agent mismatch rejection: %v", err)
	}
}

func TestProcessAgentRestoreRejectsMissingSnapshotRef(t *testing.T) {
	root := t.TempDir()
	runtime, err := NewProcessAgentWithRegistryAndRoot(DefaultAgentID, runtimeprofile.Builtins(), root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.RestoreWorldSnapshot(context.Background(), agent.WorldCheckpointRestoreRequest{
		SessionID:   "session-1",
		SnapshotRef: "",
		WorldDirRel: "world_restored",
	})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required rejection: %v", err)
	}
}

func TestProcessAgentRestoreRejectsWorldDirEscape(t *testing.T) {
	root := t.TempDir()
	runtime, err := NewProcessAgentWithRegistryAndRoot(DefaultAgentID, runtimeprofile.Builtins(), root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.RestoreWorldSnapshot(context.Background(), agent.WorldCheckpointRestoreRequest{
		SessionID:   "session-1",
		SnapshotRef: "agent-local://local/sessions/session-1/checkpoints/world.zip",
		WorldDirRel: "../escape",
	})
	if err == nil || !strings.Contains(err.Error(), "safe") {
		t.Fatalf("expected safe rejection: %v", err)
	}
}
