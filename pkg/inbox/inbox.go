// Package inbox garante processamento idempotente de mensagens (padrão Idempotent Consumer).
//
// A entrega das filas é "pelo menos uma vez": a mesma mensagem pode chegar duas vezes (reentrega
// após timeout de visibilidade, relay que publicou e caiu antes de marcar). Once registra o
// message_id na mesma transação dos efeitos do handler; se o registro já existe, os efeitos não
// são aplicados de novo.
package inbox

import (
	"context"
	"fmt"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ProcessedMessage é a linha da tabela processed_messages.
type ProcessedMessage struct {
	MessageID   string    `gorm:"primaryKey"`
	Consumer    string    `gorm:"primaryKey"`
	Type        string    `gorm:"not null"`
	ProcessedAt time.Time `gorm:"not null;index"`
}

func (ProcessedMessage) TableName() string { return "processed_messages" }

// Migrate cria/atualiza a tabela do inbox.
func Migrate(db *gorm.DB) error { return db.AutoMigrate(&ProcessedMessage{}) }

// Purge apaga os registros processados antes de before. Quem chama é a limpeza do relay do
// outbox; a retenção precisa passar da retenção das DLQs, senão um redrive reaplicaria efeitos.
func Purge(ctx context.Context, db *gorm.DB, before time.Time) (int64, error) {
	res := db.WithContext(ctx).Where("processed_at < ?", before).Delete(&ProcessedMessage{})
	if res.Error != nil {
		return 0, fmt.Errorf("inbox: limpar processadas: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// Once executa fn dentro de uma transação, uma única vez por (consumer, message_id).
// Devolve duplicate=true quando a mensagem já tinha sido processada e fn não rodou.
func Once(ctx context.Context, db *gorm.DB, consumer string, env contracts.Envelope, fn func(tx *gorm.DB) error) (duplicate bool, err error) {
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ProcessedMessage{
			MessageID:   env.MessageID,
			Consumer:    consumer,
			Type:        env.Type,
			ProcessedAt: time.Now().UTC(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			duplicate = true
			return nil
		}
		return fn(tx)
	})
	return duplicate, err
}
