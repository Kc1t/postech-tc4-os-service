package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/pkg/httpclient"
	"github.com/golang-jwt/jwt/v5"
)

const secret = "segredo-de-teste"

func cfg(url string) Config {
	return Config{Name: "billing", BaseURL: url, JWTSecret: secret,
		HTTP: httpclient.Config{Timeout: time.Second, MaxRetries: 1, BaseBackoff: time.Millisecond}}
}

func TestBillingClient_EnviaTokenDeServicoELeJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/budgets/os/os-1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		tok, err := jwt.Parse(raw, func(*jwt.Token) (any, error) { return []byte(secret), nil })
		if err != nil || !tok.Valid {
			t.Errorf("token inválido: %v", err)
		} else if c := tok.Claims.(jwt.MapClaims); c["role"] != "admin" || c["sub"] != serviceSubject {
			t.Errorf("claims = %v", c)
		}
		_, _ = w.Write([]byte(`{"id":"b1"}`))
	}))
	defer srv.Close()

	body, err := NewBillingClient(cfg(srv.URL+"/"), nil).BudgetByOS(context.Background(), "os-1")
	if err != nil || string(body) != `{"id":"b1"}` {
		t.Fatalf("%s %v", body, err)
	}
}

func TestExecutionClient_Erros(t *testing.T) {
	status := http.StatusNotFound
	body := `{}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/executions/os-9" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := NewExecutionClient(cfg(srv.URL), nil)

	if _, err := c.ExecutionByOS(context.Background(), "os-9"); !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatalf("404: %v", err)
	}
	status = http.StatusForbidden
	if _, err := c.ExecutionByOS(context.Background(), "os-9"); !errors.Is(err, httpclient.ErrUpstream) {
		t.Fatalf("403: %v", err)
	}
	status, body = http.StatusOK, `não é json`
	if _, err := c.ExecutionByOS(context.Background(), "os-9"); !errors.Is(err, httpclient.ErrUpstream) {
		t.Fatalf("json inválido: %v", err)
	}
	status = http.StatusServiceUnavailable
	if _, err := c.ExecutionByOS(context.Background(), "os-9"); err == nil {
		t.Fatal("503 deve falhar")
	}
}

func TestClient_ServicoForaDoAr(t *testing.T) {
	c := NewBillingClient(cfg("http://127.0.0.1:1"), nil)
	if _, err := c.BudgetByOS(context.Background(), "os-1"); err == nil {
		t.Fatal("conexão recusada deve falhar")
	}
}
