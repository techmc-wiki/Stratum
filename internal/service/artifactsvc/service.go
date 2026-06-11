package artifactsvc

import (
	"context"
	"fmt"
	"time"

	"github.com/stratummc/stratum/internal/domain/artifact"
)

type Repository interface {
	SaveArtifact(context.Context, artifact.Artifact) error
	ListArtifacts(context.Context) ([]artifact.Artifact, error)
}

type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }

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

// TODO: add an explicit reviewer-authorized approval workflow and UI.
