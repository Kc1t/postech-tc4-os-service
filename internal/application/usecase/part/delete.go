package partuc

import (
	"context"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type DeletePart struct {
	repo   ports.PartRepository
	orders ports.ServiceOrderRepository
}

func NewDeletePart(repo ports.PartRepository, orders ports.ServiceOrderRepository) *DeletePart {
	return &DeletePart{repo: repo, orders: orders}
}

// Execute exclui a peça do catálogo, menos quando uma OS em andamento a usa: a compensação da
// saga devolve as peças reservadas ao estoque e precisa encontrá-las.
func (uc *DeletePart) Execute(ctx context.Context, id string) error {
	inUse, err := uc.orders.ExistsActiveWithPart(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return domainerrors.ErrInUse
	}
	return uc.repo.Delete(ctx, id)
}
