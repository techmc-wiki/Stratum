package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/stratummc/stratum/internal/artifact"
	artifactapply "github.com/stratummc/stratum/internal/artifact/apply"
	artifactstaging "github.com/stratummc/stratum/internal/artifact/staging"
	"github.com/stratummc/stratum/internal/audit"
	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/operation"
	"github.com/stratummc/stratum/internal/project"
	"github.com/stratummc/stratum/internal/room"
	"github.com/stratummc/stratum/internal/session"
)

type Store struct {
	mu           sync.RWMutex
	Projects     map[string]project.Project
	Rooms        map[string]room.Room
	Sessions     map[string]session.Session
	Checkpoints  map[string]checkpoint.Checkpoint
	Artifacts    map[string]artifact.Artifact
	Environments map[string]environment.Environment
	Plans        map[string]artifactstaging.Plan
	ApplyPlans   map[string]artifactapply.Plan
	Operations   map[string]operation.Operation
	AuditEvents  []audit.Event
}

func New() *Store {
	return &Store{Projects: map[string]project.Project{}, Rooms: map[string]room.Room{}, Sessions: map[string]session.Session{}, Checkpoints: map[string]checkpoint.Checkpoint{}, Artifacts: map[string]artifact.Artifact{}, Environments: map[string]environment.Environment{}, Plans: map[string]artifactstaging.Plan{}, ApplyPlans: map[string]artifactapply.Plan{}, Operations: map[string]operation.Operation{}}
}

func (s *Store) ListProjects(_ context.Context) ([]project.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]project.Project, 0, len(s.Projects))
	for _, value := range s.Projects {
		result = append(result, value)
	}
	return result, nil
}

func (s *Store) GetProject(_ context.Context, id string) (project.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.Projects[id]
	if !ok {
		return project.Project{}, fmt.Errorf("project %q not found", id)
	}
	return value, nil
}

func (s *Store) ListRooms(_ context.Context) ([]room.Room, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]room.Room, 0, len(s.Rooms))
	for _, value := range s.Rooms {
		result = append(result, value)
	}
	return result, nil
}

func (s *Store) SaveSession(_ context.Context, value session.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sessions[value.ID] = value
	return nil
}

func (s *Store) GetSession(_ context.Context, id string) (session.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.Sessions[id]
	if !ok {
		return session.Session{}, fmt.Errorf("session %q not found", id)
	}
	return value, nil
}

func (s *Store) ListSessions(_ context.Context) ([]session.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]session.Session, 0, len(s.Sessions))
	for _, value := range s.Sessions {
		result = append(result, value)
	}
	return result, nil
}

func (s *Store) DeleteSession(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Sessions[id]; !ok {
		return fmt.Errorf("session %q not found", id)
	}
	delete(s.Sessions, id)
	return nil
}

func (s *Store) AppendAuditEvent(_ context.Context, value audit.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AuditEvents = append(s.AuditEvents, value)
	return nil
}

func (s *Store) ListAuditEvents(_ context.Context) ([]audit.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]audit.Event(nil), s.AuditEvents...), nil
}

func (s *Store) CreateOperation(_ context.Context, value operation.Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Operations[value.ID]; ok {
		return fmt.Errorf("operation %q already exists", value.ID)
	}
	s.Operations[value.ID] = value
	return nil
}

func (s *Store) GetOperation(_ context.Context, id string) (operation.Operation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.Operations[id]
	if !ok {
		return operation.Operation{}, fmt.Errorf("operation %q not found", id)
	}
	return value, nil
}

func (s *Store) ListOperations(_ context.Context) ([]operation.Operation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]operation.Operation, 0, len(s.Operations))
	for _, value := range s.Operations {
		result = append(result, value)
	}
	return result, nil
}

func (s *Store) UpdateOperation(_ context.Context, value operation.Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Operations[value.ID]; !ok {
		return fmt.Errorf("operation %q not found", value.ID)
	}
	s.Operations[value.ID] = value
	return nil
}

func (s *Store) SaveCheckpoint(_ context.Context, value checkpoint.Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Checkpoints[value.ID] = value
	return nil
}

func (s *Store) ListCheckpoints(_ context.Context) ([]checkpoint.Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]checkpoint.Checkpoint, 0, len(s.Checkpoints))
	for _, value := range s.Checkpoints {
		result = append(result, value)
	}
	return result, nil
}

