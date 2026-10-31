// Package scheduler roda tarefas periódicas do os-service.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/Kc1t/postech-tc4-os-service/pkg/logger"
	"github.com/newrelic/go-agent/v3/newrelic"
)

// SagaTimeouts verifica periodicamente as sagas com prazo vencido. Com várias réplicas, cada
// uma roda o próprio ticker; o FOR UPDATE e a revalidação do prazo evitam compensar duas vezes.
type SagaTimeouts struct {
	uc       ports.ExpireSagasUseCase
	interval time.Duration
	nr       *newrelic.Application
	log      *slog.Logger
	now      func() time.Time
}

func NewSagaTimeouts(uc ports.ExpireSagasUseCase, interval time.Duration, nr *newrelic.Application, log *slog.Logger) *SagaTimeouts {
	if interval <= 0 {
		interval = time.Minute
	}
	if log == nil {
		log = slog.Default()
	}
	return &SagaTimeouts{uc: uc, interval: interval, nr: nr, log: log, now: time.Now}
}

// Run executa até ctx ser cancelado.
func (s *SagaTimeouts) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick(ctx)
		}
	}
}

// Tick faz uma verificação. Cada verificação é uma transação de background no New Relic.
func (s *SagaTimeouts) Tick(ctx context.Context) {
	if s.nr != nil {
		txn := s.nr.StartTransaction("saga-timeouts")
		defer txn.End()
		ctx = newrelic.NewContext(ctx, txn)
	}
	ctx = logger.ContextWith(ctx, s.log)
	n, err := s.uc.Execute(ctx, s.now().UTC())
	if err != nil {
		s.log.Error("saga_timeouts_failed", "error", err)
		return
	}
	if n > 0 {
		s.log.Info("saga_timeouts_compensated", "count", n)
	}
}
