package sagauc

// Testes de contrato das mensagens do os-service, sobre o orquestrador real e o Postgres de teste:
//   - produtor: toda mensagem que os casos de uso gravam no outbox passa no JSON Schema do seu
//     tipo (pkg/contracts/schemas);
//   - consumidor: o exemplo publicado no schema de cada resposta da fila os-saga-replies é aceito
//     pelo HandleReply e move a saga como o contrato descreve.

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/outbox"
	"github.com/google/uuid"
)

// outbox devolve todas as mensagens gravadas no outbox para a saga da OS, em ordem.
func (f *fx) outbox(osID string) []outbox.Message {
	f.t.Helper()
	var rows []outbox.Message
	must(f.t, testDB.Where("saga_id = ?", f.saga(osID).ID).Order("created_at").Find(&rows).Error)
	return rows
}

// produced valida contra o schema cada mensagem do outbox da OS e conta os tipos vistos.
type produced map[string]int

func (p produced) check(t *testing.T, f *fx, osID string) {
	t.Helper()
	rows := f.outbox(osID)
	if len(rows) == 0 {
		t.Fatal("nenhuma mensagem no outbox")
	}
	for _, m := range rows {
		if err := contracts.Validate([]byte(m.Body)); err != nil {
			t.Errorf("%s gravado no outbox fora do contrato: %v\n%s", m.Type, err, m.Body)
		}
		p[m.Type]++
	}
}

// replies responde como o billing-service e o execution-service: IDs de orçamento e pagamento
// em UUID, como eles geram.
type replies struct {
	f                   *fx
	osID                string
	budgetID, paymentID string
}

func newReplies(f *fx, osID string) replies {
	return replies{f: f, osID: osID, budgetID: uuid.NewString(), paymentID: uuid.NewString()}
}

func (r replies) budgetCreated() {
	r.f.send(r.osID, contracts.EvtBudgetCreated, contracts.BudgetCreated{BudgetID: r.budgetID, Total: 330})
}

func (r replies) approved() {
	r.f.send(r.osID, contracts.EvtBudgetApproved, contracts.BudgetApproved{BudgetID: r.budgetID, ApprovedAt: r.f.now})
}

func (r replies) paid() {
	r.f.send(r.osID, contracts.EvtPaymentLinkCreated, contracts.PaymentLinkCreated{
		BudgetID: r.budgetID, PaymentID: r.paymentID, CheckoutURL: "http://localhost:8082/fake-checkout/" + r.paymentID})
	r.f.send(r.osID, contracts.EvtPaymentApproved, contracts.PaymentApproved{
		BudgetID: r.budgetID, PaymentID: r.paymentID, ProviderPaymentID: "fake-" + r.paymentID, Amount: 330})
}

