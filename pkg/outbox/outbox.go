// Package outbox implementa o padrão Transactional Outbox sobre PostgreSQL (GORM).
//
// O caso de uso grava a mudança de estado e a mensagem na mesma transação (Enqueue); o Relay
// publica depois. Assim não existe estado salvo sem mensagem nem mensagem sem estado salvo, e a
// entrega fica "pelo menos uma vez" — por isso os consumidores são idempotentes (pkg/inbox).
//
// O contexto de distributed tracing é capturado no Enqueue e reaceito pelo Relay, para que o
// trace da requisição HTTP continue na publicação e, dali, no consumidor do outro serviço.
package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
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
	Attempts        int               `gorm:"not null;default:0"`
	LastError       string
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
}

func NewRelay(db *gorm.DB, pub messaging.Publisher, nr *newrelic.Application, log *slog.Logger) *Relay {
	if log == nil {
		log = slog.Default()
	}
	return &Relay{db: db, pub: pub, nr: nr, log: log.With("component", "outbox_relay"), interval: 500 * time.Millisecond, batchSize: 50}
}

// WithInterval ajusta o intervalo entre varreduras.
func (r *Relay) WithInterval(d time.Duration) *Relay { r.interval = d; return r }

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
		}
	}
}

// Flush publica um lote de mensagens pendentes. As linhas ficam travadas com
// FOR UPDATE SKIP LOCKED, então várias réplicas do serviço podem rodar o relay em paralelo sem
// publicar a mesma linha duas vezes. Devolve quantas mensagens foram publicadas.
func (r *Relay) Flush(ctx context.Context) (int, error) {
	published := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch []Message
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("published_at IS NULL").
			Order("created_at").
			Limit(r.batchSize).
			Find(&batch).Error; err != nil {
			return err
		}
		for i := range batch {
			m := &batch[i]
			if err := r.publish(ctx, m); err != nil {
				m.Attempts++
				m.LastError = err.Error()
				r.log.Warn("outbox_publish_failed", "message_id", m.ID, "message_type", m.Type, "attempts", m.Attempts, "error", err)
			} else {
				now := time.Now().UTC()
				m.PublishedAt = &now
				m.LastError = ""
				published++
			}
			if err := tx.Model(m).Select("published_at", "attempts", "last_error").Updates(m).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return published, err
}

func (r *Relay) publish(ctx context.Context, m *Message) error {
	env, err := contracts.Parse([]byte(m.Body))
	if err != nil {
		return err
	}
	if r.nr != nil {
		txn := r.nr.StartTransaction("OutboxRelay/" + m.Type)
		defer txn.End()
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
