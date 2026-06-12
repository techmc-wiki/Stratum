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

type ErrorResponse struct {
	Error     string `json:"error"`
	Operation string `json:"operation,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
	RequestID string `json:"requestId"`
}
