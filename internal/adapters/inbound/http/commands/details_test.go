package commands

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

func TestToServiceOrderDetailsResponse_Completo(t *testing.T) {
	started := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Hour)
	deadline := started.Add(24 * time.Hour)
	so := entities.ReconstituteServiceOrder(entities.ServiceOrderSnapshot{
		ID: "os-1", Code: 7, RequesterID: "c-1", VehicleID: "v-1", Status: entities.StatusFinished,
		StartedAt: &started, FinishedAt: &finished,
	})
	d := &ports.ServiceOrderDetails{
		Order: so,
		Saga: &entities.Saga{
			ID: "s-1", State: entities.SagaState("completed"), BudgetID: "b-1", PaymentID: "p-1",
			CheckoutURL: "https://mp/checkout", DeadlineAt: &deadline,
		},
		History: []*entities.StatusHistoryEntry{{
			From: entities.StatusReceived, To: entities.StatusFinished, Actor: entities.ActorSaga,
			MessageID: "m-1", CreatedAt: finished.In(time.FixedZone("BRT", -3*3600)),
		}},
		Budget:    json.RawMessage(`{"id":"b-1"}`),
		Execution: json.RawMessage(`{"status":"finished"}`),
	}

	resp := ToServiceOrderDetailsResponse(d)
	if resp.ServiceOrder.ID != "os-1" || resp.ServiceOrder.StartedAt == nil || *resp.ServiceOrder.StartedAt != "2026-11-02T10:00:00Z" ||
		resp.ServiceOrder.FinishedAt == nil || *resp.ServiceOrder.FinishedAt != "2026-11-02T12:00:00Z" {
		t.Fatalf("OS: %+v", resp.ServiceOrder)
	}
	if resp.Saga == nil || resp.Saga.ID != "s-1" || resp.Saga.CheckoutURL != "https://mp/checkout" ||
		resp.Saga.DeadlineAt == nil || *resp.Saga.DeadlineAt != "2026-11-03T10:00:00Z" {
		t.Fatalf("saga: %+v", resp.Saga)
	}
	if len(resp.History) != 1 || resp.History[0].At != "2026-11-02T12:00:00Z" || resp.History[0].MessageID != "m-1" {
		t.Fatalf("histórico em UTC: %+v", resp.History)
	}
	if string(resp.Budget) != `{"id":"b-1"}` || string(resp.Execution) != `{"status":"finished"}` {
		t.Errorf("budget %s, execution %s", resp.Budget, resp.Execution)
	}
	if resp.Partial || resp.Unavailable == nil || len(resp.Unavailable) != 0 {
		t.Errorf("sem falha, partial=false e unavailable=[]: %v %v", resp.Partial, resp.Unavailable)
	}
}

func TestToServiceOrderDetailsResponse_Parcial(t *testing.T) {
	d := &ports.ServiceOrderDetails{
		Order:       entities.NewServiceOrder("c-1", "v-1"),
		Unavailable: []string{"billing", "execution"},
	}
	resp := ToServiceOrderDetailsResponse(d)
	if resp.Saga != nil || resp.History == nil || len(resp.History) != 0 {
		t.Fatalf("sem saga e histórico vazio: %+v", resp)
	}
	if string(resp.Budget) != "null" || string(resp.Execution) != "null" {
		t.Errorf("serviço fora vira null: budget %s, execution %s", resp.Budget, resp.Execution)
	}
	if !resp.Partial || len(resp.Unavailable) != 2 {
		t.Errorf("partial=%v unavailable=%v", resp.Partial, resp.Unavailable)
	}

	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	_ = json.Unmarshal(body, &raw)
	if raw["budget"] != nil || raw["saga"] != nil {
		t.Errorf("JSON deve trazer null: %s", body)
	}
}
