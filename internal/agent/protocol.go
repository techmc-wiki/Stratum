package agent

import (
	"context"
	"errors"
	"time"

	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

var ErrMaterializedArtifactNotFound = errors.New("materialized artifact not found")

type (
	requestIDContextKey   struct{}
	logMaxBytesContextKey struct{}
)

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
	OperationPrepare                 Operation = "prepare"
	OperationStart                   Operation = "start"
	OperationStop                    Operation = "stop"
	OperationRestart                 Operation = "restart"
	OperationFreeze                  Operation = "freeze"
	OperationUnfreeze                Operation = "unfreeze"
	OperationInspect                 Operation = "inspect"
	OperationCollectLogs             Operation = "collect-logs"
	OperationReportResources         Operation = "report-resources"
	OperationCreateCheckpoint        Operation = "create-checkpoint"
	OperationRestoreCheckpoint       Operation = "restore-checkpoint"
	OperationMaterializeArtifact     Operation = "materialize-artifact"
	OperationArtifactApply           Operation = "artifact-apply"
	OperationMaterializeEnvironment  Operation = "materialize-environment"
	OperationGetSessionRuntimeStatus Operation = "get-session-runtime-status"
	OperationSessionReadyForStart    Operation = "session-ready-for-start"
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

type MaterializedArtifact struct {
	AgentID             string
	SessionID           string
	ArtifactID          string
	StagingPlanID       string
	ArtifactName        string
	ArtifactType        string
	TargetName          string
	PayloadAlgorithm    string
	PayloadHash         string
	PayloadSize         int64
	RuntimeRelativePath string
	MaterializedAt      time.Time
	ActorID             string
	Status              string
	Metadata            map[string]string
}

type MaterializedArtifacts struct {
	AgentID   string
	SessionID string
	Status    string
	Items     []MaterializedArtifact
}

type MaterializedArtifactVerification struct {
	AgentID             string
	SessionID           string
	StagingPlanID       string
	ArtifactID          string
	TargetName          string
	RuntimeRelativePath string
	PayloadAlgorithm    string
	ExpectedHash        string
	ActualHash          string
	PayloadSize         int64
	ActualSize          int64
	Status              string
	VerifiedAt          time.Time
	ErrorMessage        string
}

type MaterializedArtifactsVerification struct {
	AgentID        string
	SessionID      string
	VerifiedAt     time.Time
	Total          int
	ValidCount     int
	MissingCount   int
	CorruptedCount int
	ErrorCount     int
	Entries        []MaterializedArtifactVerification
}

type ArtifactApplyDryRunRequest struct {
	ApplyPlanID        string
	SessionID          string
	StagingPlanID      string
	ArtifactID         string
	TargetRoot         string
	TargetRelativePath string
	ExpectedHash       string
	ExpectedSize       int64
}

type ArtifactApplyDryRunResult struct {
	AgentID                          string
	ApplyPlanID                      string
	SessionID                        string
	ArtifactID                       string
	StagingPlanID                    string
	ApplyKind                        string
	TargetRoot                       string
	TargetRelativePath               string
	SourceRuntimeRelativePath        string
	PlannedTargetRuntimeRelativePath string
	Action                           string
	Status                           string
	Issues                           []string
	CheckedAt                        time.Time
}

type ArtifactApplyExecuteRequest struct {
	ApplyPlanID        string
	SessionID          string
	StagingPlanID      string
	ArtifactID         string
	TargetRoot         string
	TargetRelativePath string
	ExpectedHash       string
	ExpectedSize       int64
}

type ArtifactApplyExecuteResult struct {
	AgentID            string
	ApplyPlanID        string
	SessionID          string
	ArtifactID         string
	StagingPlanID      string
	TargetRoot         string
	TargetRelativePath string
	SourcePath         string
	TargetPath         string
	Action             string
	Status             string
	Issues             []string
	CopiedBytes        int64
	VerifiedTargetHash string
	ExecutedAt         time.Time
}

type AppliedArtifactRecord struct {
	ApplyPlanID               string
	SessionID                 string
	ArtifactID                string
	StagingPlanID             string
	SourceRuntimeRelativePath string
	TargetRuntimeRelativePath string
	TargetRoot                string
	TargetRelativePath        string
	PayloadAlgorithm          string
	PayloadHash               string
	PayloadSize               int64
	Action                    string
	Status                    string
	ActorID                   string
	AppliedAt                 time.Time
}

type AppliedArtifactsResponse struct {
	SessionID string
	Records   []AppliedArtifactRecord
}

type AppliedArtifactVerification struct {
	SessionID                 string
	ApplyPlanID               string
	ArtifactID                string
	StagingPlanID             string
	TargetRoot                string
	TargetRelativePath        string
	TargetRuntimeRelativePath string
	PayloadAlgorithm          string
	ExpectedHash              string
	ActualHash                string
	PayloadSize               int64
	ActualSize                int64
	Status                    string
	VerifiedAt                time.Time
	ErrorMessage              string
}

type BatchAppliedArtifactVerification struct {
	SessionID      string
	VerifiedAt     time.Time
	Total          int
	ValidCount     int
	MissingCount   int
	CorruptedCount int
	ErrorCount     int
	Entries        []AppliedArtifactVerification
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

type SessionRuntimeStatus struct {
	SessionID             string
	CheckedAt             time.Time
	RuntimeRootExists     bool
	SessionRootExists     bool
	WorkDirExists         bool
	ConfigDirExists       bool
	LogsDirExists         bool
	ArtifactsDirExists    bool
	CheckpointsDirExists  bool
	TmpDirExists          bool
	EnvironmentManifest   *EnvironmentManifestStatus
	MCDRLayout            *MCDRLayoutStatus
	MaterializedArtifacts *MaterializedArtifactsStatus
	AppliedArtifacts      *AppliedArtifactsStatus
	ProcessStatus         *ProcessStatusSummary
}

type EnvironmentManifestStatus struct {
	Exists              bool
	Path                string
	RuntimeRelativePath string
	Status              string
	EnvironmentID       string
	MinecraftVersion    string
	LoaderType          string
	ServerCore          string
	RuntimeProfileID    string
	MCDRRequired        bool
	ErrorMessage        string
}

type SessionStartReadiness struct {
	SessionID            string
	CheckedAt            time.Time
	Ready                bool
	Status               string
	Issues               []SessionStartReadinessIssue
	RuntimeStatusSummary SessionStartReadinessSummary
}

type SessionStartReadinessIssue struct {
	Code     string
	Message  string
	Severity string
}

type SessionStartReadinessSummary struct {
	RuntimeRootExists         bool
	SessionRootExists         bool
	EnvironmentManifestExists bool
	EnvironmentManifestStatus string
	WorkDirExists             bool
	ConfigDirExists           bool
	LogsDirExists             bool
	ProcessState              string
	AppliedArtifactsTotal     int
	AppliedArtifactsValid     int
	AppliedArtifactsMissing   int
	AppliedArtifactsCorrupted int
	AppliedArtifactsError     int
}

type MCDRLayoutStatus struct {
	MCDRRootExists      bool
	ManifestExists      bool
	ManifestPath        string
	RuntimeRelativePath string
}

type MaterializedArtifactsStatus struct {
	ManifestExists      bool
	ManifestPath        string
	RuntimeRelativePath string
	Count               int
}

type AppliedArtifactsStatus struct {
	ManifestExists      bool
	ManifestPath        string
	RuntimeRelativePath string
	Count               int
}

type ProcessStatusSummary struct {
	Status           string
	RuntimeProfileID string
	PID              int
	Crashed          bool
	StartedAt        *time.Time
	StoppedAt        *time.Time
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
type MCDRConfigStubInspection struct {
	SessionID                   string
	Exists                      bool
	Path                        string
	Valid                       bool
	Status                      string
	PlannedConfigYMLPath        string
	PlannedServerPropertiesPath string
	PlannedEULAPath             string
	Issues                      []string
	CheckedAt                   time.Time
}

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
	InspectMaterializedArtifacts(context.Context, string) (MaterializedArtifacts, error)
	InspectMaterializedArtifact(context.Context, string, string) (MaterializedArtifact, error)
	VerifyMaterializedArtifact(context.Context, string, string) (MaterializedArtifactVerification, error)
	VerifyMaterializedArtifacts(context.Context, string) (MaterializedArtifactsVerification, error)
	DryRunArtifactApply(context.Context, ArtifactApplyDryRunRequest) (ArtifactApplyDryRunResult, error)
	ExecuteArtifactApply(context.Context, ArtifactApplyExecuteRequest) (ArtifactApplyExecuteResult, error)
	ListAppliedArtifacts(context.Context, string) (AppliedArtifactsResponse, error)
	InspectAppliedArtifact(context.Context, string, string) (AppliedArtifactRecord, error)
	VerifyAppliedArtifact(context.Context, string, string) (AppliedArtifactVerification, error)
	VerifyAllAppliedArtifacts(context.Context, string) (BatchAppliedArtifactVerification, error)
	MaterializeEnvironment(context.Context, EnvironmentMaterializationRequest) (EnvironmentMaterializationResult, error)
	GetSessionRuntimeStatus(context.Context, string) (SessionRuntimeStatus, error)
	SessionReadyForStart(context.Context, string) (SessionStartReadiness, error)
	InspectMCDRConfigStub(context.Context, string) (MCDRConfigStubInspection, error)
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
