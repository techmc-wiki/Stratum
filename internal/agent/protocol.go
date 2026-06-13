package agent

import (
	"context"
	"time"

	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

type requestIDContextKey struct{}
type logMaxBytesContextKey struct{}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

func WithLogMaxBytes(ctx context.Context, maxBytes int) context.Context {
	return context.WithValue(ctx, logMaxBytesContextKey{}, maxBytes)
}

func LogMaxBytesFromContext(ctx context.Context) int {
	value, _ := ctx.Value(logMaxBytesContextKey{}).(int)
	return value
}

type Operation string

const (
	OperationPrepare             Operation = "prepare"
	OperationStart               Operation = "start"
	OperationStop                Operation = "stop"
	OperationRestart             Operation = "restart"
	OperationFreeze              Operation = "freeze"
	OperationUnfreeze            Operation = "unfreeze"
	OperationInspect             Operation = "inspect"
	OperationCollectLogs         Operation = "collect-logs"
	OperationReportResources     Operation = "report-resources"
	OperationCreateCheckpoint    Operation = "create-checkpoint"
	OperationRestoreCheckpoint   Operation = "restore-checkpoint"
	OperationMaterializeArtifact Operation = "materialize-artifact"
)

const MaxArtifactPayloadBytes = 64 << 20

type SessionRequest struct {
	SessionID        string
	ProjectID        string
	EnvironmentID    string
	RuntimeProfileID string
}

type CheckpointRequest struct {
	SessionID    string
	CheckpointID string
}

type ArtifactMaterializationRequest struct {
	SessionID        string
	ArtifactID       string
	StagingPlanID    string
	ArtifactName     string
	ArtifactType     string
	TargetName       string
	PayloadAlgorithm string
	PayloadHash      string
	PayloadSize      int64
	ActorID          string
	Payload          []byte
}

type ArtifactMaterializationResult struct {
	AgentID             string
	SessionID           string
	ArtifactID          string
	StagingPlanID       string
	TargetName          string
	RuntimeRelativePath string
	PayloadHash         string
	PayloadSize         int64
	MaterializedAt      time.Time
	Idempotent          bool
	Status              string
}

type OperationResult struct {
	AgentID string
	Status  string
	Message string
	Mode    string
}

type Error struct {
	AgentID   string
	Operation Operation
	Message   string
}

func (e Error) Error() string {
	return "agent " + e.AgentID + " " + string(e.Operation) + " failed: " + e.Message
}

type SessionStatus struct {
	AgentID          string
	SessionID        string
	Status           string
	Running          bool
	Frozen           bool
	RuntimeEndpoint  string
	ProcessID        string
	PID              int
	RuntimeMode      string
	RuntimeProfileID string
	RuntimeType      string
	Crashed          bool
	StartedAt        *time.Time
	StoppedAt        *time.Time
	ExitCode         *int
	LastError        string
	ObservedAt       time.Time
	SessionRoot      string
	WorkDir          string
	LogsDir          string
}

type LogBatch struct {
	AgentID   string
	SessionID string
	Lines     []string
}

type ResourceReport struct {
	AgentID         string
	CPUCapacity     int
	MemoryTotalMB   int
	MemoryUsedMB    int
	DiskTotalMB     int
	DiskUsedMB      int
	RunningSessions int
	ReportedAt      time.Time
}

type AgentInfo struct {
	ID              string
	Status          string
	RuntimeEndpoint string
	Capabilities    []string
	Mode            string
}

// AgentClient is the controller-facing protocol. A future remote transport can
// implement this interface without changing controller services.
type AgentClient interface {
	Info(context.Context) (AgentInfo, error)
	RuntimeProfiles(context.Context) ([]runtimeprofile.Profile, error)
	PrepareSession(context.Context, SessionRequest) (OperationResult, error)
	StartSession(context.Context, SessionRequest) (OperationResult, error)
	StopSession(context.Context, SessionRequest) (OperationResult, error)
	RestartSession(context.Context, SessionRequest) (OperationResult, error)
	FreezeSession(context.Context, SessionRequest) (OperationResult, error)
	UnfreezeSession(context.Context, SessionRequest) (OperationResult, error)
	InspectSession(context.Context, string) (SessionStatus, error)
	CollectLogs(context.Context, string) (LogBatch, error)
	ReportResources(context.Context) (ResourceReport, error)
	CreateCheckpointStub(context.Context, CheckpointRequest) (OperationResult, error)
	RestoreCheckpointStub(context.Context, CheckpointRequest) (OperationResult, error)
	MaterializeArtifact(context.Context, ArtifactMaterializationRequest) (ArtifactMaterializationResult, error)
}

type RuntimeAgent interface {
	PrepareSession(context.Context, SessionRequest) (OperationResult, error)
	StartSession(context.Context, SessionRequest) (OperationResult, error)
	StopSession(context.Context, SessionRequest) (OperationResult, error)
	RestartSession(context.Context, SessionRequest) (OperationResult, error)
	FreezeSession(context.Context, SessionRequest) (OperationResult, error)
	UnfreezeSession(context.Context, SessionRequest) (OperationResult, error)
	InspectSession(context.Context, string) (SessionStatus, error)
}

type ProcessSupervisor interface {
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Restart(context.Context, string) error
	Inspect(context.Context, string) (SessionStatus, error)
}

type LogStreamer interface {
	CollectLogs(context.Context, string) (LogBatch, error)
}

type FileOperator interface {
	PrepareSessionFiles(context.Context, SessionRequest) error
	RemoveSessionFiles(context.Context, string) error
}

type ResourceReporter interface {
	ReportResources(context.Context) (ResourceReport, error)
}

type CheckpointWorker interface {
	CreateCheckpointStub(context.Context, CheckpointRequest) (OperationResult, error)
	RestoreCheckpointStub(context.Context, CheckpointRequest) (OperationResult, error)
}
