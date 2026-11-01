package httputil

import (
	"errors"
	"net/http"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/gin-gonic/gin"
)

// ErrorBody é o formato de erro comum a todos os microsserviços da oficina:
//
//	{"error": {"code": "SERVICE_ORDER_NOT_FOUND", "message": "..."}}
type ErrorBody struct {
	Code    string `json:"code"    example:"NOT_FOUND"`
	Message string `json:"message" example:"service order not found"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// Códigos de erro expostos pela API (tabela no README).
const (
	CodeValidation         = "VALIDATION_ERROR"
	CodeNotFound           = "NOT_FOUND"
	CodeAlreadyExists      = "ALREADY_EXISTS"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeForbidden          = "FORBIDDEN"
	CodeInvalidTransition  = "INVALID_STATUS_TRANSITION"
	CodeNotCancellable     = "ORDER_NOT_CANCELLABLE"
	CodeInsufficientStock  = "INSUFFICIENT_STOCK"
	CodeVehicleNotOwned    = "VEHICLE_NOT_FROM_REQUESTER"
	CodeConcurrentUpdate   = "CONCURRENT_UPDATE"
	CodeAccountLocked      = "ACCOUNT_LOCKED"
	CodeInternal           = "INTERNAL_ERROR"
	CodeUnprocessable      = "UNPROCESSABLE_ENTITY"
	CodeInvalidSimulateArg = "INVALID_SIMULATE_FAILURE"
	CodeInUse              = "RESOURCE_IN_USE"
)

type mapping struct {
	err    error
	status int
	code   string
}

var mappings = []mapping{
	{domainerrors.ErrNotFound, http.StatusNotFound, CodeNotFound},
	{domainerrors.ErrAlreadyExists, http.StatusConflict, CodeAlreadyExists},
	{domainerrors.ErrInUse, http.StatusConflict, CodeInUse},
	{domainerrors.ErrConcurrentUpdate, http.StatusConflict, CodeConcurrentUpdate},
	{domainerrors.ErrInvalidCredentials, http.StatusUnauthorized, CodeUnauthorized},
	{domainerrors.ErrInvalidRefreshToken, http.StatusUnauthorized, CodeUnauthorized},
	{domainerrors.ErrAccountLocked, http.StatusTooManyRequests, CodeAccountLocked},
	{domainerrors.ErrStatusNotAllowedForRequester, http.StatusForbidden, CodeForbidden},
	{domainerrors.ErrDocumentMismatch, http.StatusForbidden, CodeForbidden},
	{domainerrors.ErrInvalidID, http.StatusBadRequest, CodeValidation},
	{domainerrors.ErrInvalidDocument, http.StatusUnprocessableEntity, CodeValidation},
	{domainerrors.ErrInvalidPlate, http.StatusUnprocessableEntity, CodeValidation},
	{domainerrors.ErrInvalidSimulatedFailure, http.StatusBadRequest, CodeInvalidSimulateArg},
	{domainerrors.ErrInvalidStatus, http.StatusConflict, CodeInvalidTransition},
	{domainerrors.ErrInvalidStatusValue, http.StatusUnprocessableEntity, CodeValidation},
	{domainerrors.ErrInsufficientStock, http.StatusUnprocessableEntity, CodeInsufficientStock},
	{domainerrors.ErrOrderNotCancellable, http.StatusConflict, CodeNotCancellable},
	{domainerrors.ErrVehicleNotFromRequester, http.StatusUnprocessableEntity, CodeVehicleNotOwned},
}

// HandleError traduz um erro de domínio para status HTTP e código. msg substitui a mensagem
// padrão (o texto do erro). Erros desconhecidos viram 500 sem expor detalhes.
func HandleError(c *gin.Context, err error, msg ...string) {
	status, code := http.StatusInternalServerError, CodeInternal
	for _, m := range mappings {
		if errors.Is(err, m.err) {
			status, code = m.status, m.code
			break
		}
	}
	message := err.Error()
	switch {
	case status == http.StatusInternalServerError:
		message = "internal server error"
		_ = c.Error(err)
	case len(msg) > 0:
		message = msg[0]
	}
	Abort(c, status, code, message)
}

// HandleBadRequest responde 400 para corpo, parâmetro ou query inválidos.
func HandleBadRequest(c *gin.Context, err error) {
	Abort(c, http.StatusBadRequest, CodeValidation, err.Error())
}

// Abort escreve a resposta de erro no formato padrão e interrompe a cadeia de handlers.
func Abort(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}
