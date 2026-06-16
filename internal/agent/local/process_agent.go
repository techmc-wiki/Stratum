package local

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/mcdr"
	agentprocess "github.com/stratummc/stratum/internal/agent/process"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

type ProcessAgent struct {
	id         string
	endpoint   string
	supervisor *agentprocess.Supervisor
	profiles   *runtimeprofile.Registry
	mu         sync.RWMutex
	prepared   map[string]bool
	frozen     map[string]bool
}

var (
	_ agent.AgentClient       = (*ProcessAgent)(nil)
	_ agent.RuntimeAgent      = (*ProcessAgent)(nil)
	_ agent.ProcessSupervisor = (*ProcessAgent)(nil)
	_ agent.LogStreamer       = (*ProcessAgent)(nil)
	_ agent.ResourceReporter  = (*ProcessAgent)(nil)
	_ agent.CheckpointWorker  = (*ProcessAgent)(nil)
)

func NewProcessAgent() *ProcessAgent {
	return NewProcessAgentWithID(DefaultAgentID)
}

func NewProcessAgentWithID(id string) *ProcessAgent {
	return NewProcessAgentWithRegistry(id, runtimeprofile.Builtins())
}

func NewProcessAgentWithRegistry(id string, profiles *runtimeprofile.Registry) *ProcessAgent {
	if profiles == nil {
		profiles = runtimeprofile.Builtins()
	}
	return &ProcessAgent{id: id, endpoint: "local://agent/" + id, supervisor: agentprocess.NewSupervisor(id), profiles: profiles, prepared: map[string]bool{}, frozen: map[string]bool{}}
}

func NewProcessAgentWithRegistryAndRoot(id string, profiles *runtimeprofile.Registry, runtimeRoot string) (*ProcessAgent, error) {
	if profiles == nil {
		profiles = runtimeprofile.Builtins()
	}
	supervisor, err := agentprocess.NewSupervisorWithRoot(id, runtimeRoot, 256*1024)
	if err != nil {
		return nil, err
	}
	return &ProcessAgent{id: id, endpoint: "local://agent/" + id, supervisor: supervisor, profiles: profiles, prepared: map[string]bool{}, frozen: map[string]bool{}}, nil
}

func (a *ProcessAgent) RuntimeProfiles(context.Context) ([]runtimeprofile.Profile, error) {
	values := a.profiles.ListEnabled()
	for index := range values {
		values[index] = values[index].Public()
	}
	return values, nil
}

func (a *ProcessAgent) Info(context.Context) (agent.AgentInfo, error) {
	return agent.AgentInfo{ID: a.id, Status: "available", RuntimeEndpoint: a.endpoint, Capabilities: []string{"prepare", "start", "stop", "restart", "freeze", "unfreeze", "inspect", "logs", "resources", "dummy-process", "checkpoint-stub", "artifact-materialize", "artifact-manifest-inspect"}, Mode: agentprocess.RuntimeModeDummy}, nil
}

func (a *ProcessAgent) PrepareSession(_ context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	if request.SessionID == "" {
		return agent.OperationResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationPrepare, Message: "session id is required"}
	}
	a.mu.Lock()
	a.prepared[request.SessionID] = true
	a.mu.Unlock()
	return a.result("prepared for dummy runtime"), nil
}

func (a *ProcessAgent) StartSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	profile, err := a.profiles.Get(request.RuntimeProfileID)
	if err != nil {
		return agent.OperationResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationStart, Message: err.Error()}
	}
	model, err := a.supervisor.StartProcess(ctx, request.SessionID, profile)
	if err != nil {
		return agent.OperationResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationStart, Message: err.Error()}
	}
	a.mu.Lock()
	a.frozen[request.SessionID] = false
	a.mu.Unlock()
	return a.result("runtime " + string(model.Status)), nil
}

func (a *ProcessAgent) StopSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	model, err := a.supervisor.StopProcess(ctx, request.SessionID)
	if err != nil {
		return agent.OperationResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationStop, Message: err.Error()}
	}
	a.mu.Lock()
	a.frozen[request.SessionID] = false
	a.mu.Unlock()
	return a.result("runtime " + string(model.Status)), nil
}

