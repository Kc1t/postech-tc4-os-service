package postgresql

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/inbox"
	"gorm.io/gorm"
)

type txKey struct{}

// WithTx devolve um ctx que carrega a transacao tx. Repositorios chamados com esse ctx usam tx.
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFrom devolve a transacao corrente do ctx, se houver.
func TxFrom(ctx context.Context) (*gorm.DB, bool) {
	tx, ok := ctx.Value(txKey{}).(*gorm.DB)
	return tx, ok
}

// conn escolhe a transacao do ctx ou, fora de transacao, a conexao do repositorio.
func conn(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := TxFrom(ctx); ok {
		return tx.WithContext(ctx)
	}
	return db.WithContext(ctx)
}

// TxManager implementa ports.TxManager sobre GORM.
type TxManager struct{ db *gorm.DB }

func NewTxManager(db *gorm.DB) ports.TxManager { return &TxManager{db: db} }

// WithinTx abre uma transacao, ou reaproveita a do ctx (transacoes aninhadas viram uma so).
func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := TxFrom(ctx); ok {
		return fn(ctx)
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(WithTx(ctx, tx))
	})
}

// Inbox implementa ports.MessageInbox com a tabela processed_messages.
type Inbox struct {
	db       *gorm.DB
	consumer string
}

func NewInbox(db *gorm.DB, consumer string) ports.MessageInbox {
	return &Inbox{db: db, consumer: consumer}
}

func (i *Inbox) Once(ctx context.Context, env contracts.Envelope, fn func(ctx context.Context) error) (bool, error) {
	return inbox.Once(ctx, i.db, i.consumer, env, func(tx *gorm.DB) error {
		return fn(WithTx(ctx, tx))
	})
}
