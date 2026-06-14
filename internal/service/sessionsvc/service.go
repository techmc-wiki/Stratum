package sessionsvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/environment"
	"github.com/stratummc/stratum/internal/domain/operation"
	"github.com/stratummc/stratum/internal/domain/resourcepolicy"
	"github.com/stratummc/stratum/internal/domain/session"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/service/operationsvc"
	"github.com/stratummc/stratum/internal/service/schedulersvc"
	"github.com/stratummc/stratum/internal/util"
)

type Repository interface {
	SaveSession(context.Context, session.Session) error
	GetSession(context.Context, string) (session.Session, error)
	ListSessions(context.Context) ([]session.Session, error)
	DeleteSession(context.Context, string) error
	GetEnvironment(context.Context, string) (environment.Environment, error)
	AppendAuditEvent(context.Context, audit.Event) error
	CreateOperation(context.Context, operation.Operation) error
	GetOperation(context.Context, string) (operation.Operation, error)
	ListOperations(context.Context) ([]operation.Operation, error)
	UpdateOperation(context.Context, operation.Operation) error
}

type Clock func() time.Time
type IDGenerator func(string) (string, error)

type Service struct {
	repository   Repository
	scheduler    schedulersvc.Service
	now          Clock
	newID        IDGenerator
	agent        agent.AgentClient
	operations   *operationsvc.Service
	artifactGate ArtifactReadinessGate
}

type ArtifactReadinessGate interface {
	Check(context.Context, string) (map[string]string, error)
}

type OperationOptions struct {
	IdempotencyKey   string
	RequestID        string
	Timeout          time.Duration
	RuntimeProfileID string
}

type operationContext struct {
	ID, RequestID, RuntimeProfileID string
	Metadata                        map[string]string
}
type operationContextKey struct{}

type DeniedError struct {
	Reason  resourcepolicy.DenialReason
	Message string
}

func (e DeniedError) Error() string {
	return fmt.Sprintf("session start denied (%s): %s", e.Reason, e.Message)
}

func New(repository Repository, policy resourcepolicy.Policy, clients ...agent.AgentClient) *Service {
	service := &Service{
		repository: repository,
		scheduler:  schedulersvc.Service{Policy: policy},
		now:        func() time.Time { return time.Now().UTC() },
		newID:      util.NewID,
		operations: operationsvc.New(repository),
	}
	if len(clients) > 0 {
		service.agent = clients[0]
	}
	return service
}

func (s *Service) WithArtifactReadinessGate(gate ArtifactReadinessGate) *Service {
	s.artifactGate = gate
	return s
}

