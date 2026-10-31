package serviceuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type CreateService struct {
	repo ports.ServiceRepository
}

func NewCreateService(repo ports.ServiceRepository) *CreateService {
	return &CreateService{repo: repo}
}

func (uc *CreateService) Execute(ctx context.Context, s *entities.Service) error {
	return uc.repo.Create(ctx, s)
}
