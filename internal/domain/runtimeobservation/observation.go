package runtimeobservation

import "time"

type MismatchType string
type Severity string
type RecommendedAction string

const (
	MismatchNone                          MismatchType = "none"
	MismatchControllerRunningAgentStopped MismatchType = "controller_running_agent_stopped"
	MismatchControllerRunningAgentCrashed MismatchType = "controller_running_agent_crashed"
	MismatchControllerStoppedAgentRunning MismatchType = "controller_stopped_agent_running"
	MismatchControllerUnknownAgentKnown   MismatchType = "controller_unknown_agent_known"
	MismatchAgentUnknownControllerRunning MismatchType = "agent_unknown_controller_running"
	MismatchAssignedAgent                 MismatchType = "assigned_agent_mismatch"
	MismatchRuntimeProfile                MismatchType = "runtime_profile_mismatch"
	MismatchStaleObservation              MismatchType = "stale_observation"
	MismatchUnknown                       MismatchType = "unknown"

	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"

	ActionNone           RecommendedAction = "none"
	ActionInspect        RecommendedAction = "inspect"
	ActionMarkCrashed    RecommendedAction = "mark_crashed"
	ActionMarkStopped    RecommendedAction = "mark_stopped"
	ActionStopRuntime    RecommendedAction = "stop_runtime"
	ActionRestartRuntime RecommendedAction = "restart_runtime"
	ActionManualReview   RecommendedAction = "manual_review"
)

type ResourceSnapshot struct {
	CPUCapacity     int       `json:"cpuCapacity,omitempty"`
	MemoryTotalMB   int       `json:"memoryTotalMB,omitempty"`
	MemoryUsedMB    int       `json:"memoryUsedMB,omitempty"`
	DiskTotalMB     int       `json:"diskTotalMB,omitempty"`
	DiskUsedMB      int       `json:"diskUsedMB,omitempty"`
	RunningSessions int       `json:"runningSessions,omitempty"`
	ReportedAt      time.Time `json:"reportedAt,omitempty"`
}

type Observation struct {
	ID                     string            `json:"id"`
	SessionID              string            `json:"sessionId"`
	ProjectID              string            `json:"projectId,omitempty"`
	RoomID                 string            `json:"roomId,omitempty"`
	ObservedAt             time.Time         `json:"observedAt"`
	ObserverAgentID        string            `json:"observerAgentId,omitempty"`
	ControllerSessionState string            `json:"controllerSessionState"`
	AgentRuntimeStatus     string            `json:"agentRuntimeStatus,omitempty"`
	RuntimeProfileID       string            `json:"runtimeProfileId,omitempty"`
	ProcessID              string            `json:"processId,omitempty"`
	PID                    int               `json:"pid,omitempty"`
	ExitCode               *int              `json:"exitCode,omitempty"`
	Crashed                bool              `json:"crashed"`
	LastError              string            `json:"lastError,omitempty"`
	LogsAvailable          bool              `json:"logsAvailable"`
	ResourceSnapshot       *ResourceSnapshot `json:"resourceSnapshot,omitempty"`
	MismatchDetected       bool              `json:"mismatchDetected"`
	MismatchType           MismatchType      `json:"mismatchType"`
	Severity               Severity          `json:"severity"`
	RecommendedAction      RecommendedAction `json:"recommendedAction"`
	Metadata               map[string]string `json:"metadata,omitempty"`
}
