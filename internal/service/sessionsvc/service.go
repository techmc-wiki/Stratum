package sessionsvc

import (
	"context"
	"fmt"
	"time"

	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/session"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/service/schedulersvc"
	"github.com/stratummc/stratum/internal/util"
)

type Repository interface {
	SaveSession(context.Context, session.Session) error
	GetSession(context.Context, string) (session.Session, error)
	ListSessions(context.Context) ([]session.Session, error)
	DeleteSession(context.Context, string) error
	AppendAuditEvent(context.Context, audit.Event) error
}

type Clock func() time.Time
type IDGenerator func(string) (string, error)

type Service struct {
	repository Repository
	scheduler  schedulersvc.Service
	now        Clock
	newID      IDGenerator
}

type DeniedError struct {
	Reason  resourcepolicy.DenialReason
	Message string
}

func (e DeniedError) Error() string {
	return fmt.Sprintf("session start denied (%s): %s", e.Reason, e.Message)
}

func New(repository Repository, policy resourcepolicy.Policy) *Service {
	return &Service{
		repository: repository,
		scheduler:  schedulersvc.Service{Policy: policy},
		now:        func() time.Time { return time.Now().UTC() },
		newID:      util.NewID,
	}
}

func (s *Service) Create(ctx context.Context, value session.Session) error {
	if value.ID == "" || value.ProjectID == "" || value.OwnerUserID == "" || value.EnvironmentID == "" {
		return fmt.Errorf("session requires id, project, owner, and environment")
	}
	if value.State == "" {
		value.State = session.StateCreated
	}
	return s.repository.SaveSession(ctx, value)
}

func (s *Service) List(ctx context.Context) ([]session.Session, error) {
	return s.repository.ListSessions(ctx)
}

func (s *Service) Prepare(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	return s.transition(ctx, "prepare", id, actor, "", []session.State{session.StatePreparing})
}

func (s *Service) Start(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	value, err := s.repository.GetSession(ctx, id)
	if err != nil {
		return s.auditLoadFailure(ctx, "start", id, actor, session.StateRunning, err)
	}
	previous := value.State
	path, ok := startPath(previous)
	if !ok {
		return s.fail(ctx, "start", value, actor, session.StateRunning, fmt.Errorf("session %q cannot start from %q", id, previous))
	}
	usage, err := s.currentUsage(ctx, value)
	if err != nil {
		return s.fail(ctx, "start", value, actor, session.StateRunning, err)
	}
	decision := s.scheduler.Decide(usage, value.Type)
	if !decision.Allowed {
		denied := DeniedError{Reason: decision.Reason, Message: decision.Message}
		return s.fail(ctx, "start", value, actor, session.StateRunning, denied)
	}
	if err := applyPath(&value, path); err != nil {
		return s.fail(ctx, "start", value, actor, session.StateRunning, err)
	}
	value.LastActiveAt = s.now()
	return s.persistSuccess(ctx, "start", previous, value, actor, "")
}

func (s *Service) Stop(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	value, err := s.repository.GetSession(ctx, id)
	if err != nil {
		return s.auditLoadFailure(ctx, "stop", id, actor, session.StateStopped, err)
	}
	path := []session.State{session.StateStopping, session.StateStopped}
	if value.State == session.StateCrashed {
		path = []session.State{session.StateStopped}
	}
	previous := value.State
	if err := applyPath(&value, path); err != nil {
		return s.fail(ctx, "stop", value, actor, session.StateStopped, err)
	}
	value.LastActiveAt = s.now()
	return s.persistSuccess(ctx, "stop", previous, value, actor, "")
}

func (s *Service) Restart(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	// TODO: create a pre-restart checkpoint before runtime integration is enabled.
	value, err := s.repository.GetSession(ctx, id)
	if err != nil {
		return s.auditLoadFailure(ctx, "restart", id, actor, session.StateRunning, err)
	}
	previous := value.State
	var path []session.State
	switch value.State {
	case session.StateRunning:
		path = []session.State{session.StateStopping, session.StateStopped, session.StateStarting, session.StateRunning}
	case session.StateStopped:
		path = []session.State{session.StateStarting, session.StateRunning}
	default:
		return s.fail(ctx, "restart", value, actor, session.StateRunning, fmt.Errorf("session %q cannot restart from %q", id, value.State))
	}
	usage, err := s.currentUsage(ctx, value)
	if err != nil {
		return s.fail(ctx, "restart", value, actor, session.StateRunning, err)
	}
	if previous == session.StateRunning {
		usage.GlobalRunning--
		usage.ProjectRunning--
		if value.Type == session.TypePrivate || value.Type == session.TypeFork {
			usage.UserRunning--
		}
		if value.Type == session.TypeReview {
			usage.ReviewRunning--
		}
	}
	decision := s.scheduler.Decide(usage, value.Type)
	if !decision.Allowed {
		return s.fail(ctx, "restart", value, actor, session.StateRunning, DeniedError{Reason: decision.Reason, Message: decision.Message})
	}
	if err := applyPath(&value, path); err != nil {
		return s.fail(ctx, "restart", value, actor, session.StateRunning, err)
	}
	value.LastActiveAt = s.now()
	return s.persistSuccess(ctx, "restart", previous, value, actor, "")
}

func (s *Service) Freeze(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	return s.transition(ctx, "freeze", id, actor, "", []session.State{session.StateFrozen})
}

func (s *Service) Unfreeze(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	return s.transition(ctx, "unfreeze", id, actor, "", []session.State{session.StateRunning})
}

