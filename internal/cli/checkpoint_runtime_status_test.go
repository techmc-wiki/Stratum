package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/httptransport"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/repository/filesystem"
)

type checkpointRuntimeStatusAgent struct {
	agent.AgentClient
	status agent.SessionRuntimeStatus
	err    error
}

func (a checkpointRuntimeStatusAgent) GetSessionRuntimeStatus(context.Context, string) (agent.SessionRuntimeStatus, error) {
	return a.status, a.err
}

func TestCheckpointCreateCapturesAgentRuntimeStatus(t *testing.T) {
	status := agent.SessionRuntimeStatus{
		SessionID: "session-1", CheckedAt: time.Date(2026, 6, 14, 14, 0, 0, 0, time.UTC),
		RuntimeRootExists: true, SessionRootExists: true,
		EnvironmentManifest: &agent.EnvironmentManifestStatus{
			Exists: true, Status: "prepared", EnvironmentID: "env-test", MinecraftVersion: "1.17.1",
			LoaderType: "fabric", ServerCore: "carpet", RuntimeProfileID: "dummy-process",
		},
		MCDRLayout:            &agent.MCDRLayoutStatus{MCDRRootExists: true, ManifestExists: true},
		MaterializedArtifacts: &agent.MaterializedArtifactsStatus{ManifestExists: true, Count: 2},
		AppliedArtifacts:      &agent.AppliedArtifactsStatus{ManifestExists: true, Count: 1},
		ProcessStatus:         &agent.ProcessStatusSummary{Status: "running", RuntimeProfileID: "dummy-process", PID: 42},
	}
	server := httptest.NewServer(httptransport.NewServer(checkpointRuntimeStatusAgent{AgentClient: local.NewFake(), status: status}, "", nil).Handler())
	defer server.Close()

	dataDirectory := filepath.Join(t.TempDir(), "data")
	setupCheckpointSession(t, dataDirectory)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--data-dir", dataDirectory, "--agent-url", server.URL, "checkpoints", "create", "--id", "checkpoint-status", "--session", "session-1", "--actor", "actor-1"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("create: code=%d stderr=%q", code, stderr.String())
	}

	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := store.GetCheckpoint(context.Background(), "checkpoint-status")
	if err != nil {
		t.Fatal(err)
	}
	if cp.RuntimeStatusSnapshot == nil || !cp.RuntimeStatusSnapshot.EnvironmentManifestExists || cp.RuntimeStatusSnapshot.ProcessState != "running" || cp.RuntimeStatusSnapshot.MaterializedArtifactsCount != 2 || cp.RuntimeStatusSnapshot.AppliedArtifactsCount != 1 {
		t.Fatalf("runtime status snapshot = %+v", cp.RuntimeStatusSnapshot)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--data-dir", dataDirectory, "checkpoints", "inspect", "--id", cp.ID}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect: code=%d stderr=%q", code, stderr.String())
	}
	for _, expected := range []string{"Runtime Status Snapshot: yes", "Environment Manifest:       true", "Process State:              running"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("inspect output missing %q: %q", expected, stdout.String())
		}
	}
}

func TestCheckpointRuntimeStatusFailureDoesNotPersist(t *testing.T) {
	server := httptest.NewServer(httptransport.NewServer(checkpointRuntimeStatusAgent{AgentClient: local.NewFake(), err: errors.New("runtime status unavailable")}, "", nil).Handler())
	defer server.Close()

	dataDirectory := filepath.Join(t.TempDir(), "data")
	setupCheckpointSession(t, dataDirectory)
	store, err := filesystem.New(dataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--data-dir", dataDirectory, "--agent-url", server.URL, "checkpoints", "create", "--id", "checkpoint-failed", "--session", "session-1", "--actor", "actor-1"}, &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "runtime status unavailable") {
		t.Fatalf("create: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := store.GetCheckpoint(context.Background(), "checkpoint-failed"); err == nil {
		t.Fatal("checkpoint persisted after runtime status failure")
	}
	after, err := store.ListAuditEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("audit written after runtime status failure: before=%d after=%d", len(before), len(after))
	}
}

func setupCheckpointSession(t *testing.T, dataDirectory string) {
	t.Helper()
	setupTestProjectRoomEnvironment(t, dataDirectory)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--data-dir", dataDirectory, "sessions", "create", "--id", "session-1", "--project", "project-1", "--room", "room-1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create session: code=%d stderr=%q", code, stderr.String())
	}
}
