package authhandler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports/mocks"
	"github.com/gin-gonic/gin"
	"go.uber.org/mock/gomock"
)

func registerRouter(t *testing.T) (*gin.Engine, *mocks.MockRegisterUseCase) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	reg := mocks.NewMockRegisterUseCase(ctrl)
	h := NewAuthHandler(reg, nil, nil, nil, 15)
	r := gin.New()
	r.POST("/auth/register", h.Register)
	return r, reg
}

func postRegister(r *gin.Engine, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestRegister_CriaUsuarioDaEquipe(t *testing.T) {
	r, reg := registerRouter(t)
	reg.EXPECT().Execute(gomock.Any(), "Mecânico", "mecanico@workshop.com", "senha-forte-123", entities.RoleAdmin).Return(nil)

	w := postRegister(r, `{"name":"Mecânico","email":"mecanico@workshop.com","password":"senha-forte-123"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegister_Erros(t *testing.T) {
	r, reg := registerRouter(t)
	reg.EXPECT().Execute(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(domainerrors.ErrAlreadyExists)

	if w := postRegister(r, `{"name":"x","email":"nao-e-email","password":"curta"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("corpo inválido: expected 400, got %d", w.Code)
	}
	if w := postRegister(r, `{"name":"x","email":"x@workshop.com","password":"senha-forte-123"}`); w.Code != http.StatusConflict {
		t.Fatalf("e-mail repetido: expected 409, got %d", w.Code)
	}
}
