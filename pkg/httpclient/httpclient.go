// Package httpclient é o cliente HTTP para chamadas síncronas entre microsserviços.
//
// Combina três defesas: timeout por tentativa, retry com backoff exponencial e jitter (só para
// métodos idempotentes e respostas 5xx/429 ou erro de rede) e circuit breaker por serviço de
// destino. Com o circuito aberto a chamada falha na hora com ErrCircuitOpen, e quem chama
// devolve uma resposta degradada em vez de esperar um serviço que já está fora.
package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sony/gobreaker/v2"
)

// ErrCircuitOpen é devolvido quando o circuit breaker do destino está aberto.
var ErrCircuitOpen = errors.New("httpclient: circuito aberto")

// ErrUpstream indica resposta 5xx (ou 429) depois de esgotadas as tentativas.
var ErrUpstream = errors.New("httpclient: falha no serviço de destino")

// Config ajusta o cliente.
type Config struct {
	Name             string        // nome do serviço de destino (métricas e logs)
	Timeout          time.Duration // timeout de cada tentativa
	MaxRetries       int           // tentativas extras além da primeira
	BaseBackoff      time.Duration // primeiro intervalo de espera
	FailureThreshold uint32        // falhas consecutivas que abrem o circuito
	OpenTimeout      time.Duration // tempo em aberto antes de ir para meio-aberto
}

func (c Config) withDefaults() Config {
	if c.Timeout == 0 {
		c.Timeout = 2 * time.Second
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 2
	}
	if c.BaseBackoff == 0 {
		c.BaseBackoff = 100 * time.Millisecond
	}
	if c.FailureThreshold == 0 {
		c.FailureThreshold = 5
	}
	if c.OpenTimeout == 0 {
		c.OpenTimeout = 30 * time.Second
	}
	return c
}

// Client executa requisições com timeout, retry e circuit breaker.
type Client struct {
	cfg     Config
	http    *http.Client
	breaker *gobreaker.CircuitBreaker[*http.Response]
	log     *slog.Logger
	sleep   func(context.Context, time.Duration) error
}

// New cria um cliente. O transporte é instrumentado pelo New Relic para que a chamada apareça
// como external no trace e propague os headers de distributed tracing.
func New(cfg Config, log *slog.Logger) *Client {
	cfg = cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	log = log.With("upstream", cfg.Name)
	st := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: 1,
		Timeout:     cfg.OpenTimeout,
		ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= cfg.FailureThreshold },
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit_breaker_state_changed", "from", from.String(), "to", to.String())
		},
	}
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Transport: newrelic.NewRoundTripper(http.DefaultTransport)},
		breaker: gobreaker.NewCircuitBreaker[*http.Response](st), //nolint:bodyclose // tipo genérico, não é uma resposta
		log:     log,
		sleep:   sleepCtx,
	}
}

// State devolve o estado atual do circuito ("closed", "open", "half-open").
func (c *Client) State() string { return c.breaker.State().String() }

// Do executa a requisição. newReq é chamado a cada tentativa para gerar um *http.Request novo
// (o corpo de uma requisição não pode ser relido). O chamador fecha o corpo da resposta.
func (c *Client) Do(ctx context.Context, newReq func(ctx context.Context) (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			if err := c.sleep(ctx, c.backoff(attempt)); err != nil {
				return nil, err
			}
		}
		resp, err := c.breaker.Execute(func() (*http.Response, error) {
			return c.once(ctx, newReq)
		})
		switch {
		case errors.Is(err, gobreaker.ErrOpenState), errors.Is(err, gobreaker.ErrTooManyRequests):
			return nil, fmt.Errorf("%w: %s", ErrCircuitOpen, c.cfg.Name)
		case err == nil:
			return resp, nil
		}
		lastErr = err
		if !retryable(err) {
			return nil, err
		}
		c.log.Warn("upstream_call_retry", "attempt", attempt+1, "error", err)
	}
	return nil, lastErr
}

// statusError carrega a resposta 5xx/429 para decidir o retry e devolver ao chamador.
type statusError struct{ code int }

func (e statusError) Error() string { return fmt.Sprintf("%v: status %d", ErrUpstream, e.code) }
func (e statusError) Unwrap() error { return ErrUpstream }

type nonIdempotentError struct{ err error }

func (e nonIdempotentError) Error() string { return e.err.Error() }
func (e nonIdempotentError) Unwrap() error { return e.err }

func (c *Client) once(ctx context.Context, newReq func(ctx context.Context) (*http.Request, error)) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	req, err := newReq(ctx)
	if err != nil {
		cancel()
		return nil, nonIdempotentError{err}
	}
	if txn := newrelic.FromContext(ctx); txn != nil {
		req = newrelic.RequestWithTransactionContext(req, txn)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		if !idempotent(req.Method) {
			return nil, nonIdempotentError{err}
		}
		return nil, err
	}
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		cancel()
		if !idempotent(req.Method) {
			return nil, nonIdempotentError{statusError{resp.StatusCode}}
		}
		return nil, statusError{resp.StatusCode}
	}
	resp.Body = cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

func (c *Client) backoff(attempt int) time.Duration {
	d := c.cfg.BaseBackoff << (attempt - 1)
	jitter := time.Duration(rand.Int64N(int64(d)/2 + 1)) // #nosec G404 -- jitter de backoff, não é segurança
	return d + jitter
}

func retryable(err error) bool {
	var ni nonIdempotentError
	if errors.As(err, &ni) {
		return false
	}
	return !errors.Is(err, context.Canceled)
}

func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
