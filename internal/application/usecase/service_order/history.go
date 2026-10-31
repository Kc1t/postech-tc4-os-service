package serviceorderuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type GetServiceOrderHistory struct {
	repo    ports.ServiceOrderRepository
	history ports.StatusHistoryRepository
}

func NewGetServiceOrderHistory(repo ports.ServiceOrderRepository, history ports.StatusHistoryRepository) *GetServiceOrderHistory {
	return &GetServiceOrderHistory{repo: repo, history: history}
}

func (uc *GetServiceOrderHistory) Execute(ctx context.Context, id string) ([]*entities.StatusHistoryEntry, error) {
	if _, err := uc.repo.FindByID(ctx, id); err != nil {
		return nil, err
	}
	return uc.history.ListByOS(ctx, id)
}
