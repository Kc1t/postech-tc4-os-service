package entities

import (
	"time"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
)

// SagaState é a etapa em que a saga de uma OS está esperando resposta.
type SagaState string

const (
	SagaAwaitingDiagnosis SagaState = "awaiting_diagnosis" // StartDiagnosis enviado
	SagaAwaitingBudget    SagaState = "awaiting_budget"    // peças reservadas, CreateBudget enviado
	SagaAwaitingApproval  SagaState = "awaiting_approval"  // orçamento criado, cliente decide
	SagaAwaitingPayment   SagaState = "awaiting_payment"   // RequestPayment enviado
	SagaExecuting         SagaState = "executing"          // pago, StartRepair enviado
	SagaCompleted         SagaState = "completed"          // reparo concluído
	SagaCompensating      SagaState = "compensating"       // aguardando confirmação do estorno
	SagaCancelled         SagaState = "cancelled"          // compensações concluídas
)

// IsActive diz se a saga ainda aceita avanços do caminho feliz.
func (s SagaState) IsActive() bool {
	switch s {
	case SagaCompleted, SagaCompensating, SagaCancelled:
		return false
	}
	return true
}

// PartReservation é uma peça baixada do estoque para a OS, devolvida na compensação.
type PartReservation struct {
	PartID   string `json:"part_id"`
	Quantity int    `json:"quantity"`
}

// Saga é o estado persistido do orquestrador para uma OS (uma saga por OS).
type Saga struct {
	ID              string
	OSID            string
	State           SagaState
	BudgetID        string
	PaymentID       string
	CheckoutURL     string
	PaymentApproved bool
	Reservations    []PartReservation
	SimulateFailure string
	DeadlineAt      *time.Time
	FailureReason   string
	Version         int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewSaga inicia a saga de uma OS recém-aberta.
func NewSaga(id, osID, simulateFailure string) *Saga {
	now := time.Now().UTC()
	return &Saga{
		ID:              id,
		OSID:            osID,
		State:           SagaAwaitingDiagnosis,
		SimulateFailure: simulateFailure,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// Advance move a saga de expected para next. Se a saga não está em expected, a resposta é
// tardia, duplicada ou fora de ordem e deve ser ignorada (ErrSagaOutOfStep).
func (s *Saga) Advance(expected, next SagaState) error {
	if s.State != expected {
		return domainerrors.ErrSagaOutOfStep
	}
	s.State = next
	s.DeadlineAt = nil
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// WaitUntil define o prazo da etapa atual (aprovação ou pagamento).
func (s *Saga) WaitUntil(deadline time.Time) {
	d := deadline.UTC()
	s.DeadlineAt = &d
}

// Expired diz se a etapa atual passou do prazo.
func (s *Saga) Expired(now time.Time) bool {
	return s.State.IsActive() && s.DeadlineAt != nil && now.After(*s.DeadlineAt)
}

// CompensationPlan lista as transações compensatórias necessárias, na ordem inversa das etapas
// já concluídas.
type CompensationPlan struct {
	RefundPayment   bool
	CancelBudget    bool
	ReleaseParts    []PartReservation
	CancelExecution bool
}

// Compensate encerra o caminho feliz e devolve o plano de compensação. Quando há pagamento
// aprovado a saga fica em compensating até o estorno ser confirmado; nos demais casos as
// compensações restantes são comandos confiáveis (outbox) e a saga termina como cancelled.
func (s *Saga) Compensate(reason string) (CompensationPlan, error) {
	if !s.State.IsActive() {
		return CompensationPlan{}, domainerrors.ErrSagaOutOfStep
	}
	plan := CompensationPlan{
		RefundPayment:   s.PaymentApproved,
		CancelBudget:    s.BudgetID != "",
		ReleaseParts:    s.Reservations,
		CancelExecution: true,
	}
	s.Reservations = nil
	s.FailureReason = reason
	s.DeadlineAt = nil
	s.UpdatedAt = time.Now().UTC()
	if plan.RefundPayment {
		s.State = SagaCompensating
	} else {
		s.State = SagaCancelled
	}
	return plan, nil
}

// ConfirmRefund conclui a compensação após o estorno.
func (s *Saga) ConfirmRefund() error {
	if s.State != SagaCompensating {
		return domainerrors.ErrSagaOutOfStep
	}
	s.State = SagaCancelled
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// RefundLatePayment trata um pagamento aprovado depois que a saga já foi cancelada (por exemplo,
// o prazo de pagamento venceu enquanto o cliente pagava): o valor precisa ser estornado, então a
// saga volta para compensating até o PaymentRefunded.
func (s *Saga) RefundLatePayment(paymentID string) error {
	if s.State != SagaCancelled || s.PaymentApproved {
		return domainerrors.ErrSagaOutOfStep
	}
	s.PaymentApproved = true
	s.PaymentID = paymentID
	s.State = SagaCompensating
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// StatusHistoryEntry registra cada mudança de status da OS (consulta de histórico).
type StatusHistoryEntry struct {
	ID        string
	OSID      string
	From      OrderStatus
	To        OrderStatus
	Actor     string // "saga", "staff", "system"
	Reason    string
	MessageID string
	CreatedAt time.Time
}

// Atores registrados no histórico.
const (
	ActorSaga   = "saga"
	ActorStaff  = "staff"
	ActorSystem = "system"
)
