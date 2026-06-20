package filesystem

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/stratummc/stratum/internal/controller/agentregistry"
)

type AgentRegistryStore struct {
	mu      sync.RWMutex
	dir     string
	agents  map[string]agentregistry.Agent
	loaded  bool
}

func NewAgentRegistryStore(dir string) *AgentRegistryStore {
	return &AgentRegistryStore{
		dir:    filepath.Join(dir, "agents"),
		agents: make(map[string]agentregistry.Agent),
	}
}

func (s *AgentRegistryStore) ensureLoaded() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		var ag agentregistry.Agent
		if json.Unmarshal(data, &ag) == nil {
			s.agents[ag.ID] = ag
		}
	}
	s.loaded = true
	return nil
}

func (s *AgentRegistryStore) SaveAgent(ag agentregistry.Agent) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[ag.ID] = ag
	data, err := json.MarshalIndent(ag, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, ag.ID+".json"), data, 0o640)
}

func (s *AgentRegistryStore) GetAgent(id string) (agentregistry.Agent, error) {
	if err := s.ensureLoaded(); err != nil {
		return agentregistry.Agent{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ag, ok := s.agents[id]
	if !ok {
		return agentregistry.Agent{}, fmt.Errorf("agent %q not found", id)
	}
	return ag, nil
}

func (s *AgentRegistryStore) ListAgents() ([]agentregistry.Agent, error) {
	if err := s.ensureLoaded(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]agentregistry.Agent, 0, len(s.agents))
	for _, ag := range s.agents {
		result = append(result, ag)
	}
	return result, nil
}

func (s *AgentRegistryStore) DeleteAgent(id string) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agents, id)
	return os.Remove(filepath.Join(s.dir, id+".json"))
}

var _ agentregistry.Repository = (*AgentRegistryStore)(nil)
