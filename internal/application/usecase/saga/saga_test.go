package sagauc

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
)

func TestSaga_CaminhoFeliz(t *testing.T) {
	f := newFx(t, Config{ApprovalTimeout: 48 * time.Hour, PaymentTimeout: 12 * time.Hour})
	so := f.open("")

	start := f.sent(so.ID(), contracts.CmdStartDiagnosis)
	if len(start) != 1 {
		t.Fatalf("abertura deve enviar 1 StartDiagnosis, enviou %d", len(start))
	}
	sd := decodeAs[contracts.StartDiagnosis](t, start[0])
	if sd.OSCode != so.Code() || sd.Vehicle.Plate != f.vehicle.Plate() || sd.Notes != "barulho ao frear" {
		t.Fatalf("StartDiagnosis incompleto: %+v", sd)
	}

	f.diagnose(so.ID(), 2)
	if f.order(so.ID()).Status() != entities.StatusAwaitingBudget || f.stock() != 3 {
		t.Fatalf("após diagnóstico: status=%s estoque=%d", f.order(so.ID()).Status(), f.stock())
	}
	cb := decodeAs[contracts.CreateBudget](t, f.sent(so.ID(), contracts.CmdCreateBudget)[0])
	if cb.Total != 330 || len(cb.Items) != 2 || cb.Requester.Email != "cliente@example.com" || cb.SimulateFailure != "" {
		t.Fatalf("CreateBudget: %+v", cb)
	}

	f.send(so.ID(), contracts.EvtBudgetCreated, contracts.BudgetCreated{BudgetID: "b-1", Total: 330})
	s := f.saga(so.ID())
	if s.State != entities.SagaAwaitingApproval || s.BudgetID != "b-1" || !s.DeadlineAt.Equal(f.now.Add(48*time.Hour)) {
		t.Fatalf("saga após orçamento: %+v", s)
	}

	f.send(so.ID(), contracts.EvtBudgetApproved, contracts.BudgetApproved{BudgetID: "b-1", ApprovedAt: f.now})
	rp := decodeAs[contracts.RequestPayment](t, f.sent(so.ID(), contracts.CmdRequestPayment)[0])
	if rp.BudgetID != "b-1" || !f.saga(so.ID()).DeadlineAt.Equal(f.now.Add(12*time.Hour)) {
		t.Fatalf("RequestPayment: %+v", rp)
	}

	f.send(so.ID(), contracts.EvtPaymentLinkCreated, contracts.PaymentLinkCreated{BudgetID: "b-1", PaymentID: "p-1", CheckoutURL: "https://mp/1"})
	events := f.sent(so.ID(), contracts.EvtServiceOrderStatusChanged)
	last := decodeAs[contracts.ServiceOrderStatusChanged](t, events[len(events)-1])
	if last.Status != string(entities.StatusAwaitingPayment) || last.CheckoutURL != "https://mp/1" {
		t.Fatalf("evento com link de pagamento: %+v", last)
	}

	f.send(so.ID(), contracts.EvtPaymentApproved, contracts.PaymentApproved{BudgetID: "b-1", PaymentID: "p-1", Amount: 330})
	sr := decodeAs[contracts.StartRepair](t, f.sent(so.ID(), contracts.CmdStartRepair)[0])
	if len(sr.Items) != 2 || f.order(so.ID()).Status() != entities.StatusInExecution || !f.saga(so.ID()).PaymentApproved {
		t.Fatalf("StartRepair: %+v", sr)
	}

	started := f.now.Add(time.Hour)
	f.send(so.ID(), contracts.EvtRepairStarted, contracts.RepairStarted{StartedAt: started})
	f.send(so.ID(), contracts.EvtRepairFinished, contracts.RepairFinished{FinishedAt: started.Add(2 * time.Hour)})
	done := f.order(so.ID())
	if done.Status() != entities.StatusFinished || f.saga(so.ID()).State != entities.SagaCompleted ||
		done.FinishedAt().Sub(*done.StartedAt()) != 2*time.Hour {
		t.Fatalf("finalização: %s %v %v", done.Status(), done.StartedAt(), done.FinishedAt())
	}

	must(t, f.deliver.Execute(f.ctx, so.ID()))
	want := []entities.OrderStatus{
		entities.StatusReceived, entities.StatusInDiagnosis, entities.StatusAwaitingBudget, entities.StatusAwaitingApproval,
		entities.StatusAwaitingPayment, entities.StatusInExecution, entities.StatusFinished, entities.StatusDelivered,
	}
	if got := f.statuses(so.ID()); !slices.Equal(got, want) {
		t.Fatalf("histórico = %v", got)
	}
	// received, in_diagnosis, awaiting_budget, awaiting_approval, awaiting_payment (com link),
	// in_execution, finished, delivered
	if n := len(f.sent(so.ID(), contracts.EvtServiceOrderStatusChanged)); n != 8 {
		t.Fatalf("eventos de status publicados: %d", n)
	}
	if err := f.deliver.Execute(f.ctx, so.ID()); !errors.Is(err, domainerrors.ErrInvalidStatus) {
		t.Fatalf("entregar duas vezes: %v", err)
	}
}

