package sagauc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/logger"
)

// CreateServiceOrder abre a OS e inicia a saga: na mesma transação grava a OS, a saga, o
// histórico, o comando StartDiagnosis para o execution-service e o evento de status.
type CreateServiceOrder struct{ o *Orchestrator }

func NewCreateServiceOrder(o *Orchestrator) *CreateServiceOrder { return &CreateServiceOrder{o: o} }

func (uc *CreateServiceOrder) Execute(ctx context.Context, input entities.ServiceOrderInput) (*entities.ServiceOrder, error) {
	simulate, err := uc.o.simulatedFailure(input.SimulateFailure)
	if err != nil {
		return nil, err
	}
	requester, err := uc.o.d.Requesters.FindByDocument(ctx, input.RequesterDocument)
	if err != nil {
		return nil, err
	}
	vehicle, err := uc.o.d.Vehicles.FindByPlate(ctx, input.VehiclePlate)
	if err != nil {
		return nil, err
	}
	if vehicle.RequesterID() != requester.ID() {
		return nil, domainerrors.ErrVehicleNotFromRequester
	}

	so := entities.NewServiceOrder(requester.ID(), vehicle.ID())
	so.SetNotes(input.Notes)
	var saga *entities.Saga
	err = uc.o.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.o.d.Orders.Create(ctx, so); err != nil {
			return err
		}
		saga = entities.NewSaga(uc.o.d.NewID(), so.ID(), simulate)
		if err := uc.o.d.Sagas.Create(ctx, saga); err != nil {
			return err
		}
		st := &step{
			saga: saga, so: so, requester: requester, actor: entities.ActorStaff,
			meta: contracts.Meta{SagaID: saga.ID, OSID: so.ID(), CorrelationID: logger.CorrelationID(ctx)},
		}
		if err := uc.o.changeStatus(ctx, st, "", "", true); err != nil {
			return err
		}
		return uc.o.d.Messenger.Send(ctx, contracts.CmdStartDiagnosis, st.meta, contracts.StartDiagnosis{
			OSCode: so.Code(),
			Vehicle: contracts.Vehicle{
				Plate: vehicle.Plate(), Brand: vehicle.Brand(), Model: vehicle.Model(), Year: vehicle.Year(),
			},
			Notes: so.Notes(),
		})
	})
	if err != nil {
		return nil, err
	}
	logger.WithTrace(ctx, logger.FromContext(ctx)).Info("saga_started",
		"saga_id", saga.ID, "os_id", so.ID(), "service_order_code", so.Code(),
		"requester_id", requester.ID(), "vehicle_id", vehicle.ID(), "simulate_failure", simulate)
	return so, nil
}

// simulatedFailure valida o campo de demonstração. Com a injeção desligada ele é ignorado.
func (o *Orchestrator) simulatedFailure(v string) (string, error) {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || !o.cfg.FaultInjection {
		return "", nil
	}
	switch v {
	case contracts.FailBudget, contracts.FailPayment, contracts.FailRepair:
		return v, nil
	}
	return "", domainerrors.ErrInvalidSimulatedFailure
}

// CancelServiceOrder é o cancelamento pela oficina: compensa as etapas já concluídas
// (estorno, orçamento, peças, execução) e encerra a OS.
type CancelServiceOrder struct{ o *Orchestrator }

func NewCancelServiceOrder(o *Orchestrator) *CancelServiceOrder { return &CancelServiceOrder{o: o} }

func (uc *CancelServiceOrder) Execute(ctx context.Context, id, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "cancelada pela oficina"
	} else {
		reason = "cancelada pela oficina: " + reason
	}
	var st *step
	err := uc.o.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		// Mesma ordem de travamento dos handlers (saga, depois OS) para não haver deadlock.
		saga, err := uc.o.d.Sagas.FindByOSIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		so, err := uc.o.d.Orders.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if so.Status() == entities.StatusFinished || so.Status().IsFinal() || !saga.State.IsActive() {
			return domainerrors.ErrOrderNotCancellable
		}
		st = &step{
			saga: saga, so: so, actor: entities.ActorStaff,
			meta: contracts.Meta{SagaID: saga.ID, OSID: so.ID(), CorrelationID: logger.CorrelationID(ctx)},
		}
		if err := uc.o.compensate(ctx, st, reason, closed{}); err != nil {
			return err
		}
		return uc.o.save(ctx, st)
	})
	if err != nil {
		return err
	}
	st.committed()
	return nil
}

// DeliverServiceOrder registra a entrega do veículo ao cliente (finished → delivered).
type DeliverServiceOrder struct{ o *Orchestrator }

func NewDeliverServiceOrder(o *Orchestrator) *DeliverServiceOrder { return &DeliverServiceOrder{o: o} }

func (uc *DeliverServiceOrder) Execute(ctx context.Context, id string) error {
	return uc.o.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		so, err := uc.o.d.Orders.FindByID(ctx, id)
		if err != nil {
			return err
		}
		saga, err := uc.o.d.Sagas.FindByOSID(ctx, id)
		if err != nil {
			return err
		}
		from := so.Status()
		if err := so.TransitionTo(entities.StatusDelivered); err != nil {
			return err
		}
		st := &step{
			saga: saga, so: so, actor: entities.ActorStaff,
			meta: contracts.Meta{SagaID: saga.ID, OSID: so.ID(), CorrelationID: logger.CorrelationID(ctx)},
		}
		if err := uc.o.changeStatus(ctx, st, from, "", true); err != nil {
			return err
		}
		return uc.o.d.Orders.Update(ctx, so)
	})
}

// ExpireSagas compensa as sagas paradas além do prazo (orçamento sem resposta do cliente ou
// pagamento não confirmado). Roda periodicamente num worker do os-service.
type ExpireSagas struct {
	o     *Orchestrator
	batch int
}

func NewExpireSagas(o *Orchestrator) *ExpireSagas { return &ExpireSagas{o: o, batch: 50} }

func (uc *ExpireSagas) Execute(ctx context.Context, now time.Time) (int, error) {
	due, err := uc.o.d.Sagas.ListExpired(ctx, now, uc.batch)
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, candidate := range due {
		var st *step
		err := uc.o.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
			saga, err := uc.o.d.Sagas.FindByOSIDForUpdate(ctx, candidate.OSID)
			if err != nil {
				return err
			}
			if !saga.Expired(now) { // outra réplica ou uma resposta chegou antes
				return nil
			}
			so, err := uc.o.d.Orders.FindByID(ctx, saga.OSID)
			if err != nil {
				return err
			}
			reason := "prazo de pagamento expirado"
			if saga.State == entities.SagaAwaitingApproval {
				reason = "prazo de aprovação do orçamento expirado"
			}
			st = &step{
				saga: saga, so: so, actor: entities.ActorSystem,
				meta: contracts.Meta{SagaID: saga.ID, OSID: so.ID(), CorrelationID: "timeout-" + saga.ID},
			}
			if err := uc.o.compensate(ctx, st, reason, closed{}); err != nil {
				return err
			}
			return uc.o.save(ctx, st)
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return expired, err
			}
			logger.FromContext(ctx).Error("saga_expire_failed", "saga_id", candidate.ID, "os_id", candidate.OSID, "error", err)
			continue
		}
		if st != nil {
			expired++
			st.committed()
			uc.o.log(ctx, st).Warn("saga_expired", "reason", st.saga.FailureReason)
		}
	}
	return expired, nil
}
