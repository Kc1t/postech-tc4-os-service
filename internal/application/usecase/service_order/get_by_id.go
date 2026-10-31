package serviceorderuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type GetServiceOrder struct {
	repo ports.ServiceOrderRepository
}

func NewGetServiceOrder(repo ports.ServiceOrderRepository) *GetServiceOrder {
	return &GetServiceOrder{repo: repo}
}

func (uc *GetServiceOrder) Execute(ctx context.Context, id string) (*entities.ServiceOrder, error) {
	return uc.repo.FindByID(ctx, id)
}
