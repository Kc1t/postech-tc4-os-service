package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func noSleep(context.Context, time.Duration) error { return nil }

func get(url string) func(ctx context.Context) (*http.Request, error) {
	return func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	}
}

func post(url string) func(ctx context.Context) (*http.Request, error) {
	return func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	}
}

func TestDo_RetryAteSucesso(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	c := New(Config{Name: "billing", MaxRetries: 2}, nil)
	c.sleep = noSleep
	resp, err := c.Do(context.Background(), get(srv.URL))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "ok" || calls.Load() != 3 {
		t.Fatalf("esperava sucesso na 3ª tentativa, veio %q após %d", b, calls.Load())
	}
}

func TestDo_EsgotaTentativas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(Config{Name: "billing", MaxRetries: 1, FailureThreshold: 100}, nil)
	c.sleep = noSleep
	if _, err := c.Do(context.Background(), get(srv.URL)); !errors.Is(err, ErrUpstream) {
		t.Fatalf("esperava ErrUpstream, veio %v", err)
	}
}

func TestDo_PostNaoRepete(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := New(Config{Name: "billing", MaxRetries: 3, FailureThreshold: 100}, nil)
	c.sleep = noSleep
	if _, err := c.Do(context.Background(), post(srv.URL)); err == nil {
		t.Fatal("esperava erro")
	}
	if calls.Load() != 1 {
		t.Fatalf("POST não é idempotente e não pode ser repetido; chamadas = %d", calls.Load())
	}
}

func TestDo_4xxNaoEhFalha(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(Config{Name: "execution", FailureThreshold: 1}, nil)
	for i := 0; i < 3; i++ {
		resp, err := c.Do(context.Background(), get(srv.URL))
		if err != nil {
			t.Fatalf("4xx deveria voltar como resposta, veio %v", err)
		}
		_ = resp.Body.Close()
	}
	if c.State() != "closed" {
		t.Fatalf("4xx não deve abrir o circuito; estado = %s", c.State())
	}
}

func TestDo_CircuitoAbre(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(Config{Name: "billing", MaxRetries: 1, FailureThreshold: 2, OpenTimeout: time.Hour}, nil)
	c.sleep = noSleep
	_, _ = c.Do(context.Background(), get(srv.URL)) // duas falhas: abre
	if c.State() != "open" {
		t.Fatalf("esperava circuito aberto, veio %s", c.State())
	}
	before := calls.Load()
	if _, err := c.Do(context.Background(), get(srv.URL)); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("esperava ErrCircuitOpen, veio %v", err)
	}
	if calls.Load() != before {
		t.Fatal("com o circuito aberto o servidor não pode ser chamado")
	}
}

func TestDo_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer srv.Close()

	c := New(Config{Name: "slow", Timeout: 20 * time.Millisecond, MaxRetries: 1, FailureThreshold: 100}, nil)
	c.sleep = noSleep
	start := time.Now()
	if _, err := c.Do(context.Background(), get(srv.URL)); err == nil {
		t.Fatal("esperava timeout")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("timeout por tentativa não foi respeitado")
	}
}

func TestDo_ErroAoMontarRequisicao(t *testing.T) {
	c := New(Config{Name: "x"}, nil)
	_, err := c.Do(context.Background(), func(context.Context) (*http.Request, error) { return nil, errors.New("url ruim") })
	if err == nil {
		t.Fatal("esperava erro")
	}
}

func TestDo_ContextoCanceladoNoBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(Config{Name: "x", MaxRetries: 2, FailureThreshold: 100, BaseBackoff: time.Hour}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	if _, err := c.Do(ctx, get(srv.URL)); !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava context.Canceled, veio %v", err)
	}
}
