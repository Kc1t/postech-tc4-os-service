package entities

import (
	"errors"
	"testing"
	"time"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
)

func TestSaga_AdvanceRespeitaEtapa(t *testing.T) {
	s := NewSaga("s1", "o1", "payment")
	if s.State != SagaAwaitingDiagnosis || s.SimulateFailure != "payment" {
		t.Fatal("saga nova inconsistente")
	}
	s.WaitUntil(time.Now().Add(time.Hour))
	if err := s.Advance(SagaAwaitingDiagnosis, SagaAwaitingBudget); err != nil {
		t.Fatal(err)
	}
	if s.DeadlineAt != nil {
		t.Fatal("avançar limpa o prazo da etapa anterior")
	}
	if err := s.Advance(SagaAwaitingDiagnosis, SagaAwaitingBudget); !errors.Is(err, domainerrors.ErrSagaOutOfStep) {
		t.Fatal("resposta repetida deve ser ErrSagaOutOfStep")
	}
}

func TestSaga_Expired(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	s := NewSaga("s1", "o1", "")
	if s.Expired(now) {
		t.Fatal("sem prazo não expira")
	}
	s.WaitUntil(now.Add(-time.Minute))
	if !s.Expired(now) {
		t.Fatal("prazo vencido deveria expirar")
	}
	s.State = SagaCancelled
	if s.Expired(now) {
		t.Fatal("saga encerrada não expira")
	}
}

func TestSaga_CompensateSemPagamento(t *testing.T) {
	s := NewSaga("s1", "o1", "")
	s.State = SagaAwaitingApproval
	s.BudgetID = "b1"
	s.Reservations = []PartReservation{{PartID: "p1", Quantity: 2}}
	plan, err := s.Compensate("recusado")
	if err != nil {
		t.Fatal(err)
	}
	if plan.RefundPayment || !plan.CancelBudget || len(plan.ReleaseParts) != 1 || !plan.CancelExecution {
		t.Fatalf("plano errado: %+v", plan)
	}
	if s.State != SagaCancelled || s.FailureReason != "recusado" || s.Reservations != nil {
		t.Fatalf("saga após compensar: %+v", s)
	}
	if _, err := s.Compensate("de novo"); !errors.Is(err, domainerrors.ErrSagaOutOfStep) {
		t.Fatal("compensar duas vezes deve falhar")
	}
}

func TestSaga_CompensateComPagamentoAguardaEstorno(t *testing.T) {
	s := NewSaga("s1", "o1", "")
	s.State = SagaExecuting
	s.BudgetID = "b1"
	s.PaymentApproved = true
	plan, err := s.Compensate("reparo recusado")
	if err != nil || !plan.RefundPayment {
		t.Fatalf("plano deveria estornar: %+v %v", plan, err)
	}
	if s.State != SagaCompensating || s.State.IsActive() {
		t.Fatal("saga deve aguardar o estorno em compensating")
	}
	if err := s.ConfirmRefund(); err != nil || s.State != SagaCancelled {
		t.Fatal("estorno confirmado encerra a saga")
	}
	if err := s.ConfirmRefund(); !errors.Is(err, domainerrors.ErrSagaOutOfStep) {
		t.Fatal("confirmação repetida deve ser ignorada")
	}
}

func TestSaga_RefundLatePayment(t *testing.T) {
	s := NewSaga("s1", "o1", "")
	if err := s.RefundLatePayment("p1"); !errors.Is(err, domainerrors.ErrSagaOutOfStep) {
		t.Fatal("saga ativa não estorna pagamento tardio")
	}
	s.State = SagaCancelled
	if err := s.RefundLatePayment("p1"); err != nil {
		t.Fatal(err)
	}
	if s.State != SagaCompensating || !s.PaymentApproved || s.PaymentID != "p1" {
		t.Fatalf("saga após pagamento tardio: %+v", s)
	}
}
