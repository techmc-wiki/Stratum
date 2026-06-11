package projectsvc

import (
	"context"

	"github.com/stratummc/stratum/internal/domain/project"
)

type Repository interface {
	ListProjects(context.Context) ([]project.Project, error)
}

type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) List(ctx context.Context) ([]project.Project, error) {
	return s.repository.ListProjects(ctx)
}
