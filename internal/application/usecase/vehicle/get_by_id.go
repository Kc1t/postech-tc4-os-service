package vehicleuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type GetVehicle struct {
	repo ports.VehicleRepository
}

func NewGetVehicle(repo ports.VehicleRepository) *GetVehicle {
	return &GetVehicle{repo: repo}
}

func (uc *GetVehicle) Execute(ctx context.Context, id string) (*entities.Vehicle, error) {
	return uc.repo.FindByID(ctx, id)
}
