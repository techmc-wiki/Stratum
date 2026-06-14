package sessionsvc

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/domain/operation"
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/session"
	"github.com/stratummc/stratum/internal/repository/filesystem"
)

type orderedReadinessAgent struct {
	agent.AgentClient
	readiness    agent.SessionStartReadiness
	readinessErr error
	stopErr      error
	calls        []string
}

func (a *orderedReadinessAgent) MaterializeEnvironment(ctx context.Context, request agent.EnvironmentMaterializationRequest) (agent.EnvironmentMaterializationResult, error) {
	a.calls = append(a.calls, "materialize")
	return a.AgentClient.MaterializeEnvironment(ctx, request)
}

func (a *orderedReadinessAgent) SessionReadyForStart(context.Context, string) (agent.SessionStartReadiness, error) {
	a.calls = append(a.calls, "readiness")
	return a.readiness, a.readinessErr
}

func (a *orderedReadinessAgent) StartSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	a.calls = append(a.calls, "start")
	return a.AgentClient.StartSession(ctx, request)
}

func (a *orderedReadinessAgent) StopSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	a.calls = append(a.calls, "stop")
	if a.stopErr != nil {
		return agent.OperationResult{}, a.stopErr
	}
	return a.AgentClient.StopSession(ctx, request)
}

func TestStartChecksRuntimeReadinessBeforeAgentLaunch(t *testing.T) {
	agentClient := newOrderedReadinessAgent(readyRuntime())
	ctx, store, service := runtimeReadinessLifecycle(t, agentClient, session.StateCreated)

	value, _, err := service.StartWithOptions(ctx, "session-1", "actor-1", OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(agentClient.calls, []string{"materialize", "readiness", "start"}) {
		t.Fatalf("calls = %v", agentClient.calls)
	}
	if value.Status != operation.StatusSucceeded || value.Metadata["runtimeReadinessStatus"] != "ready" || value.Metadata["runtimeReadinessReady"] != "true" {
		t.Fatalf("operation = %+v", value)
	}
	stored, _ := store.GetSession(ctx, "session-1")
	if stored.State != session.StateRunning {
		t.Fatalf("state = %s", stored.State)
	}
}

func TestStartRuntimeReadinessFailureBlocksAgentLaunch(t *testing.T) {
	for _, test := range []struct {
		name      string
		readiness agent.SessionStartReadiness
		err       error
		status    string
		issues    string
	}{
		{name: "not ready", readiness: notReadyRuntime(), status: "not_ready", issues: "environment_manifest_missing"},
		{name: "agent unreachable", err: errors.New("agent unavailable"), status: "error", issues: "agent unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			agentClient := newOrderedReadinessAgent(test.readiness)
			agentClient.readinessErr = test.err
			ctx, store, service := runtimeReadinessLifecycle(t, agentClient, session.StateCreated)

			value, _, err := service.StartWithOptions(ctx, "session-1", "actor-1", OperationOptions{})
			if err == nil || value.Status != operation.StatusFailed {
				t.Fatalf("operation=%+v err=%v", value, err)
			}
			if !reflect.DeepEqual(agentClient.calls, []string{"materialize", "readiness"}) {
				t.Fatalf("calls = %v", agentClient.calls)
			}
			if value.Metadata["runtimeReadinessStatus"] != test.status || value.Metadata["runtimeReadinessReady"] != "false" || value.Metadata["runtimeReadinessIssues"] != test.issues {
				t.Fatalf("metadata = %+v", value.Metadata)
			}
			stored, _ := store.GetSession(ctx, "session-1")
			if stored.State != session.StateCreated {
				t.Fatalf("state = %s", stored.State)
			}
		})
	}
}

func TestRestartStopsBeforeRuntimeReadinessAndStartsSafely(t *testing.T) {
	for _, test := range []struct {
		name         string
		sourceState  session.State
		readiness    agent.SessionStartReadiness
		readinessErr error
		wantState    session.State
		wantCalls    []string
		wantStop     string
		wantStart    string
		wantIssues   string
		wantError    bool
	}{
		{name: "running ready", sourceState: session.StateRunning, readiness: readyRuntime(), wantState: session.StateRunning, wantCalls: []string{"stop", "materialize", "readiness", "start"}, wantStop: "succeeded", wantStart: "true"},
		{name: "running not ready", sourceState: session.StateRunning, readiness: notReadyRuntime(), wantState: session.StateStopped, wantCalls: []string{"stop", "materialize", "readiness"}, wantStop: "succeeded", wantStart: "false", wantIssues: "environment_manifest_missing", wantError: true},
		{name: "running readiness unavailable", sourceState: session.StateRunning, readinessErr: errors.New("agent unavailable"), wantState: session.StateStopped, wantCalls: []string{"stop", "materialize", "readiness"}, wantStop: "succeeded", wantStart: "false", wantIssues: "agent unavailable", wantError: true},
		{name: "stopped ready", sourceState: session.StateStopped, readiness: readyRuntime(), wantState: session.StateRunning, wantCalls: []string{"materialize", "readiness", "start"}, wantStop: "not_required", wantStart: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			agentClient := newOrderedReadinessAgent(test.readiness)
			agentClient.readinessErr = test.readinessErr
			ctx, store, service := runtimeReadinessLifecycle(t, agentClient, test.sourceState)

			value, _, err := service.RestartWithOptions(ctx, "session-1", "actor-1", OperationOptions{})
			if (err != nil) != test.wantError {
				t.Fatalf("operation=%+v err=%v", value, err)
			}
			if !reflect.DeepEqual(agentClient.calls, test.wantCalls) {
				t.Fatalf("calls = %v", agentClient.calls)
			}
			stored, _ := store.GetSession(ctx, "session-1")
			if stored.State != test.wantState {
				t.Fatalf("state = %s, want %s", stored.State, test.wantState)
			}
			if value.Metadata["restartStopStatus"] != test.wantStop {
				t.Fatalf("restartStopStatus = %q", value.Metadata["restartStopStatus"])
			}
			if value.Metadata["restartStartAttempted"] != test.wantStart {
				t.Fatalf("restartStartAttempted = %q, want %q", value.Metadata["restartStartAttempted"], test.wantStart)
			}
			events, auditErr := store.ListAuditEvents(ctx)
			if auditErr != nil {
				t.Fatal(auditErr)
			}
			foundSequenceAudit := false
			for _, event := range events {
				if event.Action == "operation.completed" && event.Metadata["restartStopStatus"] == test.wantStop && event.Metadata["restartStartAttempted"] == test.wantStart {
					foundSequenceAudit = true
					break
				}
			}
			if !foundSequenceAudit {
				t.Fatalf("restart sequence audit metadata missing: %+v", events)
			}
			if test.wantError && (value.Status != operation.StatusFailed || value.Metadata["runtimeReadinessIssues"] != test.wantIssues) {
				t.Fatalf("operation = %+v", value)
			}
		})
	}
}

