package authhandler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/commands"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports/mocks"
	"github.com/gin-gonic/gin"
	"go.uber.org/mock/gomock"
)

type sessionMocks struct {
	login   *mocks.MockLoginUseCase
	refresh *mocks.MockRefreshTokenUseCase
	logout  *mocks.MockLogoutUseCase
}

// sessionRouter monta login, refresh e logout; userID != nil simula o middleware de JWT.
func sessionRouter(t *testing.T, userID any) (*gin.Engine, sessionMocks) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctrl := gomock.NewController(t)
	m := sessionMocks{
		login:   mocks.NewMockLoginUseCase(ctrl),
		refresh: mocks.NewMockRefreshTokenUseCase(ctrl),
		logout:  mocks.NewMockLogoutUseCase(ctrl),
	}
	h := NewAuthHandler(nil, m.login, m.refresh, m.logout, 15)
	r := gin.New()
	r.POST("/auth/login", h.Login)
	r.POST("/auth/refresh", h.Refresh)
	r.POST("/auth/logout", func(c *gin.Context) {
		if userID != nil {
			c.Set("user_id", userID)
		}
		c.Next()
	}, h.Logout)
	return r, m
}

func post(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func decodeAuth(t *testing.T, w *httptest.ResponseRecorder) commands.AuthResponse {
	t.Helper()
	var resp commands.AuthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("corpo inválido: %s", w.Body.String())
	}
	return resp
}

func TestLogin(t *testing.T) {
	r, m := sessionRouter(t, nil)
	m.login.EXPECT().Execute(gomock.Any(), "admin@workshop.com", "admin123").Return("access", "refresh", nil)
	m.login.EXPECT().Execute(gomock.Any(), "admin@workshop.com", "errada").Return("", "", domainerrors.ErrInvalidCredentials)
	m.login.EXPECT().Execute(gomock.Any(), "bloqueado@workshop.com", gomock.Any()).Return("", "", domainerrors.ErrAccountLocked)

	w := post(r, "/auth/login", `{"email":"admin@workshop.com","password":"admin123"}`)
	resp := decodeAuth(t, w)
	if w.Code != http.StatusOK || resp.AccessToken != "access" || resp.RefreshToken != "refresh" || resp.ExpiresIn != 900 || resp.TokenType != "Bearer" {
		t.Fatalf("login: %d %+v", w.Code, resp)
	}

	for body, want := range map[string]int{
		`{"email":"nao-e-email","password":"x"}`:                http.StatusBadRequest,
		`{"email":"admin@workshop.com","password":"errada"}`:    http.StatusUnauthorized,
		`{"email":"bloqueado@workshop.com","password":"senha"}`: http.StatusTooManyRequests,
	} {
		if w := post(r, "/auth/login", body); w.Code != want {
			t.Errorf("%s: expected %d, got %d", body, want, w.Code)
		}
	}
}

func TestRefresh(t *testing.T) {
	r, m := sessionRouter(t, nil)
	m.refresh.EXPECT().Execute(gomock.Any(), "valido").Return("access-2", "refresh-2", nil)
	m.refresh.EXPECT().Execute(gomock.Any(), "revogado").Return("", "", domainerrors.ErrInvalidRefreshToken)

	w := post(r, "/auth/refresh", `{"refresh_token":"valido"}`)
	if resp := decodeAuth(t, w); w.Code != http.StatusOK || resp.AccessToken != "access-2" || resp.RefreshToken != "refresh-2" {
		t.Fatalf("refresh: %d %+v", w.Code, resp)
	}
	if w := post(r, "/auth/refresh", `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("sem refresh_token: expected 400, got %d", w.Code)
	}
	if w := post(r, "/auth/refresh", `{"refresh_token":"revogado"}`); w.Code != http.StatusUnauthorized {
		t.Errorf("revogado: expected 401, got %d", w.Code)
	}
}

func TestLogout(t *testing.T) {
	t.Run("encerra a sessão do usuário do token", func(t *testing.T) {
		r, m := sessionRouter(t, "u-1")
		m.logout.EXPECT().Execute(gomock.Any(), "u-1").Return(nil)
		if w := post(r, "/auth/logout", ""); w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})
	t.Run("sem usuário no contexto", func(t *testing.T) {
		r, _ := sessionRouter(t, nil)
		if w := post(r, "/auth/logout", ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})
	t.Run("user_id de outro tipo", func(t *testing.T) {
		r, _ := sessionRouter(t, 42)
		if w := post(r, "/auth/logout", ""); w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
	t.Run("falha ao revogar", func(t *testing.T) {
		r, m := sessionRouter(t, "u-1")
		m.logout.EXPECT().Execute(gomock.Any(), "u-1").Return(errors.New("banco fora"))
		if w := post(r, "/auth/logout", ""); w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}
