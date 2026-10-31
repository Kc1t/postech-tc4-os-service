package serviceuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type ListServices struct {
	repo ports.ServiceRepository
}

func NewListServices(repo ports.ServiceRepository) *ListServices {
	return &ListServices{repo: repo}
}

func (uc *ListServices) Execute(ctx context.Context) ([]*entities.Service, error) {
	return uc.repo.FindAll(ctx)
}
