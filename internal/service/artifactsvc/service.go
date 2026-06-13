package artifactsvc

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stratummc/stratum/internal/domain/artifact"
	"github.com/stratummc/stratum/internal/domain/audit"
	"github.com/stratummc/stratum/internal/domain/project"
	stratumerrors "github.com/stratummc/stratum/internal/errors"
	"github.com/stratummc/stratum/internal/util"
)

const ActionCreated = "artifact.created"
const ActionApproved = "artifact.approved"
const ActionRejected = "artifact.rejected"
const ActionPayloadImported = "artifact.payload.imported"

type BlobStore interface {
	HashFile(string) (algorithm, hash string, size int64, err error)
	StoreFile(context.Context, string) (algorithm, hash, reference string, size int64, err error)
}

type PayloadVerifier interface {
	VerifyPayload(context.Context, string) (algorithm, hash, reference string, size int64, err error)
}

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
	blobStore  BlobStore
	verifier   PayloadVerifier
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

func NewWithBlobStore(repository Repository, blobStore BlobStore) *Service {
	service := New(repository)
	service.blobStore = blobStore
	if verifier, ok := blobStore.(PayloadVerifier); ok {
		service.verifier = verifier
	}
	return service
}

func NewWithPayloadVerifier(repository Repository, verifier PayloadVerifier) *Service {
	service := New(repository)
	service.verifier = verifier
	return service
}

func (s *Service) ImportFile(ctx context.Context, id, path, actor string) (artifact.Artifact, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(path) == "" || strings.TrimSpace(actor) == "" {
		return artifact.Artifact{}, fmt.Errorf("artifact payload import requires id, file, and actor")
	}
	if s.blobStore == nil {
		return artifact.Artifact{}, fmt.Errorf("artifact blob store is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return artifact.Artifact{}, fmt.Errorf("inspect import file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return artifact.Artifact{}, fmt.Errorf("import file %q is not a regular file", path)
	}
	value, err := s.repository.GetArtifact(ctx, id)
	if err != nil {
		if stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
			return artifact.Artifact{}, fmt.Errorf("artifact %q not found: %w", id, err)
		}
		return artifact.Artifact{}, fmt.Errorf("load artifact %q: %w", id, err)
	}
	if value.Status != artifact.StatusPending {
		return artifact.Artifact{}, fmt.Errorf("artifact %q must be pending to import a payload; current status is %q", id, value.Status)
	}

	hasPayload := value.PayloadStatus == artifact.PayloadAvailable || value.SHA256 != ""
	if hasPayload {
		algorithm, hash, size, err := s.blobStore.HashFile(path)
		if err != nil {
			return artifact.Artifact{}, fmt.Errorf("hash import file: %w", err)
		}
		currentAlgorithm := value.PayloadAlgorithm
		if currentAlgorithm == "" && value.SHA256 != "" {
			currentAlgorithm = algorithm
		}
		if currentAlgorithm == algorithm && value.SHA256 == hash && value.SizeBytes == size {
			storedAlgorithm, storedHash, storedReference, storedSize, err := s.blobStore.StoreFile(ctx, path)
			if err != nil {
				return artifact.Artifact{}, fmt.Errorf("verify idempotent artifact payload: %w", err)
			}
			if storedAlgorithm != algorithm || storedHash != hash || storedSize != size || (value.PayloadReference != "" && value.PayloadReference != storedReference) {
				return artifact.Artifact{}, fmt.Errorf("artifact %q payload metadata does not match blob storage", id)
			}
			return value, nil
		}
		return artifact.Artifact{}, fmt.Errorf("artifact %q already has a different payload; replace is not supported", id)
	}

	algorithm, hash, reference, size, err := s.blobStore.StoreFile(ctx, path)
	if err != nil {
		return artifact.Artifact{}, fmt.Errorf("store artifact payload: %w", err)
	}
	importedAt := s.now()
	value.PayloadAlgorithm = algorithm
	value.SHA256 = hash
	value.SizeBytes = size
	value.PayloadReference = reference
	value.PayloadStatus = artifact.PayloadAvailable
	value.PayloadImportedBy = actor
	value.PayloadImportedAt = &importedAt
	if err := s.repository.SaveArtifact(ctx, value); err != nil {
		return artifact.Artifact{}, err
	}
	if err := s.auditPayloadImported(ctx, value, actor); err != nil {
		return artifact.Artifact{}, err
	}
	return value, nil
}

