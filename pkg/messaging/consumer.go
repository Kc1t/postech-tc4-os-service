package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/logger"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/newrelic/go-agent/v3/newrelic"
)

// SQSReceiver é o subconjunto do cliente SQS usado pelo consumidor.
type SQSReceiver interface {
	ReceiveMessage(ctx context.Context, in *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, in *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

// Consumer faz long polling numa fila SQS e despacha cada envelope ao Handler do seu tipo.
//
// Cada mensagem roda numa transação de background do New Relic que aceita o contexto de trace
// vindo nos atributos — é isso que liga o span do publicador ao do consumidor no trace
// distribuído. Mensagens sem handler são registradas e removidas; mensagens com falha ficam na
// fila para nova tentativa e, esgotadas as tentativas, a redrive policy as move para a DLQ.
type Consumer struct {
	client      SQSReceiver
	queueURL    string
	name        string
	nr          *newrelic.Application
	log         *slog.Logger
	handlers    map[string]Handler
	waitSeconds int32
	maxMessages int32
	errBackoff  time.Duration
}

// NewConsumer cria um consumidor. name identifica o consumidor em logs e nas transações.
func NewConsumer(client SQSReceiver, queueURL, name string, nr *newrelic.Application, log *slog.Logger) *Consumer {
	if log == nil {
		log = slog.Default()
	}
	return &Consumer{
		client:      client,
		queueURL:    queueURL,
		name:        name,
		nr:          nr,
		log:         log.With("consumer", name),
		handlers:    map[string]Handler{},
		waitSeconds: 20,
		maxMessages: 10,
		errBackoff:  2 * time.Second,
	}
}

// Handle registra o handler de um tipo de mensagem.
func (c *Consumer) Handle(msgType string, h Handler) { c.handlers[msgType] = h }

// Run consome até ctx ser cancelado.
func (c *Consumer) Run(ctx context.Context) {
	c.log.Info("consumer_started", "queue", Destination{Kind: Queue, Name: c.queueURL}.ShortName())
	for {
		if ctx.Err() != nil {
			c.log.Info("consumer_stopped")
			return
		}
		if _, err := c.Poll(ctx); err != nil && !errors.Is(err, context.Canceled) {
			c.log.Error("consumer_receive_failed", "error", err)
			select {
			case <-ctx.Done():
			case <-time.After(c.errBackoff):
			}
		}
	}
}

// Poll faz uma chamada de ReceiveMessage e processa o lote. Devolve quantas mensagens foram
// processadas com sucesso.
func (c *Consumer) Poll(ctx context.Context) (int, error) {
	out, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:                    aws.String(c.queueURL),
		MaxNumberOfMessages:         c.maxMessages,
		WaitTimeSeconds:             c.waitSeconds,
		MessageAttributeNames:       []string{"All"},
		MessageSystemAttributeNames: []sqstypes.MessageSystemAttributeName{sqstypes.MessageSystemAttributeNameApproximateReceiveCount},
	})
	if err != nil {
		return 0, err
	}
	ok := 0
	for _, m := range out.Messages {
		if c.process(ctx, m) {
			ok++
		}
	}
	return ok, nil
}

func (c *Consumer) process(ctx context.Context, m sqstypes.Message) bool {
	body, attrs := unwrap(aws.ToString(m.Body), m.MessageAttributes)

	env, err := contracts.Parse([]byte(body))
	if err != nil {
		// Mensagem malformada: não remove; depois do maxReceiveCount ela vai para a DLQ,
		// onde fica disponível para análise.
		c.log.Error("message_malformed", "error", err, "sqs_message_id", aws.ToString(m.MessageId))
		return false
	}

	var txn *newrelic.Transaction
	if c.nr != nil {
		txn = c.nr.StartTransaction("Consume/" + env.Type)
		defer txn.End()
		txn.AcceptDistributedTraceHeaders(newrelic.TransportQueue, TraceHeaders(attrs))
		txn.AddAttribute("messaging.consumer", c.name)
		txn.AddAttribute("message_type", env.Type)
		txn.AddAttribute("message_id", env.MessageID)
		txn.AddAttribute("saga_id", env.SagaID)
		txn.AddAttribute("os_id", env.OSID)
		txn.AddAttribute("receive_count", m.Attributes[string(sqstypes.MessageSystemAttributeNameApproximateReceiveCount)])
		ctx = newrelic.NewContext(ctx, txn)
	}

	log := logger.WithTrace(ctx, c.log).With(
		"message_type", env.Type,
		"message_id", env.MessageID,
		"saga_id", env.SagaID,
		"os_id", env.OSID,
		"correlation_id", env.CorrelationID,
	)
	ctx = logger.ContextWith(ctx, log)

	h, found := c.handlers[env.Type]
	if !found {
		log.Warn("message_ignored", "reason", "sem handler para o tipo")
		c.delete(ctx, m)
		return true
	}

	start := time.Now()
	if err := h(ctx, env); err != nil {
		if txn != nil {
			txn.NoticeError(err)
		}
		log.Error("message_failed", "error", err,
			"receive_count", m.Attributes[string(sqstypes.MessageSystemAttributeNameApproximateReceiveCount)])
		return false
	}
	log.Info("message_processed", "duration_ms", time.Since(start).Milliseconds())
	c.delete(ctx, m)
	return true
}

func (c *Consumer) delete(ctx context.Context, m sqstypes.Message) {
	if _, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: m.ReceiptHandle,
	}); err != nil {
		c.log.Error("message_delete_failed", "error", err, "sqs_message_id", aws.ToString(m.MessageId))
	}
}

// snsNotification é o invólucro que o SNS usa quando a assinatura não tem raw delivery.
type snsNotification struct {
	Type              string `json:"Type"`
	Message           string `json:"Message"`
	MessageAttributes map[string]struct {
		Type  string `json:"Type"`
		Value string `json:"Value"`
	} `json:"MessageAttributes"`
}

// unwrap devolve o corpo do envelope e os atributos, aceitando tanto raw delivery quanto o
// invólucro de notificação do SNS.
func unwrap(body string, msgAttrs map[string]sqstypes.MessageAttributeValue) (string, map[string]string) {
	attrs := make(map[string]string, len(msgAttrs))
	for k, v := range msgAttrs {
		attrs[k] = aws.ToString(v.StringValue)
	}

	var n snsNotification
	if err := json.Unmarshal([]byte(body), &n); err == nil && n.Type == "Notification" && n.Message != "" {
		for k, v := range n.MessageAttributes {
			if _, exists := attrs[k]; !exists {
				attrs[k] = v.Value
			}
		}
		return n.Message, attrs
	}
	return body, attrs
}
