//go:build bdd

package sagauc

// BDD da saga (godog) sobre o orquestrador real e o Postgres real: os passos fazem o papel do
// billing-service e do execution-service, lendo os comandos gravados no outbox e respondendo
// com os eventos que esses serviços publicariam.
//
//	make bdd   (TEST_DB_* apontando para o Postgres de teste)

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../../../features"},
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("cenários BDD falharam")
	}
}

// scenarioT adapta o T do cenário do godog ao que o fixture usa; os Cleanup rodam no fim do cenário.
type scenarioT struct {
	godog.TestingT
	cleanups []func()
}

func (s *scenarioT) Helper()          {}
func (s *scenarioT) Cleanup(f func()) { s.cleanups = append(s.cleanups, f) }

type world struct {
	t         *scenarioT
	f         *fx
	osID      string
	completed contracts.Envelope
}

type worldKey struct{}

func initializeScenario(sc *godog.ScenarioContext) {
	w := &world{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w.t = &scenarioT{TestingT: godog.T(ctx)}
		return context.WithValue(ctx, worldKey{}, w), nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		for i := len(w.t.cleanups) - 1; i >= 0; i-- {
			w.t.cleanups[i]()
		}
		return ctx, nil
	})

	sc.Step(`^um cliente com um veículo cadastrado$`, w.clienteComVeiculo)
	sc.Step(`^uma peça com 5 unidades em estoque$`, func() error { return w.estoque(5) })
	sc.Step(`^uma ordem de serviço com o orçamento aguardando aprovação$`, w.ordemAguardandoAprovacao)

	sc.Step(`^a equipe abre uma ordem de serviço$`, func() error { return w.abre("") })
	sc.Step(`^a equipe abre uma ordem de serviço simulando falha em "([^"]*)"$`, w.abre)
	sc.Step(`^o mecânico conclui o diagnóstico pedindo (\d+) unidades da peça$`, w.diagnostico)
	sc.Step(`^a mesma confirmação de diagnóstico chega de novo$`, w.reentrega)
	sc.Step(`^o billing-service confirma o orçamento$`, w.orcamentoCriado)
	sc.Step(`^o cliente aprova o orçamento$`, w.aprova)
	sc.Step(`^o cliente recusa o orçamento$`, w.recusa)
	sc.Step(`^o billing-service gera o link de pagamento$`, w.link)
	sc.Step(`^o pagamento é aprovado$`, w.pagamentoAprovado)
	sc.Step(`^o pagamento é recusado$`, w.pagamentoRecusado)
	sc.Step(`^o mecânico inicia e finaliza o reparo$`, w.reparo)
	sc.Step(`^o execution-service recusa o reparo$`, w.reparoRecusado)
	sc.Step(`^o billing-service confirma o estorno$`, w.estorno)
	sc.Step(`^a equipe entrega o veículo$`, w.entrega)
	sc.Step(`^passam (\d+) horas sem resposta do cliente$`, w.passam)
	sc.Step(`^o verificador de prazos roda$`, w.verificaPrazos)
	sc.Step(`^a saga percorre o fluxo até a etapa com falha$`, w.percorre)

	sc.Step(`^o (?:execution|billing)-service recebe o comando "([^"]*)"$`, func(cmd string) error { return w.recebe(cmd, -1) })
	sc.Step(`^o (?:execution|billing)-service recebe (\d+) vez o comando "([^"]*)"$`, func(n int, cmd string) error { return w.recebe(cmd, n) })
	sc.Step(`^o (?:execution|billing)-service não recebe o comando "([^"]*)"$`, func(cmd string) error { return w.recebe(cmd, 0) })
	sc.Step(`^o billing-service recebe o comando "billing.CreateBudget" somando R\$ ([\d.]+)$`, w.orcamentoSoma)
	sc.Step(`^o estoque da peça fica em (\d+)$`, w.estoque)
	sc.Step(`^a ordem de serviço fica "([^"]*)"$`, w.statusOS)
	sc.Step(`^a saga fica "([^"]*)"$`, w.statusSaga)
	sc.Step(`^o histórico da ordem é "([^"]*)"$`, w.historico)
	sc.Step(`^o cliente é notificado (\d+) vezes$`, w.notificacoes)
}

