package sagauc

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/logger"
)

type replyHandler func(ctx context.Context, st *step, env contracts.Envelope) error

// HandleReply consome as respostas do billing-service e do execution-service (fila
// os-saga-replies) e avança ou compensa a saga.
//
// Contrato dos handlers: toda verificação de etapa vem antes de qualquer escrita. Um handler que
// devolve ErrSagaOutOfStep não escreveu nada; a mensagem é registrada no inbox e ignorada
// (resposta tardia, duplicada com outro id ou fora de ordem). Qualquer outro erro desfaz a
// transação e a mensagem volta para a fila (retry e, depois, DLQ).
type HandleReply struct {
	o        *Orchestrator
	handlers map[string]replyHandler
	acks     map[string]bool
}

func NewHandleReply(o *Orchestrator) *HandleReply {
	h := &HandleReply{o: o, acks: map[string]bool{
		// Confirmações de compensações: não mudam a saga, servem de auditoria nos logs.
		contracts.EvtBudgetCancelled:    true,
		contracts.EvtExecutionCancelled: true,
	}}
	h.handlers = map[string]replyHandler{
		contracts.EvtDiagnosisStarted:     h.onDiagnosisStarted,
		contracts.EvtDiagnosisCompleted:   h.onDiagnosisCompleted,
		contracts.EvtBudgetCreated:        h.onBudgetCreated,
		contracts.EvtBudgetCreationFailed: h.onBudgetCreationFailed,
		contracts.EvtBudgetApproved:       h.onBudgetApproved,
		contracts.EvtBudgetRejected:       h.onBudgetRejected,
		contracts.EvtPaymentLinkCreated:   h.onPaymentLinkCreated,
		contracts.EvtPaymentApproved:      h.onPaymentApproved,
		contracts.EvtPaymentFailed:        h.onPaymentFailed,
		contracts.EvtPaymentRefunded:      h.onPaymentRefunded,
		contracts.EvtRepairStarted:        h.onRepairStarted,
		contracts.EvtRepairFinished:       h.onRepairFinished,
		contracts.EvtRepairRejected:       h.onRepairRejected,
	}
	return h
}

// Types lista os tipos de mensagem que o orquestrador consome (para registrar no consumidor).
func (h *HandleReply) Types() []string {
	out := make([]string, 0, len(h.handlers)+len(h.acks))
	for t := range h.handlers {
		out = append(out, t)
	}
	for t := range h.acks {
		out = append(out, t)
	}
	return out
}

func (h *HandleReply) Execute(ctx context.Context, env contracts.Envelope) error {
	// O consumidor (pkg/messaging) já coloca no logger do ctx trace.id, span.id, saga_id, os_id,
	// message_id e message_type.
	l := logger.FromContext(ctx)

	if h.acks[env.Type] {
		l.Info("saga_compensation_confirmed")
		return nil
	}
	handler, ok := h.handlers[env.Type]
	if !ok {
		l.Warn("saga_reply_unknown_type")
		return nil
	}

	var (
		outcome = "applied"
		st      *step
	)
	dup, err := h.o.d.Inbox.Once(ctx, env, func(ctx context.Context) error {
		saga, err := h.o.d.Sagas.FindByOSIDForUpdate(ctx, env.OSID)
		if errors.Is(err, domainerrors.ErrNotFound) {
			outcome = "unknown_saga"
			return nil
		}
		if err != nil {
			return err
		}
		so, err := h.o.d.Orders.FindByID(ctx, env.OSID)
		if err != nil {
			return err
		}
		st = &step{saga: saga, so: so, meta: env.Reply(), actor: entities.ActorSaga, messageID: env.MessageID}
		if err := handler(ctx, st, env); err != nil {
			if errors.Is(err, domainerrors.ErrSagaOutOfStep) {
				outcome = "ignored"
				return nil
			}
			return err
		}
		return h.o.save(ctx, st)
	})
	switch {
	case err != nil:
		l.Error("saga_reply_failed", "error", err)
		return err
	case dup:
		l.Info("saga_reply_duplicate")
	case outcome == "applied":
		l.Info("saga_step", "saga_state", string(st.saga.State), "os_status", string(st.so.Status()))
		if st.saga.State == entities.SagaCompleted {
			l.Info("saga_completed", "service_order_code", st.so.Code())
		}
	case outcome == "ignored":
		l.Info("saga_reply_ignored", "saga_state", string(st.saga.State), "os_status", string(st.so.Status()))
	default:
		l.Warn("saga_reply_unknown_saga")
	}
	return nil
}

