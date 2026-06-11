package roomsvc

import (
	"context"

	"github.com/stratummc/stratum/internal/domain/room"
)

type Repository interface {
	ListRooms(context.Context) ([]room.Room, error)
}
type Service struct{ repository Repository }

func New(repository Repository) *Service                         { return &Service{repository: repository} }
func (s *Service) List(ctx context.Context) ([]room.Room, error) { return s.repository.ListRooms(ctx) }
