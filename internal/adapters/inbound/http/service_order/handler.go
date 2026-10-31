package serviceorderhandler

import (
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/gin-gonic/gin"
)

// Deps são os casos de uso expostos pelas rotas de OS.
type Deps struct {
	Create               ports.CreateServiceOrderUseCase
	GetByID              ports.GetServiceOrderUseCase
	GetByCode            ports.GetServiceOrderByCodeUseCase
	ListAll              ports.ListServiceOrdersUseCase
	ListByDocument       ports.ListServiceOrdersByDocumentUseCase
	Cancel               ports.CancelServiceOrderUseCase
	Deliver              ports.DeliverServiceOrderUseCase
	History              ports.GetServiceOrderHistoryUseCase
	Details              ports.GetServiceOrderDetailsUseCase
	AverageExecutionTime ports.GetAverageExecutionTimeUseCase
}

type ServiceOrderHandler struct{ d Deps }

func NewServiceOrderHandler(d Deps) *ServiceOrderHandler { return &ServiceOrderHandler{d: d} }

// SetupRoutes registra as rotas da equipe da oficina (JWT de admin).
// As etapas entre a abertura e a finalização são dirigidas pela saga: o diagnóstico e o
// reparo acontecem no execution-service, orçamento e pagamento no billing-service.
func (h *ServiceOrderHandler) SetupRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/service-orders")
	g.POST("", h.Create)
	g.GET("", h.FindAll)
	g.GET("/metrics/execution-time", h.AverageExecutionTime)
	g.GET("/:id", h.FindByID)
	g.GET("/:id/details", h.Details)
	g.GET("/:id/history", h.History)
	g.POST("/:id/cancel", h.Cancel)
	g.POST("/:id/deliver", h.Deliver)
}

// SetupRequesterRoutes registra as rotas do cliente, autenticado pelo CPF.
// Aprovar ou recusar o orçamento é feito no billing-service.
func (h *ServiceOrderHandler) SetupRequesterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/service-orders")
	g.GET("/code/:code", h.FindByCode)
	g.GET("/requester", h.FindByDocument)
}
