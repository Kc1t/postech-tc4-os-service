package vehicleuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type DeleteVehicle struct {
	repo ports.VehicleRepository
}

func NewDeleteVehicle(repo ports.VehicleRepository) *DeleteVehicle {
	return &DeleteVehicle{repo: repo}
}

func (uc *DeleteVehicle) Execute(ctx context.Context, id string) error {
	// Um veículo com OS (em andamento ou no histórico) não sai: a FK ON DELETE RESTRICT recusa e o
	// repositório devolve domainerrors.ErrInUse (409).
	return uc.repo.Delete(ctx, id)
}
