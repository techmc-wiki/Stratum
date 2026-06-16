package audit

import (
	"errors"
	"time"
)

type Event struct {
	ID         string            `json:"id"`
	ProjectID  string            `json:"projectId,omitempty"`
	ActorID    string            `json:"actorId"`
	Action     string            `json:"action"`
	TargetType string            `json:"targetType"`
	TargetID   string            `json:"targetId"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"createdAt"`
}

func NewEvent(id, actorID, action, targetType, targetID string, at time.Time) (Event, error) {
	if id == "" || actorID == "" || action == "" || targetType == "" || targetID == "" {
		return Event{}, errors.New("audit event requires id, actor, action, target type, and target id")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return Event{ID: id, ActorID: actorID, Action: action, TargetType: targetType, TargetID: targetID, Metadata: map[string]string{}, CreatedAt: at}, nil
}
