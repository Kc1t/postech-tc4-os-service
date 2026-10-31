// Package internalapi expõe rotas usadas só por outros componentes da plataforma (não pelo
// API Gateway público). A Lambda de autenticação consulta o cliente por CPF aqui, em vez de
// ler o banco do os-service: cada serviço é dono dos próprios dados.
package internalapi

import (
	"crypto/subtle"
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/gin-gonic/gin"
)

// TokenHeader carrega o segredo compartilhado entre o os-service e os clientes internos.
const TokenHeader = "X-Internal-Token"

type Handler struct {
	token  string
	byDocs ports.GetRequesterByDocumentUseCase
}

func NewHandler(token string, byDocument ports.GetRequesterByDocumentUseCase) *Handler {
	return &Handler{token: token, byDocs: byDocument}
}

// SetupRoutes registra /internal/* protegido pelo token interno. Sem token configurado as
// rotas respondem 503: melhor indisponível do que aberto.
func (h *Handler) SetupRoutes(r gin.IRouter) {
	g := r.Group("/internal", h.requireToken)
	g.GET("/requesters/:document", h.RequesterByDocument)
}

func (h *Handler) requireToken(c *gin.Context) {
	if h.token == "" {
		httputil.Abort(c, http.StatusServiceUnavailable, "INTERNAL_API_DISABLED", "internal api token not configured")
		return
	}
	got := c.GetHeader(TokenHeader)
	if subtle.ConstantTimeCompare([]byte(got), []byte(h.token)) != 1 {
		httputil.Abort(c, http.StatusUnauthorized, httputil.CodeUnauthorized, "invalid internal token")
		return
	}
	c.Next()
}

type RequesterResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Document string `json:"document"`
	Status   string `json:"status" example:"active"`
}

// RequesterByDocument godoc
// @Summary     Cliente por CPF/CNPJ (uso interno da Lambda de autenticação)
// @Description Rota servida fora do prefixo /api/v1, em GET /internal/requesters/{document}; não é exposta no API Gateway.
// @Tags        internal
// @Produce     json
// @Param       document path string true "CPF ou CNPJ (só dígitos)"
// @Param       X-Internal-Token header string true "Token interno"
// @Success     200 {object} internalapi.RequesterResponse
// @Failure     401 {object} httputil.ErrorResponse
// @Failure     404 {object} httputil.ErrorResponse
// @Router      /internal/requesters/{document} [get]
func (h *Handler) RequesterByDocument(c *gin.Context) {
	doc, err := entities.NewDocument(c.Param("document"))
	if err != nil {
		httputil.HandleBadRequest(c, err)
		return
	}
	r, err := h.byDocs.Execute(c.Request.Context(), doc.Value())
	if err != nil {
		httputil.HandleError(c, err, "requester not found")
		return
	}
	c.JSON(http.StatusOK, RequesterResponse{
		ID: r.ID(), Name: r.Name(), Email: r.Email(), Document: r.Document(), Status: string(r.Status()),
	})
}
