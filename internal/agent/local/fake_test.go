package local

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stratummc/stratum/internal/agent"
)

func TestFakeAgentStartSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	fake := NewFake()
	request := agent.SessionRequest{SessionID: "session-1", ProjectID: "project-1", EnvironmentID: "environment-1"}
	result, err := fake.StartSession(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.AgentID != DefaultAgentID || result.Status != "success" {
		t.Fatalf("result = %+v", result)
	}
	status, err := fake.InspectSession(ctx, request.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running || status.Status != "running" {
		t.Fatalf("status = %+v", status)
	}

	failing := NewFake()
	failing.SetFailure(agent.OperationStart, "configured test failure")
	if _, err := failing.StartSession(ctx, request); err == nil || !strings.Contains(err.Error(), "configured test failure") {
		t.Fatalf("error = %v", err)
	} else {
		var agentErr agent.Error
		if !errors.As(err, &agentErr) || agentErr.AgentID != DefaultAgentID || agentErr.Operation != agent.OperationStart {
			t.Fatalf("structured error = %#v", err)
		}
	}
}

func TestFakeResourceReportAndLogs(t *testing.T) {
	ctx := context.Background()
	fake := NewFake()
	_, _ = fake.StartSession(ctx, agent.SessionRequest{SessionID: "session-1"})
	report, err := fake.ReportResources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.AgentID != DefaultAgentID || report.CPUCapacity != 8 || report.MemoryTotalMB != 16384 || report.DiskTotalMB != 262144 || report.RunningSessions != 1 {
		t.Fatalf("report = %+v", report)
	}
	logs, err := fake.CollectLogs(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs.Lines) != 2 || !strings.Contains(logs.Lines[0], "fake-agent") {
		t.Fatalf("logs = %+v", logs)
	}
}

func TestFakeUnfreezeAndLowLevelStubs(t *testing.T) {
	ctx := context.Background()
	fake := NewFake()
	request := agent.SessionRequest{SessionID: "session-1"}
	if _, err := fake.FreezeSession(ctx, request); err != nil {
		t.Fatal(err)
	}
	result, err := fake.UnfreezeSession(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Message != "unfrozen" {
		t.Fatalf("result = %+v", result)
	}
	status, err := fake.Inspect(ctx, request.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running || status.Frozen {
		t.Fatalf("status = %+v", status)
	}
	if err := fake.PrepareSessionFiles(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := fake.RemoveSessionFiles(ctx, request.SessionID); err != nil {
		t.Fatal(err)
	}
}

func TestFakeSendCommand(t *testing.T) {
	ctx := context.Background()
	fake := NewFake()
	_, _ = fake.StartSession(ctx, agent.SessionRequest{SessionID: "session-1"})
	result, err := fake.SendCommand(ctx, "session-1", "save-all")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "sent" || result.AgentID != DefaultAgentID {
		t.Fatalf("result = %+v", result)
	}
	_, err = fake.SendCommand(ctx, "session-1", "")
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("empty command err=%v", err)
	}
	failing := NewFake()
	failing.SetFailure(agent.OperationSendCommand, "injected send failure")
	if _, err := failing.SendCommand(ctx, "session-1", "save-all"); err == nil || !strings.Contains(err.Error(), "injected send failure") {
		t.Fatalf("failure err=%v", err)
	}
}

func TestFakeRestoreWorldSnapshot(t *testing.T) {
	ctx := context.Background()
	fake := NewFake()
	req := agent.WorldCheckpointRestoreRequest{
		SessionID:   "session-1",
		SnapshotRef: "agent-local://local/sessions/session-1/checkpoints/world.zip",
		WorldDirRel: "world_restored",
	}
	result, err := fake.RestoreWorldSnapshot(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "session-1" {
		t.Fatalf("session id: %q", result.SessionID)
	}
	if !strings.Contains(result.RestoredRef, "agent-local://fake") {
		t.Fatalf("restored ref: %q", result.RestoredRef)
	}
	if result.EntryCount != 42 {
		t.Fatalf("entry count: %d", result.EntryCount)
	}

	failing := NewFake()
	failing.SetFailure(agent.OperationRestoreWorldSnapshot, "injected restore failure")
	_, err = failing.RestoreWorldSnapshot(ctx, req)
	if err == nil || !strings.Contains(err.Error(), "injected restore failure") {
		t.Fatalf("failure err=%v", err)
	}
}

func TestFakeRestoreWorldSnapshotDefaultWorldDir(t *testing.T) {
	ctx := context.Background()
	fake := NewFake()
	req := agent.WorldCheckpointRestoreRequest{
		SessionID:   "session-1",
		SnapshotRef: "agent-local://local/sessions/session-1/checkpoints/world.zip",
	}
	result, err := fake.RestoreWorldSnapshot(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.RestoredRef, "world_restored") {
		t.Fatalf("restored ref: %q", result.RestoredRef)
	}
}
