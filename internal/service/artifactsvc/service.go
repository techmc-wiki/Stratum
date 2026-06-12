package artifactsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/project"
	"github.com/stratummc/stratum/internal/util"
)

const ActionCreated = "artifact.created"
const ActionApproved = "artifact.approved"
const ActionRejected = "artifact.rejected"

type Repository interface {
	CreateArtifact(context.Context, artifact.Artifact) error
	SaveArtifact(context.Context, artifact.Artifact) error
	GetArtifact(context.Context, string) (artifact.Artifact, error)
	ListArtifacts(context.Context) ([]artifact.Artifact, error)
	GetProject(context.Context, string) (project.Project, error)
	AppendAuditEvent(context.Context, audit.Event) error
}

type Service struct {
	repository Repository
	now        func() time.Time
	newID      func(string) (string, error)
}

func (s *Service) CreateMetadata(ctx context.Context, id, name string, kind artifact.Type, projectID, actor string) (artifact.Artifact, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(projectID) == "" || strings.TrimSpace(actor) == "" {
		return artifact.Artifact{}, fmt.Errorf("artifact requires id, name, type, project, and actor")
	}
	if err := artifact.ValidateType(kind); err != nil {
		return artifact.Artifact{}, err
	}
	if _, err := s.repository.GetProject(ctx, projectID); err != nil {
		return artifact.Artifact{}, fmt.Errorf("load project: %w", err)
	}
	value := artifact.Artifact{
		ID: id, ProjectID: projectID, Name: name, Type: kind, UploaderID: actor,
		PayloadStatus: artifact.PayloadMetadataOnly, TargetMinecraftVersions: []string{}, LoaderCompatibility: []string{},
		Status: artifact.StatusPending, CreatedAt: s.now(),
	}
	if err := s.repository.CreateArtifact(ctx, value); err != nil {
		return artifact.Artifact{}, err
	}
	if err := s.auditCreated(ctx, value, actor); err != nil {
		return artifact.Artifact{}, err
	}
	return value, nil
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
	value := artifact.Artifact{ID: id, Name: name, Type: kind, UploaderID: uploader, SHA256: hash, SizeBytes: size, PayloadStatus: artifact.PayloadAvailable, TargetMinecraftVersions: versions, LoaderCompatibility: loaders, Status: artifact.StatusPending, CreatedAt: time.Now().UTC()}
	if err := s.repository.SaveArtifact(ctx, value); err != nil {
		return artifact.Artifact{}, err
	}
	return value, nil
}

func (s *Service) auditCreated(ctx context.Context, value artifact.Artifact, actor string) error {
	id, err := s.newID("audit")
	if err != nil {
		return err
	}
	return s.repository.AppendAuditEvent(ctx, audit.Event{
		ID: id, ProjectID: value.ProjectID, ActorID: actor, Action: ActionCreated, TargetType: "artifact", TargetID: value.ID,
		Metadata: map[string]string{
			"artifactId": value.ID, "artifactName": value.Name, "artifactType": string(value.Type),
			"projectId": value.ProjectID, "actor": actor, "status": string(value.Status),
		},
		CreatedAt: s.now(),
	})
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
		ID: id, ProjectID: value.ProjectID, ActorID: actor, Action: action, TargetType: "artifact", TargetID: value.ID,
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