func (s *Service) Create(ctx context.Context, value session.Session) error {
	if value.ID == "" || value.ProjectID == "" || value.OwnerUserID == "" || value.EnvironmentID == "" {
		return fmt.Errorf("session requires id, project, owner, and environment")
	}
	if _, err := s.repository.GetEnvironment(ctx, value.EnvironmentID); err != nil {
		return fmt.Errorf("environment %q not found: %w", value.EnvironmentID, err)
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
	_, _, err := s.PrepareWithOptions(ctx, id, actor, OperationOptions{})
	return err
}
func (s *Service) Start(ctx context.Context, id, actor string) error {
	_, _, err := s.StartWithOptions(ctx, id, actor, OperationOptions{})
	return err
}
func (s *Service) Stop(ctx context.Context, id, actor string) error {
	_, _, err := s.StopWithOptions(ctx, id, actor, OperationOptions{})
	return err
}
func (s *Service) Restart(ctx context.Context, id, actor string) error {
	_, _, err := s.RestartWithOptions(ctx, id, actor, OperationOptions{})
	return err
}
func (s *Service) Freeze(ctx context.Context, id, actor string) error {
	_, _, err := s.FreezeWithOptions(ctx, id, actor, OperationOptions{})
	return err
}
func (s *Service) Unfreeze(ctx context.Context, id, actor string) error {
	_, _, err := s.UnfreezeWithOptions(ctx, id, actor, OperationOptions{})
	return err
}
func (s *Service) MarkCrashed(ctx context.Context, id, actor, reason string) error {
	_, _, err := s.MarkCrashedWithOptions(ctx, id, actor, reason, OperationOptions{})
	return err
}
func (s *Service) Archive(ctx context.Context, id, actor string) error {
	_, _, err := s.ArchiveWithOptions(ctx, id, actor, OperationOptions{})
	return err
}
func (s *Service) Delete(ctx context.Context, id, actor string) error {
	_, _, err := s.DeleteWithOptions(ctx, id, actor, OperationOptions{})
	return err
}

func (s *Service) PrepareWithOptions(ctx context.Context, id, actor string, options OperationOptions) (operation.Operation, bool, error) {
	return s.coordinate(ctx, "prepare", id, actor, session.StatePreparing, options, func(callCtx context.Context) error { return s.prepare(callCtx, id, actor) })
}
func (s *Service) StartWithOptions(ctx context.Context, id, actor string, options OperationOptions) (operation.Operation, bool, error) {
	return s.coordinate(ctx, "start", id, actor, session.StateRunning, options, func(callCtx context.Context) error { return s.start(callCtx, id, actor) })
}
func (s *Service) StopWithOptions(ctx context.Context, id, actor string, options OperationOptions) (operation.Operation, bool, error) {
	return s.coordinate(ctx, "stop", id, actor, session.StateStopped, options, func(callCtx context.Context) error { return s.stop(callCtx, id, actor) })
}
func (s *Service) RestartWithOptions(ctx context.Context, id, actor string, options OperationOptions) (operation.Operation, bool, error) {
	return s.coordinate(ctx, "restart", id, actor, session.StateRunning, options, func(callCtx context.Context) error { return s.restart(callCtx, id, actor) })
}
func (s *Service) FreezeWithOptions(ctx context.Context, id, actor string, options OperationOptions) (operation.Operation, bool, error) {
	return s.coordinate(ctx, "freeze", id, actor, session.StateFrozen, options, func(callCtx context.Context) error { return s.freeze(callCtx, id, actor) })
}
func (s *Service) UnfreezeWithOptions(ctx context.Context, id, actor string, options OperationOptions) (operation.Operation, bool, error) {
	return s.coordinate(ctx, "unfreeze", id, actor, session.StateRunning, options, func(callCtx context.Context) error { return s.unfreeze(callCtx, id, actor) })
}
func (s *Service) MarkCrashedWithOptions(ctx context.Context, id, actor, reason string, options OperationOptions) (operation.Operation, bool, error) {
	return s.coordinate(ctx, "mark-crashed", id, actor, session.StateCrashed, options, func(callCtx context.Context) error { return s.markCrashed(callCtx, id, actor, reason) })
}
func (s *Service) ArchiveWithOptions(ctx context.Context, id, actor string, options OperationOptions) (operation.Operation, bool, error) {
	return s.coordinate(ctx, "archive", id, actor, session.StateArchived, options, func(callCtx context.Context) error { return s.archive(callCtx, id, actor) })
}
func (s *Service) DeleteWithOptions(ctx context.Context, id, actor string, options OperationOptions) (operation.Operation, bool, error) {
	return s.coordinate(ctx, "delete", id, actor, session.StateDeleted, options, func(callCtx context.Context) error { return s.delete(callCtx, id, actor) })
}

func (s *Service) coordinate(ctx context.Context, action, id, actor string, intended session.State, options OperationOptions, call func(context.Context) error) (operation.Operation, bool, error) {
	if err := validateActor(actor); err != nil {
		return operation.Operation{}, false, err
	}
	current, err := s.repository.GetSession(ctx, id)
	if err != nil {
		return operation.Operation{}, false, err
	}
	if (action == "start" || action == "restart") && options.RuntimeProfileID == "" {
		options.RuntimeProfileID = runtimeprofile.DefaultProfileID
	}
	metadata := map[string]string{}
	if (action == "start" || action == "restart") && options.RuntimeProfileID != "" {
		metadata["runtimeProfileId"] = options.RuntimeProfileID
	}
	if action == "start" {
		metadata["artifactCheckEnabled"] = fmt.Sprintf("%t", s.artifactGate != nil)
	}
	value, replay, err := s.operations.Begin(ctx, operationsvc.BeginParams{RequestID: options.RequestID, IdempotencyKey: options.IdempotencyKey, ActorID: actor, Action: action, TargetType: "session", TargetID: id, ProjectID: current.ProjectID, SessionID: id, PreviousState: string(current.State), IntendedState: string(intended), Metadata: metadata})
	if err != nil {
		return operation.Operation{}, false, err
	}
	if replay {
		return value, true, nil
	}
	runtimeMetadata := map[string]string{}
	callCtx := context.WithValue(agent.WithRequestID(ctx, value.RequestID), operationContextKey{}, operationContext{ID: value.ID, RequestID: value.RequestID, RuntimeProfileID: options.RuntimeProfileID, Metadata: runtimeMetadata})
	cancel := func() {}
	if options.Timeout > 0 {
		callCtx, cancel = context.WithTimeout(callCtx, options.Timeout)
	}
	defer cancel()
	err = call(callCtx)
	final := current.State
	if updated, loadErr := s.repository.GetSession(ctx, id); loadErr == nil {
		final = updated.State
	}
	if err == nil && action == "delete" {
		final = intended
	}
	completionMetadata := map[string]string{}
	if updated, loadErr := s.repository.GetSession(ctx, id); loadErr == nil {
		completionMetadata["agentId"] = updated.AssignedAgentID
		completionMetadata["agentResult"] = updated.LastAgentStatus
	}
	if options.RuntimeProfileID != "" {
		completionMetadata["runtimeProfileId"] = options.RuntimeProfileID
	}
	for key, item := range runtimeMetadata {
		completionMetadata[key] = item
	}
	if err == nil {
		value, completeErr := s.operations.Complete(ctx, value, operation.StatusSucceeded, string(final), "success", "", "", completionMetadata)
		return value, false, completeErr
	}
	status, code := operation.StatusFailed, "operation_failed"
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		status, code = operation.StatusTimedOut, "operation_timeout"
	}
	value, completeErr := s.operations.Complete(ctx, value, status, string(final), "failure", code, err.Error(), completionMetadata)
	if completeErr != nil {
		return value, false, fmt.Errorf("%w; persist operation result: %v", err, completeErr)
	}
	return value, false, err
}

