package requesterhandler

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	"github.com/gin-gonic/gin"
)

// Create godoc
// @Summary     Criar cliente
// @Tags        requesters
// @Accept      json
// @Produce     json
// @Param       body body commands.CreateRequesterRequest true "Dados do cliente"
// @Success     201 {object} commands.RequesterResponse
// @Failure     400 {object} httputil.ErrorResponse "corpo ou CPF/CNPJ inválidos"
// @Failure     409 {object} httputil.ErrorResponse "documento já cadastrado"
// @Security    BearerAuth
// @Router      /requesters [post]
func (h *RequesterHandler) Create(c *gin.Context) {
	var req commands.CreateRequesterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.HandleBadRequest(c, err)
		return
	}

	requester, err := req.ToDomain()
	if err != nil {
		httputil.HandleBadRequest(c, err)
		return
	}
	if err := h.create.Execute(c.Request.Context(), requester); err != nil {
		httputil.HandleError(c, err, msgAlreadyExists)
		return
	}

	c.JSON(http.StatusCreated, commands.ToRequesterResponse(requester))
}
