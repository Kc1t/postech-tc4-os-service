package ports

import (
	"context"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
)

//go:generate mockgen -source=./usecases.go -destination=./mocks/usecases.go -package=mocks

// --- Requester ---

type CreateRequesterUseCase interface {
	Execute(ctx context.Context, c *entities.Requester) error
}

type GetRequesterUseCase interface {
	Execute(ctx context.Context, id string) (*entities.Requester, error)
}

type GetRequesterByDocumentUseCase interface {
	Execute(ctx context.Context, document string) (*entities.Requester, error)
}

type ListRequestersUseCase interface {
	Execute(ctx context.Context) ([]*entities.Requester, error)
}

type UpdateRequesterUseCase interface {
	Execute(ctx context.Context, c *entities.Requester) error
}

type DeleteRequesterUseCase interface {
	Execute(ctx context.Context, id string) error
}

// --- Vehicle ---

type CreateVehicleUseCase interface {
	Execute(ctx context.Context, input entities.VehicleInput) (*entities.Vehicle, error)
}

type GetVehicleUseCase interface {
	Execute(ctx context.Context, id string) (*entities.Vehicle, error)
}

type ListVehiclesUseCase interface {
	Execute(ctx context.Context) ([]*entities.Vehicle, error)
}

type ListVehiclesByRequesterUseCase interface {
	Execute(ctx context.Context, requesterID string) ([]*entities.Vehicle, error)
}

type UpdateVehicleUseCase interface {
	Execute(ctx context.Context, v *entities.Vehicle) error
}

type DeleteVehicleUseCase interface {
	Execute(ctx context.Context, id string) error
}

// --- ServiceOrder ---

type CreateServiceOrderUseCase interface {
	Execute(ctx context.Context, input entities.ServiceOrderInput) (*entities.ServiceOrder, error)
}

type GetServiceOrderUseCase interface {
	Execute(ctx context.Context, id string) (*entities.ServiceOrder, error)
}

type ListServiceOrdersUseCase interface {
	Execute(ctx context.Context) ([]*entities.ServiceOrder, error)
}

type ListServiceOrdersByRequesterUseCase interface {
	Execute(ctx context.Context, requesterID string) ([]*entities.ServiceOrder, error)
}

type GetServiceOrderByCodeUseCase interface {
	Execute(ctx context.Context, code int, requesterDocument string) (*entities.ServiceOrder, error)
}

type ListServiceOrdersByDocumentUseCase interface {
	Execute(ctx context.Context, document string) ([]*entities.ServiceOrder, error)
}

// CancelServiceOrderUseCase cancela a OS pela oficina, compensando as etapas da saga.
type CancelServiceOrderUseCase interface {
	Execute(ctx context.Context, id, reason string) error
}

// DeliverServiceOrderUseCase registra a entrega do veiculo (finished -> delivered).
type DeliverServiceOrderUseCase interface {
	Execute(ctx context.Context, id string) error
}

// GetServiceOrderHistoryUseCase devolve as mudancas de status da OS, em ordem cronologica.
type GetServiceOrderHistoryUseCase interface {
	Execute(ctx context.Context, id string) ([]*entities.StatusHistoryEntry, error)
}

// GetServiceOrderDetailsUseCase compoe a OS com o orcamento (billing) e a execucao (execution).
type GetServiceOrderDetailsUseCase interface {
	Execute(ctx context.Context, id string) (*ServiceOrderDetails, error)
}

// HandleSagaReplyUseCase processa uma resposta da fila os-saga-replies.
type HandleSagaReplyUseCase interface {
	Execute(ctx context.Context, env contracts.Envelope) error
	Types() []string
}

// ExpireSagasUseCase compensa as sagas com prazo vencido; devolve quantas expiraram.
type ExpireSagasUseCase interface {
	Execute(ctx context.Context, now time.Time) (int, error)
}

type GetAverageExecutionTimeUseCase interface {
	Execute(ctx context.Context) (time.Duration, error)
}

// --- Service ---

type CreateServiceUseCase interface {
	Execute(ctx context.Context, s *entities.Service) error
}

type GetServiceUseCase interface {
	Execute(ctx context.Context, id string) (*entities.Service, error)
}

type ListServicesUseCase interface {
	Execute(ctx context.Context) ([]*entities.Service, error)
}

type UpdateServiceUseCase interface {
	Execute(ctx context.Context, s *entities.Service) error
}

type DeleteServiceUseCase interface {
	Execute(ctx context.Context, id string) error
}

// --- Part ---

type CreatePartUseCase interface {
	Execute(ctx context.Context, p *entities.Part) error
}

type GetPartUseCase interface {
	Execute(ctx context.Context, id string) (*entities.Part, error)
}

type ListPartsUseCase interface {
	Execute(ctx context.Context) ([]*entities.Part, error)
}

type UpdatePartUseCase interface {
	Execute(ctx context.Context, p *entities.Part) error
}

type DeletePartUseCase interface {
	Execute(ctx context.Context, id string) error
}

type AdjustPartStockUseCase interface {
	Execute(ctx context.Context, id string, delta int) error
}

// --- Auth ---

// RegisterUseCase cria um novo usuario no sistema.
type RegisterUseCase interface {
	Execute(ctx context.Context, name, email, rawPassword string, role entities.Role) error
}

// LoginUseCase autentica um usuario e retorna access + refresh tokens.
type LoginUseCase interface {
	Execute(ctx context.Context, email, password string) (accessToken, refreshToken string, err error)
}

// RefreshTokenUseCase rotaciona o par de tokens (refresh token rotation).
type RefreshTokenUseCase interface {
	Execute(ctx context.Context, rawRefreshToken string) (accessToken, newRefreshToken string, err error)
}

// LogoutUseCase invalida todos os refresh tokens do usuario.
type LogoutUseCase interface {
	Execute(ctx context.Context, userID string) error
}
