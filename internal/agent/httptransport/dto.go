package httptransport

import (
	"time"

	"github.com/stratummc/stratum/internal/agent/runtimeprofile"
)

type AgentInfoResponse struct {
	ID              string   `json:"id"`
	Status          string   `json:"status"`
	RuntimeEndpoint string   `json:"runtimeEndpoint"`
	Capabilities    []string `json:"capabilities"`
	RequestID       string   `json:"requestId"`
}

type ResourceReportResponse struct {
	AgentID         string    `json:"agentId"`
	CPUCapacity     int       `json:"cpuCapacity"`
	MemoryTotalMB   int       `json:"memoryTotalMb"`
	MemoryUsedMB    int       `json:"memoryUsedMb"`
	DiskTotalMB     int       `json:"diskTotalMb"`
	DiskUsedMB      int       `json:"diskUsedMb"`
	RunningSessions int       `json:"runningSessions"`
	ReportedAt      time.Time `json:"reportedAt"`
	RequestID       string    `json:"requestId"`
}

type SessionOperationRequest struct {
	ProjectID        string `json:"projectId,omitempty"`
	EnvironmentID    string `json:"environmentId,omitempty"`
	RuntimeProfileID string `json:"runtimeProfileId,omitempty"`
}

type SessionOperationResponse struct {
	AgentID   string `json:"agentId"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

type SessionInspectResponse struct {
	AgentID          string     `json:"agentId"`
	SessionID        string     `json:"sessionId"`
	Status           string     `json:"status"`
	Running          bool       `json:"running"`
	Frozen           bool       `json:"frozen"`
	RuntimeEndpoint  string     `json:"runtimeEndpoint"`
	ProcessID        string     `json:"processId,omitempty"`
	PID              int        `json:"pid,omitempty"`
	RuntimeMode      string     `json:"runtimeMode,omitempty"`
	RuntimeProfileID string     `json:"runtimeProfileId,omitempty"`
	RuntimeType      string     `json:"runtimeType,omitempty"`
	Crashed          bool       `json:"crashed"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	StoppedAt        *time.Time `json:"stoppedAt,omitempty"`
	ExitCode         *int       `json:"exitCode,omitempty"`
	LastError        string     `json:"lastError,omitempty"`
	ObservedAt       time.Time  `json:"observedAt"`
	SessionRoot      string     `json:"sessionRoot,omitempty"`
	WorkDir          string     `json:"workDir,omitempty"`
	LogsDir          string     `json:"logsDir,omitempty"`
	RequestID        string     `json:"requestId"`
}

type RuntimeProfilesResponse struct {
	AgentID   string                   `json:"agentId"`
	Profiles  []runtimeprofile.Profile `json:"profiles"`
	RequestID string                   `json:"requestId"`
}

type LogsResponse struct {
	AgentID   string   `json:"agentId"`
	SessionID string   `json:"sessionId"`
	Lines     []string `json:"lines"`
	RequestID string   `json:"requestId"`
}

type CheckpointStubRequest struct {
	SessionID    string `json:"sessionId"`
	CheckpointID string `json:"checkpointId"`
}

type CheckpointStubResponse = SessionOperationResponse

type ArtifactMaterializationRequest struct {
	SessionID        string `json:"sessionId"`
	ArtifactID       string `json:"artifactId"`
	StagingPlanID    string `json:"stagingPlanId"`
	ArtifactName     string `json:"artifactName"`
	ArtifactType     string `json:"artifactType"`
	TargetName       string `json:"targetName"`
	PayloadAlgorithm string `json:"payloadAlgorithm"`
	PayloadHash      string `json:"payloadHash"`
	PayloadSize      int64  `json:"payloadSize"`
	ActorID          string `json:"actorId"`
	Payload          []byte `json:"payload"`
}

type ArtifactMaterializationResponse struct {
	AgentID             string    `json:"agentId"`
	SessionID           string    `json:"sessionId"`
	ArtifactID          string    `json:"artifactId"`
	StagingPlanID       string    `json:"stagingPlanId"`
	TargetName          string    `json:"targetName"`
	RuntimeRelativePath string    `json:"runtimeRelativePath"`
	PayloadHash         string    `json:"payloadHash"`
	PayloadSize         int64     `json:"payloadSize"`
	MaterializedAt      time.Time `json:"materializedAt"`
	Idempotent          bool      `json:"idempotent"`
	Status              string    `json:"status"`
	RequestID           string    `json:"requestId"`
}

