package reconcilesvc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/local"
	"github.com/stratummc/stratum/internal/domain/operation"
	"github.com/stratummc/stratum/internal/domain/runtimeobservation"
	"github.com/stratummc/stratum/internal/domain/session"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/repository/memory"
)

func TestMarkStoppedAllowedStates(t *testing.T) {
	for _, source := range []session.State{session.StateRunning, session.StateCrashed, session.StateFrozen, session.StateStarting, session.StateStopping} {
		t.Run(string(source), func(t *testing.T) {
			store := memory.New()
			value := reconciliationSession("session-"+string(source), source)
			if err := store.SaveSession(context.Background(), value); err != nil {
				t.Fatal(err)
			}
			service := New(store)
			result, replay, err := service.MarkStopped(context.Background(), value.ID, "actor-1", "confirmed runtime is stopped", Options{})
			if err != nil || replay || result.Status != operation.StatusSucceeded || result.Action != ActionMarkStopped {
				t.Fatalf("operation=%+v replay=%t err=%v", result, replay, err)
			}
			got, err := store.GetSession(context.Background(), value.ID)
			if err != nil || got.State != session.StateStopped || got.LastRuntimeMessage != "manually reconciled as stopped" || got.AssignedAgentID != value.AssignedAgentID {
				t.Fatalf("session=%+v err=%v", got, err)
			}
			operations, _ := store.ListOperations(context.Background())
			if len(operations) != 1 || operations[0].Metadata["reason"] == "" || operations[0].Metadata["observationAvailable"] != "false" {
				t.Fatalf("operations=%+v", operations)
			}
			events, _ := store.ListAuditEvents(context.Background())
			found := false
			for _, event := range events {
				if event.Action == ActionMarkStopped && event.Metadata["operationId"] == result.ID && event.Metadata["reason"] == "confirmed runtime is stopped" && event.Metadata["result"] == "success" {
					found = true
				}
			}
			if !found {
				t.Fatalf("reconciliation audit missing: %+v", events)
			}
		})
	}
}

func TestStopRuntimeStopsAgentWithoutChangingControllerState(t *testing.T) {
	for _, source := range []session.State{session.StateStopped, session.StateRunning} {
		t.Run(string(source), func(t *testing.T) {
			ctx := context.Background()
			store := memory.New()
			value := reconciliationSession("session-"+string(source), source)
			_ = store.SaveSession(ctx, value)
			client := local.NewFake()
			if _, err := client.StartSession(ctx, agent.SessionRequest{SessionID: value.ID}); err != nil {
				t.Fatal(err)
			}
			result, replay, err := New(store).StopRuntime(ctx, client, value.ID, "actor-1", "stop orphan runtime", StopRuntimeOptions{AgentMode: "http", RequestID: "request-stop-1"})
			if err != nil || replay || result.Status != operation.StatusSucceeded || result.Action != ActionStopRuntime {
				t.Fatalf("operation=%+v replay=%t err=%v", result, replay, err)
			}
			got, err := store.GetSession(ctx, value.ID)
			if err != nil || got.State != source || got.LastRuntimeMessage != value.LastRuntimeMessage {
				t.Fatalf("controller session changed: %+v err=%v", got, err)
			}
			status, err := client.InspectSession(ctx, value.ID)
			if err != nil || status.Running || status.Status != "stopped" {
				t.Fatalf("agent status=%+v err=%v", status, err)
			}
			if result.Metadata["agentRuntimeStatus"] != "running" || result.Metadata["agentResult"] != "success" || result.Metadata["agentMode"] != "http" || result.Metadata["runtimeProfileId"] == "" {
				t.Fatalf("metadata=%+v", result.Metadata)
			}
			events, _ := store.ListAuditEvents(ctx)
			found := false
			for _, event := range events {
				if event.Action == ActionStopRuntime && event.Metadata["operationId"] == result.ID && event.Metadata["reason"] == "stop orphan runtime" && event.Metadata["agentResult"] == "success" {
					found = true
				}
			}
			if !found {
				t.Fatalf("runtime stop audit missing: %+v", events)
			}
		})
	}
}

func TestStopRuntimeFailureDoesNotChangeControllerState(t *testing.T) {
	for _, failure := range []agent.Operation{agent.OperationInspect, agent.OperationStop} {
		t.Run(string(failure), func(t *testing.T) {
			ctx := context.Background()
			store := memory.New()
			value := reconciliationSession("session-"+string(failure), session.StateRunning)
			_ = store.SaveSession(ctx, value)
			client := local.NewFake()
			_, _ = client.StartSession(ctx, agent.SessionRequest{SessionID: value.ID})
			client.SetFailure(failure, "planned failure")
			result, _, err := New(store).StopRuntime(ctx, client, value.ID, "actor-1", "manual runtime stop", StopRuntimeOptions{AgentMode: "http"})
			if err == nil || result.Status != operation.StatusFailed || !strings.Contains(result.ErrorMessage, "planned failure") {
				t.Fatalf("operation=%+v err=%v", result, err)
			}
			got, loadErr := store.GetSession(ctx, value.ID)
			if loadErr != nil || got.State != session.StateRunning {
				t.Fatalf("session=%+v err=%v", got, loadErr)
			}
		})
	}
}

