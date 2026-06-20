package agentregistry

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/stratummc/stratum/internal/agent"
)

type Service struct {
	mu           sync.RWMutex
	repo         Repository
	staleTimeout time.Duration
	selectFn     func([]Agent, AgentSelectionRequest) (Agent, error)
}

func New(repo Repository, staleTimeout time.Duration) *Service {
	if staleTimeout <= 0 {
		staleTimeout = 30 * time.Second
	}
	return &Service{
		repo:         repo,
		staleTimeout: staleTimeout,
		selectFn:     defaultSelectAgent,
	}
}

func (s *Service) SetSelectFn(fn func([]Agent, AgentSelectionRequest) (Agent, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selectFn = fn
}

func (s *Service) Register(ctx context.Context, info agent.AgentInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.repo.GetAgent(info.ID)
	if err == nil && existing.Status == StatusOnline {
		return fmt.Errorf("agent %q already registered", info.ID)
	}

	now := time.Now().UTC()
	ag := Agent{
		ID:              info.ID,
		Name:            info.ID,
		Endpoint:        info.RuntimeEndpoint,
		Capabilities:    info.Capabilities,
		Mode:            info.Mode,
		Status:          StatusOnline,
		RegisteredAt:    now,
		LastHeartbeatAt: now,
	}

	if err := s.repo.SaveAgent(ag); err != nil {
		return fmt.Errorf("save agent %q: %w", info.ID, err)
	}
	return nil
}

func (s *Service) Heartbeat(ctx context.Context, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ag, err := s.repo.GetAgent(agentID)
	if err != nil {
		return fmt.Errorf("agent %q not found", agentID)
	}

	ag.LastHeartbeatAt = time.Now().UTC()
	ag.Status = StatusOnline

	if err := s.repo.SaveAgent(ag); err != nil {
		return fmt.Errorf("update agent %q heartbeat: %w", agentID, err)
	}
	return nil
}

func (s *Service) Deregister(ctx context.Context, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ag, err := s.repo.GetAgent(agentID)
	if err != nil {
		return err
	}

	ag.Status = StatusOffline
	return s.repo.SaveAgent(ag)
}

func (s *Service) List(ctx context.Context) ([]Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents, err := s.repo.ListAgents()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	result := make([]Agent, 0, len(agents))
	for _, ag := range agents {
		if ag.Status == StatusOnline && now.Sub(ag.LastHeartbeatAt) > s.staleTimeout {
			ag.Status = StatusStale
			_ = s.repo.SaveAgent(ag)
		}
		result = append(result, ag)
	}
	return result, nil
}

func (s *Service) SelectAgent(ctx context.Context, req AgentSelectionRequest) (Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents, err := s.repo.ListAgents()
	if err != nil {
		return Agent{}, err
	}

	online := filterOnline(agents, s.staleTimeout)
	if len(online) == 0 {
		return Agent{}, errors.New("no online agents available")
	}

	return s.selectFn(online, req)
}

func filterOnline(agents []Agent, staleTimeout time.Duration) []Agent {
	now := time.Now().UTC()
	var result []Agent
	for _, ag := range agents {
		if ag.Status == StatusOnline && now.Sub(ag.LastHeartbeatAt) <= staleTimeout {
			result = append(result, ag)
		}
	}
	return result
}

func defaultSelectAgent(agents []Agent, req AgentSelectionRequest) (Agent, error) {
	if req.PreferAgentID != "" {
		for _, ag := range agents {
			if ag.ID == req.PreferAgentID {
				return ag, nil
			}
		}
	}
	if len(req.MinCapabilities) > 0 {
		filtered := filterByCapabilities(agents, req.MinCapabilities)
		if len(filtered) > 0 {
			return filtered[rand.Intn(len(filtered))], nil
		}
	}
	return agents[rand.Intn(len(agents))], nil
}

func filterByCapabilities(agents []Agent, required []string) []Agent {
	var result []Agent
	for _, ag := range agents {
		if hasAllCapabilities(ag.Capabilities, required) {
			result = append(result, ag)
		}
	}
	return result
}

func hasAllCapabilities(have, need []string) bool {
	capSet := make(map[string]bool, len(have))
	for _, c := range have {
		capSet[c] = true
	}
	for _, c := range need {
		if !capSet[c] {
			return false
		}
	}
	return true
}
