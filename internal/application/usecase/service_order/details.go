package serviceorderuc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/Kc1t/postech-tc4-os-service/pkg/logger"
)

// Fontes da API composition.
const (
	SourceBilling   = "billing"
	SourceExecution = "execution"
)

// GetServiceOrderDetails é a API composition da OS: dados locais (OS, saga, histórico) mais o
// orçamento do billing-service e a execução do execution-service, consultados em paralelo.
// Se um serviço estiver fora (timeout, erro ou circuito aberto) a resposta sai parcial em vez
// de falhar inteira.
type GetServiceOrderDetails struct {
	orders    ports.ServiceOrderRepository
	sagas     ports.SagaRepository
	history   ports.StatusHistoryRepository
	billing   ports.BudgetQuery
	execution ports.ExecutionQuery
}

func NewGetServiceOrderDetails(
	orders ports.ServiceOrderRepository,
	sagas ports.SagaRepository,
	history ports.StatusHistoryRepository,
	billing ports.BudgetQuery,
	execution ports.ExecutionQuery,
) *GetServiceOrderDetails {
	return &GetServiceOrderDetails{orders: orders, sagas: sagas, history: history, billing: billing, execution: execution}
}

func (uc *GetServiceOrderDetails) Execute(ctx context.Context, id string) (*ports.ServiceOrderDetails, error) {
	so, err := uc.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &ports.ServiceOrderDetails{Order: so}
	if out.Saga, err = uc.sagas.FindByOSID(ctx, id); err != nil && !errors.Is(err, domainerrors.ErrNotFound) {
		return nil, err
	}
	if out.History, err = uc.history.ListByOS(ctx, id); err != nil {
		return nil, err
	}

	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		unavailable = map[string]bool{}
	)
	fetch := func(source string, call func() (json.RawMessage, error), dst *json.RawMessage) {
		defer wg.Done()
		body, err := call()
		if errors.Is(err, domainerrors.ErrNotFound) {
			return
		}
		if err != nil {
			logger.WithTrace(ctx, logger.FromContext(ctx)).Warn("composition_source_unavailable",
				"source", source, "os_id", id, "error", err)
			mu.Lock()
			unavailable[source] = true
			mu.Unlock()
			return
		}
		*dst = body
	}
	wg.Add(2)
	go fetch(SourceBilling, func() (json.RawMessage, error) { return uc.billing.BudgetByOS(ctx, id) }, &out.Budget)
	go fetch(SourceExecution, func() (json.RawMessage, error) { return uc.execution.ExecutionByOS(ctx, id) }, &out.Execution)
	wg.Wait()

	for _, s := range []string{SourceBilling, SourceExecution} {
		if unavailable[s] {
			out.Unavailable = append(out.Unavailable, s)
		}
	}
	return out, nil
}
