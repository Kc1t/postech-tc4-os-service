package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/cmd/api/bootstrap"
	"github.com/Kc1t/postech-tc4-os-service/cmd/api/middleware"
	"github.com/Kc1t/postech-tc4-os-service/cmd/api/routes"
	"github.com/Kc1t/postech-tc4-os-service/config"
	"github.com/gin-gonic/gin"
	"github.com/newrelic/go-agent/v3/integrations/nrgin"
)

// @title           OS Service — Oficina Mecânica
// @version         2.0
// @description     Ordens de serviço, clientes, veículos, catálogo e estoque. Orquestra a saga da OS
// @description     (execution-service e billing-service) via SNS/SQS com outbox e consumidor idempotente.
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
func main() {
	c := bootstrap.GetContainer()
	log := c.Logger

	if c.Config.AppEnv == config.EnvProduction {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	if c.NR != nil {
		router.Use(nrgin.Middleware(c.NR))
		router.Use(middleware.NewRelicContext())
	}
	router.Use(middleware.CorrelationID())
	router.Use(middleware.RequestLogger(log, routes.HealthPaths...))
	routes.Setup(router, c)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Workers: relay do outbox, consumidor das respostas da saga e verificação de prazos.
	workersCtx, stopWorkers := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	start := func(name string, run func(context.Context)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			log.Info("worker_started", "worker", name)
			run(workersCtx)
			log.Info("worker_stopped", "worker", name)
		}()
	}
	if c.OutboxRelay != nil {
		start("outbox-relay", c.OutboxRelay.Run)
		start("saga-replies-consumer", c.SagaConsumer.Run)
		start("saga-timeouts", c.SagaTimeouts.Run)
	}

	srv := &http.Server{
		Addr:              ":" + c.Config.AppPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info("server listening", "addr", srv.Addr, "env", string(c.Config.AppEnv),
			"messaging", c.Config.MessagingEnabled(), "fault_injection", c.Config.FaultInjectionEnabled)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutdown_started")
	// Ordem: para de aceitar requisições, termina as em andamento, para consumidor, relay e
	// verificação de prazos e só então fecha o agente do New Relic. O que ficar pendente no
	// outbox é publicado por outra réplica ou na próxima subida: nada se perde.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), c.Config.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http_shutdown_failed", "error", err)
	}
	stopWorkers()
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		log.Warn("workers_shutdown_timeout")
	}
	if c.NR != nil {
		c.NR.Shutdown(5 * time.Second)
	}
	log.Info("shutdown_completed")
}
