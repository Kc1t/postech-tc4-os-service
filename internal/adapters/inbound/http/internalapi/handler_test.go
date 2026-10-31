package internalapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports/mocks"
	"github.com/gin-gonic/gin"
	"go.uber.org/mock/gomock"
)

func init() { gin.SetMode(gin.TestMode) }

func call(h *Handler, path, token string) *httptest.ResponseRecorder {
	r := gin.New()
	h.SetupRoutes(r)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set(TokenHeader, token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequesterByDocument(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := mocks.NewMockGetRequesterByDocumentUseCase(ctrl)
	h := NewHandler("s3cr3t", uc)
	now := time.Now()
	uc.EXPECT().Execute(gomock.Any(), "52998224725").Return(
		entities.ReconstituteRequester("r1", "Ana", "52998224725", "ana@x.com", "", now, now), nil)

	w := call(h, "/internal/requesters/529.982.247-25", "s3cr3t")
	var got RequesterResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || got.ID != "r1" || got.Email != "ana@x.com" || got.Status == "" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}

	uc.EXPECT().Execute(gomock.Any(), "52998224725").Return(nil, domainerrors.ErrNotFound)
	if w := call(h, "/internal/requesters/52998224725", "s3cr3t"); w.Code != 404 {
		t.Fatalf("inexistente: %d", w.Code)
	}
	if w := call(h, "/internal/requesters/123", "s3cr3t"); w.Code != 400 {
		t.Fatalf("documento inválido: %d", w.Code)
	}
}

func TestRequireToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := mocks.NewMockGetRequesterByDocumentUseCase(ctrl)
	if w := call(NewHandler("s3cr3t", uc), "/internal/requesters/52998224725", "errado"); w.Code != 401 {
		t.Fatalf("token errado: %d", w.Code)
	}
	if w := call(NewHandler("s3cr3t", uc), "/internal/requesters/52998224725", ""); w.Code != 401 {
		t.Fatalf("sem token: %d", w.Code)
	}
	if w := call(NewHandler("", uc), "/internal/requesters/52998224725", "qualquer"); w.Code != 503 {
		t.Fatalf("sem token configurado: %d", w.Code)
	}
}
