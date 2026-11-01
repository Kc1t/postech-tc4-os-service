package httputil

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func run(fn func(c *gin.Context)) (*httptest.ResponseRecorder, ErrorResponse) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	fn(c)
	var resp ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	return w, resp
}

func TestHandleError_Mapeamento(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{domainerrors.ErrNotFound, 404, CodeNotFound},
		{fmt.Errorf("repo: %w", domainerrors.ErrNotFound), 404, CodeNotFound},
		{domainerrors.ErrAlreadyExists, 409, CodeAlreadyExists},
		{domainerrors.ErrInUse, 409, CodeInUse},
		{domainerrors.ErrConcurrentUpdate, 409, CodeConcurrentUpdate},
		{domainerrors.ErrInvalidCredentials, 401, CodeUnauthorized},
		{domainerrors.ErrInvalidRefreshToken, 401, CodeUnauthorized},
		{domainerrors.ErrAccountLocked, 429, CodeAccountLocked},
		{domainerrors.ErrDocumentMismatch, 403, CodeForbidden},
		{domainerrors.ErrInvalidDocument, 422, CodeValidation},
		{domainerrors.ErrInvalidStatus, 409, CodeInvalidTransition},
		{domainerrors.ErrOrderNotCancellable, 409, CodeNotCancellable},
		{domainerrors.ErrInsufficientStock, 422, CodeInsufficientStock},
		{domainerrors.ErrVehicleNotFromRequester, 422, CodeVehicleNotOwned},
		{domainerrors.ErrInvalidSimulatedFailure, 400, CodeInvalidSimulateArg},
	}
	for _, tc := range cases {
		w, resp := run(func(c *gin.Context) { HandleError(c, tc.err) })
		if w.Code != tc.status || resp.Error.Code != tc.code || resp.Error.Message == "" {
			t.Errorf("%v: status=%d code=%q msg=%q", tc.err, w.Code, resp.Error.Code, resp.Error.Message)
		}
	}
}

func TestHandleError_InternoNaoVazaDetalhes(t *testing.T) {
	w, resp := run(func(c *gin.Context) { HandleError(c, errors.New("pq: senha do banco")) })
	if w.Code != 500 || resp.Error.Code != CodeInternal || resp.Error.Message != "internal server error" {
		t.Fatalf("resposta 500 inadequada: %d %+v", w.Code, resp)
	}
}

func TestHandleError_MensagemCustomizada(t *testing.T) {
	_, resp := run(func(c *gin.Context) { HandleError(c, domainerrors.ErrNotFound, "service order not found") })
	if resp.Error.Message != "service order not found" {
		t.Fatalf("mensagem = %q", resp.Error.Message)
	}
}

func TestHandleBadRequest(t *testing.T) {
	w, resp := run(func(c *gin.Context) { HandleBadRequest(c, errors.New("campo obrigatório")) })
	if w.Code != 400 || resp.Error.Code != CodeValidation || resp.Error.Message != "campo obrigatório" {
		t.Fatalf("%d %+v", w.Code, resp)
	}
}