func (a *ProcessAgent) RestartSession(ctx context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	profile, err := a.profiles.Get(request.RuntimeProfileID)
	if err != nil {
		return agent.OperationResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationRestart, Message: err.Error()}
	}
	model, err := a.supervisor.RestartProcess(ctx, request.SessionID, profile)
	if err != nil {
		return agent.OperationResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationRestart, Message: err.Error()}
	}
	a.mu.Lock()
	a.frozen[request.SessionID] = false
	a.mu.Unlock()
	return a.result("runtime " + string(model.Status)), nil
}

func (a *ProcessAgent) FreezeSession(_ context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	if !a.supervisor.IsRunning(request.SessionID) {
		return agent.OperationResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationFreeze, Message: "dummy runtime is not running"}
	}
	a.mu.Lock()
	a.frozen[request.SessionID] = true
	a.mu.Unlock()
	return a.result("frozen metadata only"), nil
}

func (a *ProcessAgent) UnfreezeSession(_ context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	if !a.supervisor.IsRunning(request.SessionID) {
		return agent.OperationResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationUnfreeze, Message: "dummy runtime is not running"}
	}
	a.mu.Lock()
	a.frozen[request.SessionID] = false
	a.mu.Unlock()
	return a.result("unfrozen"), nil
}

func (a *ProcessAgent) InspectSession(_ context.Context, sessionID string) (agent.SessionStatus, error) {
	model := a.supervisor.InspectProcess(sessionID)
	a.mu.RLock()
	frozen := a.frozen[sessionID]
	a.mu.RUnlock()
	return agent.SessionStatus{AgentID: a.id, SessionID: sessionID, Status: string(model.Status), Running: model.Status == agentprocess.StatusRunning, Frozen: frozen, RuntimeEndpoint: a.endpoint, ProcessID: model.ProcessID, PID: model.PID, RuntimeMode: model.RuntimeMode, RuntimeProfileID: model.RuntimeProfileID, RuntimeType: model.RuntimeType, Crashed: model.Crashed, StartedAt: model.StartedAt, StoppedAt: model.StoppedAt, ExitCode: model.ExitCode, LastError: model.LastError, ObservedAt: time.Now().UTC(), SessionRoot: model.SessionRoot, WorkDir: model.WorkDir, LogsDir: model.LogsDir}, nil
}

func (a *ProcessAgent) CollectLogs(ctx context.Context, sessionID string) (agent.LogBatch, error) {
	return agent.LogBatch{AgentID: a.id, SessionID: sessionID, Lines: a.supervisor.CollectLogs(sessionID, agent.LogMaxBytesFromContext(ctx))}, nil
}

func (a *ProcessAgent) ReportResources(context.Context) (agent.ResourceReport, error) {
	return agent.ResourceReport{AgentID: a.id, CPUCapacity: 8, MemoryTotalMB: 16384, MemoryUsedMB: 2048, DiskTotalMB: 262144, DiskUsedMB: 32768, RunningSessions: a.supervisor.RunningCount(), ReportedAt: time.Now().UTC()}, nil
}

func (a *ProcessAgent) CreateCheckpointStub(_ context.Context, request agent.CheckpointRequest) (agent.OperationResult, error) {
	return a.result("checkpoint-stub-created:" + request.CheckpointID), nil
}

func (a *ProcessAgent) RestoreCheckpointStub(_ context.Context, request agent.CheckpointRequest) (agent.OperationResult, error) {
	return a.result("checkpoint-stub-restored:" + request.CheckpointID), nil
}

func (a *ProcessAgent) MaterializeArtifact(ctx context.Context, request agent.ArtifactMaterializationRequest) (agent.ArtifactMaterializationResult, error) {
	result, err := agentprocess.MaterializeArtifact(ctx, a.supervisor.RuntimeRoot(), request, time.Now().UTC())
	if err != nil {
		return agent.ArtifactMaterializationResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationMaterializeArtifact, Message: err.Error()}
	}
	result.AgentID = a.id
	return result, nil
}

