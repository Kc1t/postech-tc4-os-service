package serviceorderhandler

import (
	"fmt"
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/gin-gonic/gin"
)

// FindByDocument godoc
// @Summary     Listar OS do cliente por CPF/CNPJ (cliente autenticado por CPF)
// @Tags        service-orders-requester
// @Produce     json
// @Param       document query string true "CPF ou CNPJ do cliente"
// @Success     200 {array} commands.ServiceOrderResponse
// @Failure     400 {object} httputil.ErrorResponse "CPF/CNPJ ausente ou inválido"
// @Failure     403 {object} httputil.ErrorResponse "o CPF do token não é o do filtro"
// @Security    BearerAuth
// @Router      /service-orders/requester [get]
func (h *ServiceOrderHandler) FindByDocument(c *gin.Context) {
	raw := c.Query("document")
	if raw == "" {
		httputil.HandleBadRequest(c, fmt.Errorf("query parameter 'document' is required"))
		return
	}

	doc, err := entities.NewDocument(raw)
	if err != nil {
		httputil.HandleBadRequest(c, err)
		return
	}

	if !authorizeDocument(c, doc.Value()) {
		return
	}

	orders, err := h.d.ListByDocument.Execute(c.Request.Context(), doc.Value())
	if err != nil {
		httputil.HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, commands.ToServiceOrderListResponse(orders))
}