func (s *Store) ListCheckpointsBySession(_ context.Context, sessionID string) ([]checkpoint.Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []checkpoint.Checkpoint
	for _, cp := range s.Checkpoints {
		if cp.SourceSessionID == sessionID {
			result = append(result, cp)
		}
	}
	return result, nil
}

func (s *Store) SaveArtifact(_ context.Context, value artifact.Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Artifacts[value.ID] = value
	return nil
}

func (s *Store) CreateArtifact(_ context.Context, value artifact.Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Artifacts[value.ID]; ok {
		return fmt.Errorf("artifact %q already exists", value.ID)
	}
	s.Artifacts[value.ID] = value
	return nil
}

func (s *Store) GetArtifact(_ context.Context, id string) (artifact.Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.Artifacts[id]
	if !ok {
		return artifact.Artifact{}, fmt.Errorf("artifact %q not found", id)
	}
	return value, nil
}

func (s *Store) ListArtifacts(_ context.Context) ([]artifact.Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]artifact.Artifact, 0, len(s.Artifacts))
	for _, value := range s.Artifacts {
		result = append(result, value)
	}
	return result, nil
}

func (s *Store) CreateArtifactStagingPlan(_ context.Context, value artifactstaging.Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Plans[value.ID]; ok {
		return fmt.Errorf("artifact staging plan %q already exists", value.ID)
	}
	s.Plans[value.ID] = value
	return nil
}

func (s *Store) GetArtifactStagingPlan(_ context.Context, id string) (artifactstaging.Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.Plans[id]
	if !ok {
		return artifactstaging.Plan{}, fmt.Errorf("artifact staging plan %q not found", id)
	}
	return value, nil
}

func (s *Store) ListArtifactStagingPlans(_ context.Context) ([]artifactstaging.Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]artifactstaging.Plan, 0, len(s.Plans))
	for _, value := range s.Plans {
		result = append(result, value)
	}
	return result, nil
}

func (s *Store) ListArtifactStagingPlansBySession(ctx context.Context, sessionID string) ([]artifactstaging.Plan, error) {
	values, err := s.ListArtifactStagingPlans(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]artifactstaging.Plan, 0)
	for _, value := range values {
		if value.SessionID == sessionID {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Store) ListArtifactStagingPlansByArtifact(ctx context.Context, artifactID string) ([]artifactstaging.Plan, error) {
	values, err := s.ListArtifactStagingPlans(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]artifactstaging.Plan, 0)
	for _, value := range values {
		if value.ArtifactID == artifactID {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Store) CreateArtifactApplyPlan(_ context.Context, value artifactapply.Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ApplyPlans[value.ID]; ok {
		return fmt.Errorf("artifact apply plan %q already exists", value.ID)
	}
	s.ApplyPlans[value.ID] = value
	return nil
}

func (s *Store) GetArtifactApplyPlan(_ context.Context, id string) (artifactapply.Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.ApplyPlans[id]
	if !ok {
		return artifactapply.Plan{}, fmt.Errorf("artifact apply plan %q not found", id)
	}
	return value, nil
}

func (s *Store) ListArtifactApplyPlans(_ context.Context) ([]artifactapply.Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]artifactapply.Plan, 0, len(s.ApplyPlans))
	for _, value := range s.ApplyPlans {
		result = append(result, value)
	}
	return result, nil
}

func (s *Store) ListArtifactApplyPlansBySession(ctx context.Context, sessionID string) ([]artifactapply.Plan, error) {
	values, err := s.ListArtifactApplyPlans(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]artifactapply.Plan, 0)
	for _, value := range values {
		if value.SessionID == sessionID {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Store) CreateEnvironment(_ context.Context, value environment.Environment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Environments[value.ID]; ok {
		return fmt.Errorf("environment %q already exists", value.ID)
	}
	s.Environments[value.ID] = value
	return nil
}

func (s *Store) GetEnvironment(_ context.Context, id string) (environment.Environment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.Environments[id]
	if !ok {
		return environment.Environment{}, fmt.Errorf("environment %q not found", id)
	}
	return value, nil
}

func (s *Store) ListEnvironments(_ context.Context) ([]environment.Environment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]environment.Environment, 0, len(s.Environments))
	for _, value := range s.Environments {
		result = append(result, value)
	}
	return result, nil
}
