package reconcilesvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/operation"
	"github.com/stratummc/stratum/internal/domain/runtimeobservation"
	"github.com/stratummc/stratum/internal/domain/session"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/service/operationsvc"
	"github.com/stratummc/stratum/internal/util"
)

const ActionMarkStopped = "session.reconcile.mark-stopped"

type Repository interface {
	SaveSession(context.Context, session.Session) error
	GetSession(context.Context, string) (session.Session, error)
	AppendAuditEvent(context.Context, audit.Event) error
	CreateOperation(context.Context, operation.Operation) error
	GetOperation(context.Context, string) (operation.Operation, error)
	ListOperations(context.Context) ([]operation.Operation, error)
	UpdateOperation(context.Context, operation.Operation) error
}

type Options struct {
	RequestID      string
	IdempotencyKey string
	Observation    *runtimeobservation.Observation
	InspectError   string
}

type Service struct {
	repository Repository
	operations *operationsvc.Service
	now        func() time.Time
	newID      func(string) (string, error)
}

func New(repository Repository) *Service {
	return &Service{
		repository: repository,
		operations: operationsvc.New(repository),
		now:        func() time.Time { return time.Now().UTC() },
		newID:      util.NewID,
	}
}

func (s *Service) MarkStopped(ctx context.Context, sessionID, actor, reason string, options Options) (operation.Operation, bool, error) {
	if strings.TrimSpace(actor) == "" {
		return operation.Operation{}, false, validationError("actor is required")
	}
	if strings.TrimSpace(reason) == "" {
		return operation.Operation{}, false, validationError("reason is required")
	}
	current, err := s.repository.GetSession(ctx, sessionID)
	if err != nil {
		return operation.Operation{}, false, err
	}
	metadata := reconciliationMetadata(current.State, reason, options)
	value, replay, err := s.operations.Begin(ctx, operationsvc.BeginParams{
		RequestID:      options.RequestID,
		IdempotencyKey: options.IdempotencyKey,
		ActorID:        actor,
		Action:         ActionMarkStopped,
		TargetType:     "session",
		TargetID:       sessionID,
		ProjectID:      current.ProjectID,
		SessionID:      sessionID,
		PreviousState:  string(current.State),
		IntendedState:  string(session.StateStopped),
		Metadata:       metadata,
	})
	if err != nil || replay {
		return value, replay, err
	}

	if !allowedSource(current.State) {
		reconcileErr := stratumerrors.Error{
			Kind:      stratumerrors.KindConflict,
			Operation: "reconcilesvc.MarkStopped",
			Message:   fmt.Sprintf("session %q cannot be manually reconciled as stopped from %q", sessionID, current.State),
		}
		return s.fail(ctx, value, current, actor, reason, metadata, reconcileErr)
	}

	updated := current
	updated.State = session.StateStopped
	updated.LastRuntimeMessage = "manually reconciled as stopped"
	updated.LastActiveAt = s.now()
	if err := s.repository.SaveSession(ctx, updated); err != nil {
		return s.fail(ctx, value, current, actor, reason, metadata, err)
	}
	if err := s.audit(ctx, value, updated, actor, reason, "success", metadata); err != nil {
		return value, false, err
	}
	completed, err := s.operations.Complete(ctx, value, operation.StatusSucceeded, string(session.StateStopped), "success", "", "", metadata)
	return completed, false, err
}

func (s *Service) fail(ctx context.Context, value operation.Operation, current session.Session, actor, reason string, metadata map[string]string, reconcileErr error) (operation.Operation, bool, error) {
	metadata["failure"] = reconcileErr.Error()
	completed, completeErr := s.operations.Complete(ctx, value, operation.StatusFailed, string(current.State), "failure", "reconciliation_failed", reconcileErr.Error(), metadata)
	auditErr := s.audit(ctx, value, current, actor, reason, "failure", metadata)
	if completeErr != nil {
		return completed, false, fmt.Errorf("%w; complete operation: %v", reconcileErr, completeErr)
	}
	if auditErr != nil {
		return completed, false, fmt.Errorf("%w; append reconciliation audit: %v", reconcileErr, auditErr)
	}
	return completed, false, reconcileErr
}

func (s *Service) audit(ctx context.Context, value operation.Operation, current session.Session, actor, reason, result string, metadata map[string]string) error {
	id, err := s.newID("audit")
	if err != nil {
		return err
	}
	auditMetadata := map[string]string{
		"operationId":   value.ID,
		"requestId":     value.RequestID,
		"actor":         actor,
		"reason":        reason,
		"previousState": value.PreviousState,
		"nextState":     string(session.StateStopped),
		"result":        result,
	}
	for key, item := range metadata {
		auditMetadata[key] = item
	}
	return s.repository.AppendAuditEvent(ctx, audit.Event{
		ID: id, ProjectID: current.ProjectID, ActorID: actor,
		Action: ActionMarkStopped, TargetType: "session", TargetID: current.ID,
		Metadata: auditMetadata, CreatedAt: s.now(),
	})
}

func reconciliationMetadata(previous session.State, reason string, options Options) map[string]string {
	metadata := map[string]string{
		"reason":               reason,
		"previousState":        string(previous),
		"nextState":            string(session.StateStopped),
		"observationAvailable": "false",
	}
	if options.InspectError != "" {
		metadata["observationError"] = options.InspectError
	}
	if options.Observation != nil {
		metadata["observationAvailable"] = "true"
		metadata["mismatchType"] = string(options.Observation.MismatchType)
		metadata["severity"] = string(options.Observation.Severity)
		metadata["recommendedAction"] = string(options.Observation.RecommendedAction)
		if !options.Observation.MismatchDetected {
			metadata["observationWarning"] = "no mismatch detected"
		}
	}
	return metadata
}

func allowedSource(state session.State) bool {
	switch state {
	case session.StateRunning, session.StateCrashed, session.StateFrozen, session.StateStarting, session.StateStopping:
		return true
	default:
		return false
	}
}

func validationError(message string) error {
	return stratumerrors.Error{Kind: stratumerrors.KindValidation, Operation: "reconcilesvc.MarkStopped", Message: message}
}
