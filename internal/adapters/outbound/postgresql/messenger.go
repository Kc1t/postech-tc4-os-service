package postgresql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/messaging"
	"github.com/Kc1t/postech-tc4-os-service/pkg/outbox"
)

// Producer é o nome do os-service no campo producer dos envelopes.
const Producer = "os-service"

// Routes são os destinos das mensagens que o os-service produz.
type Routes struct {
	BillingCommandsQueueURL   string
	ExecutionCommandsQueueURL string
	OSEventsTopicARN          string
}

// ErrNoTransaction indica uma tentativa de enviar mensagem fora de transação: sem ela o outbox
// não garante que mensagem e estado sejam gravados juntos.
var ErrNoTransaction = errors.New("messenger: envio fora de transação")

type outboxMessenger struct{ routes Routes }

// NewOutboxMessenger implementa ports.SagaMessenger gravando no outbox da transação corrente.
// O destino sai do prefixo do tipo: billing.* → fila billing-commands, execution.* → fila
// execution-commands, os.* → tópico os-events.
func NewOutboxMessenger(r Routes) ports.SagaMessenger { return &outboxMessenger{routes: r} }

func (m *outboxMessenger) Send(ctx context.Context, msgType string, meta contracts.Meta, payload any) error {
	tx, ok := TxFrom(ctx)
	if !ok {
		return ErrNoTransaction
	}
	dest, err := m.route(msgType)
	if err != nil {
		return err
	}
	env, err := contracts.NewEnvelope(msgType, Producer, meta, payload)
	if err != nil {
		return err
	}
	return outbox.Enqueue(ctx, tx, dest, env)
}

func (m *outboxMessenger) route(msgType string) (messaging.Destination, error) {
	switch {
	case strings.HasPrefix(msgType, "billing."):
		return messaging.Destination{Kind: messaging.Queue, Name: m.routes.BillingCommandsQueueURL}, nil
	case strings.HasPrefix(msgType, "execution."):
		return messaging.Destination{Kind: messaging.Queue, Name: m.routes.ExecutionCommandsQueueURL}, nil
	case strings.HasPrefix(msgType, "os."):
		return messaging.Destination{Kind: messaging.Topic, Name: m.routes.OSEventsTopicARN}, nil
	}
	return messaging.Destination{}, fmt.Errorf("messenger: sem rota para %q", msgType)
}