func (s *Service) MarkCrashed(ctx context.Context, id, actor, reason string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	// TODO: request a crash snapshot after agent/runtime integration is enabled.
	return s.transition(ctx, "mark-crashed", id, actor, reason, []session.State{session.StateCrashed})
}

func (s *Service) Archive(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	// TODO: create a pre-archive checkpoint before moving runtime data.
	return s.transition(ctx, "archive", id, actor, "", []session.State{session.StateArchived})
}

func (s *Service) Delete(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	// TODO: create a final metadata/checkpoint record before destructive cleanup.
	value, err := s.repository.GetSession(ctx, id)
	if err != nil {
		return s.auditLoadFailure(ctx, "delete", id, actor, session.StateDeleted, err)
	}
	previous := value.State
	if err := value.Transition(session.StateDeleted); err != nil {
		return s.fail(ctx, "delete", value, actor, session.StateDeleted, err)
	}
	if err := s.repository.DeleteSession(ctx, id); err != nil {
		return s.fail(ctx, "delete", session.Session{ID: id, ProjectID: value.ProjectID, State: previous}, actor, session.StateDeleted, err)
	}
	return s.audit(ctx, "delete", value.ProjectID, id, actor, previous, session.StateDeleted, "success", "")
}

func (s *Service) transition(ctx context.Context, action, id, actor, detail string, path []session.State) error {
	value, err := s.repository.GetSession(ctx, id)
	if err != nil {
		return s.auditLoadFailure(ctx, action, id, actor, finalState(path), err)
	}
	previous := value.State
	if err := applyPath(&value, path); err != nil {
		return s.fail(ctx, action, value, actor, finalState(path), err)
	}
	value.LastActiveAt = s.now()
	return s.persistSuccess(ctx, action, previous, value, actor, detail)
}

func (s *Service) persistSuccess(ctx context.Context, action string, previous session.State, value session.Session, actor, detail string) error {
	if err := s.repository.SaveSession(ctx, value); err != nil {
		return s.fail(ctx, action, session.Session{ID: value.ID, ProjectID: value.ProjectID, State: previous}, actor, value.State, err)
	}
	return s.audit(ctx, action, value.ProjectID, value.ID, actor, previous, value.State, "success", detail)
}

func (s *Service) fail(ctx context.Context, action string, value session.Session, actor string, next session.State, operationErr error) error {
	auditErr := s.audit(ctx, action, value.ProjectID, value.ID, actor, value.State, next, "failure", operationErr.Error())
	if auditErr != nil {
		return fmt.Errorf("%w; append failure audit: %v", operationErr, auditErr)
	}
	return operationErr
}

func (s *Service) auditLoadFailure(ctx context.Context, action, id, actor string, next session.State, operationErr error) error {
	auditErr := s.audit(ctx, action, "", id, actor, "", next, "failure", operationErr.Error())
	if auditErr != nil {
		return fmt.Errorf("%w; append failure audit: %v", operationErr, auditErr)
	}
	return operationErr
}

func (s *Service) audit(ctx context.Context, action, projectID, id, actor string, previous, next session.State, result, reason string) error {
	if actor == "" {
		return stratumerrors.Error{Kind: stratumerrors.KindValidation, Operation: "sessionsvc.audit", Message: "actor is required"}
	}
	eventID, err := s.newID("audit")
	if err != nil {
		return err
	}
	metadata := map[string]string{
		"previousState": string(previous),
		"nextState":     string(next),
		"result":        result,
	}
	if reason != "" {
		metadata["reason"] = reason
	}
	return s.repository.AppendAuditEvent(ctx, audit.Event{
		ID: eventID, ProjectID: projectID, ActorID: actor, Action: "session." + action,
		TargetType: "session", TargetID: id, Metadata: metadata, CreatedAt: s.now(),
	})
}

func (s *Service) currentUsage(ctx context.Context, target session.Session) (resourcepolicy.Usage, error) {
	values, err := s.repository.ListSessions(ctx)
	if err != nil {
		return resourcepolicy.Usage{}, err
	}
	var usage resourcepolicy.Usage
	for _, value := range values {
		if value.State != session.StateRunning {
			continue
		}
		usage.GlobalRunning++
		if value.ProjectID == target.ProjectID {
			usage.ProjectRunning++
		}
		if value.OwnerUserID == target.OwnerUserID && (value.Type == session.TypePrivate || value.Type == session.TypeFork) {
			usage.UserRunning++
		}
		if value.Type == session.TypeReview {
			usage.ReviewRunning++
		}
	}
	return usage, nil
}

func startPath(state session.State) ([]session.State, bool) {
	switch state {
	case session.StateCreated:
		return []session.State{session.StatePreparing, session.StateStarting, session.StateRunning}, true
	case session.StatePreparing:
		return []session.State{session.StateStarting, session.StateRunning}, true
	case session.StateStopped:
		return []session.State{session.StateStarting, session.StateRunning}, true
	default:
		return nil, false
	}
}

func applyPath(value *session.Session, path []session.State) error {
	for _, next := range path {
		if err := value.Transition(next); err != nil {
			return err
		}
	}
	return nil
}

func finalState(path []session.State) session.State {
	if len(path) == 0 {
		return ""
	}
	return path[len(path)-1]
}

func validateActor(actor string) error {
	if actor == "" {
		return stratumerrors.Error{Kind: stratumerrors.KindValidation, Operation: "sessionsvc.lifecycle", Message: "actor is required"}
	}
	return nil
}
