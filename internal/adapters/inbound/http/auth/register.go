package authhandler

import (
	"net/http"

	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/gin-gonic/gin"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
)

// Register godoc
// @Summary     Registrar usuario da equipe (somente admin)
// @Description O cliente não se cadastra aqui: ele se autentica pelo CPF na Lambda auth-issuer.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body commands.RegisterRequest true "Dados do usuario"
// @Success     201 {object} commands.MessageResponse
// @Failure     400 {object} httputil.ErrorResponse
// @Failure     401 {object} httputil.ErrorResponse
// @Failure     403 {object} httputil.ErrorResponse
// @Failure     409 {object} httputil.ErrorResponse "e-mail já cadastrado"
// @Security    BearerAuth
// @Router      /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req commands.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.HandleBadRequest(c, err)
		return
	}

	err := h.register.Execute(c.Request.Context(), req.Name, req.Email, req.Password, entities.RoleAdmin)
	if err != nil {
		httputil.HandleError(c, err, "email already registered")
		return
	}

	c.JSON(http.StatusCreated, commands.MessageResponse{Message: "user registered successfully"})
}