func TestSaga_MensagemDuplicadaNaoReaplica(t *testing.T) {
	f := newFx(t, Config{})
	so := f.open("")
	f.send(so.ID(), contracts.EvtDiagnosisStarted, contracts.DiagnosisStarted{})
	completed := f.send(so.ID(), contracts.EvtDiagnosisCompleted, contracts.DiagnosisCompleted{
		Parts: []contracts.DiagnosedPart{{ManufacturerCode: f.part.ManufacturerCode(), Quantity: 1}},
	})
	must(t, f.reply.Execute(f.ctx, completed)) // reentrega do SQS: mesmo message_id
	if f.stock() != 4 || len(f.sent(so.ID(), contracts.CmdCreateBudget)) != 1 {
		t.Fatalf("efeitos duplicados: estoque=%d", f.stock())
	}
	// mesma resposta com outro id (produtor reenviou): a saga já avançou e ignora
	f.send(so.ID(), contracts.EvtDiagnosisCompleted, contracts.DiagnosisCompleted{
		Parts: []contracts.DiagnosedPart{{ManufacturerCode: f.part.ManufacturerCode(), Quantity: 1}},
	})
	if f.stock() != 4 || len(f.sent(so.ID(), contracts.CmdCreateBudget)) != 1 {
		t.Fatal("resposta fora de etapa não pode ter efeito")
	}
}

func TestSaga_DiagnosticoForaDeOrdem(t *testing.T) {
	f := newFx(t, Config{})
	so := f.open("")
	f.send(so.ID(), contracts.EvtDiagnosisCompleted, contracts.DiagnosisCompleted{
		Services: []contracts.DiagnosedService{{Code: f.service.Code()}, {Code: f.service.Code()}},
	})
	f.send(so.ID(), contracts.EvtDiagnosisStarted, contracts.DiagnosisStarted{})
	got := f.order(so.ID())
	if got.Status() != entities.StatusAwaitingBudget || len(got.Services()) != 1 || got.TotalAmount() != 150 {
		t.Fatalf("OS: %s serviços=%d total=%.2f", got.Status(), len(got.Services()), got.TotalAmount())
	}
}

func TestSaga_OrcamentoRecusadoCompensa(t *testing.T) {
	f := newFx(t, Config{})
	so := f.open("")
	f.diagnose(so.ID(), 2)
	f.send(so.ID(), contracts.EvtBudgetCreated, contracts.BudgetCreated{BudgetID: "b-1"})
	f.send(so.ID(), contracts.EvtBudgetRejected, contracts.BudgetRejected{BudgetID: "b-1", Reason: "caro demais"})

	got := f.order(so.ID())
	if got.Status() != entities.StatusCancelled || got.CancelReason() != "orçamento recusado pelo cliente: caro demais" {
		t.Fatalf("OS: %s %q", got.Status(), got.CancelReason())
	}
	if f.stock() != 5 {
		t.Fatalf("peças não voltaram ao estoque: %d", f.stock())
	}
	if len(f.sent(so.ID(), contracts.CmdCancelExecution)) != 1 || len(f.sent(so.ID(), contracts.CmdCancelBudget)) != 0 ||
		len(f.sent(so.ID(), contracts.CmdRefundPayment)) != 0 {
		t.Fatal("compensações erradas para orçamento recusado")
	}
	if s := f.saga(so.ID()); s.State != entities.SagaCancelled || s.Reservations != nil {
		t.Fatalf("saga: %+v", s)
	}
	// resposta tardia depois do cancelamento é ignorada
	f.send(so.ID(), contracts.EvtBudgetApproved, contracts.BudgetApproved{BudgetID: "b-1"})
	if f.order(so.ID()).Status() != entities.StatusCancelled {
		t.Fatal("OS cancelada não pode voltar ao fluxo")
	}
}

