package fork

import (
	"context"
	"fmt"
	"time"

	"github.com/stratummc/stratum/internal/audit"
	"github.com/stratummc/stratum/internal/checkpoint"
	"github.com/stratummc/stratum/internal/checkpoint/consistency"
	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/idgen"
	"github.com/stratummc/stratum/internal/room"
	"github.com/stratummc/stratum/internal/session"
)

type Repository interface {
	GetSession(ctx context.Context, id string) (session.Session, error)
	GetRoom(ctx context.Context, id string) (room.Room, error)
	GetCheckpoint(ctx context.Context, id string) (checkpoint.Checkpoint, error)
	GetEnvironment(ctx context.Context, id string) (environment.Environment, error)
	SaveSession(ctx context.Context, value session.Session) error
	CreateCheckpoint(ctx context.Context, cp checkpoint.Checkpoint) error
	AppendAuditEvent(ctx context.Context, event audit.Event) error
}

type Service struct {
	repo  Repository
	now   func() time.Time
	newID func(string) (string, error)
}

func New(repo Repository) *Service {
	return &Service{
		repo:  repo,
		now:   func() time.Time { return time.Now().UTC() },
		newID: idgen.NewID,
	}
}

func (s *Service) CreateFork(ctx context.Context, opts ForkOptions) (session.Session, error) {
	if opts.SourceType == "" || opts.SourceID == "" || opts.CreatorID == "" || opts.ProjectID == "" {
		return session.Session{}, fmt.Errorf("fork requires source type, source id, creator, and project")
	}

	if opts.SourceType != SourceTypeRoom && opts.SourceType != SourceTypeSession && opts.SourceType != SourceTypeCheckpoint {
		return session.Session{}, fmt.Errorf("invalid source type %q", opts.SourceType)
	}

	sourceSession, err := s.resolveSourceSession(ctx, opts)
	if err != nil {
		return session.Session{}, fmt.Errorf("resolve source: %w", err)
	}

	sessionID := opts.ID
	if sessionID == "" {
		id, err := s.newID("session")
		if err != nil {
			return session.Session{}, fmt.Errorf("generate session id: %w", err)
		}
		sessionID = id
	}

	envID := opts.EnvironmentID
	if envID == "" {
		envID = sourceSession.EnvironmentID
	}
	if _, err := s.repo.GetEnvironment(ctx, envID); err != nil {
		return session.Session{}, fmt.Errorf("environment %q: %w", envID, err)
	}

	var preForkCheckpointID string
	if !opts.SkipCheckpoint && opts.SourceType != SourceTypeCheckpoint && sourceSession.ID != "" {
		preForkCheckpointID, err = s.createPreForkCheckpoint(ctx, sourceSession, opts)
		if err != nil {
			return session.Session{}, fmt.Errorf("pre-fork checkpoint: %w", err)
		}
	}

	now := s.now()
	forkSession := session.Session{
		ID:                 sessionID,
		ProjectID:          opts.ProjectID,
		RoomID:             opts.RoomID,
		OwnerUserID:        opts.CreatorID,
		Type:               session.TypeFork,
		State:              session.StateCreated,
		EnvironmentID:      envID,
		RuntimeProfileID:   opts.RuntimeProfileID,
		SourceCheckpointID: opts.SourceCheckpointID,
		CreatedAt:          now,
		LastActiveAt:       now,
	}

	if opts.TTL != nil && *opts.TTL > 0 {
		expiresAt := now.Add(*opts.TTL)
		forkSession.ExpiresAt = &expiresAt
	}

	forkSession.ForkProvenance = &session.ForkProvenance{
		SourceType:             string(opts.SourceType),
		SourceID:               opts.SourceID,
		SourceSessionID:        sourceSession.ID,
		SourceCheckpointID:     opts.SourceCheckpointID,
		CreatorID:              opts.CreatorID,
		Reason:                 opts.Reason,
		PreForkCheckpointID:    preForkCheckpointID,
		InheritedEnvironmentID: envID,
		InheritedArtifactIDs:   cloneSlice(opts.InheritedArtifactIDs),
		InheritedServerConfig:  cloneMap(opts.InheritedServerConfig),
		CreatedAt:              now,
	}

	if err := s.repo.SaveSession(ctx, forkSession); err != nil {
		return session.Session{}, fmt.Errorf("save fork session: %w", err)
	}

	auditErr := s.recordAudit(ctx, forkSession, preForkCheckpointID, opts)
	if auditErr != nil {
		return forkSession, fmt.Errorf("fork session created but audit failed: %w", auditErr)
	}

	return forkSession, nil
}