func TestRestartStopFailureDoesNotCheckReadinessOrStart(t *testing.T) {
	agentClient := newOrderedReadinessAgent(readyRuntime())
	agentClient.stopErr = errors.New("stop unavailable")
	ctx, store, service := runtimeReadinessLifecycle(t, agentClient, session.StateRunning)

	value, _, err := service.RestartWithOptions(ctx, "session-1", "actor-1", OperationOptions{})
	if err == nil || value.Status != operation.StatusFailed {
		t.Fatalf("operation=%+v err=%v", value, err)
	}
	if !reflect.DeepEqual(agentClient.calls, []string{"stop"}) {
		t.Fatalf("calls = %v", agentClient.calls)
	}
	if value.Metadata["restartStopStatus"] != "failed" || value.Metadata["restartStartAttempted"] != "false" {
		t.Fatalf("metadata = %+v", value.Metadata)
	}
	stored, _ := store.GetSession(ctx, "session-1")
	if stored.State != session.StateRunning {
		t.Fatalf("state = %s", stored.State)
	}
}

func TestStartReadinessRejectsAlreadyRunningAgentProcess(t *testing.T) {
	readiness := notReadyRuntime()
	readiness.Issues = []agent.SessionStartReadinessIssue{{Code: "process_running", Message: "process is running", Severity: "error"}}
	readiness.RuntimeStatusSummary.ProcessState = "running"
	agentClient := newOrderedReadinessAgent(readiness)
	ctx, store, service := runtimeReadinessLifecycle(t, agentClient, session.StateCreated)

	value, _, err := service.StartWithOptions(ctx, "session-1", "actor-1", OperationOptions{})
	if err == nil || value.Metadata["runtimeReadinessIssues"] != "process_running" {
		t.Fatalf("operation=%+v err=%v", value, err)
	}
	if !reflect.DeepEqual(agentClient.calls, []string{"materialize", "readiness"}) {
		t.Fatalf("calls = %v", agentClient.calls)
	}
	stored, _ := store.GetSession(ctx, "session-1")
	if stored.State != session.StateCreated {
		t.Fatalf("state = %s", stored.State)
	}
}

func newOrderedReadinessAgent(readiness agent.SessionStartReadiness) *orderedReadinessAgent {
	return &orderedReadinessAgent{AgentClient: local.NewFake(), readiness: readiness}
}

func runtimeReadinessLifecycle(t *testing.T, agentClient agent.AgentClient, state session.State) (context.Context, *filesystem.Store, *Service) {
	t.Helper()
	ctx, store, _, _ := newLifecycleTest(t, resourcepolicy.MVPDefault())
	createTestSession(t, store, testSession("session-1", session.TypeShared, state))
	service := New(store, resourcepolicy.MVPDefault(), agentClient)
	service.now = func() time.Time { return lifecycleTime }
	service.newID = func(prefix string) (string, error) { return prefix + "-readiness", nil }
	return ctx, store, service
}

func readyRuntime() agent.SessionStartReadiness {
	return agent.SessionStartReadiness{SessionID: "session-1", Ready: true, Status: "ready", Issues: []agent.SessionStartReadinessIssue{}, RuntimeStatusSummary: agent.SessionStartReadinessSummary{EnvironmentManifestExists: true, EnvironmentManifestStatus: "prepared", ProcessState: "not_started", AppliedArtifactsTotal: 1, AppliedArtifactsValid: 1}}
}

func notReadyRuntime() agent.SessionStartReadiness {
	return agent.SessionStartReadiness{SessionID: "session-1", Status: "not_ready", Issues: []agent.SessionStartReadinessIssue{{Code: "environment_manifest_missing", Message: "manifest missing", Severity: "error"}}, RuntimeStatusSummary: agent.SessionStartReadinessSummary{ProcessState: "not_started"}}
}
