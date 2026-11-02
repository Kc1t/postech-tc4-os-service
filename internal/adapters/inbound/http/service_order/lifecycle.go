package serviceorderhandler

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	"github.com/gin-gonic/gin"
)

// Cancel godoc
// @Summary     Cancelar OS (oficina)
// @Description Compensa as etapas já concluídas da saga (estorno, orçamento, peças, execução) e
// @Description encerra a OS como cancelled. OS finalizada ou entregue não pode ser cancelada.
// @Tags        service-orders
// @Accept      json
// @Produce     json
// @Param       id   path string                      true  "ServiceOrder ID"
// @Param       body body commands.CancelServiceOrderRequest false "Motivo"
// @Success     204
// @Failure     400 {object} httputil.ErrorResponse "id não é UUID"
// @Failure     404 {object} httputil.ErrorResponse
// @Failure     409 {object} httputil.ErrorResponse
// @Security    BearerAuth
// @Router      /service-orders/{id}/cancel [post]
func (h *ServiceOrderHandler) Cancel(c *gin.Context) {
	var req commands.CancelServiceOrderRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			httputil.HandleBadRequest(c, err)
			return
		}
	}
	if err := h.d.Cancel.Execute(c.Request.Context(), c.Param("id"), req.Reason); err != nil {
		httputil.HandleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Deliver godoc
// @Summary     Registrar entrega do veículo
// @Description finished → delivered.
// @Tags        service-orders
// @Param       id path string true "ServiceOrder ID"
// @Success     204
// @Failure     400 {object} httputil.ErrorResponse "id não é UUID"
// @Failure     404 {object} httputil.ErrorResponse
// @Failure     409 {object} httputil.ErrorResponse
// @Security    BearerAuth
// @Router      /service-orders/{id}/deliver [post]
func (h *ServiceOrderHandler) Deliver(c *gin.Context) {
	if err := h.d.Deliver.Execute(c.Request.Context(), c.Param("id")); err != nil {
		httputil.HandleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// History godoc
// @Summary     Histórico de status da OS
// @Tags        service-orders
// @Produce     json
// @Param       id path string true "ServiceOrder ID"
// @Success     200 {array} commands.StatusHistoryResponse
// @Failure     400 {object} httputil.ErrorResponse "id não é UUID"
// @Failure     404 {object} httputil.ErrorResponse
// @Security    BearerAuth
// @Router      /service-orders/{id}/history [get]
func (h *ServiceOrderHandler) History(c *gin.Context) {
	entries, err := h.d.History.Execute(c.Request.Context(), c.Param("id"))
	if err != nil {
		httputil.HandleError(c, err, msgNotFound)
		return
	}
	c.JSON(http.StatusOK, commands.ToStatusHistoryResponse(entries))
}

// Details godoc
// @Summary     Visão completa da OS (API composition)
// @Description Junta a OS e a saga (os-service), o orçamento e o pagamento (billing-service) e a
// @Description execução (execution-service). Se um serviço não responder, a resposta é parcial:
// @Description o campo dele vem nulo e o nome aparece em "unavailable".
// @Tags        service-orders
// @Produce     json
// @Param       id path string true "ServiceOrder ID"
// @Success     200 {object} commands.ServiceOrderDetailsResponse
// @Failure     400 {object} httputil.ErrorResponse "id não é UUID"
// @Failure     404 {object} httputil.ErrorResponse
// @Security    BearerAuth
// @Router      /service-orders/{id}/details [get]
func (h *ServiceOrderHandler) Details(c *gin.Context) {
	d, err := h.d.Details.Execute(c.Request.Context(), c.Param("id"))
	if err != nil {
		httputil.HandleError(c, err, msgNotFound)
		return
	}
	c.JSON(http.StatusOK, commands.ToServiceOrderDetailsResponse(d))
}