func (s *Service) resolveSourceSession(ctx context.Context, opts ForkOptions) (session.Session, error) {
	switch opts.SourceType {
	case SourceTypeSession:
		sess, err := s.repo.GetSession(ctx, opts.SourceID)
		if err != nil {
			return session.Session{}, fmt.Errorf("source session %q: %w", opts.SourceID, err)
		}
		return sess, nil

	case SourceTypeRoom:
		rm, err := s.repo.GetRoom(ctx, opts.SourceID)
		if err != nil {
			return session.Session{}, fmt.Errorf("source room %q: %w", opts.SourceID, err)
		}
		if rm.SharedSessionID == "" {
			return session.Session{}, fmt.Errorf("room %q has no shared session", opts.SourceID)
		}
		sess, err := s.repo.GetSession(ctx, rm.SharedSessionID)
		if err != nil {
			return session.Session{}, fmt.Errorf("shared session %q of room %q: %w", rm.SharedSessionID, opts.SourceID, err)
		}
		return sess, nil

	case SourceTypeCheckpoint:
		cp, err := s.repo.GetCheckpoint(ctx, opts.SourceID)
		if err != nil {
			return session.Session{}, fmt.Errorf("source checkpoint %q: %w", opts.SourceID, err)
		}
		result := session.Session{
			ProjectID:        cp.ProjectID,
			RoomID:           cp.RoomID,
			EnvironmentID:    cp.EnvironmentID,
			RuntimeProfileID: cp.RuntimeProfileID,
		}
		if cp.SourceSessionID != "" {
			if sess, err := s.repo.GetSession(ctx, cp.SourceSessionID); err == nil {
				result = sess
			}
		}
		return result, nil

	default:
		return session.Session{}, fmt.Errorf("unexpected source type %q", opts.SourceType)
	}
}

func (s *Service) createPreForkCheckpoint(ctx context.Context, sourceSession session.Session, opts ForkOptions) (string, error) {
	cpID, err := s.newID("checkpoint")
	if err != nil {
		return "", fmt.Errorf("generate checkpoint id: %w", err)
	}
	cp, err := checkpoint.New(checkpoint.CreateParams{
		ID:               cpID,
		ProjectID:        sourceSession.ProjectID,
		RoomID:           sourceSession.RoomID,
		SourceSessionID:  sourceSession.ID,
		CreatorID:        opts.CreatorID,
		Kind:             checkpoint.KindPreOperation,
		Status:           checkpoint.StatusMetadataOnly,
		ConsistencyLevel: consistency.LevelMetadataOnly,
		EnvironmentID:    sourceSession.EnvironmentID,
		RuntimeProfileID: sourceSession.RuntimeProfileID,
		Notes:            "pre-fork checkpoint created before forking session " + opts.SourceID,
	})
	if err != nil {
		return "", err
	}
	if err := s.repo.CreateCheckpoint(ctx, cp); err != nil {
		return "", err
	}
	return cpID, nil
}

func (s *Service) recordAudit(ctx context.Context, forkSession session.Session, preForkCheckpointID string, opts ForkOptions) error {
	eventID, err := s.newID("audit")
	if err != nil {
		return err
	}
	event, err := audit.NewEvent(eventID, opts.CreatorID, "session.fork", "session", forkSession.ID, s.now())
	if err != nil {
		return err
	}
	event.ProjectID = forkSession.ProjectID
	event.Metadata = map[string]string{
		"sourceType":          string(opts.SourceType),
		"sourceId":            opts.SourceID,
		"forkSessionId":       forkSession.ID,
		"environmentId":       forkSession.EnvironmentID,
		"reason":              opts.Reason,
		"preForkCheckpointId": preForkCheckpointID,
		"sourceSessionId":     forkSession.ForkProvenance.SourceSessionID,
	}
	if opts.SourceCheckpointID != "" {
		event.Metadata["sourceCheckpointId"] = opts.SourceCheckpointID
	}
	return s.repo.AppendAuditEvent(ctx, event)
}

func cloneSlice(source []string) []string {
	if source == nil {
		return nil
	}
	result := make([]string, len(source))
	copy(result, source)
	return result
}

func cloneMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