// ---- Dado ----

func (w *world) clienteComVeiculo() error {
	w.f = newFx(w.t, Config{ApprovalTimeout: 72 * time.Hour, PaymentTimeout: 24 * time.Hour, FaultInjection: true})
	return nil
}

func (w *world) ordemAguardandoAprovacao() error {
	if err := w.abre(""); err != nil {
		return err
	}
	if err := w.diagnostico(2); err != nil {
		return err
	}
	return w.orcamentoCriado()
}

// ---- Quando ----

func (w *world) abre(simulate string) error {
	w.osID = w.f.open(simulate).ID()
	return nil
}

func (w *world) diagnostico(qty int) error {
	w.f.send(w.osID, contracts.EvtDiagnosisStarted, contracts.DiagnosisStarted{StartedAt: w.f.now})
	w.completed = w.f.send(w.osID, contracts.EvtDiagnosisCompleted, contracts.DiagnosisCompleted{
		Services: []contracts.DiagnosedService{{Code: w.f.service.Code()}},
		Parts:    []contracts.DiagnosedPart{{ManufacturerCode: w.f.part.ManufacturerCode(), Quantity: qty}},
	})
	return nil
}

// reentrega simula o SQS entregando a mesma mensagem (mesmo message_id) outra vez.
func (w *world) reentrega() error {
	return w.f.reply.Execute(w.f.ctx, w.completed)
}

func (w *world) budgetID() string { return "budget-" + w.osID[:8] }

func (w *world) orcamentoCriado() error {
	w.f.send(w.osID, contracts.EvtBudgetCreated, contracts.BudgetCreated{BudgetID: w.budgetID(), Total: 330})
	return nil
}

func (w *world) aprova() error {
	w.f.send(w.osID, contracts.EvtBudgetApproved, contracts.BudgetApproved{BudgetID: w.budgetID(), ApprovedAt: w.f.now})
	return nil
}

func (w *world) recusa() error {
	w.f.send(w.osID, contracts.EvtBudgetRejected, contracts.BudgetRejected{BudgetID: w.budgetID(), Reason: "vou pensar"})
	return nil
}

func (w *world) link() error {
	w.f.send(w.osID, contracts.EvtPaymentLinkCreated, contracts.PaymentLinkCreated{
		BudgetID: w.budgetID(), PaymentID: "pay-" + w.osID[:8], CheckoutURL: "https://mp.test/checkout"})
	return nil
}

func (w *world) pagamentoAprovado() error {
	w.f.send(w.osID, contracts.EvtPaymentApproved, contracts.PaymentApproved{
		BudgetID: w.budgetID(), PaymentID: "pay-" + w.osID[:8], ProviderPaymentID: "123", Amount: 330})
	return nil
}

func (w *world) pagamentoRecusado() error {
	w.f.send(w.osID, contracts.EvtPaymentFailed, contracts.PaymentFailed{
		BudgetID: w.budgetID(), PaymentID: "pay-" + w.osID[:8], Reason: "cartão recusado"})
	return nil
}

func (w *world) reparo() error {
	start := w.f.now.Add(time.Hour)
	w.f.send(w.osID, contracts.EvtRepairStarted, contracts.RepairStarted{StartedAt: start})
	w.f.send(w.osID, contracts.EvtRepairFinished, contracts.RepairFinished{FinishedAt: start.Add(2 * time.Hour)})
	return nil
}

func (w *world) reparoRecusado() error {
	w.f.send(w.osID, contracts.EvtRepairRejected, contracts.RepairRejected{Reason: "oficina sem capacidade"})
	return nil
}

func (w *world) estorno() error {
	w.f.send(w.osID, contracts.EvtPaymentRefunded, contracts.PaymentRefunded{
		BudgetID: w.budgetID(), PaymentID: "pay-" + w.osID[:8], Amount: 330})
	return nil
}

func (w *world) entrega() error {
	return w.f.deliver.Execute(w.f.ctx, w.osID)
}

