package messaging

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/newrelic/go-agent/v3/newrelic"
)

// ---- fakes ----

type fakeSNS struct {
	in  *sns.PublishInput
	err error
}

func (f *fakeSNS) Publish(_ context.Context, in *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	f.in = in
	return &sns.PublishOutput{}, f.err
}

type fakeSQS struct {
	mu       sync.Mutex
	sent     *sqs.SendMessageInput
	sendErr  error
	batches  [][]sqstypes.Message
	recvErr  error
	deleted  []string
	received int
}

func (f *fakeSQS) SendMessage(_ context.Context, in *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.sent = in
	return &sqs.SendMessageOutput{}, f.sendErr
}

func (f *fakeSQS) ReceiveMessage(_ context.Context, _ *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recvErr != nil {
		return nil, f.recvErr
	}
	f.received++
	if len(f.batches) == 0 {
		return &sqs.ReceiveMessageOutput{}, nil
	}
	b := f.batches[0]
	f.batches = f.batches[1:]
	return &sqs.ReceiveMessageOutput{Messages: b}, nil
}

func (f *fakeSQS) DeleteMessage(_ context.Context, in *sqs.DeleteMessageInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, aws.ToString(in.ReceiptHandle))
	return &sqs.DeleteMessageOutput{}, nil
}

func disabledNR(t *testing.T) *newrelic.Application {
	t.Helper()
	app, err := newrelic.NewApplication(newrelic.ConfigAppName("test"), newrelic.ConfigEnabled(false))
	if err != nil {
		t.Fatalf("newrelic: %v", err)
	}
	return app
}

