package serviceorderuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type UpdateServiceOrder struct {
	repo ports.ServiceOrderRepository
}

func NewUpdateServiceOrder(repo ports.ServiceOrderRepository) *UpdateServiceOrder {
	return &UpdateServiceOrder{repo: repo}
}

func (uc *UpdateServiceOrder) Execute(ctx context.Context, so *entities.ServiceOrder) error {
	return uc.repo.Update(ctx, so)
}
