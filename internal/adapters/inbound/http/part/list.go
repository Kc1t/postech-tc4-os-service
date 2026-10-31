package parthandler

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	"github.com/gin-gonic/gin"
)

// FindAll godoc
// @Summary     Listar pecas/insumos
// @Tags        parts
// @Produce     json
// @Success     200 {array} commands.PartResponse
// @Security    BearerAuth
// @Router      /parts [get]
func (h *PartHandler) FindAll(c *gin.Context) {
	parts, err := h.listAll.Execute(c.Request.Context())
	if err != nil {
		httputil.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, commands.ToPartListResponse(parts))
}
