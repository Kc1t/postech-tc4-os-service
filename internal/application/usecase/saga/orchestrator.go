// Package sagauc implementa o orquestrador da saga da Ordem de Serviço.
//
// O os-service é o orquestrador: guarda o estado da saga (saga_instances), envia comandos ao
// billing-service e ao execution-service e reage às respostas que chegam pela fila
// os-saga-replies. Cada passo roda numa única transação de banco que trava a linha da saga,
// atualiza OS, saga, histórico e estoque e grava as mensagens de saída no outbox; o consumo é
// idempotente (inbox). Assim nenhum passo fica pela metade e nenhuma mensagem se perde.
package sagauc

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/logger"
	"github.com/google/uuid"
)

// Config são os parâmetros de negócio da saga.
type Config struct {
	// ApprovalTimeout é o prazo para o cliente aprovar ou recusar o orçamento.
	ApprovalTimeout time.Duration
	// PaymentTimeout é o prazo para o pagamento ser confirmado depois da aprovação.
	PaymentTimeout time.Duration
	// FaultInjection habilita o campo simulate_failure na abertura da OS (demonstração).
	FaultInjection bool
}

// Deps são as portas usadas pelo orquestrador.
type Deps struct {
	Tx         ports.TxManager
	Inbox      ports.MessageInbox
	Sagas      ports.SagaRepository
	Orders     ports.ServiceOrderRepository
	History    ports.StatusHistoryRepository
	Requesters ports.RequesterRepository
	Vehicles   ports.VehicleRepository
	Services   ports.ServiceRepository
	Parts      ports.PartRepository
	Messenger  ports.SagaMessenger
	// NewID e Now podem ser substituídos nos testes.
	NewID func() string
	Now   func() time.Time
}

// Orchestrator concentra as regras da saga. Os casos de uso deste pacote (abrir, cancelar,
// entregar, tratar respostas e expirar prazos) o compartilham.
type Orchestrator struct {
	d   Deps
	cfg Config
}

func NewOrchestrator(d Deps, cfg Config) *Orchestrator {
	if d.NewID == nil {
		d.NewID = uuid.NewString
	}
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.ApprovalTimeout <= 0 {
		cfg.ApprovalTimeout = 72 * time.Hour
	}
	if cfg.PaymentTimeout <= 0 {
		cfg.PaymentTimeout = 24 * time.Hour
	}
	return &Orchestrator{d: d, cfg: cfg}
}

// step reúne o que um passo da saga lê e altera dentro da transação.
type step struct {
	saga      *entities.Saga
	so        *entities.ServiceOrder
	requester *entities.Requester
	meta      contracts.Meta
	actor     string
	messageID string
	soDirty   bool
}

func (o *Orchestrator) loadRequester(ctx context.Context, st *step) (*entities.Requester, error) {
	if st.requester != nil {
		return st.requester, nil
	}
	r, err := o.d.Requesters.FindByID(ctx, st.so.RequesterID())
	if err != nil {
		return nil, err
	}
	st.requester = r
	return r, nil
}

func requesterContract(r *entities.Requester) contracts.Requester {
	return contracts.Requester{ID: r.ID(), Name: r.Name(), Email: r.Email(), Document: r.Document()}
}

// changeStatus aplica a transição já feita na entidade (from → status atual): grava o histórico
// e, se publish, publica os.ServiceOrderStatusChanged (consumido pela notificação ao cliente).
func (o *Orchestrator) changeStatus(ctx context.Context, st *step, from entities.OrderStatus, reason string, publish bool) error {
	st.soDirty = true
	entry := &entities.StatusHistoryEntry{
		ID:        o.d.NewID(),
		OSID:      st.so.ID(),
		From:      from,
		To:        st.so.Status(),
		Actor:     st.actor,
		Reason:    reason,
		MessageID: st.messageID,
		CreatedAt: o.d.Now(),
	}
	if err := o.d.History.Add(ctx, entry); err != nil {
		return err
	}
	if !publish {
		return nil
	}
	return o.publishStatus(ctx, st, from, reason)
}