func TestSaga_FalhaNaCriacaoDoOrcamento(t *testing.T) {
	f := newFx(t, Config{})
	so := f.open("")
	f.diagnose(so.ID(), 1)
	f.send(so.ID(), contracts.EvtBudgetCreationFailed, contracts.BudgetCreationFailed{Reason: "falha simulada"})
	if f.order(so.ID()).Status() != entities.StatusCancelled || f.stock() != 5 || len(f.sent(so.ID(), contracts.CmdCancelBudget)) != 0 {
		t.Fatal("falha na criação do orçamento deve compensar sem CancelBudget")
	}
}

func TestSaga_EstoqueInsuficienteCompensa(t *testing.T) {
	f := newFx(t, Config{})
	so := f.open("")
	f.diagnose(so.ID(), 9)
	got := f.order(so.ID())
	if got.Status() != entities.StatusCancelled || f.stock() != 5 || len(f.sent(so.ID(), contracts.CmdCreateBudget)) != 0 {
		t.Fatalf("falta de estoque: status=%s estoque=%d", got.Status(), f.stock())
	}
	if len(f.sent(so.ID(), contracts.CmdCancelExecution)) != 1 {
		t.Fatal("execução deveria ser cancelada")
	}
}

func TestSaga_DiagnosticoInvalidoCompensa(t *testing.T) {
	cases := map[string]contracts.DiagnosisCompleted{
		"serviço inexistente": {Services: []contracts.DiagnosedService{{Code: -1}}},
		"peça inexistente":    {Parts: []contracts.DiagnosedPart{{ManufacturerCode: "NAO-EXISTE", Quantity: 1}}},
		"quantidade inválida": {Parts: []contracts.DiagnosedPart{{ManufacturerCode: "X", Quantity: 0}}},
		"vazio":               {},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFx(t, Config{})
			so := f.open("")
			f.send(so.ID(), contracts.EvtDiagnosisCompleted, p)
			if got := f.order(so.ID()); got.Status() != entities.StatusCancelled || got.CancelReason() == "" {
				t.Fatalf("status=%s motivo=%q", got.Status(), got.CancelReason())
			}
		})
	}
}

func TestSaga_PagamentoRecusadoCompensa(t *testing.T) {
	f := newFx(t, Config{})
	so := f.open("")
	f.toAwaitingPayment(so.ID())
	f.send(so.ID(), contracts.EvtPaymentFailed, contracts.PaymentFailed{Reason: "cartão recusado"})
	if f.order(so.ID()).Status() != entities.StatusCancelled || f.stock() != 5 {
		t.Fatal("pagamento recusado deve cancelar a OS e devolver as peças")
	}
	if len(f.sent(so.ID(), contracts.CmdCancelBudget)) != 1 || len(f.sent(so.ID(), contracts.CmdRefundPayment)) != 0 {
		t.Fatal("deve cancelar o orçamento e não estornar")
	}
}

func TestSaga_ReparoRecusadoEstornaEConfirma(t *testing.T) {
	f := newFx(t, Config{})
	so := f.open("")
	f.toExecuting(so.ID())
	f.send(so.ID(), contracts.EvtRepairRejected, contracts.RepairRejected{Reason: "capacidade esgotada"})

	if s := f.saga(so.ID()); s.State != entities.SagaCompensating {
		t.Fatalf("saga deve aguardar o estorno: %s", s.State)
	}
	refund := decodeAs[contracts.RefundPayment](t, f.sent(so.ID(), contracts.CmdRefundPayment)[0])
	if refund.BudgetID == "" || len(f.sent(so.ID(), contracts.CmdCancelExecution)) != 0 || f.stock() != 5 {
		t.Fatalf("compensação do reparo: %+v", refund)
	}
	f.send(so.ID(), contracts.EvtPaymentRefunded, contracts.PaymentRefunded{PaymentID: "pay-1", Amount: 330})
	if f.saga(so.ID()).State != entities.SagaCancelled || f.order(so.ID()).Status() != entities.StatusCancelled {
		t.Fatal("estorno confirmado encerra a saga")
	}
}

