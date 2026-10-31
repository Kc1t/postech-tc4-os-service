package serviceuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type UpdateService struct {
	repo ports.ServiceRepository
}

func NewUpdateService(repo ports.ServiceRepository) *UpdateService {
	return &UpdateService{repo: repo}
}

func (uc *UpdateService) Execute(ctx context.Context, s *entities.Service) error {
	return uc.repo.Update(ctx, s)
}