func TestContrato_ProdutorOutboxSegueOSchema(t *testing.T) {
	seen := produced{}

	// Caminho feliz até a entrega do veículo.
	f := newFx(t, Config{})
	so := f.open("")
	r := newReplies(f, so.ID())
	f.diagnose(so.ID(), 2)
	r.budgetCreated()
	r.approved()
	r.paid()
	f.send(so.ID(), contracts.EvtRepairStarted, contracts.RepairStarted{StartedAt: f.now})
	f.send(so.ID(), contracts.EvtRepairFinished, contracts.RepairFinished{FinishedAt: f.now.Add(time.Hour)})
	must(t, f.deliver.Execute(f.ctx, so.ID()))
	seen.check(t, f, so.ID())

	// Cancelamento pela oficina com o pagamento aprovado: estorno, orçamento e execução.
	cancelled := f.open("")
	r = newReplies(f, cancelled.ID())
	f.diagnose(cancelled.ID(), 1)
	r.budgetCreated()
	r.approved()
	r.paid()
	must(t, f.cancel.Execute(f.ctx, cancelled.ID(), "cliente desistiu"))
	seen.check(t, f, cancelled.ID())

	// Prazo de aprovação vencido (correlation_id "timeout-<saga>").
	short := newFx(t, Config{ApprovalTimeout: time.Hour})
	expired := short.open("")
	short.diagnose(expired.ID(), 1)
	newReplies(short, expired.ID()).budgetCreated()
	if _, err := short.expire.Execute(short.ctx, short.now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if short.saga(expired.ID()).State != entities.SagaCancelled {
		t.Fatal("a saga deveria ter expirado")
	}
	seen.check(t, short, expired.ID())

	// Falhas simuladas: cada comando sai com o simulate_failure da sua etapa.
	fi := newFx(t, Config{FaultInjection: true})
	commandOf := map[string]string{
		contracts.FailBudget: contracts.CmdCreateBudget, contracts.FailPayment: contracts.CmdRequestPayment,
		contracts.FailRepair: contracts.CmdStartRepair,
	}
	for stage, cmd := range commandOf {
		so := fi.open(stage)
		r := newReplies(fi, so.ID())
		fi.diagnose(so.ID(), 1)
		if stage != contracts.FailBudget {
			r.budgetCreated()
			r.approved()
		}
		if stage == contracts.FailRepair {
			r.paid()
		}
		seen.check(t, fi, so.ID())
		sent := fi.sent(so.ID(), cmd)
		if len(sent) != 1 || !strings.Contains(string(sent[0].Payload), `"simulate_failure":"`+stage+`"`) {
			t.Fatalf("%s deveria sair com simulate_failure=%s: %+v", cmd, stage, sent)
		}
	}

	want := []string{
		contracts.CmdStartDiagnosis, contracts.CmdStartRepair, contracts.CmdCancelExecution,
		contracts.CmdCreateBudget, contracts.CmdRequestPayment, contracts.CmdCancelBudget, contracts.CmdRefundPayment,
		contracts.EvtServiceOrderStatusChanged,
	}
	for _, typ := range want {
		if seen[typ] == 0 {
			t.Errorf("nenhuma mensagem %s produzida: o teste precisa cobrir todos os tipos do os-service", typ)
		}
	}
	for typ := range seen {
		if !slices.Contains(want, typ) {
			t.Errorf("o os-service produziu %s, que não está na lista de tipos que ele publica", typ)
		}
	}
	t.Logf("mensagens do outbox validadas por tipo: %v", seen)
}

// deliverExample entrega à HandleReply o exemplo do schema do tipo, como se viesse da fila
// os-saga-replies. O payload é o do exemplo; message_id, saga_id e os_id passam a ser os da OS do
// teste (os do exemplo não existem no banco).
func (f *fx) deliverExample(osID, msgType string) {
	f.t.Helper()
	env := exampleEnvelope(f.t, msgType)
	env.MessageID, env.SagaID, env.OSID = uuid.NewString(), f.saga(osID).ID, osID
	must(f.t, f.reply.Execute(f.ctx, env))
}

func exampleEnvelope(t tb, msgType string) contracts.Envelope {
	t.Helper()
	examples, err := contracts.Examples(msgType)
	must(t, err)
	env, err := contracts.Parse(examples[0])
	must(t, err)
	return env
}

func examplePayload[T any](t tb, msgType string) T {
	t.Helper()
	return decodeAs[T](t, exampleEnvelope(t, msgType))
}

// seedExampleCatalog cadastra os serviços e peças citados no exemplo do DiagnosisCompleted.
func (f *fx) seedExampleCatalog(d contracts.DiagnosisCompleted) {
	f.t.Helper()
	for _, s := range d.Services {
		testDB.Exec("DELETE FROM services WHERE code = ?", s.Code)
		must(f.t, f.o.d.Services.Create(f.ctx, entities.NewService(s.Code, "Serviço do exemplo", "", 150, 60)))
	}
	for _, p := range d.Parts {
		testDB.Exec("DELETE FROM parts WHERE manufacturer_code = ?", p.ManufacturerCode)
		must(f.t, f.o.d.Parts.Create(f.ctx, entities.NewPart(p.ManufacturerCode, "Peça do exemplo", "", "un", 90, p.Quantity+3)))
	}
	f.t.Cleanup(func() {
		for _, s := range d.Services {
			testDB.Exec("DELETE FROM services WHERE code = ?", s.Code)
		}
		for _, p := range d.Parts {
			testDB.Exec("DELETE FROM parts WHERE manufacturer_code = ?", p.ManufacturerCode)
		}
	})
}

func TestContrato_ConsumidorAceitaOsExemplosDasRespostas(t *testing.T) {
	f := newFx(t, Config{})
	covered := map[string]bool{}
	deliver := func(osID, msgType string) {
		t.Helper()
		f.deliverExample(osID, msgType)
		covered[msgType] = true
	}
	expect := func(osID string, status entities.OrderStatus, state entities.SagaState) {
		t.Helper()
		if got := f.order(osID).Status(); got != status {
			t.Fatalf("status da OS = %s, esperava %s", got, status)
		}
		if got := f.saga(osID).State; got != state {
			t.Fatalf("estado da saga = %s, esperava %s", got, state)
		}
	}

	// Caminho feliz só com os exemplos dos schemas.
	so := f.open("")
	deliver(so.ID(), contracts.EvtDiagnosisStarted)
	expect(so.ID(), entities.StatusInDiagnosis, entities.SagaAwaitingDiagnosis)

	diagnosis := examplePayload[contracts.DiagnosisCompleted](t, contracts.EvtDiagnosisCompleted)
	f.seedExampleCatalog(diagnosis)
	deliver(so.ID(), contracts.EvtDiagnosisCompleted)
	expect(so.ID(), entities.StatusAwaitingBudget, entities.SagaAwaitingBudget)
	cb := decodeAs[contracts.CreateBudget](t, f.sent(so.ID(), contracts.CmdCreateBudget)[0])
	if len(cb.Items) != len(diagnosis.Services)+len(diagnosis.Parts) || cb.Items[0].Ref != "4012" || cb.Total != 330 {
		t.Fatalf("CreateBudget do diagnóstico do exemplo: %+v", cb)
	}

	created := examplePayload[contracts.BudgetCreated](t, contracts.EvtBudgetCreated)
	deliver(so.ID(), contracts.EvtBudgetCreated)
	expect(so.ID(), entities.StatusAwaitingApproval, entities.SagaAwaitingApproval)
	if f.saga(so.ID()).BudgetID != created.BudgetID {
		t.Fatalf("budget_id não guardado: %+v", f.saga(so.ID()))
	}

	deliver(so.ID(), contracts.EvtBudgetApproved)
	expect(so.ID(), entities.StatusAwaitingPayment, entities.SagaAwaitingPayment)
	if rp := decodeAs[contracts.RequestPayment](t, f.sent(so.ID(), contracts.CmdRequestPayment)[0]); rp.BudgetID != created.BudgetID {
		t.Fatalf("RequestPayment: %+v", rp)
	}

	link := examplePayload[contracts.PaymentLinkCreated](t, contracts.EvtPaymentLinkCreated)
	deliver(so.ID(), contracts.EvtPaymentLinkCreated)
	events := f.sent(so.ID(), contracts.EvtServiceOrderStatusChanged)
	if last := decodeAs[contracts.ServiceOrderStatusChanged](t, events[len(events)-1]); last.CheckoutURL != link.CheckoutURL {
		t.Fatalf("notificação sem o link do exemplo: %+v", last)
	}

	paid := examplePayload[contracts.PaymentApproved](t, contracts.EvtPaymentApproved)
	deliver(so.ID(), contracts.EvtPaymentApproved)
	expect(so.ID(), entities.StatusInExecution, entities.SagaExecuting)
	if s := f.saga(so.ID()); !s.PaymentApproved || s.PaymentID != paid.PaymentID {
		t.Fatalf("pagamento não registrado: %+v", s)
	}

	started := examplePayload[contracts.RepairStarted](t, contracts.EvtRepairStarted)
	deliver(so.ID(), contracts.EvtRepairStarted)
	if at := f.order(so.ID()).StartedAt(); at == nil || !at.Equal(started.StartedAt) {
		t.Fatalf("started_at = %v, esperava %v", at, started.StartedAt)
	}

	finished := examplePayload[contracts.RepairFinished](t, contracts.EvtRepairFinished)
	deliver(so.ID(), contracts.EvtRepairFinished)
	expect(so.ID(), entities.StatusFinished, entities.SagaCompleted)
	if at := f.order(so.ID()).FinishedAt(); at == nil || !at.Equal(finished.FinishedAt) {
		t.Fatalf("finished_at = %v, esperava %v", at, finished.FinishedAt)
	}

	// Respostas que compensam a saga.
	cancelledWith := func(osID, msgType string) {
		t.Helper()
		deliver(osID, msgType)
		p := examplePayload[struct{ Reason string }](t, msgType)
		if reason := f.order(osID).CancelReason(); !strings.Contains(reason, p.Reason) {
			t.Fatalf("%s: motivo da OS %q não traz o do exemplo %q", msgType, reason, p.Reason)
		}
	}

	failed := f.open("")
	f.diagnose(failed.ID(), 1)
	cancelledWith(failed.ID(), contracts.EvtBudgetCreationFailed)
	expect(failed.ID(), entities.StatusCancelled, entities.SagaCancelled)

	rejected := f.open("")
	f.diagnose(rejected.ID(), 1)
	newReplies(f, rejected.ID()).budgetCreated()
	cancelledWith(rejected.ID(), contracts.EvtBudgetRejected)
	expect(rejected.ID(), entities.StatusCancelled, entities.SagaCancelled)

	unpaid := f.open("")
	f.toAwaitingPayment(unpaid.ID())
	cancelledWith(unpaid.ID(), contracts.EvtPaymentFailed)
	expect(unpaid.ID(), entities.StatusCancelled, entities.SagaCancelled)

	refused := f.open("")
	f.toExecuting(refused.ID())
	cancelledWith(refused.ID(), contracts.EvtRepairRejected)
	expect(refused.ID(), entities.StatusCancelled, entities.SagaCompensating)
	if len(f.sent(refused.ID(), contracts.CmdRefundPayment)) != 1 {
		t.Fatal("a recusa do reparo depois do pagamento precisa pedir o estorno")
	}
	deliver(refused.ID(), contracts.EvtPaymentRefunded)
	expect(refused.ID(), entities.StatusCancelled, entities.SagaCancelled)

	// Confirmações de compensação: aceitas, sem mudar nada.
	before := len(f.outbox(refused.ID()))
	deliver(refused.ID(), contracts.EvtBudgetCancelled)
	deliver(refused.ID(), contracts.EvtExecutionCancelled)
	expect(refused.ID(), entities.StatusCancelled, entities.SagaCancelled)
	if len(f.outbox(refused.ID())) != before {
		t.Fatal("confirmação de compensação não gera mensagens")
	}

	for _, typ := range f.reply.Types() {
		if !covered[typ] {
			t.Errorf("o os-service consome %s, mas o teste não entregou o exemplo dele", typ)
		}
	}
}