func decode[T any](env contracts.Envelope) (T, error) {
	v, err := contracts.Decode[T](env)
	if err != nil {
		return v, fmt.Errorf("saga: %w", err)
	}
	return v, nil
}

func (h *HandleReply) onDiagnosisStarted(ctx context.Context, st *step, _ contracts.Envelope) error {
	// O DiagnosisCompleted pode chegar antes (SNS/SQS sem ordem): nesse caso a OS já avançou.
	if st.saga.State != entities.SagaAwaitingDiagnosis || st.so.Status() != entities.StatusReceived {
		return domainerrors.ErrSagaOutOfStep
	}
	from := st.so.Status()
	if err := st.so.TransitionTo(entities.StatusInDiagnosis); err != nil {
		return err
	}
	return h.o.changeStatus(ctx, st, from, "", true)
}

func (h *HandleReply) onDiagnosisCompleted(ctx context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.DiagnosisCompleted](env)
	if err != nil {
		return err
	}
	if err := st.saga.Advance(entities.SagaAwaitingDiagnosis, entities.SagaAwaitingBudget); err != nil {
		return err
	}
	services, parts, reason, err := h.resolveCatalog(ctx, p)
	if err != nil {
		return err
	}
	if reason != "" {
		return h.o.compensate(ctx, st, reason, closed{})
	}
	// Reserva as peças. Uma falta de estoque compensa a saga; as reservas já feitas nesta
	// transação estão em saga.Reservations e voltam ao estoque na compensação.
	for _, part := range parts {
		err := h.o.d.Parts.ReserveStock(ctx, part.PartID, part.Quantity)
		if errors.Is(err, domainerrors.ErrInsufficientStock) {
			return h.o.compensate(ctx, st, "estoque insuficiente para a peça "+part.ManufacturerCode, closed{})
		}
		if err != nil {
			return err
		}
		st.saga.Reservations = append(st.saga.Reservations,
			entities.PartReservation{PartID: part.PartID, Quantity: part.Quantity})
	}
	st.so.SetServices(services)
	st.so.SetParts(parts)
	from := st.so.Status()
	if err := st.so.TransitionTo(entities.StatusAwaitingBudget); err != nil {
		return err
	}
	if err := h.o.changeStatus(ctx, st, from, "", true); err != nil {
		return err
	}
	r, err := h.o.loadRequester(ctx, st)
	if err != nil {
		return err
	}
	return h.o.d.Messenger.Send(ctx, contracts.CmdCreateBudget, st.meta, contracts.CreateBudget{
		OSCode:          st.so.Code(),
		Requester:       requesterContract(r),
		Items:           serviceItems(st.so),
		Total:           st.so.TotalAmount(),
		SimulateFailure: failureFor(st.saga, contracts.FailBudget),
	})
}