func (s *Service) prepare(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	return s.agentTransition(ctx, "prepare", id, actor, []session.State{session.StatePreparing}, func(value session.Session) (agent.OperationResult, error) {
		return s.agent.PrepareSession(ctx, agentRequest(ctx, value))
	})
}

func (s *Service) start(ctx context.Context, id, actor string) error {
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
	if s.artifactGate != nil {
		metadata, gateErr := s.artifactGate.Check(ctx, id)
		setOperationMetadata(ctx, metadata)
		if gateErr != nil {
			return s.fail(ctx, "start", value, actor, session.StateRunning, gateErr)
		}
	}
	requestedProfileID := getOperationContext(ctx).RuntimeProfileID
	env, envErr := s.repository.GetEnvironment(ctx, value.EnvironmentID)
	if envErr != nil {
		envMetadata := map[string]string{"environmentId": value.EnvironmentID, "environmentLoadError": envErr.Error()}
		setOperationMetadata(ctx, envMetadata)
		return s.fail(ctx, "start", value, actor, session.StateRunning, fmt.Errorf("environment %q: %w", value.EnvironmentID, envErr))
	}
	selectedProfileID := requestedProfileID
	if selectedProfileID == "" && env.RuntimeProfileID != "" {
		selectedProfileID = env.RuntimeProfileID
	}
	profileMetadata := map[string]string{
		"environmentId":                     env.ID,
		"environmentRuntimeProfileId":       env.RuntimeProfileID,
		"environmentRuntimeProfileRequired": fmt.Sprintf("%t", env.RuntimeProfileRequired),
		"requestedRuntimeProfileId":         requestedProfileID,
		"selectedRuntimeProfileId":          selectedProfileID,
	}
	if env.RuntimeProfileRequired {
		if env.RuntimeProfileID == "" {
			profileMetadata["runtimeProfileCompatibilityStatus"] = "failed"
			profileMetadata["runtimeProfileCompatibilityReason"] = "environment requires runtime profile but runtimeProfileId is empty"
			setOperationMetadata(ctx, profileMetadata)
			return s.fail(ctx, "start", value, actor, session.StateRunning, fmt.Errorf("environment %q requires runtime profile but has no runtimeProfileId", env.ID))
		}
		if selectedProfileID != env.RuntimeProfileID {
			profileMetadata["runtimeProfileCompatibilityStatus"] = "failed"
			profileMetadata["runtimeProfileCompatibilityReason"] = fmt.Sprintf("environment requires %q but selected %q", env.RuntimeProfileID, selectedProfileID)
			setOperationMetadata(ctx, profileMetadata)
			return s.fail(ctx, "start", value, actor, session.StateRunning, fmt.Errorf("environment %q requires runtime profile %q but selected %q", env.ID, env.RuntimeProfileID, selectedProfileID))
		}
	}
	profileMetadata["runtimeProfileCompatibilityStatus"] = "ok"
	setOperationMetadata(ctx, profileMetadata)
	if selectedProfileID != requestedProfileID {
		opCtx := getOperationContext(ctx)
		opCtx.RuntimeProfileID = selectedProfileID
		ctx = context.WithValue(ctx, operationContextKey{}, opCtx)
	}
	var agentResult *agent.OperationResult
	if s.agent != nil {
		if previous == session.StateCreated {
			result, callErr := s.agent.PrepareSession(ctx, agentRequest(ctx, value))
			if callErr != nil {
				return s.failWithAgent(ctx, "start", value, actor, session.StateRunning, callErr, nil)
			}
			agentResult = &result
		}
		materializationRequest := agent.EnvironmentMaterializationRequest{
			SessionID:              value.ID,
			EnvironmentID:          env.ID,
			EnvironmentName:        env.Name,
			MinecraftVersion:       env.MinecraftVersion,
			JavaVersion:            env.JavaVersion,
			LoaderType:             string(env.LoaderType),
			LoaderVersion:          env.LoaderVersion,
			ServerCore:             string(env.ServerCore),
			MCDRRequired:           env.MCDRRequired,
			CarpetRequired:         env.CarpetRequired,
			RuntimeProfileID:       selectedProfileID,
			RuntimeProfileRequired: env.RuntimeProfileRequired,
			ActorID:                actor,
		}
		materializationResult, callErr := s.agent.MaterializeEnvironment(ctx, materializationRequest)
		if callErr != nil {
			setOperationMetadata(ctx, map[string]string{"environmentMaterializationStatus": "failed", "environmentMaterializationError": callErr.Error()})
			return s.failWithAgent(ctx, "start", value, actor, session.StateRunning, callErr, agentResult)
		}
		metadataMap := map[string]string{
			"environmentMaterializationStatus":      materializationResult.Status,
			"environmentMaterializationDirectories": fmt.Sprintf("%d", len(materializationResult.Directories)),
		}
		if manifestPath, ok := materializationResult.Metadata["manifestPath"]; ok {
			metadataMap["environmentMaterializationManifest"] = manifestPath
		}
		setOperationMetadata(ctx, metadataMap)
		if readinessErr := s.checkRuntimeReadiness(ctx, value.ID); readinessErr != nil {
			return s.failWithAgent(ctx, "start", value, actor, session.StateRunning, readinessErr, agentResult)
		}
		result, callErr := s.agent.StartSession(ctx, agentRequest(ctx, value))
		if callErr != nil {
			return s.failWithAgent(ctx, "start", value, actor, session.StateRunning, callErr, agentResult)
		}
		agentResult = &result
		s.applyAgentMetadata(ctx, &value, result)
	}
	if err := applyPath(&value, path); err != nil {
		return s.fail(ctx, "start", value, actor, session.StateRunning, err)
	}
	value.LastActiveAt = s.now()
	return s.persistAgentSuccess(ctx, "start", previous, value, actor, agentResult)
}

