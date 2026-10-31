package vehicleuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type ListVehiclesByRequester struct {
	repo ports.VehicleRepository
}

func NewListVehiclesByRequester(repo ports.VehicleRepository) *ListVehiclesByRequester {
	return &ListVehiclesByRequester{repo: repo}
}

func (uc *ListVehiclesByRequester) Execute(ctx context.Context, requesterID string) ([]*entities.Vehicle, error) {
	return uc.repo.FindByRequesterID(ctx, requesterID)
}
