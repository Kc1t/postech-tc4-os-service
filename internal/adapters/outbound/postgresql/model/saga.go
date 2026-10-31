package pgmodel

import (
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
)

// Saga e a tabela saga_instances: estado do orquestrador, uma linha por OS.
type Saga struct {
	ID              string             `gorm:"primaryKey;type:uuid"`
	OSID            string             `gorm:"type:uuid;not null;uniqueIndex"`
	State           entities.SagaState `gorm:"not null;index"`
	BudgetID        string
	PaymentID       string
	CheckoutURL     string
	PaymentApproved bool                       `gorm:"not null;default:false"`
	Reservations    []entities.PartReservation `gorm:"serializer:json"`
	SimulateFailure string
	DeadlineAt      *time.Time `gorm:"index"`
	FailureReason   string
	Version         int `gorm:"not null;default:1"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (Saga) TableName() string { return "saga_instances" }

func FromSaga(s *entities.Saga) *Saga {
	v := s.Version
	if v == 0 {
		v = 1
	}
	return &Saga{
		ID: s.ID, OSID: s.OSID, State: s.State, BudgetID: s.BudgetID, PaymentID: s.PaymentID,
		CheckoutURL: s.CheckoutURL, PaymentApproved: s.PaymentApproved, Reservations: s.Reservations,
		SimulateFailure: s.SimulateFailure, DeadlineAt: s.DeadlineAt, FailureReason: s.FailureReason,
		Version: v, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

func (m *Saga) ToDomain() *entities.Saga {
	return &entities.Saga{
		ID: m.ID, OSID: m.OSID, State: m.State, BudgetID: m.BudgetID, PaymentID: m.PaymentID,
		CheckoutURL: m.CheckoutURL, PaymentApproved: m.PaymentApproved, Reservations: m.Reservations,
		SimulateFailure: m.SimulateFailure, DeadlineAt: m.DeadlineAt, FailureReason: m.FailureReason,
		Version: m.Version, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// StatusHistory e a tabela service_order_status_history.
type StatusHistory struct {
	ID         string               `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	OSID       string               `gorm:"type:uuid;not null;index"`
	FromStatus entities.OrderStatus `gorm:"not null"`
	ToStatus   entities.OrderStatus `gorm:"not null"`
	Actor      string               `gorm:"not null"`
	Reason     string
	MessageID  string
	CreatedAt  time.Time `gorm:"not null;index"`
}

func (StatusHistory) TableName() string { return "service_order_status_history" }

func FromStatusHistory(e *entities.StatusHistoryEntry) *StatusHistory {
	return &StatusHistory{
		ID: e.ID, OSID: e.OSID, FromStatus: e.From, ToStatus: e.To, Actor: e.Actor,
		Reason: e.Reason, MessageID: e.MessageID, CreatedAt: e.CreatedAt,
	}
}

func (m *StatusHistory) ToDomain() *entities.StatusHistoryEntry {
	return &entities.StatusHistoryEntry{
		ID: m.ID, OSID: m.OSID, From: m.FromStatus, To: m.ToStatus, Actor: m.Actor,
		Reason: m.Reason, MessageID: m.MessageID, CreatedAt: m.CreatedAt,
	}
}
