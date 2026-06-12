package operationsvc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/operation"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/util"
)

type Repository interface {
	CreateOperation(context.Context, operation.Operation) error
	GetOperation(context.Context, string) (operation.Operation, error)
	ListOperations(context.Context) ([]operation.Operation, error)
	UpdateOperation(context.Context, operation.Operation) error
	AppendAuditEvent(context.Context, audit.Event) error
}

type BeginParams struct {
	RequestID, IdempotencyKey, ActorID, Action, TargetType, TargetID string
	ProjectID, SessionID, PreviousState, IntendedState               string
	Metadata                                                         map[string]string
}

type ConflictError struct{ ActiveOperationID string }

func (e ConflictError) Error() string {
	return fmt.Sprintf("session has active operation %q", e.ActiveOperationID)
}

type Service struct {
	repository Repository
	now        func() time.Time
	newID      func(string) (string, error)
}

var coordinationMu sync.Mutex

func New(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }, newID: util.NewID}
}

func (s *Service) Begin(ctx context.Context, params BeginParams) (operation.Operation, bool, error) {
	coordinationMu.Lock()
	defer coordinationMu.Unlock()
	if params.ActorID == "" || params.Action == "" || params.TargetID == "" {
		return operation.Operation{}, false, stratumerrors.Error{Kind: stratumerrors.KindValidation, Operation: "operationsvc.Begin", Message: "actor, action, and target are required"}
	}
	values, err := s.repository.ListOperations(ctx)
	if err != nil {
		return operation.Operation{}, false, err
	}
	for _, value := range values {
		if params.IdempotencyKey != "" && value.IdempotencyKey == params.IdempotencyKey && value.ActorID == params.ActorID && value.Action == params.Action && value.SessionID == params.SessionID {
			return value, true, nil
		}
	}
	for _, value := range values {
		if params.SessionID != "" && value.SessionID == params.SessionID && value.Active() {
			return operation.Operation{}, false, stratumerrors.Error{Kind: stratumerrors.KindConflict, Operation: "operationsvc.Begin", Message: ConflictError{ActiveOperationID: value.ID}.Error(), Cause: ConflictError{ActiveOperationID: value.ID}}
		}
	}
	id, err := s.newID("operation")
	if err != nil {
		return operation.Operation{}, false, err
	}
	if params.RequestID == "" {
		params.RequestID, err = s.newID("request")
		if err != nil {
			return operation.Operation{}, false, err
		}
	}
	now := s.now()
	metadata := map[string]string{}
	for key, item := range params.Metadata {
		metadata[key] = item
	}
	value := operation.Operation{ID: id, RequestID: params.RequestID, IdempotencyKey: params.IdempotencyKey, ActorID: params.ActorID, Action: params.Action, TargetType: params.TargetType, TargetID: params.TargetID, ProjectID: params.ProjectID, SessionID: params.SessionID, Status: operation.StatusPending, CreatedAt: now, PreviousState: params.PreviousState, IntendedState: params.IntendedState, AgentRequestID: params.RequestID, Metadata: metadata}
	if err := s.repository.CreateOperation(ctx, value); err != nil {
		return operation.Operation{}, false, err
	}
	if err := s.audit(ctx, value, "operation.created"); err != nil {
		return operation.Operation{}, false, err
	}
	value.Status, value.StartedAt = operation.StatusRunning, &now
	if err := s.repository.UpdateOperation(ctx, value); err != nil {
		return operation.Operation{}, false, err
	}
	return value, false, s.audit(ctx, value, "operation.started")
}

func (s *Service) Complete(ctx context.Context, value operation.Operation, status operation.Status, finalState, result, code, message string, metadata map[string]string) (operation.Operation, error) {
	now := s.now()
	value.Status, value.CompletedAt, value.FinalState, value.Result = status, &now, finalState, result
	value.ErrorCode, value.ErrorMessage = code, message
	if value.Metadata == nil {
		value.Metadata = map[string]string{}
	}
	for key, item := range metadata {
		value.Metadata[key] = item
	}
	value.AgentID, value.AgentMode = value.Metadata["agentId"], value.Metadata["agentMode"]
	if err := s.repository.UpdateOperation(ctx, value); err != nil {
		return value, err
	}
	action := "operation.completed"
	if status == operation.StatusTimedOut {
		action = "operation.timed_out"
	}
	return value, s.audit(ctx, value, action)
}

func (s *Service) audit(ctx context.Context, value operation.Operation, action string) error {
	id, err := s.newID("audit")
	if err != nil {
		return err
	}
	metadata := map[string]string{"requestId": value.RequestID, "sessionId": value.SessionID, "operationAction": value.Action, "status": string(value.Status)}
	if profileID := value.Metadata["runtimeProfileId"]; profileID != "" {
		metadata["runtimeProfileId"] = profileID
	}
	return s.repository.AppendAuditEvent(ctx, audit.Event{ID: id, ProjectID: value.ProjectID, ActorID: value.ActorID, Action: action, TargetType: "operation", TargetID: value.ID, Metadata: metadata, CreatedAt: s.now()})
}