func (s *Service) stop(ctx context.Context, id, actor string) error {
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
	var agentResult *agent.OperationResult
	if s.agent != nil {
		result, callErr := s.agent.StopSession(ctx, agentRequest(ctx, value))
		if callErr != nil {
			return s.failWithAgent(ctx, "stop", session.Session{ID: value.ID, ProjectID: value.ProjectID, State: previous}, actor, session.StateStopped, callErr, nil)
		}
		agentResult = &result
		s.applyAgentMetadata(ctx, &value, result)
	}
	value.LastActiveAt = s.now()
	return s.persistAgentSuccess(ctx, "stop", previous, value, actor, agentResult)
}

func (s *Service) restart(ctx context.Context, id, actor string) error {
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
	requestedProfileID := getOperationContext(ctx).RuntimeProfileID
	env, envErr := s.repository.GetEnvironment(ctx, value.EnvironmentID)
	if envErr != nil {
		envMetadata := map[string]string{"environmentId": value.EnvironmentID, "environmentLoadError": envErr.Error()}
		setOperationMetadata(ctx, envMetadata)
		return s.fail(ctx, "restart", value, actor, session.StateRunning, fmt.Errorf("environment %q: %w", value.EnvironmentID, envErr))
	}
	selectedProfileID := requestedProfileID
	if selectedProfileID == "" && env.RuntimeProfileID != "" {
		selectedProfileID = env.RuntimeProfileID
	}
	profileMetadata := map[string]string{
		"environmentId":                     env.ID,
		"environmentRuntimeProfileId":       env.RuntimeProfileID,
		"environmentRuntimeProfileRequired": fmt.Sprintf("%t", env.RuntimeProfileRequired),
		"requestedRuntimeProfileId":         requestedProfileID,
		"selectedRuntimeProfileId":          selectedProfileID,
	}
	if env.RuntimeProfileRequired {
		if env.RuntimeProfileID == "" {
			profileMetadata["runtimeProfileCompatibilityStatus"] = "failed"
			profileMetadata["runtimeProfileCompatibilityReason"] = "environment requires runtime profile but runtimeProfileId is empty"
			setOperationMetadata(ctx, profileMetadata)
			return s.fail(ctx, "restart", value, actor, session.StateRunning, fmt.Errorf("environment %q requires runtime profile but has no runtimeProfileId", env.ID))
		}
		if selectedProfileID != env.RuntimeProfileID {
			profileMetadata["runtimeProfileCompatibilityStatus"] = "failed"
			profileMetadata["runtimeProfileCompatibilityReason"] = fmt.Sprintf("environment requires %q but selected %q", env.RuntimeProfileID, selectedProfileID)
			setOperationMetadata(ctx, profileMetadata)
			return s.fail(ctx, "restart", value, actor, session.StateRunning, fmt.Errorf("environment %q requires runtime profile %q but selected %q", env.ID, env.RuntimeProfileID, selectedProfileID))
		}
	}
	profileMetadata["runtimeProfileCompatibilityStatus"] = "ok"
	setOperationMetadata(ctx, profileMetadata)
	if selectedProfileID != requestedProfileID {
		opCtx := getOperationContext(ctx)
		opCtx.RuntimeProfileID = selectedProfileID
		ctx = context.WithValue(ctx, operationContextKey{}, opCtx)
	}
	var agentResult *agent.OperationResult
	if s.agent != nil {
		setOperationMetadata(ctx, map[string]string{"restartStartAttempted": "false"})
		if previous == session.StateRunning {
			setOperationMetadata(ctx, map[string]string{"restartStopStatus": "attempted"})
			stopResult, callErr := s.agent.StopSession(ctx, agentRequest(ctx, value))
			if callErr != nil {
				setOperationMetadata(ctx, map[string]string{"restartStopStatus": "failed"})
				return s.failWithAgent(ctx, "restart", value, actor, session.StateRunning, callErr, nil)
			}
			setOperationMetadata(ctx, map[string]string{"restartStopStatus": "succeeded"})
			if err := applyPath(&value, []session.State{session.StateStopping, session.StateStopped}); err != nil {
				return s.failWithAgent(ctx, "restart", value, actor, session.StateRunning, err, &stopResult)
			}
			s.applyAgentMetadata(ctx, &value, stopResult)
			value.LastActiveAt = s.now()
			if err := s.repository.SaveSession(ctx, value); err != nil {
				return s.failWithAgent(ctx, "restart", session.Session{ID: value.ID, ProjectID: value.ProjectID, State: previous}, actor, session.StateStopped, err, &stopResult)
			}
			path = []session.State{session.StateStarting, session.StateRunning}
		} else {
			setOperationMetadata(ctx, map[string]string{"restartStopStatus": "not_required"})
		}
		materializationRequest := agent.EnvironmentMaterializationRequest{
			SessionID:              value.ID,
			EnvironmentID:          env.ID,
			EnvironmentName:        env.Name,
			MinecraftVersion:       env.MinecraftVersion,
			JavaVersion:            env.JavaVersion,
			LoaderType:             string(env.LoaderType),
			LoaderVersion:          env.LoaderVersion,
			ServerCore:             string(env.ServerCore),
			MCDRRequired:           env.MCDRRequired,
			CarpetRequired:         env.CarpetRequired,
			RuntimeProfileID:       selectedProfileID,
			RuntimeProfileRequired: env.RuntimeProfileRequired,
			ActorID:                actor,
		}
		materializationResult, callErr := s.agent.MaterializeEnvironment(ctx, materializationRequest)
		if callErr != nil {
			setOperationMetadata(ctx, map[string]string{"environmentMaterializationStatus": "failed", "environmentMaterializationError": callErr.Error()})
			return s.failWithAgent(ctx, "restart", value, actor, session.StateRunning, callErr, nil)
		}
		metadataMap := map[string]string{
			"environmentMaterializationStatus":      materializationResult.Status,
			"environmentMaterializationDirectories": fmt.Sprintf("%d", len(materializationResult.Directories)),
		}
		if manifestPath, ok := materializationResult.Metadata["manifestPath"]; ok {
			metadataMap["environmentMaterializationManifest"] = manifestPath
		}
		setOperationMetadata(ctx, metadataMap)
		if readinessErr := s.checkRuntimeReadiness(ctx, value.ID); readinessErr != nil {
			return s.failWithAgent(ctx, "restart", value, actor, session.StateRunning, readinessErr, nil)
		}
		setOperationMetadata(ctx, map[string]string{"restartStartAttempted": "true"})
		result, callErr := s.agent.StartSession(ctx, agentRequest(ctx, value))
		if callErr != nil {
			return s.failWithAgent(ctx, "restart", value, actor, session.StateRunning, callErr, nil)
		}
		agentResult = &result
		s.applyAgentMetadata(ctx, &value, result)
	}
	if err := applyPath(&value, path); err != nil {
		return s.fail(ctx, "restart", value, actor, session.StateRunning, err)
	}
	value.LastActiveAt = s.now()
	return s.persistAgentSuccess(ctx, "restart", previous, value, actor, agentResult)
}