func envelope(t *testing.T, msgType string) contracts.Envelope {
	t.Helper()
	env, err := contracts.NewEnvelope(msgType, "test", contracts.Meta{SagaID: "saga-1", OSID: "os-1"}, map[string]int{"n": 1})
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func body(t *testing.T, env contracts.Envelope) *string {
	t.Helper()
	b, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return aws.String(string(b))
}

// ---- publisher ----

func TestAWSPublisher_Topic(t *testing.T) {
	s := &fakeSNS{}
	p := NewAWSPublisher(s, &fakeSQS{})
	env := envelope(t, contracts.EvtBudgetCreated)

	txn := disabledNR(t).StartTransaction("pub")
	ctx := newrelic.NewContext(context.Background(), txn)
	if err := p.Publish(ctx, Destination{Kind: Topic, Name: "arn:aws:sns:us-east-1:1:billing-events"}, env); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if aws.ToString(s.in.TopicArn) != "arn:aws:sns:us-east-1:1:billing-events" {
		t.Fatalf("tópico errado: %v", aws.ToString(s.in.TopicArn))
	}
	got := s.in.MessageAttributes
	if aws.ToString(got[AttrMessageType].StringValue) != contracts.EvtBudgetCreated ||
		aws.ToString(got[AttrSagaID].StringValue) != "saga-1" ||
		aws.ToString(got[AttrOSID].StringValue) != "os-1" ||
		aws.ToString(got[AttrMessageID].StringValue) != env.MessageID {
		t.Fatalf("atributos de roteamento errados: %+v", got)
	}
	if len(got) > 10 {
		t.Fatalf("SNS/SQS aceitam no máximo 10 atributos, veio %d", len(got))
	}
}

func TestAWSPublisher_Queue(t *testing.T) {
	q := &fakeSQS{}
	p := NewAWSPublisher(&fakeSNS{}, q)
	env := envelope(t, contracts.CmdCreateBudget)
	if err := p.Publish(context.Background(), Destination{Kind: Queue, Name: "http://localstack:4566/000/billing-commands"}, env); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if aws.ToString(q.sent.QueueUrl) != "http://localstack:4566/000/billing-commands" {
		t.Fatal("fila errada")
	}
	parsed, err := contracts.Parse([]byte(aws.ToString(q.sent.MessageBody)))
	if err != nil || parsed.MessageID != env.MessageID {
		t.Fatalf("corpo não é o envelope: %v", err)
	}
}

func TestAWSPublisher_Erros(t *testing.T) {
	boom := errors.New("boom")
	p := NewAWSPublisher(&fakeSNS{err: boom}, &fakeSQS{sendErr: boom})
	env := envelope(t, "x")
	if err := p.Publish(context.Background(), Destination{Kind: Topic, Name: "t"}, env); !errors.Is(err, boom) {
		t.Fatalf("esperava erro do SNS, veio %v", err)
	}
	if err := p.Publish(context.Background(), Destination{Kind: Queue, Name: "q"}, env); !errors.Is(err, boom) {
		t.Fatalf("esperava erro do SQS, veio %v", err)
	}
	if err := p.Publish(context.Background(), Destination{Kind: "x", Name: "q"}, env); !errors.Is(err, errUnknownKind) {
		t.Fatalf("esperava errUnknownKind, veio %v", err)
	}
}

func TestDestinationShortName(t *testing.T) {
	cases := map[string]string{
		"arn:aws:sns:us-east-1:123:os-events":                    "os-events",
		"https://sqs.us-east-1.amazonaws.com/123/billing-commands": "billing-commands",
		"plain": "plain",
	}
	for in, want := range cases {
		if got := (Destination{Name: in}).ShortName(); got != want {
			t.Errorf("ShortName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTraceHeadersRoundTrip(t *testing.T) {
	h := TraceHeaders(map[string]string{"traceparent": "00-abc-def-01", "newrelic": "x", "other": "y"})
	if h.Get("traceparent") != "00-abc-def-01" || h.Get("newrelic") != "x" || h.Get("other") != "" {
		t.Fatalf("headers errados: %v", h)
	}
	back := traceAttributes(h)
	if len(back) != 2 {
		t.Fatalf("esperava 2 atributos de trace, veio %v", back)
	}
}

// ---- consumer ----

func msg(id string, b *string, attrs map[string]string) sqstypes.Message {
	ma := map[string]sqstypes.MessageAttributeValue{}
	for k, v := range attrs {
		ma[k] = sqstypes.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(v)}
	}
	return sqstypes.Message{MessageId: aws.String(id), ReceiptHandle: aws.String("rh-" + id), Body: b, MessageAttributes: ma}
}

func TestConsumer_Poll(t *testing.T) {
	ok := envelope(t, contracts.EvtBudgetCreated)
	fail := envelope(t, contracts.EvtPaymentFailed)
	unknown := envelope(t, "desconhecido")
	snsWrapped := envelope(t, contracts.EvtRepairFinished)
	wrappedBody := `{"Type":"Notification","Message":` + jsonString(*body(t, snsWrapped)) +
		`,"MessageAttributes":{"traceparent":{"Type":"String","Value":"00-a-b-01"}}}`

	q := &fakeSQS{batches: [][]sqstypes.Message{{
		msg("1", body(t, ok), map[string]string{"traceparent": "00-a-b-01"}),
		msg("2", body(t, fail), nil),
		msg("3", body(t, unknown), nil),
		msg("4", aws.String("não é json"), nil),
		msg("5", aws.String(wrappedBody), nil),
	}}}

	c := NewConsumer(q, "http://x/queue", "test", disabledNR(t), nil)
	var handled []string
	c.Handle(contracts.EvtBudgetCreated, func(ctx context.Context, env contracts.Envelope) error {
		handled = append(handled, env.Type)
		return nil
	})
	c.Handle(contracts.EvtPaymentFailed, func(context.Context, contracts.Envelope) error { return errors.New("falhou") })
	c.Handle(contracts.EvtRepairFinished, func(ctx context.Context, env contracts.Envelope) error {
		handled = append(handled, env.Type)
		return nil
	})

	n, err := c.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if n != 3 { // ok, unknown (descartada) e o envelope dentro do invólucro SNS
		t.Fatalf("esperava 3 processadas, veio %d", n)
	}
	if len(handled) != 2 {
		t.Fatalf("handlers chamados: %v", handled)
	}
	want := map[string]bool{"rh-1": true, "rh-3": true, "rh-5": true}
	if len(q.deleted) != len(want) {
		t.Fatalf("removidas: %v", q.deleted)
	}
	for _, d := range q.deleted {
		if !want[d] {
			t.Fatalf("não deveria remover %s (falha e malformada ficam para a DLQ)", d)
		}
	}
}

func TestConsumer_PollErroDeRecebimento(t *testing.T) {
	q := &fakeSQS{recvErr: errors.New("sem rede")}
	c := NewConsumer(q, "u", "test", nil, nil)
	if _, err := c.Poll(context.Background()); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestConsumer_RunParaComContexto(t *testing.T) {
	q := &fakeSQS{}
	c := NewConsumer(q, "u", "test", nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	cancel()
	<-done
}

func TestConsumer_RunComErroFazBackoff(t *testing.T) {
	q := &fakeSQS{recvErr: errors.New("sem rede")}
	c := NewConsumer(q, "u", "test", nil, nil)
	c.errBackoff = 0
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	cancel()
	<-done
}

func TestMemoryPublisher(t *testing.T) {
	p := NewMemoryPublisher()
	_ = p.Publish(context.Background(), Destination{Kind: Queue, Name: "q"}, envelope(t, "a"))
	_ = p.Publish(context.Background(), Destination{Kind: Queue, Name: "q"}, envelope(t, "b"))
	if len(p.Messages()) != 2 || len(p.OfType("a")) != 1 {
		t.Fatal("contagem errada")
	}
	p.Reset()
	if len(p.Messages()) != 0 {
		t.Fatal("reset não limpou")
	}
	p.Err = errors.New("x")
	if err := p.Publish(context.Background(), Destination{}, envelope(t, "c")); err == nil {
		t.Fatal("esperava erro configurado")
	}
}

func jsonString(s string) string {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, r := range s {
		switch r {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		default:
			b = append(b, string(r)...)
		}
	}
	return string(append(b, '"'))
}
