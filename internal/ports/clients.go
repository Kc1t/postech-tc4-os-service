package ports

import (
	"context"
	"encoding/json"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
)

//go:generate mockgen -source=./clients.go -destination=./mocks/clients.go -package=mocks

// BudgetQuery le o orcamento (com o pagamento) da OS no billing-service.
// Devolve domainerrors.ErrNotFound quando o orcamento ainda nao existe.
type BudgetQuery interface {
	BudgetByOS(ctx context.Context, osID string) (json.RawMessage, error)
}

// ExecutionQuery le a execucao (estado e linha do tempo) da OS no execution-service.
// Devolve domainerrors.ErrNotFound quando a execucao ainda nao existe.
type ExecutionQuery interface {
	ExecutionByOS(ctx context.Context, osID string) (json.RawMessage, error)
}

// ServiceOrderDetails e a visao composta da OS. Budget e Execution chegam como o JSON do
// servico dono; ficam nulos quando ainda nao existem. Unavailable lista as fontes que nao
// responderam: a resposta e parcial, mas a OS sempre e devolvida.
type ServiceOrderDetails struct {
	Order       *entities.ServiceOrder
	Saga        *entities.Saga
	History     []*entities.StatusHistoryEntry
	Budget      json.RawMessage
	Execution   json.RawMessage
	Unavailable []string
}