func (w *world) passam(hours int) error {
	w.f.now = w.f.now.Add(time.Duration(hours) * time.Hour)
	return nil
}

func (w *world) verificaPrazos() error {
	_, err := w.f.expire.Execute(w.f.ctx, w.f.now)
	return err
}

// percorre faz o papel dos dois serviços: a cada comando, responde como eles responderiam,
// inclusive com a falha pedida em simulate_failure (é o que billing e execution fazem de fato).
func (w *world) percorre() error {
	if err := w.diagnostico(2); err != nil {
		return err
	}
	cb := decodeAs[contracts.CreateBudget](w.t, w.ultimo(contracts.CmdCreateBudget))
	if cb.SimulateFailure == contracts.FailBudget {
		w.f.send(w.osID, contracts.EvtBudgetCreationFailed, contracts.BudgetCreationFailed{Reason: "falha simulada"})
		return nil
	}
	_ = w.orcamentoCriado()
	_ = w.aprova()
	rp := decodeAs[contracts.RequestPayment](w.t, w.ultimo(contracts.CmdRequestPayment))
	if rp.SimulateFailure == contracts.FailPayment {
		return w.pagamentoRecusado()
	}
	_ = w.link()
	_ = w.pagamentoAprovado()
	sr := decodeAs[contracts.StartRepair](w.t, w.ultimo(contracts.CmdStartRepair))
	if sr.SimulateFailure == contracts.FailRepair {
		_ = w.reparoRecusado()
		decodeAs[contracts.RefundPayment](w.t, w.ultimo(contracts.CmdRefundPayment))
		return w.estorno()
	}
	return fmt.Errorf("nenhuma etapa falhou (simulate_failure=%q)", sr.SimulateFailure)
}

// ---- Então ----

func (w *world) ultimo(cmd string) contracts.Envelope {
	sent := w.f.sent(w.osID, cmd)
	if len(sent) == 0 {
		w.t.Fatal("comando " + cmd + " não foi enviado")
	}
	return sent[len(sent)-1]
}

// recebe confere quantos comandos do tipo foram para o outbox; n < 0 = ao menos um.
func (w *world) recebe(cmd string, n int) error {
	got := len(w.f.sent(w.osID, cmd))
	switch {
	case n < 0 && got == 0:
		return fmt.Errorf("esperava o comando %s, nenhum foi enviado", cmd)
	case n >= 0 && got != n:
		return fmt.Errorf("esperava %d comando(s) %s, foram %d", n, cmd, got)
	}
	return nil
}

func (w *world) orcamentoSoma(total string) error {
	want, _ := strconv.ParseFloat(total, 64)
	cb := decodeAs[contracts.CreateBudget](w.t, w.ultimo(contracts.CmdCreateBudget))
	if cb.Total != want {
		return fmt.Errorf("orçamento soma %.2f, esperava %.2f", cb.Total, want)
	}
	return nil
}

func (w *world) estoque(want int) error {
	if w.f == nil {
		return fmt.Errorf("cadastre o cliente antes")
	}
	if got := w.f.stock(); got != want {
		return fmt.Errorf("estoque %d, esperava %d", got, want)
	}
	return nil
}

func (w *world) statusOS(want string) error {
	if got := w.f.order(w.osID).Status(); string(got) != want {
		return fmt.Errorf("OS em %s, esperava %s", got, want)
	}
	return nil
}

func (w *world) statusSaga(want string) error {
	if got := w.f.saga(w.osID).State; string(got) != want {
		return fmt.Errorf("saga em %s, esperava %s", got, want)
	}
	return nil
}

func (w *world) historico(want string) error {
	statuses := w.f.statuses(w.osID)
	got := make([]string, 0, len(statuses))
	for _, s := range statuses {
		got = append(got, string(s))
	}
	if strings.Join(got, ", ") != want {
		return fmt.Errorf("histórico %q", strings.Join(got, ", "))
	}
	return nil
}

func (w *world) notificacoes(want int) error {
	if got := len(w.f.sent(w.osID, contracts.EvtServiceOrderStatusChanged)); got != want {
		return fmt.Errorf("%d eventos de status para a notificação, esperava %d", got, want)
	}
	return nil
}
