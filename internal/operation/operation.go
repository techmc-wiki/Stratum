package operation

import "time"

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusTimedOut  Status = "timed_out"
	StatusCancelled Status = "cancelled"
)

type Operation struct {
	ID             string            `json:"id"`
	RequestID      string            `json:"requestId"`
	IdempotencyKey string            `json:"idempotencyKey,omitempty"`
	ActorID        string            `json:"actorId"`
	Action         string            `json:"action"`
	TargetType     string            `json:"targetType"`
	TargetID       string            `json:"targetId"`
	ProjectID      string            `json:"projectId,omitempty"`
	SessionID      string            `json:"sessionId,omitempty"`
	Status         Status            `json:"status"`
	CreatedAt      time.Time         `json:"createdAt"`
	StartedAt      *time.Time        `json:"startedAt,omitempty"`
	CompletedAt    *time.Time        `json:"completedAt,omitempty"`
	PreviousState  string            `json:"previousState,omitempty"`
	IntendedState  string            `json:"intendedState,omitempty"`
	FinalState     string            `json:"finalState,omitempty"`
	AgentID        string            `json:"agentId,omitempty"`
	AgentMode      string            `json:"agentMode,omitempty"`
	AgentRequestID string            `json:"agentRequestId,omitempty"`
	Result         string            `json:"result,omitempty"`
	ErrorCode      string            `json:"errorCode,omitempty"`
	ErrorMessage   string            `json:"errorMessage,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

func (o Operation) Active() bool {
	return o.Status == StatusPending || o.Status == StatusRunning
}
