package serviceorderuc

import (
	"context"
	"errors"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type DeleteServiceOrder struct {
	repo ports.ServiceOrderRepository
}

func NewDeleteServiceOrder(repo ports.ServiceOrderRepository) *DeleteServiceOrder {
	return &DeleteServiceOrder{repo: repo}
}

func (uc *DeleteServiceOrder) Execute(ctx context.Context, id string) error {
	so, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, domainerrors.ErrNotFound) {
			return domainerrors.ErrNotFound
		}
		return err
	}

	if so.Status() != entities.StatusReceived {
		return domainerrors.ErrOrderNotCancellable
	}

	return uc.repo.Delete(ctx, id)
}
