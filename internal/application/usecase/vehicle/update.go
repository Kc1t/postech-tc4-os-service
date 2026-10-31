package vehicleuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type UpdateVehicle struct {
	repo ports.VehicleRepository
}

func NewUpdateVehicle(repo ports.VehicleRepository) *UpdateVehicle {
	return &UpdateVehicle{repo: repo}
}

func (uc *UpdateVehicle) Execute(ctx context.Context, v *entities.Vehicle) error {
	return uc.repo.Update(ctx, v)
}
