package serviceuc

import (
	"context"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type DeleteService struct {
	repo   ports.ServiceRepository
	orders ports.ServiceOrderRepository
}

func NewDeleteService(repo ports.ServiceRepository, orders ports.ServiceOrderRepository) *DeleteService {
	return &DeleteService{repo: repo, orders: orders}
}

// Execute exclui o serviço do catálogo, menos quando uma OS em andamento o usa. As OS encerradas
// guardam uma cópia do item (descrição e preço), então o histórico não depende do catálogo.
func (uc *DeleteService) Execute(ctx context.Context, id string) error {
	inUse, err := uc.orders.ExistsActiveWithService(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return domainerrors.ErrInUse
	}
	return uc.repo.Delete(ctx, id)
}