func (a *ProcessAgent) InspectMaterializedArtifacts(ctx context.Context, sessionID string) (agent.MaterializedArtifacts, error) {
	result, err := agentprocess.InspectMaterializedArtifacts(ctx, a.supervisor.RuntimeRoot(), sessionID)
	if err != nil {
		return agent.MaterializedArtifacts{}, agent.Error{AgentID: a.id, Operation: agent.OperationInspect, Message: err.Error()}
	}
	result.AgentID = a.id
	return result, nil
}

func (a *ProcessAgent) InspectMaterializedArtifact(ctx context.Context, sessionID, stagingPlanID string) (agent.MaterializedArtifact, error) {
	result, err := agentprocess.InspectMaterializedArtifact(ctx, a.supervisor.RuntimeRoot(), sessionID, stagingPlanID)
	if err != nil {
		if errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
			return agent.MaterializedArtifact{}, err
		}
		return agent.MaterializedArtifact{}, agent.Error{AgentID: a.id, Operation: agent.OperationInspect, Message: err.Error()}
	}
	result.AgentID = a.id
	return result, nil
}

func (a *ProcessAgent) VerifyMaterializedArtifact(ctx context.Context, sessionID, stagingPlanID string) (agent.MaterializedArtifactVerification, error) {
	result, err := agentprocess.VerifyMaterializedArtifact(ctx, a.supervisor.RuntimeRoot(), sessionID, stagingPlanID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, agent.ErrMaterializedArtifactNotFound) {
			return agent.MaterializedArtifactVerification{}, err
		}
		return agent.MaterializedArtifactVerification{}, agent.Error{AgentID: a.id, Operation: agent.OperationInspect, Message: err.Error()}
	}
	result.AgentID = a.id
	return result, nil
}

func (a *ProcessAgent) VerifyMaterializedArtifacts(ctx context.Context, sessionID string) (agent.MaterializedArtifactsVerification, error) {
	result, err := agentprocess.VerifyMaterializedArtifacts(ctx, a.supervisor.RuntimeRoot(), sessionID, time.Now().UTC())
	if err != nil {
		return agent.MaterializedArtifactsVerification{}, agent.Error{AgentID: a.id, Operation: agent.OperationInspect, Message: err.Error()}
	}
	result.AgentID = a.id
	for index := range result.Entries {
		result.Entries[index].AgentID = a.id
	}
	return result, nil
}

func (a *ProcessAgent) DryRunArtifactApply(ctx context.Context, req agent.ArtifactApplyDryRunRequest) (agent.ArtifactApplyDryRunResult, error) {
	result, err := agentprocess.DryRunArtifactApply(ctx, a.supervisor.RuntimeRoot(), req, time.Now().UTC())
	if err != nil {
		return agent.ArtifactApplyDryRunResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationInspect, Message: err.Error()}
	}
	result.AgentID = a.id
	return result, nil
}

func (a *ProcessAgent) ExecuteArtifactApply(ctx context.Context, req agent.ArtifactApplyExecuteRequest) (agent.ArtifactApplyExecuteResult, error) {
	result, err := agentprocess.ExecuteArtifactApply(ctx, a.supervisor.RuntimeRoot(), req, time.Now().UTC())
	if err != nil {
		return agent.ArtifactApplyExecuteResult{}, agent.Error{AgentID: a.id, Operation: agent.OperationArtifactApply, Message: err.Error()}
	}
	result.AgentID = a.id
	return result, nil
}

