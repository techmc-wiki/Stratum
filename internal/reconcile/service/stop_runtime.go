package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/audit"
	observationsvc "github.com/stratummc/stratum/internal/observation/service"
	"github.com/stratummc/stratum/internal/operation"
	operationsvc "github.com/stratummc/stratum/internal/operation/service"
	runtimeobservation "github.com/stratummc/stratum/internal/runtime/observation"
	"github.com/stratummc/stratum/internal/session"
	stratumerrors "github.com/stratummc/stratum/internal/stratumerr"
)

const ActionStopRuntime = "session.reconcile.stop-runtime"

type StopRuntimeOptions struct {
	RequestID                string
	IdempotencyKey           string
	AgentMode                string
	ExpectedRuntimeProfileID string
}

func (s *Service) StopRuntime(ctx context.Context, client agent.AgentClient, sessionID, actor, reason string, options StopRuntimeOptions) (operation.Operation, bool, error) {
	if strings.TrimSpace(actor) == "" {
		return operation.Operation{}, false, stopRuntimeValidationError("actor is required")
	}
	if strings.TrimSpace(reason) == "" {
		return operation.Operation{}, false, stopRuntimeValidationError("reason is required")
	}
	if client == nil {
		return operation.Operation{}, false, stopRuntimeValidationError("agent client is required")
	}
	current, err := s.repository.GetSession(ctx, sessionID)
	if err != nil {
		return operation.Operation{}, false, err
	}
	metadata := map[string]string{
		"reason":                 reason,
		"controllerSessionState": string(current.State),
		"agentMode":              options.AgentMode,
	}
	value, replay, err := s.operations.Begin(ctx, operationsvc.BeginParams{
		RequestID:      options.RequestID,
		IdempotencyKey: options.IdempotencyKey,
		ActorID:        actor,
		Action:         ActionStopRuntime,
		TargetType:     "session",
		TargetID:       sessionID,
		ProjectID:      current.ProjectID,
		SessionID:      sessionID,
		PreviousState:  string(current.State),
		IntendedState:  string(current.State),
		Metadata:       metadata,
	})
	if err != nil || replay {
		return value, replay, err
	}
	if !stopRuntimeAllowedSource(current.State) {
		reconcileErr := stratumerrors.Error{
			Kind:      stratumerrors.KindConflict,
			Operation: "reconcilesvc.StopRuntime",
			Message:   fmt.Sprintf("session %q cannot stop runtime during reconciliation from %q", sessionID, current.State),
		}
		return s.failStopRuntime(ctx, value, current, actor, reason, metadata, reconcileErr)
	}

	status, err := client.InspectSession(ctx, sessionID)
	if err != nil {
		metadata["agentResult"] = "failure"
		metadata["agentError"] = err.Error()
		return s.failStopRuntime(ctx, value, current, actor, reason, metadata, fmt.Errorf("inspect agent runtime: %w", err))
	}
	observation := observationsvc.Observe(current, &status, observationsvc.Options{
		ExpectedRuntimeProfileID: options.ExpectedRuntimeProfileID,
	})
	addRuntimeObservationMetadata(metadata, status, observation)

	result, err := client.StopSession(agent.WithRequestID(ctx, value.RequestID), agent.SessionRequest{
		SessionID: sessionID, ProjectID: current.ProjectID, EnvironmentID: current.EnvironmentID,
	})
	if err != nil {
		metadata["agentResult"] = "failure"
		metadata["agentError"] = err.Error()
		return s.failStopRuntime(ctx, value, current, actor, reason, metadata, err)
	}
	metadata["agentId"] = result.AgentID
	metadata["agentResult"] = result.Status
	metadata["agentMessage"] = result.Message
	if err := s.auditStopRuntime(ctx, value, current, actor, reason, "success", metadata); err != nil {
		return value, false, err
	}
	completed, err := s.operations.Complete(ctx, value, operation.StatusSucceeded, string(current.State), "success", "", "", metadata)
	return completed, false, err
}

func (s *Service) failStopRuntime(ctx context.Context, value operation.Operation, current session.Session, actor, reason string, metadata map[string]string, reconcileErr error) (operation.Operation, bool, error) {
	completed, completeErr := s.operations.Complete(ctx, value, operation.StatusFailed, string(current.State), "failure", "reconciliation_failed", reconcileErr.Error(), metadata)
	auditErr := s.auditStopRuntime(ctx, value, current, actor, reason, "failure", metadata)
	if completeErr != nil {
		return completed, false, fmt.Errorf("%w; complete operation: %v", reconcileErr, completeErr)
	}
	if auditErr != nil {
		return completed, false, fmt.Errorf("%w; append reconciliation audit: %v", reconcileErr, auditErr)
	}
	return completed, false, reconcileErr
}

func (s *Service) auditStopRuntime(ctx context.Context, value operation.Operation, current session.Session, actor, reason, result string, metadata map[string]string) error {
	id, err := s.newID("audit")
	if err != nil {
		return err
	}
	auditMetadata := map[string]string{
		"operationId": value.ID,
		"requestId":   value.RequestID,
		"actor":       actor,
		"reason":      reason,
		"sessionId":   current.ID,
		"result":      result,
	}
	for key, item := range metadata {
		auditMetadata[key] = item
	}
	return s.repository.AppendAuditEvent(ctx, audit.Event{
		ID: id, ProjectID: current.ProjectID, ActorID: actor,
		Action: ActionStopRuntime, TargetType: "session", TargetID: current.ID,
		Metadata: auditMetadata, CreatedAt: s.now(),
	})
}

func addRuntimeObservationMetadata(metadata map[string]string, status agent.SessionStatus, observation runtimeobservation.Observation) {
	metadata["agentRuntimeStatus"] = status.Status
	metadata["runtimeProfileId"] = status.RuntimeProfileID
	metadata["processId"] = status.ProcessID
	metadata["mismatchType"] = string(observation.MismatchType)
	metadata["severity"] = string(observation.Severity)
	metadata["recommendedAction"] = string(observation.RecommendedAction)
	metadata["observationAvailable"] = "true"
}

func stopRuntimeAllowedSource(state session.State) bool {
	switch state {
	case session.StateStopped, session.StateCrashed, session.StateRunning, session.StateFrozen, session.StateStarting, session.StateStopping:
		return true
	default:
		return false
	}
}

func stopRuntimeValidationError(message string) error {
	return stratumerrors.Error{Kind: stratumerrors.KindValidation, Operation: "reconcilesvc.StopRuntime", Message: message}
}
