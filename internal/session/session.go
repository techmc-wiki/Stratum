package session

import (
	"fmt"
	"time"
)

type Type string

const (
	TypeShared   Type = "shared"
	TypeFork     Type = "fork"
	TypePrivate  Type = "private"
	TypeReview   Type = "review"
	TypeArchived Type = "archived"
)

type State string

const (
	StateCreated   State = "created"
	StatePreparing State = "preparing"
	StateStarting  State = "starting"
	StateRunning   State = "running"
	StateStopping  State = "stopping"
	StateStopped   State = "stopped"
	StateCrashed   State = "crashed"
	StateFrozen    State = "frozen"
	StateArchived  State = "archived"
	StateDeleted   State = "deleted"
)

var allowedTransitions = map[State]map[State]struct{}{
	StateCreated:   set(StatePreparing),
	StatePreparing: set(StateStarting),
	StateStarting:  set(StateRunning, StateCrashed),
	StateRunning:   set(StateStopping, StateCrashed, StateFrozen),
	StateStopping:  set(StateStopped),
	StateStopped:   set(StateStarting, StateArchived),
	StateCrashed:   set(StateStopped, StateArchived),
	StateFrozen:    set(StateRunning),
	StateArchived:  set(StateDeleted),
	StateDeleted:   {},
}

type ForkProvenance struct {
	SourceType             string            `json:"sourceType"`
	SourceID               string            `json:"sourceId"`
	SourceSessionID        string            `json:"sourceSessionId,omitempty"`
	SourceCheckpointID     string            `json:"sourceCheckpointId,omitempty"`
	CreatorID              string            `json:"creatorId"`
	Reason                 string            `json:"reason"`
	InheritedEnvironmentID string            `json:"inheritedEnvironmentId,omitempty"`
	InheritedArtifactIDs   []string          `json:"inheritedArtifactIds,omitempty"`
	InheritedServerConfig  map[string]string `json:"inheritedServerConfig,omitempty"`
	CreatedAt              time.Time         `json:"createdAt"`
}

type Session struct {
	ID                 string          `json:"id"`
	ProjectID          string          `json:"projectId"`
	RoomID             string          `json:"roomId,omitempty"`
	OwnerUserID        string          `json:"ownerUserId"`
	Type               Type            `json:"type"`
	State              State           `json:"state"`
	EnvironmentID      string          `json:"environmentId"`
	RuntimeProfileID   string          `json:"runtimeProfileId,omitempty"`
	SourceCheckpointID string          `json:"sourceCheckpointId,omitempty"`
	AssignedAgentID    string          `json:"assignedAgentId,omitempty"`
	LastAgentStatus    string          `json:"lastAgentStatus,omitempty"`
	LastRuntimeMessage string          `json:"lastRuntimeMessage,omitempty"`
	RuntimeEndpoint    string          `json:"runtimeEndpoint,omitempty"`
	ForkProvenance     *ForkProvenance `json:"forkProvenance,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	ExpiresAt          *time.Time      `json:"expiresAt,omitempty"`
	LastActiveAt       time.Time       `json:"lastActiveAt"`
}

func (s *Session) Transition(to State) error {
	if !CanTransition(s.State, to) {
		return fmt.Errorf("session %q cannot transition from %q to %q", s.ID, s.State, to)
	}
	s.State = to
	return nil
}

func CanTransition(from, to State) bool {
	next, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = next[to]
	return ok
}

func set(states ...State) map[State]struct{} {
	result := make(map[State]struct{}, len(states))
	for _, state := range states {
		result[state] = struct{}{}
	}
	return result
}