func (a *ProcessAgent) ListAppliedArtifacts(ctx context.Context, sessionID string) (agent.AppliedArtifactsResponse, error) {
	records, err := agentprocess.ReadAppliedArtifacts(ctx, a.supervisor.RuntimeRoot(), sessionID)
	if err != nil {
		return agent.AppliedArtifactsResponse{}, agent.Error{AgentID: a.id, Operation: "list-applied-artifacts", Message: err.Error()}
	}
	result := agent.AppliedArtifactsResponse{SessionID: sessionID, Records: make([]agent.AppliedArtifactRecord, len(records))}
	for i, r := range records {
		result.Records[i] = agent.AppliedArtifactRecord{ApplyPlanID: r.ApplyPlanID, SessionID: r.SessionID, ArtifactID: r.ArtifactID, StagingPlanID: r.StagingPlanID, SourceRuntimeRelativePath: r.SourceRuntimeRelativePath, TargetRuntimeRelativePath: r.TargetRuntimeRelativePath, TargetRoot: r.TargetRoot, TargetRelativePath: r.TargetRelativePath, PayloadAlgorithm: r.PayloadAlgorithm, PayloadHash: r.PayloadHash, PayloadSize: r.PayloadSize, Action: r.Action, Status: r.Status, ActorID: r.ActorID, AppliedAt: r.AppliedAt}
	}
	return result, nil
}

func (a *ProcessAgent) InspectAppliedArtifact(ctx context.Context, sessionID, applyPlanID string) (agent.AppliedArtifactRecord, error) {
	record, err := agentprocess.ReadAppliedArtifact(ctx, a.supervisor.RuntimeRoot(), sessionID, applyPlanID)
	if err != nil {
		return agent.AppliedArtifactRecord{}, agent.Error{AgentID: a.id, Operation: "inspect-applied-artifact", Message: err.Error()}
	}
	return agent.AppliedArtifactRecord{ApplyPlanID: record.ApplyPlanID, SessionID: record.SessionID, ArtifactID: record.ArtifactID, StagingPlanID: record.StagingPlanID, SourceRuntimeRelativePath: record.SourceRuntimeRelativePath, TargetRuntimeRelativePath: record.TargetRuntimeRelativePath, TargetRoot: record.TargetRoot, TargetRelativePath: record.TargetRelativePath, PayloadAlgorithm: record.PayloadAlgorithm, PayloadHash: record.PayloadHash, PayloadSize: record.PayloadSize, Action: record.Action, Status: record.Status, ActorID: record.ActorID, AppliedAt: record.AppliedAt}, nil
}

func (a *ProcessAgent) VerifyAppliedArtifact(ctx context.Context, sessionID, applyPlanID string) (agent.AppliedArtifactVerification, error) {
	result, err := agentprocess.VerifyAppliedArtifact(ctx, a.supervisor.RuntimeRoot(), sessionID, applyPlanID, time.Now())
	if err != nil {
		return agent.AppliedArtifactVerification{}, agent.Error{AgentID: a.id, Operation: "verify-applied-artifact", Message: err.Error()}
	}
	return agent.AppliedArtifactVerification{SessionID: result.SessionID, ApplyPlanID: result.ApplyPlanID, ArtifactID: result.ArtifactID, StagingPlanID: result.StagingPlanID, TargetRoot: result.TargetRoot, TargetRelativePath: result.TargetRelativePath, TargetRuntimeRelativePath: result.TargetRuntimeRelativePath, PayloadAlgorithm: result.PayloadAlgorithm, ExpectedHash: result.ExpectedHash, ActualHash: result.ActualHash, PayloadSize: result.PayloadSize, ActualSize: result.ActualSize, Status: result.Status, VerifiedAt: result.VerifiedAt, ErrorMessage: result.ErrorMessage}, nil
}

func (a *ProcessAgent) VerifyAllAppliedArtifacts(ctx context.Context, sessionID string) (agent.BatchAppliedArtifactVerification, error) {
	result, err := agentprocess.VerifyAllAppliedArtifacts(ctx, a.supervisor.RuntimeRoot(), sessionID, time.Now())
	if err != nil {
		return agent.BatchAppliedArtifactVerification{}, agent.Error{AgentID: a.id, Operation: "verify-all-applied-artifacts", Message: err.Error()}
	}
	batch := agent.BatchAppliedArtifactVerification{SessionID: result.SessionID, VerifiedAt: result.VerifiedAt, Total: result.Total, ValidCount: result.ValidCount, MissingCount: result.MissingCount, CorruptedCount: result.CorruptedCount, ErrorCount: result.ErrorCount, Entries: make([]agent.AppliedArtifactVerification, len(result.Entries))}
	for i, e := range result.Entries {
		batch.Entries[i] = agent.AppliedArtifactVerification{SessionID: e.SessionID, ApplyPlanID: e.ApplyPlanID, ArtifactID: e.ArtifactID, StagingPlanID: e.StagingPlanID, TargetRoot: e.TargetRoot, TargetRelativePath: e.TargetRelativePath, TargetRuntimeRelativePath: e.TargetRuntimeRelativePath, PayloadAlgorithm: e.PayloadAlgorithm, ExpectedHash: e.ExpectedHash, ActualHash: e.ActualHash, PayloadSize: e.PayloadSize, ActualSize: e.ActualSize, Status: e.Status, VerifiedAt: e.VerifiedAt, ErrorMessage: e.ErrorMessage}
	}
	return batch, nil
}

