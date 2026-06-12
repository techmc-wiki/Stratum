package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
