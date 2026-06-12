package artifactsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/util"
)

const ActionApproved = "artifact.approved"
const ActionRejected = "artifact.rejected"

type Repository interface {
	SaveArtifact(context.Context, artifact.Artifact) error
	GetArtifact(context.Context, string) (artifact.Artifact, error)
	ListArtifacts(context.Context) ([]artifact.Artifact, error)
	AppendAuditEvent(context.Context, audit.Event) error
}

type Service struct {
	repository Repository
	now        func() time.Time
	newID      func(string) (string, error)
}

func New(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }, newID: util.NewID}
}

func (s *Service) RegisterFile(ctx context.Context, id, name, path, uploader string, kind artifact.Type, versions, loaders []string) (artifact.Artifact, error) {
	if id == "" || name == "" || uploader == "" {
		return artifact.Artifact{}, fmt.Errorf("artifact requires id, name, and uploader")
	}
	hash, size, err := artifact.HashFile(path)
	if err != nil {
		return artifact.Artifact{}, fmt.Errorf("hash artifact: %w", err)
	}
	value := artifact.Artifact{ID: id, Name: name, Type: kind, UploaderID: uploader, SHA256: hash, SizeBytes: size, TargetMinecraftVersions: versions, LoaderCompatibility: loaders, Status: artifact.StatusPending, CreatedAt: time.Now().UTC()}
	if err := s.repository.SaveArtifact(ctx, value); err != nil {
		return artifact.Artifact{}, err
	}
	return value, nil
}

func (s *Service) List(ctx context.Context) ([]artifact.Artifact, error) {
	return s.repository.ListArtifacts(ctx)
}

func (s *Service) ApproveArtifact(ctx context.Context, id, actor, reason string) (artifact.Artifact, error) {
	return s.review(ctx, id, actor, reason, artifact.StatusApproved, ActionApproved)
}

func (s *Service) RejectArtifact(ctx context.Context, id, actor, reason string) (artifact.Artifact, error) {
	return s.review(ctx, id, actor, reason, artifact.StatusRejected, ActionRejected)
}

func (s *Service) review(ctx context.Context, id, actor, reason string, next artifact.Status, action string) (artifact.Artifact, error) {
	if strings.TrimSpace(actor) == "" {
		return artifact.Artifact{}, fmt.Errorf("reviewer actor is required")
	}
	if strings.TrimSpace(reason) == "" {
		return artifact.Artifact{}, fmt.Errorf("review reason is required")
	}
	value, err := s.repository.GetArtifact(ctx, id)
	if err != nil {
		return artifact.Artifact{}, fmt.Errorf("load artifact: %w", err)
	}
	previous := value.Status
	if previous != artifact.StatusPending {
		return artifact.Artifact{}, fmt.Errorf("artifact %q cannot transition from %q to %q by review", id, previous, next)
	}
	now := s.now()
	value.Status = next
	value.ReviewedBy = actor
	value.ReviewedAt = &now
	value.ReviewReason = reason
	if err := s.repository.SaveArtifact(ctx, value); err != nil {
		return artifact.Artifact{}, err
	}
	if err := s.audit(ctx, value, previous, next, actor, reason, action); err != nil {
		return artifact.Artifact{}, err
	}
	return value, nil
}

func (s *Service) audit(ctx context.Context, value artifact.Artifact, previous, next artifact.Status, actor, reason, action string) error {
	id, err := s.newID("audit")
	if err != nil {
		return err
	}
	return s.repository.AppendAuditEvent(ctx, audit.Event{
		ID: id, ActorID: actor, Action: action, TargetType: "artifact", TargetID: value.ID,
		Metadata: map[string]string{
			"artifactId":     value.ID,
			"artifactName":   value.Name,
			"previousStatus": string(previous),
			"nextStatus":     string(next),
			"reviewer":       actor,
			"reason":         reason,
		},
		CreatedAt: s.now(),
	})
}
