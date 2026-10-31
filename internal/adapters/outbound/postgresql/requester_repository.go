package postgresql

import (
	"context"

	pgmodel "github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql/model"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"gorm.io/gorm"
)

type requesterRepository struct {
	db *gorm.DB
}

func NewRequesterRepository(db *gorm.DB) ports.RequesterRepository {
	return &requesterRepository{db: db}
}

func (r *requesterRepository) Create(ctx context.Context, c *entities.Requester) error {
	m := pgmodel.FromRequester(c)
	if err := conn(ctx, r.db).Create(m).Error; err != nil {
		return mapError(err)
	}
	c.SetID(m.ID)
	return nil
}

func (r *requesterRepository) FindByID(ctx context.Context, id string) (*entities.Requester, error) {
	var m pgmodel.Requester
	if err := conn(ctx, r.db).First(&m, "id = ?", id).Error; err != nil {
		return nil, mapError(err)
	}
	return m.ToDomain(), nil
}

func (r *requesterRepository) FindByDocument(ctx context.Context, document string) (*entities.Requester, error) {
	var m pgmodel.Requester
	if err := conn(ctx, r.db).First(&m, "document = ?", document).Error; err != nil {
		return nil, mapError(err)
	}
	return m.ToDomain(), nil
}

func (r *requesterRepository) FindAll(ctx context.Context) ([]*entities.Requester, error) {
	var docs []pgmodel.Requester
	if err := conn(ctx, r.db).Find(&docs).Error; err != nil {
		return nil, mapError(err)
	}
	requesters := make([]*entities.Requester, 0, len(docs))
	for i := range docs {
		requesters = append(requesters, docs[i].ToDomain())
	}
	return requesters, nil
}

func (r *requesterRepository) Update(ctx context.Context, c *entities.Requester) error {
	m := pgmodel.FromRequester(c)
	return mapError(conn(ctx, r.db).Save(m).Error)
}

func (r *requesterRepository) Delete(ctx context.Context, id string) error {
	return mapError(conn(ctx, r.db).Delete(&pgmodel.Requester{}, "id = ?", id).Error)
}
