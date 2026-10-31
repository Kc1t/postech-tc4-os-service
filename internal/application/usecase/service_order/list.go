package serviceorderuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type ListServiceOrders struct {
	repo ports.ServiceOrderRepository
}

func NewListServiceOrders(repo ports.ServiceOrderRepository) *ListServiceOrders {
	return &ListServiceOrders{repo: repo}
}

func (uc *ListServiceOrders) Execute(ctx context.Context) ([]*entities.ServiceOrder, error) {
	return uc.repo.FindAll(ctx)
}
