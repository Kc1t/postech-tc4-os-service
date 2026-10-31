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
	// TODO: verificar OS abertas para o veiculo
	return uc.repo.Delete(ctx, id)
}
