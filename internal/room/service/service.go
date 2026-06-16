package service

import (
	"context"
	"fmt"

	"github.com/stratummc/stratum/internal/environment"
	"github.com/stratummc/stratum/internal/room"
)

type RoomRepository interface {
	CreateRoom(context.Context, room.Room) error
	ListRooms(context.Context) ([]room.Room, error)
}

type EnvironmentRepository interface {
	GetEnvironment(context.Context, string) (environment.Environment, error)
}

type Service struct {
	rooms        RoomRepository
	environments EnvironmentRepository
}

func New(rooms RoomRepository, environments EnvironmentRepository) *Service {
	return &Service{rooms: rooms, environments: environments}
}

func (s *Service) CreateRoom(ctx context.Context, rm room.Room, actor string) error {
	if rm.EnvironmentID != "" {
		if _, err := s.environments.GetEnvironment(ctx, rm.EnvironmentID); err != nil {
			return fmt.Errorf("environment %q not found: %w", rm.EnvironmentID, err)
		}
	}
	return s.rooms.CreateRoom(ctx, rm)
}

func (s *Service) List(ctx context.Context) ([]room.Room, error) {
	return s.rooms.ListRooms(ctx)
}
