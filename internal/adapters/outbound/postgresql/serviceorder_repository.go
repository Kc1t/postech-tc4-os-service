package postgresql

import (
	"context"
	"time"

	pgmodel "github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql/model"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"gorm.io/gorm"
)

type serviceOrderRepository struct {
	db *gorm.DB
}

func NewServiceOrderRepository(db *gorm.DB) ports.ServiceOrderRepository {
	return &serviceOrderRepository{db: db}
}

func (r *serviceOrderRepository) Create(ctx context.Context, so *entities.ServiceOrder) error {
	m := pgmodel.FromServiceOrder(so)
	if err := conn(ctx, r.db).Create(m).Error; err != nil {
		return mapError(err)
	}
	so.SetID(m.ID)
	so.SetCode(m.Code)
	return nil
}

func (r *serviceOrderRepository) FindByID(ctx context.Context, id string) (*entities.ServiceOrder, error) {
	var m pgmodel.ServiceOrder
	if err := conn(ctx, r.db).First(&m, "id = ?", id).Error; err != nil {
		return nil, mapError(err)
	}
	return m.ToDomain(), nil
}

func (r *serviceOrderRepository) FindByCode(ctx context.Context, code int) (*entities.ServiceOrder, error) {
	var m pgmodel.ServiceOrder
	if err := conn(ctx, r.db).First(&m, "code = ?", code).Error; err != nil {
		return nil, mapError(err)
	}
	return m.ToDomain(), nil
}

// FindAll lista as OS em andamento para a operacao: as mais adiantadas no fluxo primeiro e,
// dentro do mesmo status, as mais antigas. Finalizadas, entregues e canceladas ficam de fora
// (continuam no banco e na consulta por id).
func (r *serviceOrderRepository) FindAll(ctx context.Context) ([]*entities.ServiceOrder, error) {
	var docs []pgmodel.ServiceOrder
	err := conn(ctx, r.db).
		Where("status NOT IN ?", []string{
			string(entities.StatusFinished),
			string(entities.StatusDelivered),
			string(entities.StatusCancelled),
		}).
		Order("CASE status" +
			" WHEN 'in_execution' THEN 1" +
			" WHEN 'awaiting_payment' THEN 2" +
			" WHEN 'awaiting_approval' THEN 3" +
			" WHEN 'awaiting_budget' THEN 4" +
			" WHEN 'in_diagnosis' THEN 5" +
			" WHEN 'received' THEN 6" +
			" END").
		Order("created_at ASC").
		Find(&docs).Error
	if err != nil {
		return nil, mapError(err)
	}
	orders := make([]*entities.ServiceOrder, 0, len(docs))
	for i := range docs {
		orders = append(orders, docs[i].ToDomain())
	}
	return orders, nil
}

func (r *serviceOrderRepository) FindByRequesterID(ctx context.Context, requesterID string) ([]*entities.ServiceOrder, error) {
	var docs []pgmodel.ServiceOrder
	if err := conn(ctx, r.db).Find(&docs, "requester_id = ?", requesterID).Error; err != nil {
		return nil, mapError(err)
	}
	orders := make([]*entities.ServiceOrder, 0, len(docs))
	for i := range docs {
		orders = append(orders, docs[i].ToDomain())
	}
	return orders, nil
}

// Update grava a OS com optimistic locking: so atualiza se a versao lida ainda e a do banco.
func (r *serviceOrderRepository) Update(ctx context.Context, so *entities.ServiceOrder) error {
	m := pgmodel.FromServiceOrder(so)
	current := m.Version
	m.Version = current + 1
	res := conn(ctx, r.db).Model(&pgmodel.ServiceOrder{}).
		Where("id = ? AND version = ?", so.ID(), current).
		Select("status", "services", "parts", "total_amount", "notes", "cancel_reason",
			"version", "updated_at", "started_at", "finished_at").
		Updates(m)
	if res.Error != nil {
		return mapError(res.Error)
	}
	if res.RowsAffected == 0 {
		var count int64
		if err := conn(ctx, r.db).Model(&pgmodel.ServiceOrder{}).Where("id = ?", so.ID()).Count(&count).Error; err != nil {
			return mapError(err)
		}
		if count == 0 {
			return domainerrors.ErrNotFound
		}
		return domainerrors.ErrConcurrentUpdate
	}
	so.SetVersion(m.Version)
	return nil
}

func (r *serviceOrderRepository) Delete(ctx context.Context, id string) error {
	return mapError(conn(ctx, r.db).Delete(&pgmodel.ServiceOrder{}, "id = ?", id).Error)
}

func (r *serviceOrderRepository) AverageExecutionTime(ctx context.Context) (time.Duration, error) {
	var averageSeconds float64
	err := conn(ctx, r.db).
		Model(&pgmodel.ServiceOrder{}).
		Where("started_at IS NOT NULL AND finished_at IS NOT NULL").
		Select("COALESCE(EXTRACT(EPOCH FROM AVG(finished_at - started_at)), 0)").
		Scan(&averageSeconds).Error
	if err != nil {
		return 0, mapError(err)
	}
	return time.Duration(averageSeconds * float64(time.Second)), nil
}

// OS em andamento (nem entregue nem cancelada) cujo JSON de itens (coluna text) contém o id.
const (
	activeWithService = `status NOT IN ? AND COALESCE(NULLIF(services, ''), '[]')::jsonb @> jsonb_build_array(jsonb_build_object('service_id', ?::text))`
	activeWithPart    = `status NOT IN ? AND COALESCE(NULLIF(parts, ''), '[]')::jsonb @> jsonb_build_array(jsonb_build_object('part_id', ?::text))`
)

func (r *serviceOrderRepository) existsActive(ctx context.Context, where, id string) (bool, error) {
	var n int64
	finals := []entities.OrderStatus{entities.StatusDelivered, entities.StatusCancelled}
	if err := conn(ctx, r.db).Model(&pgmodel.ServiceOrder{}).Where(where, finals, id).Count(&n).Error; err != nil {
		return false, mapError(err)
	}
	return n > 0, nil
}

func (r *serviceOrderRepository) ExistsActiveWithService(ctx context.Context, serviceID string) (bool, error) {
	return r.existsActive(ctx, activeWithService, serviceID)
}

func (r *serviceOrderRepository) ExistsActiveWithPart(ctx context.Context, partID string) (bool, error) {
	return r.existsActive(ctx, activeWithPart, partID)
}
