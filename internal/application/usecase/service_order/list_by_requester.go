package serviceorderuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type ListServiceOrdersByRequester struct {
	repo ports.ServiceOrderRepository
}

func NewListServiceOrdersByRequester(repo ports.ServiceOrderRepository) *ListServiceOrdersByRequester {
	return &ListServiceOrdersByRequester{repo: repo}
}

func (uc *ListServiceOrdersByRequester) Execute(ctx context.Context, requesterID string) ([]*entities.ServiceOrder, error) {
	return uc.repo.FindByRequesterID(ctx, requesterID)
}
