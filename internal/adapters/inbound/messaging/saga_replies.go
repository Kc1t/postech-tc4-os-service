// Package messaging liga a fila os-saga-replies ao orquestrador da saga.
package messaging

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	pkgmessaging "github.com/Kc1t/postech-tc4-os-service/pkg/messaging"
)

// Registrar é o pedaço do messaging.Consumer usado aqui (facilita o teste).
type Registrar interface {
	Handle(msgType string, h pkgmessaging.Handler)
}

// RegisterSagaReplies registra o orquestrador para todos os tipos de resposta que ele trata.
func RegisterSagaReplies(r Registrar, uc ports.HandleSagaReplyUseCase) {
	for _, t := range uc.Types() {
		r.Handle(t, func(ctx context.Context, env contracts.Envelope) error {
			return uc.Execute(ctx, env)
		})
	}
}
