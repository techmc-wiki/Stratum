package observationsvc

import (
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/domain/runtimeobservation"
	"github.com/stratummc/stratum/internal/domain/session"
)

func TestObserveClassifiesRuntimeState(t *testing.T) {
	now := time.Date(2026, 6, 12, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		state    session.State
		status   *agent.SessionStatus
		mismatch runtimeobservation.MismatchType
		severity runtimeobservation.Severity
		action   runtimeobservation.RecommendedAction
	}{
		{name: "running running", state: session.StateRunning, status: runtimeStatus("running", true, false, now), mismatch: runtimeobservation.MismatchNone, severity: runtimeobservation.SeverityInfo, action: runtimeobservation.ActionNone},
		{name: "running exited", state: session.StateRunning, status: runtimeStatus("exited", false, false, now), mismatch: runtimeobservation.MismatchControllerRunningAgentStopped, severity: runtimeobservation.SeverityWarning, action: runtimeobservation.ActionMarkStopped},
		{name: "running crashed", state: session.StateRunning, status: runtimeStatus("crashed", false, true, now), mismatch: runtimeobservation.MismatchControllerRunningAgentCrashed, severity: runtimeobservation.SeverityCritical, action: runtimeobservation.ActionMarkCrashed},
		{name: "stopped running", state: session.StateStopped, status: runtimeStatus("running", true, false, now), mismatch: runtimeobservation.MismatchControllerStoppedAgentRunning, severity: runtimeobservation.SeverityCritical, action: runtimeobservation.ActionStopRuntime},
		{name: "missing agent", state: session.StateRunning, status: nil, mismatch: runtimeobservation.MismatchAgentUnknownControllerRunning, severity: runtimeobservation.SeverityCritical, action: runtimeobservation.ActionInspect},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller := session.Session{ID: "session-1", ProjectID: "project-1", RoomID: "room-1", State: test.state, AssignedAgentID: "agent-1"}
			result := Observe(controller, test.status, Options{ObservedAt: now})
			if result.MismatchType != test.mismatch || result.Severity != test.severity || result.RecommendedAction != test.action {
				t.Fatalf("observation=%+v", result)
			}
			if result.MismatchDetected != (test.mismatch != runtimeobservation.MismatchNone) {
				t.Fatalf("mismatchDetected=%t type=%s", result.MismatchDetected, result.MismatchType)
			}
		})
	}
}

func TestObserveClassifiesAgentAndProfileMismatch(t *testing.T) {
	now := time.Date(2026, 6, 12, 8, 0, 0, 0, time.UTC)
	controller := session.Session{ID: "session-1", State: session.StateRunning, AssignedAgentID: "agent-expected"}
	status := runtimeStatus("running", true, false, now)
	status.AgentID = "agent-other"
	result := Observe(controller, status, Options{ObservedAt: now})
	if result.MismatchType != runtimeobservation.MismatchAssignedAgent || result.Severity != runtimeobservation.SeverityCritical || result.RecommendedAction != runtimeobservation.ActionManualReview {
		t.Fatalf("assigned agent observation=%+v", result)
	}

	status.AgentID = controller.AssignedAgentID
	result = Observe(controller, status, Options{ObservedAt: now, ExpectedRuntimeProfileID: "expected-profile"})
	if result.MismatchType != runtimeobservation.MismatchRuntimeProfile || result.Severity != runtimeobservation.SeverityWarning || result.RecommendedAction != runtimeobservation.ActionManualReview {
		t.Fatalf("profile observation=%+v", result)
	}
}

func TestObserveClassifiesUnassignedAndStaleRuntime(t *testing.T) {
	now := time.Date(2026, 6, 12, 8, 0, 0, 0, time.UTC)
	status := runtimeStatus("running", true, false, now)
	controller := session.Session{ID: "session-1", State: session.StateCreated}
	result := Observe(controller, status, Options{ObservedAt: now})
	if result.MismatchType != runtimeobservation.MismatchControllerUnknownAgentKnown {
		t.Fatalf("unassigned observation=%+v", result)
	}

	controller.State = session.StateRunning
	controller.AssignedAgentID = "agent-1"
	status.ObservedAt = now.Add(-10 * time.Minute)
	result = Observe(controller, status, Options{ObservedAt: now, StaleAfter: time.Minute})
	if result.MismatchType != runtimeobservation.MismatchStaleObservation || result.RecommendedAction != runtimeobservation.ActionInspect {
		t.Fatalf("stale observation=%+v", result)
	}
}

func runtimeStatus(status string, running, crashed bool, observedAt time.Time) *agent.SessionStatus {
	exitCode := 0
	value := &agent.SessionStatus{AgentID: "agent-1", SessionID: "session-1", Status: status, Running: running, Crashed: crashed, RuntimeProfileID: "actual-profile", ProcessID: "process-1", PID: 42, ObservedAt: observedAt}
	if status == "exited" || status == "crashed" {
		value.ExitCode = &exitCode
	}
	return value
}
