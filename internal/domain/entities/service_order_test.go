package entities

import (
	"errors"
	"testing"
	"time"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
)

func orderAt(status OrderStatus) *ServiceOrder {
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return ReconstituteServiceOrder(ServiceOrderSnapshot{
		ID: "order-1", RequesterID: "cust-1", VehicleID: "veh-1", Status: status, Version: 3,
		CreatedAt: t, UpdatedAt: t,
	})
}

func TestNewServiceOrder_StatusInicial(t *testing.T) {
	so := NewServiceOrder("cust-1", "veh-1")
	if so.Status() != StatusReceived || so.TotalAmount() != 0 || len(so.Services()) != 0 || len(so.Parts()) != 0 {
		t.Fatalf("OS nova inconsistente: %+v", so)
	}
	if so.RequesterID() != "cust-1" || so.VehicleID() != "veh-1" || so.CreatedAt().IsZero() {
		t.Fatal("campos de criação não preenchidos")
	}
}

func TestTransitionTo_CaminhoFeliz(t *testing.T) {
	so := NewServiceOrder("c", "v")
	for _, s := range []OrderStatus{
		StatusInDiagnosis, StatusAwaitingBudget, StatusAwaitingApproval, StatusAwaitingPayment,
		StatusInExecution, StatusFinished, StatusDelivered,
	} {
		if err := so.TransitionTo(s); err != nil {
			t.Fatalf("transição para %s: %v", s, err)
		}
	}
	if !so.Status().IsFinal() {
		t.Fatal("delivered é final")
	}
}

func TestTransitionTo_DiagnosticoForaDeOrdem(t *testing.T) {
	// DiagnosisCompleted pode chegar antes de DiagnosisStarted.
	if err := orderAt(StatusReceived).TransitionTo(StatusAwaitingBudget); err != nil {
		t.Fatalf("received → awaiting_budget deve ser aceito: %v", err)
	}
}

func TestTransitionTo_Invalidas(t *testing.T) {
	cases := []struct{ from, to OrderStatus }{
		{StatusReceived, StatusInExecution},
		{StatusAwaitingApproval, StatusInExecution},
		{StatusAwaitingPayment, StatusFinished},
		{StatusFinished, StatusCancelled},
		{StatusDelivered, StatusReceived},
		{StatusCancelled, StatusReceived},
	}
	for _, c := range cases {
		so := orderAt(c.from)
		if err := so.TransitionTo(c.to); !errors.Is(err, domainerrors.ErrInvalidStatus) {
			t.Errorf("%s → %s: esperava ErrInvalidStatus, veio %v", c.from, c.to, err)
		}
		if so.Status() != c.from {
			t.Errorf("status mudou numa transição inválida (%s)", so.Status())
		}
	}
}

func TestTransitionTo_StatusDesconhecido(t *testing.T) {
	if err := orderAt(StatusReceived).TransitionTo("xpto"); !errors.Is(err, domainerrors.ErrInvalidStatusValue) {
		t.Fatalf("esperava ErrInvalidStatusValue, veio %v", err)
	}
	if IsValidStatus("xpto") || !IsValidStatus(StatusCancelled) {
		t.Fatal("IsValidStatus incorreto")
	}
}

func TestCancel(t *testing.T) {
	for _, s := range []OrderStatus{StatusReceived, StatusInDiagnosis, StatusAwaitingBudget,
		StatusAwaitingApproval, StatusAwaitingPayment, StatusInExecution} {
		so := orderAt(s)
		if err := so.Cancel("motivo"); err != nil {
			t.Fatalf("cancelar em %s: %v", s, err)
		}
		if so.Status() != StatusCancelled || so.CancelReason() != "motivo" {
			t.Fatalf("cancelamento não registrado em %s", s)
		}
	}
	for _, s := range []OrderStatus{StatusFinished, StatusDelivered, StatusCancelled} {
		if err := orderAt(s).Cancel("x"); !errors.Is(err, domainerrors.ErrOrderNotCancellable) {
			t.Fatalf("%s não pode ser cancelada, veio %v", s, err)
		}
	}
}

func TestMarkRepairStartedEFinish(t *testing.T) {
	so := orderAt(StatusInExecution)
	start := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	so.MarkRepairStarted(start)
	so.MarkRepairStarted(start.Add(time.Hour)) // segunda chamada não sobrescreve
	if so.StartedAt() == nil || !so.StartedAt().Equal(start) {
		t.Fatalf("startedAt = %v", so.StartedAt())
	}
	end := start.Add(90 * time.Minute)
	if err := so.Finish(end); err != nil {
		t.Fatal(err)
	}
	if so.Status() != StatusFinished || !so.FinishedAt().Equal(end) {
		t.Fatal("finish não registrado")
	}
}

func TestFinish_SemInicioUsaFimComoInicio(t *testing.T) {
	so := orderAt(StatusInExecution)
	end := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	if err := so.Finish(end); err != nil {
		t.Fatal(err)
	}
	if !so.StartedAt().Equal(end) {
		t.Fatal("startedAt deveria ser preenchido")
	}
	if err := orderAt(StatusAwaitingPayment).Finish(end); !errors.Is(err, domainerrors.ErrInvalidStatus) {
		t.Fatal("finish fora de execução deve falhar")
	}
}

func TestItensRecalculamTotal(t *testing.T) {
	so := NewServiceOrder("c", "v")
	so.SetServices([]ServiceItem{{ServiceID: "s1", Code: 1, Description: "Troca de óleo", Price: 100}})
	so.SetParts([]PartItem{{PartID: "p1", ManufacturerCode: "F-1", Quantity: 2, UnitPrice: 25.5}})
	if so.TotalAmount() != 151 {
		t.Fatalf("total = %.2f", so.TotalAmount())
	}
	so.SetServices(nil)
	if so.TotalAmount() != 51 {
		t.Fatalf("total após remover serviços = %.2f", so.TotalAmount())
	}
}

func TestSettersESnapshot(t *testing.T) {
	so := orderAt(StatusReceived)
	so.SetID("x")
	so.SetCode(7)
	so.SetVersion(9)
	so.SetNotes("barulho")
	if so.ID() != "x" || so.Code() != 7 || so.Version() != 9 || so.Notes() != "barulho" || so.UpdatedAt().IsZero() {
		t.Fatal("setters não aplicados")
	}
}
