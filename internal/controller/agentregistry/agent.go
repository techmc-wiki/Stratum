package agentregistry

import "time"

type AgentStatus string

const (
	StatusOnline  AgentStatus = "online"
	StatusOffline AgentStatus = "offline"
	StatusStale   AgentStatus = "stale"
)

type Agent struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Endpoint        string            `json:"endpoint"`
	Capabilities    []string          `json:"capabilities"`
	Mode            string            `json:"mode"`
	Status          AgentStatus       `json:"status"`
	RegisteredAt    time.Time         `json:"registeredAt"`
	LastHeartbeatAt time.Time         `json:"lastHeartbeatAt"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type AgentSelectionRequest struct {
	MinCapabilities []string
	PreferAgentID   string
}

type Repository interface {
	SaveAgent(agent Agent) error
	GetAgent(id string) (Agent, error)
	ListAgents() ([]Agent, error)
	DeleteAgent(id string) error
}
