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
	stratumerrors "github.com/stratummc/stratum/internal/errors"
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

type PayloadVerifier interface {
	VerifyPayload(context.Context, string) (algorithm, hash, reference string, size int64, err error)
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
	verifier   PayloadVerifier
	now        func() time.Time
	newID      func(string) (string, error)
}

func New(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }, newID: util.NewID}
}

func NewWithPayloadVerifier(repository Repository, verifier PayloadVerifier) *Service {
	service := New(repository)
	service.verifier = verifier
	return service
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
	verificationStatus := "verified"
	if artifactValue.Status != artifact.StatusApproved {
		status = artifactstaging.StatusRejected
		rejectionReason = fmt.Sprintf("artifact status %q is not approved", artifactValue.Status)
		verificationStatus = "not_attempted"
	} else if err := s.verifyPayload(ctx, artifactValue); err != nil {
		status = artifactstaging.StatusRejected
		rejectionReason = err.Error()
		verificationStatus = "failed"
	}
	planID, err := s.newID("artifact-staging-plan")
	if err != nil {
		return artifactstaging.Plan{}, err
	}
	now := s.now()
	metadata := cloneMetadata(params.Metadata)
	if metadata == nil {
		metadata = make(map[string]string)
	}
	metadata["verificationStatus"] = verificationStatus
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
		Metadata:          metadata,
	}
	if err := s.repository.CreateArtifactStagingPlan(ctx, plan); err != nil {
		return artifactstaging.Plan{}, err
	}
	if err := s.audit(ctx, plan); err != nil {
		return artifactstaging.Plan{}, err
	}
	return plan, nil
}

func (s *Service) verifyPayload(ctx context.Context, value artifact.Artifact) error {
	return verifyArtifactPayload(ctx, s.verifier, value)
}

func verifyArtifactPayload(ctx context.Context, verifier PayloadVerifier, value artifact.Artifact) error {
	if verifier == nil {
		return errors.New("payload verifier is unavailable")
	}
	if value.PayloadStatus != artifact.PayloadAvailable || value.PayloadAlgorithm == "" || value.SHA256 == "" || value.PayloadReference == "" || value.SizeBytes < 0 {
		return errors.New("payload metadata is missing")
	}
	if value.PayloadAlgorithm != "sha256" {
		return fmt.Errorf("unsupported payload algorithm %q", value.PayloadAlgorithm)
	}
	if !validSHA256(value.SHA256) {
		return errors.New("invalid payload SHA-256 hash")
	}
	algorithm, hash, reference, size, err := verifier.VerifyPayload(ctx, value.SHA256)
	if err != nil {
		if stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
			return fmt.Errorf("payload blob is missing: %w", err)
		}
		if strings.Contains(err.Error(), "hash mismatch") {
			return fmt.Errorf("payload blob is corrupted: %w", err)
		}
		return fmt.Errorf("payload verification failed: %w", err)
	}
	if algorithm != value.PayloadAlgorithm || hash != value.SHA256 || reference != value.PayloadReference || size != value.SizeBytes {
		return errors.New("payload metadata does not match verified blob")
	}
	return nil
}

func validSHA256(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, character := range hash {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
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
		"payloadHash": plan.ArtifactHash,
	}
	if verificationStatus := plan.Metadata["verificationStatus"]; verificationStatus != "" {
		metadata["verificationStatus"] = verificationStatus
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
