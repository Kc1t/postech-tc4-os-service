package serviceorderuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type GetServiceOrderByCode struct {
	repo          ports.ServiceOrderRepository
	requesterRepo ports.RequesterRepository
}

func NewGetServiceOrderByCode(repo ports.ServiceOrderRepository, requesterRepo ports.RequesterRepository) *GetServiceOrderByCode {
	return &GetServiceOrderByCode{repo: repo, requesterRepo: requesterRepo}
}

func (uc *GetServiceOrderByCode) Execute(ctx context.Context, code int, requesterDocument string) (*entities.ServiceOrder, error) {
	requester, err := uc.requesterRepo.FindByDocument(ctx, requesterDocument)
	if err != nil {
		return nil, err
	}

	so, err := uc.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	if so.RequesterID() != requester.ID() {
		return nil, domainerrors.ErrNotFound
	}

	return so, nil
}