func (s *Service) checkRuntimeReadiness(ctx context.Context, sessionID string) error {
	readiness, err := s.agent.SessionReadyForStart(ctx, sessionID)
	if err != nil {
		setOperationMetadata(ctx, map[string]string{"runtimeReadinessStatus": "error", "runtimeReadinessReady": "false", "runtimeReadinessIssues": err.Error()})
		return fmt.Errorf("check Agent runtime readiness: %w", err)
	}
	issues := make([]string, 0, len(readiness.Issues))
	for _, issue := range readiness.Issues {
		issues = append(issues, issue.Code)
	}
	summary := readiness.RuntimeStatusSummary
	setOperationMetadata(ctx, map[string]string{
		"runtimeReadinessStatus":                    readiness.Status,
		"runtimeReadinessReady":                     fmt.Sprintf("%t", readiness.Ready),
		"runtimeReadinessIssues":                    strings.Join(issues, ","),
		"runtimeReadinessProcessState":              summary.ProcessState,
		"runtimeReadinessEnvironmentManifestExists": fmt.Sprintf("%t", summary.EnvironmentManifestExists),
		"runtimeReadinessAppliedTotal":              fmt.Sprintf("%d", summary.AppliedArtifactsTotal),
		"runtimeReadinessAppliedValid":              fmt.Sprintf("%d", summary.AppliedArtifactsValid),
		"runtimeReadinessAppliedMissing":            fmt.Sprintf("%d", summary.AppliedArtifactsMissing),
		"runtimeReadinessAppliedCorrupted":          fmt.Sprintf("%d", summary.AppliedArtifactsCorrupted),
		"runtimeReadinessAppliedError":              fmt.Sprintf("%d", summary.AppliedArtifactsError),
	})
	if !readiness.Ready {
		return fmt.Errorf("Agent runtime is not ready for start: status=%s issues=%s", readiness.Status, strings.Join(issues, ","))
	}
	return nil
}