func (o *Orchestrator) publishStatus(ctx context.Context, st *step, from entities.OrderStatus, reason string) error {
	r, err := o.loadRequester(ctx, st)
	if err != nil {
		return err
	}
	evt := contracts.ServiceOrderStatusChanged{
		OSCode:         st.so.Code(),
		Status:         string(st.so.Status()),
		PreviousStatus: string(from),
		Requester:      requesterContract(r),
		TotalAmount:    st.so.TotalAmount(),
		Reason:         reason,
	}
	if st.so.Status() == entities.StatusAwaitingPayment {
		evt.CheckoutURL = st.saga.CheckoutURL
	}
	return o.d.Messenger.Send(ctx, contracts.EvtServiceOrderStatusChanged, st.meta, evt)
}

// save grava saga e OS (optimistic lock nas duas).
func (o *Orchestrator) save(ctx context.Context, st *step) error {
	if err := o.d.Sagas.Update(ctx, st.saga); err != nil {
		return err
	}
	if st.soDirty {
		return o.d.Orders.Update(ctx, st.so)
	}
	return nil
}

// closed indica etapas que já estão encerradas no serviço remoto e dispensam compensação.
type closed struct {
	budget    bool
	execution bool
}

// compensate encerra a saga e dispara as transações compensatórias na ordem inversa das etapas
// concluídas: estorno, cancelamento do orçamento, devolução das peças ao estoque e cancelamento
// da execução. A OS termina cancelada com o motivo.
func (o *Orchestrator) compensate(ctx context.Context, st *step, reason string, done closed) error {
	plan, err := st.saga.Compensate(reason)
	if err != nil {
		return err
	}
	if plan.RefundPayment {
		if err := o.d.Messenger.Send(ctx, contracts.CmdRefundPayment, st.meta,
			contracts.RefundPayment{BudgetID: st.saga.BudgetID, Reason: reason}); err != nil {
			return err
		}
	}
	if plan.CancelBudget && !done.budget {
		if err := o.d.Messenger.Send(ctx, contracts.CmdCancelBudget, st.meta,
			contracts.CancelBudget{BudgetID: st.saga.BudgetID, Reason: reason}); err != nil {
			return err
		}
	}
	for _, r := range plan.ReleaseParts {
		if err := o.d.Parts.UpdateStock(ctx, r.PartID, r.Quantity); err != nil {
			return err
		}
	}
	if plan.CancelExecution && !done.execution {
		if err := o.d.Messenger.Send(ctx, contracts.CmdCancelExecution, st.meta,
			contracts.CancelExecution{Reason: reason}); err != nil {
			return err
		}
	}
	from := st.so.Status()
	if err := st.so.Cancel(reason); err != nil {
		return err
	}
	o.log(ctx, st).Warn("saga_compensated",
		"reason", reason,
		"refund_payment", plan.RefundPayment,
		"cancel_budget", plan.CancelBudget && !done.budget,
		"released_parts", len(plan.ReleaseParts),
		"previous_status", string(from))
	return o.changeStatus(ctx, st, from, reason, true)
}

// log devolve o logger do passo. Vindo de mensagem, o logger do ctx já tem trace e
// identificadores (o consumidor os coloca); vindo de HTTP ou do worker de prazos, acrescenta.
func (o *Orchestrator) log(ctx context.Context, st *step) *slog.Logger {
	l := logger.FromContext(ctx)
	if st.messageID == "" {
		l = logger.WithTrace(ctx, l).With("saga_id", st.saga.ID, "os_id", st.so.ID())
	}
	return l.With("service_order_code", st.so.Code(), "actor", st.actor)
}

func serviceItems(so *entities.ServiceOrder) []contracts.Item {
	items := make([]contracts.Item, 0, len(so.Services())+len(so.Parts()))
	for _, s := range so.Services() {
		items = append(items, contracts.Item{
			Kind: contracts.ItemKindService, Ref: strconv.Itoa(s.Code), Description: s.Description,
			Quantity: 1, UnitPrice: s.Price,
		})
	}
	for _, p := range so.Parts() {
		items = append(items, contracts.Item{
			Kind: contracts.ItemKindPart, Ref: p.ManufacturerCode, Description: p.Description,
			Quantity: p.Quantity, UnitPrice: p.UnitPrice,
		})
	}
	return items
}

// failureFor devolve a falha simulada se ela for da etapa indicada.
func failureFor(s *entities.Saga, stage string) string {
	if s.SimulateFailure == stage {
		return stage
	}
	return ""
}
