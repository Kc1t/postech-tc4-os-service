package authhandler

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	"github.com/gin-gonic/gin"
)

// Logout godoc
// @Summary     Encerrar sessao (revoga todos os refresh tokens)
// @Tags        auth
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} commands.MessageResponse
// @Router      /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		httputil.Abort(c, http.StatusUnauthorized, httputil.CodeUnauthorized, "user not identified")
		return
	}

	userID, ok := userIDVal.(string)
	if !ok {
		httputil.Abort(c, http.StatusInternalServerError, httputil.CodeInternal, "internal server error")
		return
	}

	if err := h.logout.Execute(c.Request.Context(), userID); err != nil {
		httputil.HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, commands.MessageResponse{Message: "logged out successfully"})
}
