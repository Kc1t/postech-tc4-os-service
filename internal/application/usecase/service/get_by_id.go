package serviceuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type GetService struct {
	repo ports.ServiceRepository
}

func NewGetService(repo ports.ServiceRepository) *GetService {
	return &GetService{repo: repo}
}

func (uc *GetService) Execute(ctx context.Context, id string) (*entities.Service, error) {
	return uc.repo.FindByID(ctx, id)
}
