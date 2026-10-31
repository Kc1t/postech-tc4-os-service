package postgresql

import (
	"context"

	pgmodel "github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql/model"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"gorm.io/gorm"
)

type vehicleRepository struct {
	db *gorm.DB
}

func NewVehicleRepository(db *gorm.DB) ports.VehicleRepository {
	return &vehicleRepository{db: db}
}

func (r *vehicleRepository) Create(ctx context.Context, v *entities.Vehicle) error {
	m := pgmodel.FromVehicle(v)
	if err := conn(ctx, r.db).Create(m).Error; err != nil {
		return mapError(err)
	}
	v.SetID(m.ID)
	return nil
}

func (r *vehicleRepository) FindByID(ctx context.Context, id string) (*entities.Vehicle, error) {
	var m pgmodel.Vehicle
	if err := conn(ctx, r.db).First(&m, "id = ?", id).Error; err != nil {
		return nil, mapError(err)
	}
	return m.ToDomain(), nil
}

func (r *vehicleRepository) FindByPlate(ctx context.Context, plate string) (*entities.Vehicle, error) {
	var m pgmodel.Vehicle
	if err := conn(ctx, r.db).First(&m, "plate = ?", plate).Error; err != nil {
		return nil, mapError(err)
	}
	return m.ToDomain(), nil
}

func (r *vehicleRepository) FindByRequesterID(ctx context.Context, requesterID string) ([]*entities.Vehicle, error) {
	var docs []pgmodel.Vehicle
	if err := conn(ctx, r.db).Find(&docs, "requester_id = ?", requesterID).Error; err != nil {
		return nil, mapError(err)
	}
	vehicles := make([]*entities.Vehicle, 0, len(docs))
	for i := range docs {
		vehicles = append(vehicles, docs[i].ToDomain())
	}
	return vehicles, nil
}

func (r *vehicleRepository) FindAll(ctx context.Context) ([]*entities.Vehicle, error) {
	var docs []pgmodel.Vehicle
	if err := conn(ctx, r.db).Find(&docs).Error; err != nil {
		return nil, mapError(err)
	}
	vehicles := make([]*entities.Vehicle, 0, len(docs))
	for i := range docs {
		vehicles = append(vehicles, docs[i].ToDomain())
	}
	return vehicles, nil
}

func (r *vehicleRepository) Update(ctx context.Context, v *entities.Vehicle) error {
	m := pgmodel.FromVehicle(v)
	return mapError(conn(ctx, r.db).Save(m).Error)
}

func (r *vehicleRepository) Delete(ctx context.Context, id string) error {
	return mapError(conn(ctx, r.db).Delete(&pgmodel.Vehicle{}, "id = ?", id).Error)
}