func TestSaga_PrazoDeAprovacaoExpiraEPagamentoTardioEstorna(t *testing.T) {
	f := newFx(t, Config{ApprovalTimeout: time.Hour})
	so := f.open("")
	f.diagnose(so.ID(), 1)
	f.send(so.ID(), contracts.EvtBudgetCreated, contracts.BudgetCreated{BudgetID: "b-1"})

	n, err := f.expire.Execute(f.ctx, f.now.Add(30*time.Minute))
	if err != nil || f.order(so.ID()).Status() != entities.StatusAwaitingApproval {
		t.Fatalf("dentro do prazo não expira: n=%d err=%v", n, err)
	}
	if _, err := f.expire.Execute(f.ctx, f.now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got := f.order(so.ID())
	if got.Status() != entities.StatusCancelled || got.CancelReason() != "prazo de aprovação do orçamento expirado" {
		t.Fatalf("expiração: %s %q", got.Status(), got.CancelReason())
	}
	if len(f.sent(so.ID(), contracts.CmdCancelBudget)) != 1 || f.stock() != 5 {
		t.Fatal("expiração deve cancelar o orçamento e devolver peças")
	}
	h, _ := f.o.d.History.ListByOS(f.ctx, so.ID())
	if h[len(h)-1].Actor != entities.ActorSystem {
		t.Fatalf("ator da expiração = %s", h[len(h)-1].Actor)
	}

	// o cliente pagou depois do cancelamento: o valor volta
	f.send(so.ID(), contracts.EvtPaymentApproved, contracts.PaymentApproved{BudgetID: "b-1", PaymentID: "p-late"})
	if s := f.saga(so.ID()); s.State != entities.SagaCompensating || len(f.sent(so.ID(), contracts.CmdRefundPayment)) != 1 {
		t.Fatalf("pagamento tardio deve gerar estorno: %+v", s)
	}
}

func TestSaga_PrazoDePagamentoExpira(t *testing.T) {
	f := newFx(t, Config{PaymentTimeout: time.Hour})
	so := f.open("")
	f.toAwaitingPayment(so.ID())
	if _, err := f.expire.Execute(f.ctx, f.now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := f.order(so.ID()); got.CancelReason() != "prazo de pagamento expirado" {
		t.Fatalf("motivo = %q", got.CancelReason())
	}
}

func TestSaga_CancelamentoPelaOficina(t *testing.T) {
	f := newFx(t, Config{})
	so := f.open("")
	f.toExecuting(so.ID())
	must(t, f.cancel.Execute(f.ctx, so.ID(), "cliente retirou o carro"))
	got := f.order(so.ID())
	if got.Status() != entities.StatusCancelled || got.CancelReason() != "cancelada pela oficina: cliente retirou o carro" {
		t.Fatalf("%s %q", got.Status(), got.CancelReason())
	}
	if len(f.sent(so.ID(), contracts.CmdRefundPayment)) != 1 || len(f.sent(so.ID(), contracts.CmdCancelExecution)) != 1 {
		t.Fatal("cancelamento com pagamento aprovado deve estornar e cancelar a execução")
	}
	if err := f.cancel.Execute(f.ctx, so.ID(), ""); !errors.Is(err, domainerrors.ErrOrderNotCancellable) {
		t.Fatalf("cancelar duas vezes: %v", err)
	}

	recent := f.open("")
	must(t, f.cancel.Execute(f.ctx, recent.ID(), ""))
	if f.order(recent.ID()).CancelReason() != "cancelada pela oficina" {
		t.Fatal("motivo padrão")
	}
	if err := f.cancel.Execute(f.ctx, "00000000-0000-0000-0000-000000000000", ""); !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatalf("OS inexistente: %v", err)
	}
}

func TestSaga_InjecaoDeFalhas(t *testing.T) {
	on := newFx(t, Config{FaultInjection: true})
	so := on.open("Payment")
	if on.saga(so.ID()).SimulateFailure != contracts.FailPayment {
		t.Fatal("falha simulada não registrada")
	}
	on.diagnose(so.ID(), 1)
	if cb := decodeAs[contracts.CreateBudget](t, on.sent(so.ID(), contracts.CmdCreateBudget)[0]); cb.SimulateFailure != "" {
		t.Fatal("falha de pagamento não vai no CreateBudget")
	}
	on.send(so.ID(), contracts.EvtBudgetCreated, contracts.BudgetCreated{BudgetID: "b-1"})
	on.send(so.ID(), contracts.EvtBudgetApproved, contracts.BudgetApproved{BudgetID: "b-1"})
	if rp := decodeAs[contracts.RequestPayment](t, on.sent(so.ID(), contracts.CmdRequestPayment)[0]); rp.SimulateFailure != contracts.FailPayment {
		t.Fatal("falha de pagamento deve ir no RequestPayment")
	}
	if _, err := on.create.Execute(on.ctx, entities.ServiceOrderInput{
		RequesterDocument: on.requester.Document(), VehiclePlate: on.vehicle.Plate(), SimulateFailure: "meteoro",
	}); !errors.Is(err, domainerrors.ErrInvalidSimulatedFailure) {
		t.Fatalf("valor inválido: %v", err)
	}

	off := newFx(t, Config{FaultInjection: false})
	if s := off.saga(off.open("budget").ID()); s.SimulateFailure != "" {
		t.Fatal("com a injeção desligada o campo é ignorado")
	}
}

func TestSaga_AberturaValidaClienteEVeiculo(t *testing.T) {
	f := newFx(t, Config{})
	other := newFx(t, Config{})
	_, err := f.create.Execute(f.ctx, entities.ServiceOrderInput{
		RequesterDocument: f.requester.Document(), VehiclePlate: other.vehicle.Plate(),
	})
	if !errors.Is(err, domainerrors.ErrVehicleNotFromRequester) {
		t.Fatalf("veículo de outro cliente: %v", err)
	}
	if _, err := f.create.Execute(f.ctx, entities.ServiceOrderInput{RequesterDocument: "00000000000", VehiclePlate: f.vehicle.Plate()}); !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatalf("cliente inexistente: %v", err)
	}
	if _, err := f.create.Execute(f.ctx, entities.ServiceOrderInput{RequesterDocument: f.requester.Document(), VehiclePlate: "ZZZ0000"}); !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatalf("veículo inexistente: %v", err)
	}
}