func (s *Service) auditPayloadImported(ctx context.Context, value artifact.Artifact, actor string) error {
	id, err := s.newID("audit")
	if err != nil {
		return err
	}
	return s.repository.AppendAuditEvent(ctx, audit.Event{
		ID: id, ProjectID: value.ProjectID, ActorID: actor, Action: ActionPayloadImported, TargetType: "artifact", TargetID: value.ID,
		Metadata: map[string]string{
			"artifactId": value.ID, "artifactName": value.Name, "actor": actor,
			"payloadAlgorithm": value.PayloadAlgorithm, "payloadHash": value.SHA256, "payloadSize": strconv.FormatInt(value.SizeBytes, 10),
		},
		CreatedAt: s.now(),
	})
}

func (s *Service) RegisterFile(ctx context.Context, id, name, path, uploader string, kind artifact.Type, versions, loaders []string) (artifact.Artifact, error) {
	if id == "" || name == "" || uploader == "" {
		return artifact.Artifact{}, fmt.Errorf("artifact requires id, name, and uploader")
	}
	hash, size, err := artifact.HashFile(path)
	if err != nil {
		return artifact.Artifact{}, fmt.Errorf("hash artifact: %w", err)
	}
	value := artifact.Artifact{ID: id, Name: name, Type: kind, UploaderID: uploader, SHA256: hash, SizeBytes: size, PayloadStatus: artifact.PayloadAvailable, PayloadAlgorithm: "sha256", TargetMinecraftVersions: versions, LoaderCompatibility: loaders, Status: artifact.StatusPending, CreatedAt: time.Now().UTC()}
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
	if next == artifact.StatusApproved {
		if err := s.verifyPayloadForApproval(ctx, value); err != nil {
			return artifact.Artifact{}, err
		}
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

func (s *Service) verifyPayloadForApproval(ctx context.Context, value artifact.Artifact) error {
	if s.verifier == nil {
		return fmt.Errorf("artifact %q cannot be approved: payload verifier is unavailable", value.ID)
	}
	if value.PayloadStatus != artifact.PayloadAvailable || value.PayloadAlgorithm == "" || value.SHA256 == "" || value.SizeBytes < 0 || value.PayloadReference == "" || value.PayloadImportedBy == "" || value.PayloadImportedAt == nil {
		return fmt.Errorf("artifact %q cannot be approved: payload metadata is missing", value.ID)
	}
	if value.PayloadAlgorithm != "sha256" {
		return fmt.Errorf("artifact %q cannot be approved: unsupported payload algorithm %q", value.ID, value.PayloadAlgorithm)
	}
	if len(value.SHA256) != 64 {
		return fmt.Errorf("artifact %q cannot be approved: invalid payload SHA-256 hash", value.ID)
	}
	for _, character := range value.SHA256 {
		if (character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') {
			continue
		}
		return fmt.Errorf("artifact %q cannot be approved: invalid payload SHA-256 hash", value.ID)
	}
	algorithm, hash, reference, size, err := s.verifier.VerifyPayload(ctx, value.SHA256)
	if err != nil {
		if stratumerrors.IsKind(err, stratumerrors.KindNotFound) {
			return fmt.Errorf("artifact %q cannot be approved: payload blob is missing: %w", value.ID, err)
		}
		if strings.Contains(err.Error(), "hash mismatch") {
			return fmt.Errorf("artifact %q cannot be approved: payload blob is corrupted: %w", value.ID, err)
		}
		return fmt.Errorf("artifact %q cannot be approved: payload verification failed: %w", value.ID, err)
	}
	if algorithm != value.PayloadAlgorithm || hash != value.SHA256 || reference != value.PayloadReference || size != value.SizeBytes {
		return fmt.Errorf("artifact %q cannot be approved: payload metadata does not match verified blob", value.ID)
	}
	return nil
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
