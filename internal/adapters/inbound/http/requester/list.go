package requesterhandler

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	"github.com/gin-gonic/gin"
)

// FindAll godoc
// @Summary     Listar clientes
// @Tags        requesters
// @Produce     json
// @Success     200 {array} commands.RequesterResponse
// @Security    BearerAuth
// @Router      /requesters [get]
func (h *RequesterHandler) FindAll(c *gin.Context) {
	requesters, err := h.listAll.Execute(c.Request.Context())
	if err != nil {
		httputil.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, commands.ToRequesterListResponse(requesters))
}
