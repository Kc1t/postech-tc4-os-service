package ports

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
)

//go:generate mockgen -source=./transactions.go -destination=./mocks/transactions.go -package=mocks

// TxManager executa fn numa transacao de banco. Repositorios, inbox e mensageiro chamados com
// o ctx recebido por fn participam da mesma transacao.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// MessageInbox garante que cada mensagem consumida seja aplicada uma unica vez (consumidor
// idempotente). fn roda dentro de uma transacao junto com o registro da mensagem.
type MessageInbox interface {
	Once(ctx context.Context, env contracts.Envelope, fn func(ctx context.Context) error) (duplicate bool, err error)
}

// SagaMessenger envia comandos e publica eventos de forma confiavel: a mensagem e gravada no
// outbox da transacao corrente e publicada depois. O destino e resolvido pelo tipo da mensagem.
type SagaMessenger interface {
	Send(ctx context.Context, msgType string, meta contracts.Meta, payload any) error
}
