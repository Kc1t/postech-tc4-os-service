package parthandler

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/gin-gonic/gin"
)

// Delete godoc
// @Summary     Deletar peca/insumo
// @Tags        parts
// @Param       id path string true "Part ID"
// @Success     204
// @Failure     404 {object} httputil.ErrorResponse
// @Failure     409 {object} httputil.ErrorResponse "RESOURCE_IN_USE: referenciado por ordem de serviço"
// @Security    BearerAuth
// @Router      /parts/{id} [delete]
func (h *PartHandler) Delete(c *gin.Context) {
	if err := h.delete.Execute(c.Request.Context(), c.Param("id")); err != nil {
		httputil.HandleError(c, err, msgNotFound)
		return
	}
	c.Status(http.StatusNoContent)
}
