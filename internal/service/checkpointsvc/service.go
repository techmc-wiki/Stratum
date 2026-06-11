package checkpointsvc

import (
	"context"
	"fmt"

	"github.com/stratummc/stratum/internal/domain/checkpoint"
)

type Repository interface {
	SaveCheckpoint(context.Context, checkpoint.Checkpoint) error
	ListCheckpoints(context.Context) ([]checkpoint.Checkpoint, error)
}

type Storage interface {
	CreateWorldSnapshot(context.Context, string, string) (string, error)
	RestoreWorldSnapshot(context.Context, string, string) error
}

type Service struct {
	repository Repository
	storage    Storage
}

func New(repository Repository, storage Storage) *Service {
	return &Service{repository: repository, storage: storage}
}

func (s *Service) CreateMetadata(ctx context.Context, params checkpoint.CreateParams) (checkpoint.Checkpoint, error) {
	value, err := checkpoint.New(params)
	if err != nil {
		return checkpoint.Checkpoint{}, err
	}
	if err := s.repository.SaveCheckpoint(ctx, value); err != nil {
		return checkpoint.Checkpoint{}, err
	}
	return value, nil
}

func (s *Service) List(ctx context.Context) ([]checkpoint.Checkpoint, error) {
	return s.repository.ListCheckpoints(ctx)
}

func (s *Service) Rollback(_ context.Context, checkpointID string) error {
	// TODO: require authorization, stop/freeze the session, create a
	// pre-operation checkpoint, and delegate restore to Storage.
	return fmt.Errorf("checkpoint rollback for %q is not implemented", checkpointID)
}