type MaterializedArtifactResponse struct {
	AgentID             string            `json:"agentId,omitempty"`
	SessionID           string            `json:"sessionId,omitempty"`
	ArtifactID          string            `json:"artifactId"`
	StagingPlanID       string            `json:"stagingPlanId,omitempty"`
	ArtifactName        string            `json:"artifactName,omitempty"`
	ArtifactType        string            `json:"artifactType,omitempty"`
	TargetName          string            `json:"targetName"`
	PayloadAlgorithm    string            `json:"payloadAlgorithm,omitempty"`
	PayloadHash         string            `json:"payloadHash"`
	PayloadSize         int64             `json:"payloadSize"`
	RuntimeRelativePath string            `json:"runtimeRelativePath"`
	MaterializedAt      time.Time         `json:"materializedAt,omitempty"`
	ActorID             string            `json:"actorId,omitempty"`
	Status              string            `json:"status,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	RequestID           string            `json:"requestId,omitempty"`
}

type MaterializedArtifactsResponse struct {
	AgentID   string                         `json:"agentId"`
	SessionID string                         `json:"sessionId"`
	Status    string                         `json:"status"`
	Items     []MaterializedArtifactResponse `json:"items"`
	RequestID string                         `json:"requestId"`
}

type MaterializedArtifactVerificationResponse struct {
	AgentID             string    `json:"agentId"`
	SessionID           string    `json:"sessionId"`
	StagingPlanID       string    `json:"stagingPlanId"`
	ArtifactID          string    `json:"artifactId"`
	TargetName          string    `json:"targetName"`
	RuntimeRelativePath string    `json:"runtimeRelativePath"`
	PayloadAlgorithm    string    `json:"payloadAlgorithm"`
	ExpectedHash        string    `json:"expectedHash"`
	ActualHash          string    `json:"actualHash,omitempty"`
	PayloadSize         int64     `json:"payloadSize"`
	ActualSize          int64     `json:"actualSize"`
	Status              string    `json:"status"`
	VerifiedAt          time.Time `json:"verifiedAt"`
	ErrorMessage        string    `json:"errorMessage,omitempty"`
	RequestID           string    `json:"requestId"`
}

type MaterializedArtifactsVerificationResponse struct {
	AgentID        string                                     `json:"agentId"`
	SessionID      string                                     `json:"sessionId"`
	VerifiedAt     time.Time                                  `json:"verifiedAt"`
	Total          int                                        `json:"total"`
	ValidCount     int                                        `json:"validCount"`
	MissingCount   int                                        `json:"missingCount"`
	CorruptedCount int                                        `json:"corruptedCount"`
	ErrorCount     int                                        `json:"errorCount"`
	Entries        []MaterializedArtifactVerificationResponse `json:"entries"`
	RequestID      string                                     `json:"requestId"`
}

type ArtifactApplyDryRunRequestDTO struct {
	ApplyPlanID        string `json:"applyPlanId"`
	SessionID          string `json:"sessionId"`
	StagingPlanID      string `json:"stagingPlanId"`
	ArtifactID         string `json:"artifactId"`
	TargetRoot         string `json:"targetRoot"`
	TargetRelativePath string `json:"targetRelativePath"`
	ExpectedHash       string `json:"expectedHash"`
	ExpectedSize       int64  `json:"expectedSize"`
}

type ArtifactApplyDryRunResultDTO struct {
	AgentID                          string    `json:"agentId"`
	ApplyPlanID                      string    `json:"applyPlanId"`
	SessionID                        string    `json:"sessionId"`
	ArtifactID                       string    `json:"artifactId"`
	StagingPlanID                    string    `json:"stagingPlanId"`
	ApplyKind                        string    `json:"applyKind"`
	TargetRoot                       string    `json:"targetRoot"`
	TargetRelativePath               string    `json:"targetRelativePath"`
	SourceRuntimeRelativePath        string    `json:"sourceRuntimeRelativePath"`
	PlannedTargetRuntimeRelativePath string    `json:"plannedTargetRuntimeRelativePath"`
	Action                           string    `json:"action"`
	Status                           string    `json:"status"`
	Issues                           []string  `json:"issues"`
	CheckedAt                        time.Time `json:"checkedAt"`
	RequestID                        string    `json:"requestId"`
}

type ErrorResponse struct {
	Error     string `json:"error"`
	Operation string `json:"operation,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
	RequestID string `json:"requestId"`
}
