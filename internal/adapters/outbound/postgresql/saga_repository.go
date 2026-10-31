package postgresql

import (
	"context"
	"time"

	pgmodel "github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql/model"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type sagaRepository struct{ db *gorm.DB }

func NewSagaRepository(db *gorm.DB) ports.SagaRepository { return &sagaRepository{db: db} }

func (r *sagaRepository) Create(ctx context.Context, s *entities.Saga) error {
	m := pgmodel.FromSaga(s)
	if err := conn(ctx, r.db).Create(m).Error; err != nil {
		return mapError(err)
	}
	s.Version = m.Version
	return nil
}

func (r *sagaRepository) FindByOSIDForUpdate(ctx context.Context, osID string) (*entities.Saga, error) {
	var m pgmodel.Saga
	if err := conn(ctx, r.db).Clauses(clause.Locking{Strength: "UPDATE"}).First(&m, "os_id = ?", osID).Error; err != nil {
		return nil, mapError(err)
	}
	return m.ToDomain(), nil
}

func (r *sagaRepository) FindByOSID(ctx context.Context, osID string) (*entities.Saga, error) {
	var m pgmodel.Saga
	if err := conn(ctx, r.db).First(&m, "os_id = ?", osID).Error; err != nil {
		return nil, mapError(err)
	}
	return m.ToDomain(), nil
}

func (r *sagaRepository) Update(ctx context.Context, s *entities.Saga) error {
	m := pgmodel.FromSaga(s)
	current := m.Version
	m.Version = current + 1
	res := conn(ctx, r.db).Model(&pgmodel.Saga{}).
		Where("id = ? AND version = ?", s.ID, current).
		Select("state", "budget_id", "payment_id", "checkout_url", "payment_approved", "reservations",
			"simulate_failure", "deadline_at", "failure_reason", "version", "updated_at").
		Updates(m)
	if res.Error != nil {
		return mapError(res.Error)
	}
	if res.RowsAffected == 0 {
		return domainerrors.ErrConcurrentUpdate
	}
	s.Version = m.Version
	return nil
}

func (r *sagaRepository) ListExpired(ctx context.Context, now time.Time, limit int) ([]*entities.Saga, error) {
	var docs []pgmodel.Saga
	err := conn(ctx, r.db).
		Where("deadline_at IS NOT NULL AND deadline_at < ? AND state IN ?", now.UTC(),
			[]string{string(entities.SagaAwaitingApproval), string(entities.SagaAwaitingPayment)}).
		Order("deadline_at").
		Limit(limit).
		Find(&docs).Error
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*entities.Saga, 0, len(docs))
	for i := range docs {
		out = append(out, docs[i].ToDomain())
	}
	return out, nil
}

type statusHistoryRepository struct{ db *gorm.DB }

func NewStatusHistoryRepository(db *gorm.DB) ports.StatusHistoryRepository {
	return &statusHistoryRepository{db: db}
}

func (r *statusHistoryRepository) Add(ctx context.Context, e *entities.StatusHistoryEntry) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	m := pgmodel.FromStatusHistory(e)
	if err := conn(ctx, r.db).Create(m).Error; err != nil {
		return mapError(err)
	}
	e.ID = m.ID
	return nil
}

func (r *statusHistoryRepository) ListByOS(ctx context.Context, osID string) ([]*entities.StatusHistoryEntry, error) {
	var docs []pgmodel.StatusHistory
	if err := conn(ctx, r.db).Where("os_id = ?", osID).Order("created_at ASC").Find(&docs).Error; err != nil {
		return nil, mapError(err)
	}
	out := make([]*entities.StatusHistoryEntry, 0, len(docs))
	for i := range docs {
		out = append(out, docs[i].ToDomain())
	}
	return out, nil
}
