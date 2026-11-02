package requesterhandler

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/gin-gonic/gin"
)

// Delete godoc
// @Summary     Deletar cliente
// @Tags        requesters
// @Param       id path string true "Requester ID"
// @Success     204
// @Failure     400 {object} httputil.ErrorResponse "id não é UUID"
// @Failure     404 {object} httputil.ErrorResponse
// @Failure     409 {object} httputil.ErrorResponse "RESOURCE_IN_USE: referenciado por ordem de serviço"
// @Security    BearerAuth
// @Router      /requesters/{id} [delete]
func (h *RequesterHandler) Delete(c *gin.Context) {
	if err := h.delete.Execute(c.Request.Context(), c.Param("id")); err != nil {
		httputil.HandleError(c, err, msgNotFound)
		return
	}
	c.Status(http.StatusNoContent)
}
