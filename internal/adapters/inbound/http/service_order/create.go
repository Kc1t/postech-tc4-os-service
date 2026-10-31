package serviceorderhandler

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/gin-gonic/gin"
)

// SimulateFailureHeader é o header de demonstração das compensações da saga.
const SimulateFailureHeader = "X-Simulate-Failure"

// Create godoc
// @Summary     Criar ordem de servico
// @Tags        service-orders
// @Accept      json
// @Produce     json
// @Description Abre a OS e inicia a saga (StartDiagnosis para o execution-service). Com
// @Description FAULT_INJECTION_ENABLED=true, o header X-Simulate-Failure (budget, payment ou
// @Description repair) força a falha da etapa para demonstrar as compensações.
// @Param       body body commands.CreateServiceOrderRequest true "Dados da OS"
// @Param       X-Simulate-Failure header string false "Falha simulada: budget, payment ou repair"
// @Success     201 {object} commands.ServiceOrderResponse
// @Failure     400 {object} httputil.ErrorResponse
// @Failure     404 {object} httputil.ErrorResponse
// @Failure     422 {object} httputil.ErrorResponse
// @Security    BearerAuth
// @Router      /service-orders [post]
func (h *ServiceOrderHandler) Create(c *gin.Context) {
	var req commands.CreateServiceOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.HandleBadRequest(c, err)
		return
	}

	doc, err := entities.NewDocument(req.RequesterDocument)
	if err != nil {
		httputil.HandleBadRequest(c, err)
		return
	}

	input := entities.ServiceOrderInput{
		RequesterDocument: doc.Value(),
		VehiclePlate:      req.VehiclePlate,
		Notes:             req.Notes,
		SimulateFailure:   c.GetHeader(SimulateFailureHeader),
	}

	so, err := h.d.Create.Execute(c.Request.Context(), input)
	if err != nil {
		httputil.HandleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, commands.ToServiceOrderResponse(so))
}
