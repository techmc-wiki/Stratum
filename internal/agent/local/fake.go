package local

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stratummc/stratum/internal/agent"
	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

const DefaultAgentID = "local"

var defaultObservedAt = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

type Fake struct {
	mu       sync.Mutex
	id       string
	endpoint string
	failures map[agent.Operation]string
	sessions map[string]agent.SessionStatus
	calls    []agent.Operation
	now      func() time.Time
}

var (
	_ agent.AgentClient       = (*Fake)(nil)
	_ agent.RuntimeAgent      = (*Fake)(nil)
	_ agent.ProcessSupervisor = (*Fake)(nil)
	_ agent.LogStreamer       = (*Fake)(nil)
	_ agent.FileOperator      = (*Fake)(nil)
	_ agent.ResourceReporter  = (*Fake)(nil)
	_ agent.CheckpointWorker  = (*Fake)(nil)
)

func NewFake() *Fake {
	return &Fake{
		id: DefaultAgentID, endpoint: "local://agent/local", failures: map[agent.Operation]string{},
		sessions: map[string]agent.SessionStatus{}, now: func() time.Time { return defaultObservedAt },
	}
}

func (f *Fake) SetFailure(operation agent.Operation, message string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[operation] = message
}

func (f *Fake) Calls() []agent.Operation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]agent.Operation(nil), f.calls...)
}

func (f *Fake) Info(context.Context) (agent.AgentInfo, error) {
	if err := f.record(agent.OperationInspect); err != nil {
		return agent.AgentInfo{}, err
	}
	return agent.AgentInfo{
		ID: f.id, Status: "available", RuntimeEndpoint: f.endpoint,
		Capabilities: []string{"prepare", "start", "stop", "restart", "freeze", "unfreeze", "inspect", "logs", "resources", "send-command", "checkpoint-stub", "artifact-materialize", "artifact-manifest-inspect"},
		Mode:         "local",
	}, nil
}

func (f *Fake) RuntimeProfiles(context.Context) ([]runtimeprofile.Profile, error) {
	return []runtimeprofile.Profile{runtimeprofile.DummyProcess()}, nil
}

func (f *Fake) PrepareSession(_ context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	if err := f.record(agent.OperationPrepare); err != nil {
		return agent.OperationResult{}, err
	}
	f.setStatus(request.SessionID, "prepared", false, false)
	return f.result("prepared"), nil
}

func (f *Fake) StartSession(_ context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	if err := f.record(agent.OperationStart); err != nil {
		return agent.OperationResult{}, err
	}
	if request.RuntimeProfileID != "" && request.RuntimeProfileID != runtimeprofile.DefaultProfileID {
		return agent.OperationResult{}, agent.Error{AgentID: f.id, Operation: agent.OperationStart, Message: "runtime profile is not registered"}
	}
	f.setStatus(request.SessionID, "running", true, false)
	return f.result("running"), nil
}

func (f *Fake) StopSession(_ context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	if err := f.record(agent.OperationStop); err != nil {
		return agent.OperationResult{}, err
	}
	f.setStatus(request.SessionID, "stopped", false, false)
	return f.result("stopped"), nil
}

func (f *Fake) RestartSession(_ context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	if err := f.record(agent.OperationRestart); err != nil {
		return agent.OperationResult{}, err
	}
	if request.RuntimeProfileID != "" && request.RuntimeProfileID != runtimeprofile.DefaultProfileID {
		return agent.OperationResult{}, agent.Error{AgentID: f.id, Operation: agent.OperationRestart, Message: "runtime profile is not registered"}
	}
	f.setStatus(request.SessionID, "running", true, false)
	return f.result("restarted"), nil
}

func (f *Fake) FreezeSession(_ context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	if err := f.record(agent.OperationFreeze); err != nil {
		return agent.OperationResult{}, err
	}
	f.setStatus(request.SessionID, "frozen", true, true)
	return f.result("frozen-metadata-only"), nil
}

