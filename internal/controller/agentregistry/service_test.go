package agentregistry

import (
	"context"
	"testing"
	"time"

	"github.com/stratummc/stratum/internal/agent"
)

type memRepo struct {
	agents map[string]Agent
}

func (m *memRepo) SaveAgent(ag Agent) error {
	if m.agents == nil {
		m.agents = make(map[string]Agent)
	}
	m.agents[ag.ID] = ag
	return nil
}

func (m *memRepo) GetAgent(id string) (Agent, error) {
	ag, ok := m.agents[id]
	if !ok {
		return Agent{}, &notFoundError{id: id}
	}
	return ag, nil
}

func (m *memRepo) ListAgents() ([]Agent, error) {
	var result []Agent
	for _, ag := range m.agents {
		result = append(result, ag)
	}
	return result, nil
}

func (m *memRepo) DeleteAgent(id string) error {
	delete(m.agents, id)
	return nil
}

type notFoundError struct{ id string }

func (e *notFoundError) Error() string { return "agent not found: " + e.id }

func TestRegisterAgent(t *testing.T) {
	svc := New(&memRepo{}, 10*time.Second)
	err := svc.Register(context.Background(), agent.AgentInfo{
		ID:              "agent-1",
		RuntimeEndpoint: "http://10.0.0.1:8787",
		Capabilities:    []string{"start-session", "stop-session"},
		Mode:            "process",
	})
	if err != nil {
		t.Fatal(err)
	}

	agents, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].ID != "agent-1" || agents[0].Endpoint != "http://10.0.0.1:8787" {
		t.Fatalf("unexpected agents: %+v", agents)
	}
}

func TestRegisterDuplicateRejected(t *testing.T) {
	repo := &memRepo{}
	svc := New(repo, 10*time.Second)

	_ = svc.Register(context.Background(), agent.AgentInfo{ID: "agent-1", RuntimeEndpoint: "http://a:8787"})
	err := svc.Register(context.Background(), agent.AgentInfo{ID: "agent-1", RuntimeEndpoint: "http://b:8787"})
	if err == nil {
		t.Fatal("expected duplicate registration error")
	}
}

func TestHeartbeat(t *testing.T) {
	repo := &memRepo{}
	svc := New(repo, 10*time.Second)

	_ = svc.Register(context.Background(), agent.AgentInfo{ID: "agent-1", RuntimeEndpoint: "http://a:8787"})
	if err := svc.Heartbeat(context.Background(), "agent-1"); err != nil {
		t.Fatal(err)
	}

	agents, _ := svc.List(context.Background())
	if agents[0].LastHeartbeatAt.IsZero() {
		t.Fatal("heartbeat time not set")
	}
}

func TestDeregister(t *testing.T) {
	repo := &memRepo{}
	svc := New(repo, 10*time.Second)

	_ = svc.Register(context.Background(), agent.AgentInfo{ID: "agent-1", RuntimeEndpoint: "http://a:8787"})
	if err := svc.Deregister(context.Background(), "agent-1"); err != nil {
		t.Fatal(err)
	}

	agents, _ := svc.List(context.Background())
	if agents[0].Status != StatusOffline {
		t.Fatalf("expected offline, got %s", agents[0].Status)
	}
}

func TestSelectAgentPrefersSpecified(t *testing.T) {
	repo := &memRepo{}
	svc := New(repo, 1*time.Hour)
	_ = svc.Register(context.Background(), agent.AgentInfo{ID: "agent-a", RuntimeEndpoint: "http://a:8787"})
	_ = svc.Register(context.Background(), agent.AgentInfo{ID: "agent-b", RuntimeEndpoint: "http://b:8787"})

	selected, err := svc.SelectAgent(context.Background(), AgentSelectionRequest{PreferAgentID: "agent-b"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "agent-b" {
		t.Fatalf("selected %q, want agent-b", selected.ID)
	}
}

func TestSelectAgentFiltersByCapability(t *testing.T) {
	repo := &memRepo{}
	svc := New(repo, 1*time.Hour)
	_ = svc.Register(context.Background(), agent.AgentInfo{ID: "agent-a", Capabilities: []string{"start-session"}})
	_ = svc.Register(context.Background(), agent.AgentInfo{ID: "agent-b", Capabilities: []string{"send-command", "start-session"}})

	selected, err := svc.SelectAgent(context.Background(), AgentSelectionRequest{MinCapabilities: []string{"send-command"}})
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "agent-b" {
		t.Fatalf("selected %q, want agent-b (with send-command)", selected.ID)
	}
}

func TestSelectAgentNoOnlineAgents(t *testing.T) {
	repo := &memRepo{}
	svc := New(repo, 1*time.Hour)
	svc.SetSelectFn(func(agents []Agent, req AgentSelectionRequest) (Agent, error) {
		return defaultSelectAgent(agents, req)
	})

	_, err := svc.SelectAgent(context.Background(), AgentSelectionRequest{})
	if err == nil {
		t.Fatal("expected error for no agents")
	}
}

func TestStaleAgentsFiltered(t *testing.T) {
	repo := &memRepo{}
	svc := New(repo, 50*time.Millisecond)

	_ = svc.Register(context.Background(), agent.AgentInfo{ID: "agent-1", RuntimeEndpoint: "http://a:8787"})
	time.Sleep(100 * time.Millisecond)

	agents, _ := svc.List(context.Background())
	found := false
	for _, ag := range agents {
		if ag.ID == "agent-1" {
			found = true
			if ag.Status != StatusStale {
				t.Errorf("expected stale, got %s", ag.Status)
			}
		}
	}
	if !found {
		t.Fatal("agent should appear in list")
	}
}
