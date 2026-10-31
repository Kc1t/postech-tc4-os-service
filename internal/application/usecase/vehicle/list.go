package vehicleuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type ListVehicles struct {
	repo ports.VehicleRepository
}

func NewListVehicles(repo ports.VehicleRepository) *ListVehicles {
	return &ListVehicles{repo: repo}
}

func (uc *ListVehicles) Execute(ctx context.Context) ([]*entities.Vehicle, error) {
	return uc.repo.FindAll(ctx)
}