func (s *Service) freeze(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	return s.agentTransition(ctx, "freeze", id, actor, []session.State{session.StateFrozen}, func(value session.Session) (agent.OperationResult, error) {
		return s.agent.FreezeSession(ctx, agentRequest(ctx, value))
	})
}

func (s *Service) unfreeze(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	return s.agentTransition(ctx, "unfreeze", id, actor, []session.State{session.StateRunning}, func(value session.Session) (agent.OperationResult, error) {
		return s.agent.UnfreezeSession(ctx, agentRequest(ctx, value))
	})
}

func (s *Service) markCrashed(ctx context.Context, id, actor, reason string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	// TODO: request a crash snapshot after agent/runtime integration is enabled.
	return s.transition(ctx, "mark-crashed", id, actor, reason, []session.State{session.StateCrashed})
}

func (s *Service) archive(ctx context.Context, id, actor string) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	// TODO: create a pre-archive checkpoint before moving runtime data.
	return s.transition(ctx, "archive", id, actor, "", []session.State{session.StateArchived})
}

func (s *Service) delete(ctx context.Context, id, actor string) error {
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

func (s *Service) persistAgentSuccess(ctx context.Context, action string, previous session.State, value session.Session, actor string, result *agent.OperationResult) error {
	if err := s.repository.SaveSession(ctx, value); err != nil {
		return s.failWithAgent(ctx, action, session.Session{ID: value.ID, ProjectID: value.ProjectID, State: previous}, actor, value.State, err, result)
	}
	return s.audit(ctx, action, value.ProjectID, value.ID, actor, previous, value.State, "success", "", agentMetadata(result))
}

func (s *Service) agentTransition(ctx context.Context, action, id, actor string, path []session.State, call func(session.Session) (agent.OperationResult, error)) error {
	value, err := s.repository.GetSession(ctx, id)
	if err != nil {
		return s.auditLoadFailure(ctx, action, id, actor, finalState(path), err)
	}
	previous := value.State
	probe := value
	if err := applyPath(&probe, path); err != nil {
		return s.fail(ctx, action, value, actor, finalState(path), err)
	}
	var result *agent.OperationResult
	if s.agent != nil {
		operationResult, callErr := call(value)
		if callErr != nil {
			return s.failWithAgent(ctx, action, value, actor, finalState(path), callErr, nil)
		}
		result = &operationResult
		s.applyAgentMetadata(ctx, &probe, operationResult)
	}
	probe.LastActiveAt = s.now()
	return s.persistAgentSuccess(ctx, action, previous, probe, actor, result)
}

func (s *Service) fail(ctx context.Context, action string, value session.Session, actor string, next session.State, operationErr error) error {
	auditErr := s.audit(ctx, action, value.ProjectID, value.ID, actor, value.State, next, "failure", operationErr.Error())
	if auditErr != nil {
		return fmt.Errorf("%w; append failure audit: %v", operationErr, auditErr)
	}
	return operationErr
}

func (s *Service) failWithAgent(ctx context.Context, action string, value session.Session, actor string, next session.State, operationErr error, result *agent.OperationResult) error {
	metadata := agentMetadata(result)
	if metadata["agentId"] == "" && s.agent != nil {
		if info, err := s.agent.Info(ctx); err == nil {
			metadata["agentId"] = info.ID
			metadata["agentMode"] = info.Mode
		}
	}
	metadata["agentResult"] = "failure"
	metadata["agentMessage"] = operationErr.Error()
	auditErr := s.audit(ctx, action, value.ProjectID, value.ID, actor, value.State, next, "failure", operationErr.Error(), metadata)
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

func (s *Service) audit(ctx context.Context, action, projectID, id, actor string, previous, next session.State, result, reason string, extras ...map[string]string) error {
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
	if operationValue, ok := ctx.Value(operationContextKey{}).(operationContext); ok {
		metadata["operationId"] = operationValue.ID
		metadata["requestId"] = operationValue.RequestID
		if operationValue.RuntimeProfileID != "" {
			metadata["runtimeProfileId"] = operationValue.RuntimeProfileID
		}
		for key, item := range operationValue.Metadata {
			metadata[key] = item
		}
	}
	if reason != "" {
		metadata["reason"] = reason
	}
	for _, extra := range extras {
		for key, value := range extra {
			metadata[key] = value
		}
	}
	return s.repository.AppendAuditEvent(ctx, audit.Event{
		ID: eventID, ProjectID: projectID, ActorID: actor, Action: "session." + action,
		TargetType: "session", TargetID: id, Metadata: metadata, CreatedAt: s.now(),
	})
}

func getOperationContext(ctx context.Context) operationContext {
	operationValue, ok := ctx.Value(operationContextKey{}).(operationContext)
	if !ok {
		return operationContext{}
	}
	return operationValue
}

func setOperationMetadata(ctx context.Context, values map[string]string) {
	operationValue, ok := ctx.Value(operationContextKey{}).(operationContext)
	if !ok || operationValue.Metadata == nil {
		return
	}
	for key, item := range values {
		operationValue.Metadata[key] = item
	}
}

func (s *Service) applyAgentMetadata(ctx context.Context, value *session.Session, result agent.OperationResult) {
	value.AssignedAgentID = result.AgentID
	value.LastAgentStatus = result.Status
	value.LastRuntimeMessage = result.Message
	if info, err := s.agent.Info(ctx); err == nil {
		value.RuntimeEndpoint = info.RuntimeEndpoint
	}
}

func agentMetadata(result *agent.OperationResult) map[string]string {
	metadata := map[string]string{}
	if result == nil {
		return metadata
	}
	metadata["agentId"] = result.AgentID
	metadata["agentResult"] = result.Status
	metadata["agentMessage"] = result.Message
	if result.Mode != "" {
		metadata["agentMode"] = result.Mode
	}
	return metadata
}

func agentRequest(ctx context.Context, value session.Session) agent.SessionRequest {
	request := agent.SessionRequest{SessionID: value.ID, ProjectID: value.ProjectID, EnvironmentID: value.EnvironmentID}
	if operationValue, ok := ctx.Value(operationContextKey{}).(operationContext); ok {
		request.RuntimeProfileID = operationValue.RuntimeProfileID
	}
	return request
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
