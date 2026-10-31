// Package outbox implementa o padrão Transactional Outbox sobre PostgreSQL (GORM).
//
// O caso de uso grava a mudança de estado e a mensagem na mesma transação (Enqueue); o Relay
// publica depois. Assim não existe estado salvo sem mensagem nem mensagem sem estado salvo, e a
// entrega fica "pelo menos uma vez" — por isso os consumidores são idempotentes (pkg/inbox).
//
// O contexto de distributed tracing é capturado no Enqueue e reaceito pelo Relay, para que o
// trace da requisição HTTP continue na publicação e, dali, no consumidor do outro serviço.
//
// Uma mensagem que falha volta para o relay com espera exponencial (next_attempt_at), para não
// ocupar o lote e travar as outras; a partir de StuckAfter tentativas, cada falha é logada como
// erro (outbox_message_stuck), que é o sinal de alerta. O relay também faz a limpeza: apaga o
// outbox já publicado e o inbox antigo (pkg/inbox), que senão cresceriam para sempre.
package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/inbox"
	"github.com/Kc1t/postech-tc4-os-service/pkg/messaging"
	"github.com/newrelic/go-agent/v3/newrelic"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Message é a linha da tabela outbox_messages.
type Message struct {
	ID              string            `gorm:"primaryKey;type:uuid"`
	DestinationKind string            `gorm:"not null"`
	Destination     string            `gorm:"not null"`
	Type            string            `gorm:"not null;index"`
	SagaID          string            `gorm:"index"`
	Body            string            `gorm:"type:text;not null"`
	TraceHeaders    map[string]string `gorm:"serializer:json"`
	CreatedAt       time.Time         `gorm:"not null;index"`
	PublishedAt     *time.Time        `gorm:"index"`
	NextAttemptAt   *time.Time        `gorm:"index"`
	Attempts        int               `gorm:"not null;default:0"`
	LastError       string
}

const (
	// StuckAfter é o número de tentativas a partir do qual cada falha vira log de erro. Com a
	// espera exponencial, são uns 8 minutos falhando sem parar.
	StuckAfter = 10
	// MaxBackoff limita a espera entre tentativas.
	MaxBackoff = 5 * time.Minute

	// DefaultOutboxRetention é por quanto tempo uma mensagem publicada fica no outbox, para
	// auditoria e investigação de incidente.
	DefaultOutboxRetention = 7 * 24 * time.Hour
	// DefaultInboxRetention precisa ser maior que a retenção das DLQs (14 dias): um redrive
	// reentrega mensagens antigas, e é o registro no inbox que impede o efeito em dobro.
	DefaultInboxRetention = 21 * 24 * time.Hour
	// DefaultCleanupEvery é o intervalo entre limpezas.
	DefaultCleanupEvery = time.Hour
)

// Backoff devolve a espera antes da próxima tentativa, depois de attempts falhas:
// 1 s, 2 s, 4 s... até MaxBackoff.
func Backoff(attempts int) time.Duration {
	if attempts < 1 {
		return 0
	}
	if attempts > 9 { // 2^9 s já passa de MaxBackoff; evita overflow no shift
		return MaxBackoff
	}
	return min(time.Duration(1<<(attempts-1))*time.Second, MaxBackoff)
}

func (Message) TableName() string { return "outbox_messages" }

// Migrate cria/atualiza a tabela do outbox.
func Migrate(db *gorm.DB) error { return db.AutoMigrate(&Message{}) }

// Enqueue grava o envelope no outbox usando a transação tx do caso de uso.
func Enqueue(ctx context.Context, tx *gorm.DB, dest messaging.Destination, env contracts.Envelope) error {
	body, err := env.Marshal()
	if err != nil {
		return err
	}
	headers := map[string]string{}
	if txn := newrelic.FromContext(ctx); txn != nil {
		h := http.Header{}
		txn.InsertDistributedTraceHeaders(h)
		for k := range h {
			headers[k] = h.Get(k)
		}
	}
	msg := Message{
		ID:              env.MessageID,
		DestinationKind: string(dest.Kind),
		Destination:     dest.Name,
		Type:            env.Type,
		SagaID:          env.SagaID,
		Body:            string(body),
		TraceHeaders:    headers,
		CreatedAt:       time.Now().UTC(),
	}
	if err := tx.WithContext(ctx).Create(&msg).Error; err != nil {
		return fmt.Errorf("outbox: gravar %s: %w", env.Type, err)
	}
	return nil
}

// Relay publica as mensagens pendentes do outbox.
type Relay struct {
	db        *gorm.DB
	pub       messaging.Publisher
	nr        *newrelic.Application
	log       *slog.Logger
	interval  time.Duration
	batchSize int
	now       func() time.Time

	outboxRetention time.Duration
	inboxRetention  time.Duration
	cleanupEvery    time.Duration
	lastCleanup     time.Time
}

func NewRelay(db *gorm.DB, pub messaging.Publisher, nr *newrelic.Application, log *slog.Logger) *Relay {
	if log == nil {
		log = slog.Default()
	}
	return &Relay{
		db: db, pub: pub, nr: nr, log: log.With("component", "outbox_relay"),
		interval: 500 * time.Millisecond, batchSize: 50, now: func() time.Time { return time.Now().UTC() },
		outboxRetention: DefaultOutboxRetention, inboxRetention: DefaultInboxRetention, cleanupEvery: DefaultCleanupEvery,
	}
}