func TestStopRuntimeValidationAndDisallowedState(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	value := reconciliationSession("session-1", session.StateCreated)
	_ = store.SaveSession(ctx, value)
	service := New(store)
	client := local.NewFake()
	for name, call := range map[string]func() error{
		"actor": func() error {
			_, _, err := service.StopRuntime(ctx, client, value.ID, "", "reason", StopRuntimeOptions{})
			return err
		},
		"reason": func() error {
			_, _, err := service.StopRuntime(ctx, client, value.ID, "actor", " ", StopRuntimeOptions{})
			return err
		},
		"agent": func() error {
			_, _, err := service.StopRuntime(ctx, nil, value.ID, "actor", "reason", StopRuntimeOptions{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !stratumerrors.IsKind(err, stratumerrors.KindValidation) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	result, _, err := service.StopRuntime(ctx, client, value.ID, "actor", "reason", StopRuntimeOptions{})
	if !stratumerrors.IsKind(err, stratumerrors.KindConflict) || result.Status != operation.StatusFailed {
		t.Fatalf("operation=%+v err=%v", result, err)
	}
}

func TestMarkStoppedRejectsDisallowedStatesWithoutMutation(t *testing.T) {
	for _, source := range []session.State{session.StateCreated, session.StatePreparing, session.StateStopped, session.StateArchived, session.StateDeleted} {
		t.Run(string(source), func(t *testing.T) {
			store := memory.New()
			value := reconciliationSession("session-"+string(source), source)
			_ = store.SaveSession(context.Background(), value)
			result, _, err := New(store).MarkStopped(context.Background(), value.ID, "actor-1", "manual check", Options{})
			if !stratumerrors.IsKind(err, stratumerrors.KindConflict) || result.Status != operation.StatusFailed {
				t.Fatalf("operation=%+v err=%v", result, err)
			}
			got, loadErr := store.GetSession(context.Background(), value.ID)
			if loadErr != nil || got.State != source {
				t.Fatalf("session=%+v err=%v", got, loadErr)
			}
			operations, _ := store.ListOperations(context.Background())
			if len(operations) != 1 || operations[0].Status != operation.StatusFailed {
				t.Fatalf("operations=%+v", operations)
			}
		})
	}
}

func TestMarkStoppedRequiresActorAndReason(t *testing.T) {
	store := memory.New()
	value := reconciliationSession("session-1", session.StateRunning)
	_ = store.SaveSession(context.Background(), value)
	service := New(store)
	if _, _, err := service.MarkStopped(context.Background(), value.ID, "", "reason", Options{}); !stratumerrors.IsKind(err, stratumerrors.KindValidation) {
		t.Fatalf("actor error=%v", err)
	}
	if _, _, err := service.MarkStopped(context.Background(), value.ID, "actor-1", " ", Options{}); !stratumerrors.IsKind(err, stratumerrors.KindValidation) {
		t.Fatalf("reason error=%v", err)
	}
	operations, _ := store.ListOperations(context.Background())
	if len(operations) != 0 {
		t.Fatalf("invalid requests created operations: %+v", operations)
	}
}

func TestMarkStoppedIncludesObservationMetadata(t *testing.T) {
	store := memory.New()
	value := reconciliationSession("session-1", session.StateRunning)
	_ = store.SaveSession(context.Background(), value)
	observation := runtimeobservation.Observation{
		MismatchDetected:  true,
		MismatchType:      runtimeobservation.MismatchControllerRunningAgentStopped,
		Severity:          runtimeobservation.SeverityWarning,
		RecommendedAction: runtimeobservation.ActionMarkStopped,
	}
	result, _, err := New(store).MarkStopped(context.Background(), value.ID, "actor-1", "agent reports stopped", Options{Observation: &observation, RequestID: "request-1", IdempotencyKey: "idem-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequestID != "request-1" || result.Metadata["observationAvailable"] != "true" || result.Metadata["mismatchType"] != string(observation.MismatchType) || result.Metadata["recommendedAction"] != string(observation.RecommendedAction) {
		t.Fatalf("operation=%+v", result)
	}
}

func reconciliationSession(id string, state session.State) session.Session {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	return session.Session{ID: id, ProjectID: "project-1", RoomID: "room-1", OwnerUserID: "owner-1", Type: session.TypeShared, State: state, EnvironmentID: "env-1", AssignedAgentID: "agent-1", CreatedAt: now, LastActiveAt: now}
}
