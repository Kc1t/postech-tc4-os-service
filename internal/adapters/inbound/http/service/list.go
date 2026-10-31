package servicehandler

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	"github.com/gin-gonic/gin"
)

// FindAll godoc
// @Summary     Listar servicos
// @Tags        services
// @Produce     json
// @Success     200 {array} commands.ServiceResponse
// @Security    BearerAuth
// @Router      /services [get]
func (h *ServiceHandler) FindAll(c *gin.Context) {
	services, err := h.listAll.Execute(c.Request.Context())
	if err != nil {
		httputil.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, commands.ToServiceListResponse(services))
}
