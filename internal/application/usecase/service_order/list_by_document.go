package serviceorderuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type ListServiceOrdersByDocument struct {
	repo          ports.ServiceOrderRepository
	requesterRepo ports.RequesterRepository
}

func NewListServiceOrdersByDocument(repo ports.ServiceOrderRepository, requesterRepo ports.RequesterRepository) *ListServiceOrdersByDocument {
	return &ListServiceOrdersByDocument{repo: repo, requesterRepo: requesterRepo}
}

func (uc *ListServiceOrdersByDocument) Execute(ctx context.Context, document string) ([]*entities.ServiceOrder, error) {
	requester, err := uc.requesterRepo.FindByDocument(ctx, document)
	if err != nil {
		return nil, err
	}

	return uc.repo.FindByRequesterID(ctx, requester.ID())
}
