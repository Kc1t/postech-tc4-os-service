package requesterhandler

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/gin-gonic/gin"
)

// FindByDocument godoc
// @Summary     Buscar cliente por documento
// @Tags        requesters
// @Produce     json
// @Param       document path string true "CPF ou CNPJ do cliente"
// @Success     200 {object} commands.RequesterResponse
// @Failure     400 {object} httputil.ErrorResponse "CPF/CNPJ inválido"
// @Failure     404 {object} httputil.ErrorResponse
// @Security    BearerAuth
// @Router      /requesters/document/{document} [get]
func (h *RequesterHandler) FindByDocument(c *gin.Context) {
	doc, err := entities.NewDocument(c.Param("document"))
	if err != nil {
		httputil.Abort(c, http.StatusBadRequest, httputil.CodeValidation, msgInvalidDoc)
		return
	}
	requester, err := h.getByDocument.Execute(c.Request.Context(), doc.Value())
	if err != nil {
		httputil.HandleError(c, err, msgNotFound)
		return
	}
	c.JSON(http.StatusOK, commands.ToRequesterResponse(requester))
}