func TestSaga_MensagensSemEfeito(t *testing.T) {
	f := newFx(t, Config{})
	so := f.open("")

	unknown, _ := contracts.NewEnvelope(contracts.EvtBudgetCreated, "billing", contracts.Meta{OSID: "00000000-0000-0000-0000-000000000000"}, contracts.BudgetCreated{})
	must(t, f.reply.Execute(f.ctx, unknown)) // saga desconhecida: descarta
	for _, typ := range []string{contracts.EvtBudgetCancelled, contracts.EvtExecutionCancelled, "billing.Inventado"} {
		e, _ := contracts.NewEnvelope(typ, "x", contracts.Meta{OSID: so.ID()}, struct{}{})
		must(t, f.reply.Execute(f.ctx, e))
	}
	bad, _ := contracts.NewEnvelope(contracts.EvtBudgetCreated, "billing", contracts.Meta{OSID: so.ID()}, struct{}{})
	bad.Payload = []byte(`{"budget_id": 10}`)
	if err := f.reply.Execute(f.ctx, bad); err == nil {
		t.Fatal("payload inválido deve falhar (vai para retry/DLQ)")
	}
	if f.order(so.ID()).Status() != entities.StatusReceived {
		t.Fatal("nenhuma dessas mensagens pode mudar a OS")
	}
	if len(f.reply.Types()) != 15 {
		t.Fatalf("tipos consumidos = %d", len(f.reply.Types()))
	}
}

func TestSaga_RespostasForaDeEtapa(t *testing.T) {
	f := newFx(t, Config{})
	so := f.open("")
	for typ, payload := range map[string]any{
		contracts.EvtBudgetCreationFailed: contracts.BudgetCreationFailed{},
		contracts.EvtBudgetRejected:       contracts.BudgetRejected{},
		contracts.EvtPaymentLinkCreated:   contracts.PaymentLinkCreated{},
		contracts.EvtPaymentFailed:        contracts.PaymentFailed{},
		contracts.EvtPaymentRefunded:      contracts.PaymentRefunded{},
		contracts.EvtRepairStarted:        contracts.RepairStarted{},
		contracts.EvtRepairFinished:       contracts.RepairFinished{},
		contracts.EvtRepairRejected:       contracts.RepairRejected{},
		contracts.EvtBudgetApproved:       contracts.BudgetApproved{},
		contracts.EvtPaymentApproved:      contracts.PaymentApproved{},
	} {
		f.send(so.ID(), typ, payload)
	}
	if f.order(so.ID()).Status() != entities.StatusReceived || f.saga(so.ID()).State != entities.SagaAwaitingDiagnosis {
		t.Fatal("respostas fora de etapa não podem mudar a saga")
	}
}