// WithInterval ajusta o intervalo entre varreduras.
func (r *Relay) WithInterval(d time.Duration) *Relay { r.interval = d; return r }

// WithCleanup ajusta as retenções e o intervalo da limpeza; every <= 0 desliga a limpeza.
func (r *Relay) WithCleanup(outboxRetention, inboxRetention, every time.Duration) *Relay {
	r.outboxRetention, r.inboxRetention, r.cleanupEvery = outboxRetention, inboxRetention, every
	return r
}

// Run varre o outbox até ctx ser cancelado.
func (r *Relay) Run(ctx context.Context) {
	r.log.Info("outbox_relay_started", "interval_ms", r.interval.Milliseconds())
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			r.log.Info("outbox_relay_stopped")
			return
		case <-t.C:
			if _, err := r.Flush(ctx); err != nil && ctx.Err() == nil {
				r.log.Error("outbox_flush_failed", "error", err)
			}
			r.maybeCleanup(ctx)
		}
	}
}

// Flush publica um lote de mensagens pendentes cuja espera já passou. As linhas ficam travadas
// com FOR UPDATE SKIP LOCKED, então várias réplicas do serviço podem rodar o relay em paralelo
// sem publicar a mesma linha duas vezes. Devolve quantas mensagens foram publicadas.
func (r *Relay) Flush(ctx context.Context) (int, error) {
	published := 0
	now := r.now()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch []Message
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("published_at IS NULL AND (next_attempt_at IS NULL OR next_attempt_at <= ?)", now).
			Order("created_at").
			Limit(r.batchSize).
			Find(&batch).Error; err != nil {
			return err
		}
		for i := range batch {
			m := &batch[i]
			if err := r.publish(ctx, m); err != nil {
				r.recordFailure(m, err, now)
			} else {
				published++
				m.PublishedAt = &now
				m.NextAttemptAt = nil
				m.LastError = ""
			}
			if err := tx.Model(m).Select("published_at", "next_attempt_at", "attempts", "last_error").Updates(m).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return published, err
}

func (r *Relay) recordFailure(m *Message, err error, now time.Time) {
	m.Attempts++
	m.LastError = err.Error()
	next := now.Add(Backoff(m.Attempts))
	m.NextAttemptAt = &next
	attrs := []any{"message_id", m.ID, "message_type", m.Type, "saga_id", m.SagaID, "destination", m.Destination,
		"attempts", m.Attempts, "next_attempt_at", next, "error", err}
	if m.Attempts >= StuckAfter {
		r.log.Error("outbox_message_stuck", attrs...)
		return
	}
	r.log.Warn("outbox_publish_failed", attrs...)
}

func (r *Relay) maybeCleanup(ctx context.Context) {
	if r.cleanupEvery <= 0 || r.now().Sub(r.lastCleanup) < r.cleanupEvery {
		return
	}
	r.lastCleanup = r.now()
	outboxDeleted, inboxDeleted, err := r.Cleanup(ctx)
	if err != nil {
		if ctx.Err() == nil {
			r.log.Warn("outbox_cleanup_failed", "error", err)
		}
		return
	}
	if outboxDeleted > 0 || inboxDeleted > 0 {
		r.log.Info("outbox_cleanup", "outbox_deleted", outboxDeleted, "inbox_deleted", inboxDeleted)
	}
}

// Cleanup apaga as mensagens publicadas há mais que a retenção do outbox e os registros do inbox
// mais antigos que a retenção dele. Várias réplicas rodando ao mesmo tempo não fazem mal: o
// DELETE é idempotente.
func (r *Relay) Cleanup(ctx context.Context) (outboxDeleted, inboxDeleted int64, err error) {
	now := r.now()
	res := r.db.WithContext(ctx).Where("published_at < ?", now.Add(-r.outboxRetention)).Delete(&Message{})
	if res.Error != nil {
		return 0, 0, fmt.Errorf("outbox: limpar publicadas: %w", res.Error)
	}
	inboxDeleted, err = inbox.Purge(ctx, r.db, now.Add(-r.inboxRetention))
	return res.RowsAffected, inboxDeleted, err
}

func (r *Relay) publish(ctx context.Context, m *Message) (err error) {
	env, err := contracts.Parse([]byte(m.Body))
	if err != nil {
		return err
	}
	if r.nr != nil {
		txn := r.nr.StartTransaction("OutboxRelay/" + m.Type)
		defer func() {
			if err != nil {
				txn.NoticeError(err)
			}
			txn.End()
		}()
		h := http.Header{}
		for k, v := range m.TraceHeaders {
			h.Set(k, v)
		}
		txn.AcceptDistributedTraceHeaders(newrelic.TransportOther, h)
		txn.AddAttribute("message_id", env.MessageID)
		txn.AddAttribute("saga_id", env.SagaID)
		txn.AddAttribute("os_id", env.OSID)
		ctx = newrelic.NewContext(ctx, txn)
	}
	return r.pub.Publish(ctx, messaging.Destination{Kind: messaging.Kind(m.DestinationKind), Name: m.Destination}, env)
}

// Pending conta as mensagens ainda não publicadas (usado em health e testes).
func Pending(ctx context.Context, db *gorm.DB) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&Message{}).Where("published_at IS NULL").Count(&n).Error
	return n, err
}
