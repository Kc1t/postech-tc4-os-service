// Package messaging publica e consome os envelopes de pkg/contracts sobre AWS SNS e SQS.
//
// Eventos vão para tópicos SNS (pub/sub); comandos vão direto para a fila SQS do destinatário
// (ponto a ponto). Os campos de roteamento e o contexto de distributed tracing do New Relic
// viajam como MessageAttributes, o que exige raw message delivery nas assinaturas SNS → SQS.
package messaging

import (
	"context"
	"net/http"
	"strings"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
)

// Kind diz se o destino é um tópico SNS ou uma fila SQS.
type Kind string

const (
	Topic Kind = "topic"
	Queue Kind = "queue"
)

// Destination é um tópico (Name = ARN) ou uma fila (Name = URL).
type Destination struct {
	Kind Kind
	Name string
}

// ShortName devolve o nome legível do destino, usado em métricas e logs.
func (d Destination) ShortName() string {
	name := d.Name
	if i := strings.LastIndexAny(name, ":/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// Publisher envia um envelope a um destino. Implementações devem propagar o contexto de trace
// da transação New Relic presente em ctx, quando houver.
type Publisher interface {
	Publish(ctx context.Context, dest Destination, env contracts.Envelope) error
}

// Handler processa um envelope consumido. Retornar erro mantém a mensagem na fila para nova
// tentativa; depois do maxReceiveCount da redrive policy ela vai para a DLQ.
type Handler func(ctx context.Context, env contracts.Envelope) error

// Nomes dos MessageAttributes de roteamento. Somados aos até 3 headers de trace
// (traceparent, tracestate, newrelic), ficam abaixo do limite de 10 atributos do SQS.
const (
	AttrMessageType = "message_type"
	AttrMessageID   = "message_id"
	AttrSagaID      = "saga_id"
	AttrOSID        = "os_id"
)

// traceHeaderNames são os headers que o agente New Relic grava e lê no distributed tracing.
var traceHeaderNames = []string{"traceparent", "tracestate", "newrelic"}

func routingAttributes(env contracts.Envelope) map[string]string {
	attrs := map[string]string{
		AttrMessageType: env.Type,
		AttrMessageID:   env.MessageID,
	}
	if env.SagaID != "" {
		attrs[AttrSagaID] = env.SagaID
	}
	if env.OSID != "" {
		attrs[AttrOSID] = env.OSID
	}
	return attrs
}

// traceAttributes extrai os headers de trace de um http.Header preenchido pelo agente.
func traceAttributes(h http.Header) map[string]string {
	out := map[string]string{}
	for _, name := range traceHeaderNames {
		if v := h.Get(name); v != "" {
			out[name] = v
		}
	}
	return out
}

// TraceHeaders monta um http.Header com os headers de trace encontrados nos atributos,
// pronto para AcceptDistributedTraceHeaders.
func TraceHeaders(attrs map[string]string) http.Header {
	h := http.Header{}
	for _, name := range traceHeaderNames {
		if v, ok := attrs[name]; ok && v != "" {
			h.Set(name, v)
		}
	}
	return h
}