// resolveCatalog transforma o diagnóstico (códigos) em itens com preço do catálogo. reason vem
// preenchido quando o diagnóstico cita item inexistente (falha de negócio, compensa a saga).
func (h *HandleReply) resolveCatalog(ctx context.Context, p contracts.DiagnosisCompleted) (
	[]entities.ServiceItem, []entities.PartItem, string, error) {
	if len(p.Services) == 0 && len(p.Parts) == 0 {
		return nil, nil, "diagnóstico sem serviços nem peças", nil
	}

	codes := make([]int, 0, len(p.Services))
	for _, s := range p.Services {
		codes = append(codes, s.Code)
	}
	var services []entities.ServiceItem
	if len(codes) > 0 {
		found, err := h.o.d.Services.FindByCodes(ctx, codes)
		if err != nil {
			return nil, nil, "", err
		}
		byCode := make(map[int]*entities.Service, len(found))
		for _, s := range found {
			byCode[s.Code()] = s
		}
		seen := map[int]bool{}
		for _, c := range codes {
			s, ok := byCode[c]
			if !ok {
				return nil, nil, "serviço " + strconv.Itoa(c) + " não existe no catálogo", nil
			}
			if seen[c] {
				continue
			}
			seen[c] = true
			services = append(services, entities.ServiceItem{
				ServiceID: s.ID(), Code: s.Code(), Description: s.Name(), Price: s.Price(),
			})
		}
	}

	qty := map[string]int{}
	order := make([]string, 0, len(p.Parts))
	for _, dp := range p.Parts {
		if dp.Quantity <= 0 {
			return nil, nil, "quantidade inválida para a peça " + dp.ManufacturerCode, nil
		}
		if _, ok := qty[dp.ManufacturerCode]; !ok {
			order = append(order, dp.ManufacturerCode)
		}
		qty[dp.ManufacturerCode] += dp.Quantity
	}
	var parts []entities.PartItem
	if len(order) > 0 {
		found, err := h.o.d.Parts.FindByManufacturerCodes(ctx, order)
		if err != nil {
			return nil, nil, "", err
		}
		byCode := make(map[string]*entities.Part, len(found))
		for _, pt := range found {
			byCode[pt.ManufacturerCode()] = pt
		}
		for _, code := range order {
			pt, ok := byCode[code]
			if !ok {
				return nil, nil, "peça " + code + " não existe no catálogo", nil
			}
			parts = append(parts, entities.PartItem{
				PartID: pt.ID(), ManufacturerCode: code, Description: pt.Name(),
				Quantity: qty[code], UnitPrice: pt.Price(),
			})
		}
	}
	return services, parts, "", nil
}

func (h *HandleReply) onBudgetCreated(ctx context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.BudgetCreated](env)
	if err != nil {
		return err
	}
	if err := st.saga.Advance(entities.SagaAwaitingBudget, entities.SagaAwaitingApproval); err != nil {
		return err
	}
	st.saga.BudgetID = p.BudgetID
	st.saga.WaitUntil(h.o.d.Now().Add(h.o.cfg.ApprovalTimeout))
	from := st.so.Status()
	if err := st.so.TransitionTo(entities.StatusAwaitingApproval); err != nil {
		return err
	}
	return h.o.changeStatus(ctx, st, from, "", true)
}

func (h *HandleReply) onBudgetCreationFailed(ctx context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.BudgetCreationFailed](env)
	if err != nil {
		return err
	}
	if st.saga.State != entities.SagaAwaitingBudget {
		return domainerrors.ErrSagaOutOfStep
	}
	return h.o.compensate(ctx, st, "falha ao gerar o orçamento: "+p.Reason, closed{budget: true})
}

func (h *HandleReply) onBudgetApproved(ctx context.Context, st *step, env contracts.Envelope) error {
	if _, err := decode[contracts.BudgetApproved](env); err != nil {
		return err
	}
	if err := st.saga.Advance(entities.SagaAwaitingApproval, entities.SagaAwaitingPayment); err != nil {
		return err
	}
	st.saga.WaitUntil(h.o.d.Now().Add(h.o.cfg.PaymentTimeout))
	from := st.so.Status()
	if err := st.so.TransitionTo(entities.StatusAwaitingPayment); err != nil {
		return err
	}
	// A notificação de "aguardando pagamento" sai com o link, no PaymentLinkCreated.
	if err := h.o.changeStatus(ctx, st, from, "", false); err != nil {
		return err
	}
	return h.o.d.Messenger.Send(ctx, contracts.CmdRequestPayment, st.meta, contracts.RequestPayment{
		BudgetID:        st.saga.BudgetID,
		SimulateFailure: failureFor(st.saga, contracts.FailPayment),
	})
}

func (h *HandleReply) onBudgetRejected(ctx context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.BudgetRejected](env)
	if err != nil {
		return err
	}
	if st.saga.State != entities.SagaAwaitingApproval {
		return domainerrors.ErrSagaOutOfStep
	}
	reason := "orçamento recusado pelo cliente"
	if p.Reason != "" {
		reason += ": " + p.Reason
	}
	return h.o.compensate(ctx, st, reason, closed{budget: true})
}

