// Package services contém os clientes HTTP do os-service para os outros microsserviços,
// usados na API composition (GET /service-orders/{id}/details). Toda chamada passa pelo
// pkg/httpclient (timeout, retry com backoff e circuit breaker) e se autentica com um JWT
// de serviço de curta duração assinado com o segredo compartilhado.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/Kc1t/postech-tc4-os-service/pkg/httpclient"
	"github.com/golang-jwt/jwt/v5"
)

const serviceSubject = "service:os-service"

// Config de um cliente.
type Config struct {
	Name      string // billing | execution (nome do circuito e nos logs)
	BaseURL   string
	JWTSecret string
	HTTP      httpclient.Config
}

type client struct {
	cfg  Config
	http *httpclient.Client
	path func(osID string) string
}

func newClient(cfg Config, log *slog.Logger, path func(string) string) *client {
	if cfg.HTTP.Name == "" {
		cfg.HTTP.Name = cfg.Name
	}
	return &client{cfg: cfg, http: httpclient.New(cfg.HTTP, log), path: path}
}

// NewBillingClient lê o orçamento da OS no billing-service.
func NewBillingClient(cfg Config, log *slog.Logger) ports.BudgetQuery {
	return billingClient{newClient(cfg, log, func(id string) string { return "/api/v1/budgets/os/" + url.PathEscape(id) })}
}

// NewExecutionClient lê a execução da OS no execution-service.
func NewExecutionClient(cfg Config, log *slog.Logger) ports.ExecutionQuery {
	return executionClient{newClient(cfg, log, func(id string) string { return "/api/v1/executions/" + url.PathEscape(id) })}
}

type billingClient struct{ *client }

func (c billingClient) BudgetByOS(ctx context.Context, osID string) (json.RawMessage, error) {
	return c.get(ctx, osID)
}

type executionClient struct{ *client }

func (c executionClient) ExecutionByOS(ctx context.Context, osID string) (json.RawMessage, error) {
	return c.get(ctx, osID)
}

func (c *client) get(ctx context.Context, osID string) (json.RawMessage, error) {
	token, err := c.serviceToken()
	if err != nil {
		return nil, err
	}
	target := strings.TrimRight(c.cfg.BaseURL, "/") + c.path(osID)
	resp, err := c.http.Do(ctx, func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: ler resposta: %w", c.cfg.Name, err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, domainerrors.ErrNotFound
	case resp.StatusCode >= 400:
		return nil, fmt.Errorf("%w: %s respondeu %d", httpclient.ErrUpstream, c.cfg.Name, resp.StatusCode)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("%w: %s devolveu JSON inválido", httpclient.ErrUpstream, c.cfg.Name)
	}
	return body, nil
}

// serviceToken emite o JWT do os-service para chamadas entre serviços (papel admin, 2 minutos).
func (c *client) serviceToken() (string, error) {
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  serviceSubject,
		"role": "admin",
		"iat":  now.Unix(),
		"exp":  now.Add(2 * time.Minute).Unix(),
	})
	return t.SignedString([]byte(c.cfg.JWTSecret))
}