func (a *ProcessAgent) result(message string) agent.OperationResult {
	return agent.OperationResult{AgentID: a.id, Status: "success", Message: message, Mode: agentprocess.RuntimeModeDummy}
}

func (a *ProcessAgent) Start(ctx context.Context, sessionID string) error {
	_, err := a.StartSession(ctx, agent.SessionRequest{SessionID: sessionID, RuntimeProfileID: runtimeprofile.DefaultProfileID})
	return err
}

func (a *ProcessAgent) Stop(ctx context.Context, sessionID string) error {
	_, err := a.StopSession(ctx, agent.SessionRequest{SessionID: sessionID})
	return err
}

func (a *ProcessAgent) Restart(ctx context.Context, sessionID string) error {
	_, err := a.RestartSession(ctx, agent.SessionRequest{SessionID: sessionID, RuntimeProfileID: runtimeprofile.DefaultProfileID})
	return err
}

func (a *ProcessAgent) Inspect(ctx context.Context, sessionID string) (agent.SessionStatus, error) {
	return a.InspectSession(ctx, sessionID)
}

func (a *ProcessAgent) MaterializeEnvironment(ctx context.Context, request agent.EnvironmentMaterializationRequest) (agent.EnvironmentMaterializationResult, error) {
	return a.supervisor.MaterializeEnvironment(ctx, request)
}

func (a *ProcessAgent) GetSessionRuntimeStatus(ctx context.Context, sessionID string) (agent.SessionRuntimeStatus, error) {
	return a.supervisor.GetSessionRuntimeStatus(ctx, sessionID)
}

func (a *ProcessAgent) SessionReadyForStart(ctx context.Context, sessionID string) (agent.SessionStartReadiness, error) {
	result, err := a.supervisor.SessionReadyForStart(ctx, sessionID)
	if err != nil {
		return agent.SessionStartReadiness{}, agent.Error{AgentID: a.id, Operation: agent.OperationSessionReadyForStart, Message: err.Error()}
	}
	return result, nil
}

func (a *ProcessAgent) InspectMCDRConfigStub(ctx context.Context, sessionID string) (agent.MCDRConfigStubInspection, error) {
	sessionLayout, err := agentprocess.NewSessionRuntimeLayout(a.supervisor.RuntimeRoot(), sessionID)
	if err != nil {
		return agent.MCDRConfigStubInspection{}, agent.Error{AgentID: a.id, Operation: "inspect-mcdr-config-stub", Message: err.Error()}
	}
	layout, err := sessionLayout.MCDR()
	if err != nil {
		return agent.MCDRConfigStubInspection{}, agent.Error{AgentID: a.id, Operation: "inspect-mcdr-config-stub", Message: err.Error()}
	}
	result := mcdr.InspectConfigStubManifest(layout)
	return agent.MCDRConfigStubInspection{
		SessionID:                   result.SessionID,
		Exists:                      result.Exists,
		Path:                        result.Path,
		Valid:                       result.Valid,
		Status:                      result.Status,
		PlannedConfigYMLPath:        result.PlannedConfigYMLPath,
		PlannedServerPropertiesPath: result.PlannedServerPropertiesPath,
		PlannedEULAPath:             result.PlannedEULAPath,
		Issues:                      result.Issues,
		CheckedAt:                   result.CheckedAt,
	}, nil
}