func (f *Fake) UnfreezeSession(_ context.Context, request agent.SessionRequest) (agent.OperationResult, error) {
	if err := f.record(agent.OperationUnfreeze); err != nil {
		return agent.OperationResult{}, err
	}
	f.setStatus(request.SessionID, "running", true, false)
	return f.result("unfrozen"), nil
}

func (f *Fake) InspectSession(_ context.Context, sessionID string) (agent.SessionStatus, error) {
	if err := f.record(agent.OperationInspect); err != nil {
		return agent.SessionStatus{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	status, ok := f.sessions[sessionID]
	if !ok {
		status = agent.SessionStatus{AgentID: f.id, SessionID: sessionID, Status: "unknown", RuntimeEndpoint: f.endpoint, ObservedAt: f.now()}
	}
	return status, nil
}

func (f *Fake) CollectLogs(_ context.Context, sessionID string) (agent.LogBatch, error) {
	if err := f.record(agent.OperationCollectLogs); err != nil {
		return agent.LogBatch{}, err
	}
	return agent.LogBatch{AgentID: f.id, SessionID: sessionID, Lines: []string{
		"[fake-agent] session " + sessionID + " log stream is not connected to MCDR",
		"[fake-agent] no real JVM process was started",
	}}, nil
}

func (f *Fake) ReportResources(context.Context) (agent.ResourceReport, error) {
	if err := f.record(agent.OperationReportResources); err != nil {
		return agent.ResourceReport{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	running := 0
	for _, status := range f.sessions {
		if status.Running {
			running++
		}
	}
	return agent.ResourceReport{
		AgentID: f.id, CPUCapacity: 8, MemoryTotalMB: 16384, MemoryUsedMB: 2048,
		DiskTotalMB: 262144, DiskUsedMB: 32768, RunningSessions: running, ReportedAt: f.now(),
	}, nil
}

func (f *Fake) CreateCheckpointStub(_ context.Context, request agent.CheckpointRequest) (agent.OperationResult, error) {
	if err := f.record(agent.OperationCreateCheckpoint); err != nil {
		return agent.OperationResult{}, err
	}
	return f.result("checkpoint-stub-created:" + request.CheckpointID), nil
}

func (f *Fake) RestoreCheckpointStub(_ context.Context, request agent.CheckpointRequest) (agent.OperationResult, error) {
	if err := f.record(agent.OperationRestoreCheckpoint); err != nil {
		return agent.OperationResult{}, err
	}
	return f.result("checkpoint-stub-restored:" + request.CheckpointID), nil
}

func (f *Fake) MaterializeArtifact(_ context.Context, request agent.ArtifactMaterializationRequest) (agent.ArtifactMaterializationResult, error) {
	if err := f.record(agent.OperationMaterializeArtifact); err != nil {
		return agent.ArtifactMaterializationResult{}, err
	}
	return agent.ArtifactMaterializationResult{AgentID: f.id, SessionID: request.SessionID, ArtifactID: request.ArtifactID, StagingPlanID: request.StagingPlanID, TargetName: request.TargetName, RuntimeRelativePath: "artifacts/" + request.TargetName, PayloadHash: request.PayloadHash, PayloadSize: request.PayloadSize, MaterializedAt: f.now(), Status: "materialized"}, nil
}

func (f *Fake) InspectMaterializedArtifacts(_ context.Context, sessionID string) (agent.MaterializedArtifacts, error) {
	return agent.MaterializedArtifacts{AgentID: f.id, SessionID: sessionID, Status: "empty", Items: []agent.MaterializedArtifact{}}, nil
}

func (f *Fake) InspectMaterializedArtifact(_ context.Context, sessionID, stagingPlanID string) (agent.MaterializedArtifact, error) {
	return agent.MaterializedArtifact{}, fmt.Errorf("%w: staging plan %q in session %q", agent.ErrMaterializedArtifactNotFound, stagingPlanID, sessionID)
}

func (f *Fake) VerifyMaterializedArtifact(_ context.Context, sessionID, stagingPlanID string) (agent.MaterializedArtifactVerification, error) {
	return agent.MaterializedArtifactVerification{}, fmt.Errorf("%w: staging plan %q in session %q", agent.ErrMaterializedArtifactNotFound, stagingPlanID, sessionID)
}

func (f *Fake) VerifyMaterializedArtifacts(_ context.Context, sessionID string) (agent.MaterializedArtifactsVerification, error) {
	return agent.MaterializedArtifactsVerification{AgentID: f.id, SessionID: sessionID, Entries: []agent.MaterializedArtifactVerification{}}, nil
}

func (f *Fake) DryRunArtifactApply(_ context.Context, req agent.ArtifactApplyDryRunRequest) (agent.ArtifactApplyDryRunResult, error) {
	return agent.ArtifactApplyDryRunResult{AgentID: f.id, ApplyPlanID: req.ApplyPlanID, SessionID: req.SessionID, Status: "ready", Action: "would_copy", Issues: []string{}}, nil
}

func (f *Fake) ExecuteArtifactApply(_ context.Context, req agent.ArtifactApplyExecuteRequest) (agent.ArtifactApplyExecuteResult, error) {
	return agent.ArtifactApplyExecuteResult{AgentID: f.id, ApplyPlanID: req.ApplyPlanID, SessionID: req.SessionID, Status: "applied", Action: "copy", Issues: []string{}, CopiedBytes: 100}, nil
}

func (f *Fake) record(operation agent.Operation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, operation)
	if message := f.failures[operation]; message != "" {
		return agent.Error{AgentID: f.id, Operation: operation, Message: message}
	}
	return nil
}

func (f *Fake) Start(ctx context.Context, sessionID string) error {
	_, err := f.StartSession(ctx, agent.SessionRequest{SessionID: sessionID})
	return err
}

func (f *Fake) Stop(ctx context.Context, sessionID string) error {
	_, err := f.StopSession(ctx, agent.SessionRequest{SessionID: sessionID})
	return err
}

func (f *Fake) Restart(ctx context.Context, sessionID string) error {
	_, err := f.RestartSession(ctx, agent.SessionRequest{SessionID: sessionID})
	return err
}

func (f *Fake) Inspect(ctx context.Context, sessionID string) (agent.SessionStatus, error) {
	return f.InspectSession(ctx, sessionID)
}

func (f *Fake) PrepareSessionFiles(_ context.Context, request agent.SessionRequest) error {
	if err := f.record(agent.OperationPrepare); err != nil {
		return err
	}
	if request.SessionID == "" {
		return agent.Error{AgentID: f.id, Operation: agent.OperationPrepare, Message: "session id is required"}
	}
	return nil
}

func (f *Fake) RemoveSessionFiles(_ context.Context, sessionID string) error {
	if sessionID == "" {
		return agent.Error{AgentID: f.id, Operation: agent.OperationStop, Message: "session id is required"}
	}
	return nil
}

func (f *Fake) ListAppliedArtifacts(_ context.Context, sessionID string) (agent.AppliedArtifactsResponse, error) {
	return agent.AppliedArtifactsResponse{SessionID: sessionID, Records: []agent.AppliedArtifactRecord{}}, nil
}

func (f *Fake) InspectAppliedArtifact(_ context.Context, sessionID, applyPlanID string) (agent.AppliedArtifactRecord, error) {
	return agent.AppliedArtifactRecord{}, agent.ErrMaterializedArtifactNotFound
}

func (f *Fake) VerifyAppliedArtifact(_ context.Context, sessionID, applyPlanID string) (agent.AppliedArtifactVerification, error) {
	return agent.AppliedArtifactVerification{}, agent.ErrMaterializedArtifactNotFound
}

func (f *Fake) VerifyAllAppliedArtifacts(_ context.Context, sessionID string) (agent.BatchAppliedArtifactVerification, error) {
	return agent.BatchAppliedArtifactVerification{SessionID: sessionID, Total: 0, Entries: []agent.AppliedArtifactVerification{}}, nil
}

func (f *Fake) setStatus(sessionID, status string, running, frozen bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[sessionID] = agent.SessionStatus{AgentID: f.id, SessionID: sessionID, Status: status, Running: running, Frozen: frozen, RuntimeEndpoint: f.endpoint, RuntimeProfileID: runtimeprofile.DefaultProfileID, RuntimeType: string(runtimeprofile.TypeDummy), ObservedAt: f.now()}
}

func (f *Fake) result(message string) agent.OperationResult {
	return agent.OperationResult{AgentID: f.id, Status: "success", Message: message, Mode: "local"}
}

func (f *Fake) MaterializeEnvironment(_ context.Context, request agent.EnvironmentMaterializationRequest) (agent.EnvironmentMaterializationResult, error) {
	if err := f.record(agent.OperationMaterializeEnvironment); err != nil {
		return agent.EnvironmentMaterializationResult{}, err
	}
	return agent.EnvironmentMaterializationResult{
		SessionID:              request.SessionID,
		EnvironmentID:          request.EnvironmentID,
		EnvironmentName:        request.EnvironmentName,
		MinecraftVersion:       request.MinecraftVersion,
		JavaVersion:            request.JavaVersion,
		LoaderType:             request.LoaderType,
		LoaderVersion:          request.LoaderVersion,
		ServerCore:             request.ServerCore,
		MCDRRequired:           request.MCDRRequired,
		CarpetRequired:         request.CarpetRequired,
		RuntimeProfileID:       request.RuntimeProfileID,
		RuntimeProfileRequired: request.RuntimeProfileRequired,
		MaterializedAt:         f.now(),
		Status:                 "prepared",
		Directories:            []string{"config", "world", "logs", "mods"},
		Metadata:               map[string]string{},
	}, nil
}

func (f *Fake) GetSessionRuntimeStatus(_ context.Context, sessionID string) (agent.SessionRuntimeStatus, error) {
	if err := f.record(agent.OperationGetSessionRuntimeStatus); err != nil {
		return agent.SessionRuntimeStatus{}, err
	}
	return agent.SessionRuntimeStatus{
		SessionID:         sessionID,
		CheckedAt:         f.now(),
		RuntimeRootExists: true,
		SessionRootExists: false,
	}, nil
}

func (f *Fake) SessionReadyForStart(_ context.Context, sessionID string) (agent.SessionStartReadiness, error) {
	if err := f.record(agent.OperationSessionReadyForStart); err != nil {
		return agent.SessionStartReadiness{}, err
	}
	return agent.SessionStartReadiness{SessionID: sessionID, CheckedAt: f.now(), Ready: true, Status: "ready", Issues: []agent.SessionStartReadinessIssue{}, RuntimeStatusSummary: agent.SessionStartReadinessSummary{RuntimeRootExists: true, SessionRootExists: true, EnvironmentManifestExists: true, EnvironmentManifestStatus: "prepared", WorkDirExists: true, ConfigDirExists: true, LogsDirExists: true, ProcessState: "not_started"}}, nil
}

func (f *Fake) InspectMCDRConfigStub(_ context.Context, sessionID string) (agent.MCDRConfigStubInspection, error) {
	return agent.MCDRConfigStubInspection{SessionID: sessionID, Exists: false, Path: "", Valid: false, Status: "missing", CheckedAt: f.now()}, nil
}

func (f *Fake) SendCommand(_ context.Context, sessionID, command string) (agent.CommandResult, error) {
	if err := f.record(agent.OperationSendCommand); err != nil {
		return agent.CommandResult{}, err
	}
	if command == "" {
		return agent.CommandResult{}, agent.Error{AgentID: f.id, Operation: agent.OperationSendCommand, Message: "command is required"}
	}
	return agent.CommandResult{AgentID: f.id, Status: "sent", Message: "command sent"}, nil
}
