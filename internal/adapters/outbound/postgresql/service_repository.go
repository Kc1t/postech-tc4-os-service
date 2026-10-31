package postgresql

import (
	"context"

	pgmodel "github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql/model"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"gorm.io/gorm"
)

type serviceRepository struct {
	db *gorm.DB
}

func NewServiceRepository(db *gorm.DB) ports.ServiceRepository {
	return &serviceRepository{db: db}
}

func (r *serviceRepository) Create(ctx context.Context, s *entities.Service) error {
	m := pgmodel.FromService(s)
	if err := conn(ctx, r.db).Create(m).Error; err != nil {
		return mapError(err)
	}
	s.SetID(m.ID)
	return nil
}

func (r *serviceRepository) FindByID(ctx context.Context, id string) (*entities.Service, error) {
	var m pgmodel.Service
	if err := conn(ctx, r.db).First(&m, "id = ?", id).Error; err != nil {
		return nil, mapError(err)
	}
	return m.ToDomain(), nil
}

func (r *serviceRepository) FindByIDs(ctx context.Context, ids []string) ([]*entities.Service, error) {
	var docs []pgmodel.Service
	if err := conn(ctx, r.db).Where("id IN ?", ids).Find(&docs).Error; err != nil {
		return nil, mapError(err)
	}
	services := make([]*entities.Service, 0, len(docs))
	for i := range docs {
		services = append(services, docs[i].ToDomain())
	}
	return services, nil
}

func (r *serviceRepository) FindByCodes(ctx context.Context, codes []int) ([]*entities.Service, error) {
	var docs []pgmodel.Service
	if err := conn(ctx, r.db).Where("code IN ?", codes).Find(&docs).Error; err != nil {
		return nil, mapError(err)
	}
	services := make([]*entities.Service, 0, len(docs))
	for i := range docs {
		services = append(services, docs[i].ToDomain())
	}
	return services, nil
}

func (r *serviceRepository) FindAll(ctx context.Context) ([]*entities.Service, error) {
	var docs []pgmodel.Service
	if err := conn(ctx, r.db).Find(&docs).Error; err != nil {
		return nil, mapError(err)
	}
	services := make([]*entities.Service, 0, len(docs))
	for i := range docs {
		services = append(services, docs[i].ToDomain())
	}
	return services, nil
}

func (r *serviceRepository) Update(ctx context.Context, s *entities.Service) error {
	m := pgmodel.FromService(s)
	return mapError(conn(ctx, r.db).Save(m).Error)
}

func (r *serviceRepository) Delete(ctx context.Context, id string) error {
	return mapError(conn(ctx, r.db).Delete(&pgmodel.Service{}, "id = ?", id).Error)
}