func (h *HandleReply) onPaymentLinkCreated(ctx context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.PaymentLinkCreated](env)
	if err != nil {
		return err
	}
	if st.saga.State != entities.SagaAwaitingPayment {
		return domainerrors.ErrSagaOutOfStep
	}
	st.saga.PaymentID = p.PaymentID
	st.saga.CheckoutURL = p.CheckoutURL
	return h.o.publishStatus(ctx, st, entities.StatusAwaitingApproval, "")
}

func (h *HandleReply) onPaymentApproved(ctx context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.PaymentApproved](env)
	if err != nil {
		return err
	}
	if st.saga.State == entities.SagaCancelled {
		// Pagamento confirmado depois do cancelamento (prazo vencido, recusa na oficina...):
		// o dinheiro precisa voltar ao cliente.
		if err := st.saga.RefundLatePayment(p.PaymentID); err != nil {
			return err
		}
		h.o.log(ctx, st).Warn("saga_late_payment_refund", "payment_id", p.PaymentID)
		return h.o.d.Messenger.Send(ctx, contracts.CmdRefundPayment, st.meta, contracts.RefundPayment{
			BudgetID: st.saga.BudgetID, Reason: "pagamento recebido após o cancelamento da OS",
		})
	}
	if err := st.saga.Advance(entities.SagaAwaitingPayment, entities.SagaExecuting); err != nil {
		return err
	}
	st.saga.PaymentApproved = true
	st.saga.PaymentID = p.PaymentID
	from := st.so.Status()
	if err := st.so.TransitionTo(entities.StatusInExecution); err != nil {
		return err
	}
	if err := h.o.changeStatus(ctx, st, from, "", true); err != nil {
		return err
	}
	return h.o.d.Messenger.Send(ctx, contracts.CmdStartRepair, st.meta, contracts.StartRepair{
		OSCode:          st.so.Code(),
		Items:           serviceItems(st.so),
		SimulateFailure: failureFor(st.saga, contracts.FailRepair),
	})
}

func (h *HandleReply) onPaymentFailed(ctx context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.PaymentFailed](env)
	if err != nil {
		return err
	}
	if st.saga.State != entities.SagaAwaitingPayment {
		return domainerrors.ErrSagaOutOfStep
	}
	return h.o.compensate(ctx, st, "pagamento não aprovado: "+p.Reason, closed{})
}

func (h *HandleReply) onPaymentRefunded(ctx context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.PaymentRefunded](env)
	if err != nil {
		return err
	}
	if err := st.saga.ConfirmRefund(); err != nil {
		return err
	}
	h.o.log(ctx, st).Info("saga_refund_confirmed", "payment_id", p.PaymentID, "amount", p.Amount)
	return nil
}

func (h *HandleReply) onRepairStarted(_ context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.RepairStarted](env)
	if err != nil {
		return err
	}
	if st.saga.State != entities.SagaExecuting {
		return domainerrors.ErrSagaOutOfStep
	}
	at := p.StartedAt
	if at.IsZero() {
		at = h.o.d.Now()
	}
	st.so.MarkRepairStarted(at.UTC())
	st.soDirty = true
	return nil
}

func (h *HandleReply) onRepairFinished(ctx context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.RepairFinished](env)
	if err != nil {
		return err
	}
	if err := st.saga.Advance(entities.SagaExecuting, entities.SagaCompleted); err != nil {
		return err
	}
	at := p.FinishedAt
	if at.IsZero() {
		at = h.o.d.Now()
	}
	from := st.so.Status()
	if err := st.so.Finish(at.UTC()); err != nil {
		return err
	}
	return h.o.changeStatus(ctx, st, from, "", true)
}

func (h *HandleReply) onRepairRejected(ctx context.Context, st *step, env contracts.Envelope) error {
	p, err := decode[contracts.RepairRejected](env)
	if err != nil {
		return err
	}
	if st.saga.State != entities.SagaExecuting {
		return domainerrors.ErrSagaOutOfStep
	}
	return h.o.compensate(ctx, st, "execução recusada pela oficina: "+p.Reason, closed{execution: true})
}
