package artifactstagingsvc

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/artifactstaging"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/session"
	"github.com/stratummc/stratum/internal/util"
)

const ActionPlanCreated = "artifact.staging.plan.created"
const ActionPlanRejected = "artifact.staging.plan.rejected"

type Repository interface {
	GetArtifact(context.Context, string) (artifact.Artifact, error)
	GetSession(context.Context, string) (session.Session, error)
	CreateArtifactStagingPlan(context.Context, artifactstaging.Plan) error
	GetArtifactStagingPlan(context.Context, string) (artifactstaging.Plan, error)
	ListArtifactStagingPlans(context.Context) ([]artifactstaging.Plan, error)
	ListArtifactStagingPlansBySession(context.Context, string) ([]artifactstaging.Plan, error)
	ListArtifactStagingPlansByArtifact(context.Context, string) ([]artifactstaging.Plan, error)
	AppendAuditEvent(context.Context, audit.Event) error
}

type CreateParams struct {
	SessionID  string
	ArtifactID string
	ActorID    string
	Name       string
	Metadata   map[string]string
}

type Service struct {
	repository Repository
	now        func() time.Time
	newID      func(string) (string, error)
}

func New(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }, newID: util.NewID}
}

func (s *Service) CreatePlan(ctx context.Context, params CreateParams) (artifactstaging.Plan, error) {
	if strings.TrimSpace(params.SessionID) == "" || strings.TrimSpace(params.ArtifactID) == "" || strings.TrimSpace(params.ActorID) == "" {
		return artifactstaging.Plan{}, errors.New("session, artifact, and actor are required")
	}
	if err := validateStagingName(params.Name); err != nil {
		return artifactstaging.Plan{}, err
	}
	sessionValue, err := s.repository.GetSession(ctx, params.SessionID)
	if err != nil {
		return artifactstaging.Plan{}, fmt.Errorf("load session: %w", err)
	}
	artifactValue, err := s.repository.GetArtifact(ctx, params.ArtifactID)
	if err != nil {
		return artifactstaging.Plan{}, fmt.Errorf("load artifact: %w", err)
	}
	kind, err := stagingKind(artifactValue.Type)
	if err != nil {
		return artifactstaging.Plan{}, err
	}
	status := artifactstaging.StatusPlanned
	rejectionReason := ""
	if artifactValue.Status != artifact.StatusApproved {
		status = artifactstaging.StatusRejected
		rejectionReason = fmt.Sprintf("artifact status %q is not approved", artifactValue.Status)
	}
	planID, err := s.newID("artifact-staging-plan")
	if err != nil {
		return artifactstaging.Plan{}, err
	}
	now := s.now()
	plan := artifactstaging.Plan{
		ID:                planID,
		SessionID:         sessionValue.ID,
		ProjectID:         sessionValue.ProjectID,
		RoomID:            sessionValue.RoomID,
		ArtifactID:        artifactValue.ID,
		ArtifactName:      artifactValue.Name,
		ArtifactType:      string(artifactValue.Type),
		ArtifactStatus:    string(artifactValue.Status),
		ArtifactHash:      artifactValue.SHA256,
		TargetStagingName: filepath.Clean(params.Name),
		StagingKind:       kind,
		ActorID:           params.ActorID,
		CreatedAt:         now,
		Status:            status,
		RejectionReason:   rejectionReason,
		Metadata:          cloneMetadata(params.Metadata),
	}
	if err := s.repository.CreateArtifactStagingPlan(ctx, plan); err != nil {
		return artifactstaging.Plan{}, err
	}
	if err := s.audit(ctx, plan); err != nil {
		return artifactstaging.Plan{}, err
	}
	return plan, nil
}

func (s *Service) Get(ctx context.Context, id string) (artifactstaging.Plan, error) {
	return s.repository.GetArtifactStagingPlan(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]artifactstaging.Plan, error) {
	return s.repository.ListArtifactStagingPlans(ctx)
}

func (s *Service) ListBySession(ctx context.Context, sessionID string) ([]artifactstaging.Plan, error) {
	return s.repository.ListArtifactStagingPlansBySession(ctx, sessionID)
}

func (s *Service) ListByArtifact(ctx context.Context, artifactID string) ([]artifactstaging.Plan, error) {
	return s.repository.ListArtifactStagingPlansByArtifact(ctx, artifactID)
}

func (s *Service) audit(ctx context.Context, plan artifactstaging.Plan) error {
	id, err := s.newID("audit")
	if err != nil {
		return err
	}
	action := ActionPlanCreated
	if plan.Status == artifactstaging.StatusRejected {
		action = ActionPlanRejected
	}
	metadata := map[string]string{
		"planId":      plan.ID,
		"sessionId":   plan.SessionID,
		"artifactId":  plan.ArtifactID,
		"actorId":     plan.ActorID,
		"stagingKind": string(plan.StagingKind),
		"status":      string(plan.Status),
	}
	if plan.RejectionReason != "" {
		metadata["rejectionReason"] = plan.RejectionReason
	}
	return s.repository.AppendAuditEvent(ctx, audit.Event{ID: id, ProjectID: plan.ProjectID, ActorID: plan.ActorID, Action: action, TargetType: "artifact-staging-plan", TargetID: plan.ID, Metadata: metadata, CreatedAt: s.now()})
}

func stagingKind(value artifact.Type) (artifactstaging.Kind, error) {
	switch value {
	case artifact.TypeJar, artifact.TypeDatapack, artifact.TypeMCDRPlugin, artifact.TypeSchematic, artifact.TypeWorldArchive:
		return artifactstaging.KindArtifact, nil
	case artifact.TypeConfigPreset, artifact.TypeCarpetRules:
		return artifactstaging.KindConfig, nil
	default:
		return "", fmt.Errorf("unsupported artifact type %q for staging", value)
	}
}

func validateStagingName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("staging name is required")
	}
	if filepath.IsAbs(name) {
		return errors.New("staging name must be relative")
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("staging name escapes staging root")
	}
	for _, character := range name {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' || character == '/' || character == '\\' {
			continue
		}
		return fmt.Errorf("staging name %q contains unsupported characters", name)
	}
	return nil
}

func cloneMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
