package outbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/inbox"
	"github.com/Kc1t/postech-tc4-os-service/pkg/messaging"
	"github.com/newrelic/go-agent/v3/newrelic"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		env("TEST_DB_HOST", "localhost"), env("TEST_DB_PORT", "5432"), env("TEST_DB_USER", "postgres"),
		env("TEST_DB_PASSWORD", "postgres"), env("TEST_DB_NAME", "workshop"))
	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatalf("banco de teste: %v", err)
	}
	if err := Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := inbox.Migrate(db); err != nil {
		log.Fatalf("migrate inbox: %v", err)
	}
	testDB = db
	os.Exit(m.Run())
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func clean(t *testing.T) {
	t.Helper()
	if err := testDB.Exec("DELETE FROM outbox_messages").Error; err != nil {
		t.Fatal(err)
	}
}

func newEnv(t *testing.T, typ string) contracts.Envelope {
	t.Helper()
	e, err := contracts.NewEnvelope(typ, "os-service", contracts.Meta{SagaID: "s-1", OSID: "o-1"}, map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestEnqueueEFlush(t *testing.T) {
	clean(t)
	ctx := context.Background()
	app, _ := newrelic.NewApplication(newrelic.ConfigAppName("t"), newrelic.ConfigEnabled(false))
	txn := app.StartTransaction("req")
	ctx = newrelic.NewContext(ctx, txn)

	e1, e2 := newEnv(t, contracts.CmdCreateBudget), newEnv(t, contracts.EvtServiceOrderStatusChanged)
	err := testDB.Transaction(func(tx *gorm.DB) error {
		if err := Enqueue(ctx, tx, messaging.Destination{Kind: messaging.Queue, Name: "billing-commands"}, e1); err != nil {
			return err
		}
		return Enqueue(ctx, tx, messaging.Destination{Kind: messaging.Topic, Name: "os-events"}, e2)
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if n, _ := Pending(ctx, testDB); n != 2 {
		t.Fatalf("pendentes = %d, esperava 2", n)
	}

	pub := messaging.NewMemoryPublisher()
	relay := NewRelay(testDB, pub, app, nil)
	n, err := relay.Flush(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("flush: n=%d err=%v", n, err)
	}
	msgs := pub.Messages()
	if len(msgs) != 2 || msgs[0].Envelope.MessageID != e1.MessageID || msgs[0].Dest.Kind != messaging.Queue {
		t.Fatalf("publicação fora de ordem ou destino errado: %+v", msgs)
	}
	if n, _ := Pending(ctx, testDB); n != 0 {
		t.Fatalf("ainda há %d pendentes", n)
	}
	if n, _ := relay.Flush(context.Background()); n != 0 {
		t.Fatal("não pode republicar mensagens já publicadas")
	}
}

func TestRollbackDescartaMensagem(t *testing.T) {
	clean(t)
	_ = testDB.Transaction(func(tx *gorm.DB) error {
		_ = Enqueue(context.Background(), tx, messaging.Destination{Kind: messaging.Queue, Name: "q"}, newEnv(t, "x"))
		return errors.New("caso de uso falhou")
	})
	if n, _ := Pending(context.Background(), testDB); n != 0 {
		t.Fatal("mensagem de transação desfeita não pode ficar no outbox")
	}
}

func TestFlushComFalhaRegistraTentativa(t *testing.T) {
	clean(t)
	_ = testDB.Transaction(func(tx *gorm.DB) error {
		return Enqueue(context.Background(), tx, messaging.Destination{Kind: messaging.Queue, Name: "q"}, newEnv(t, "x"))
	})
	pub := messaging.NewMemoryPublisher()
	pub.Err = errors.New("sns fora")
	relay := NewRelay(testDB, pub, nil, nil)
	if n, err := relay.Flush(context.Background()); err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var m Message
	testDB.First(&m)
	if m.Attempts != 1 || m.LastError == "" || m.PublishedAt != nil || m.NextAttemptAt == nil {
		t.Fatalf("falha não registrada: %+v", m)
	}
	pub.Err = nil
	if n, _ := relay.Flush(context.Background()); n != 0 {
		t.Fatal("não pode tentar de novo antes de a espera passar")
	}
	later := m.NextAttemptAt.Add(time.Millisecond)
	relay.now = func() time.Time { return later }
	if n, _ := relay.Flush(context.Background()); n != 1 {
		t.Fatal("deveria publicar na tentativa seguinte")
	}
	var after Message
	testDB.First(&after)
	if after.PublishedAt == nil || after.NextAttemptAt != nil || after.LastError != "" {
		t.Fatalf("publicação não registrada: %+v", after)
	}
}

// failingPublisher falha só para um destino.
type failingPublisher struct {
	*messaging.MemoryPublisher
	bad string
}

func (p failingPublisher) Publish(ctx context.Context, dest messaging.Destination, env contracts.Envelope) error {
	if dest.Name == p.bad {
		return errors.New("destino fora")
	}
	return p.MemoryPublisher.Publish(ctx, dest, env)
}

func TestMensagemComFalhaNaoTravaOLote(t *testing.T) {
	clean(t)
	ruim, boa := newEnv(t, "ruim"), newEnv(t, "boa")
	_ = testDB.Transaction(func(tx *gorm.DB) error {
		if err := Enqueue(context.Background(), tx, messaging.Destination{Kind: messaging.Queue, Name: "fora"}, ruim); err != nil {
			return err
		}
		return Enqueue(context.Background(), tx, messaging.Destination{Kind: messaging.Queue, Name: "ok"}, boa)
	})
	pub := failingPublisher{MemoryPublisher: messaging.NewMemoryPublisher(), bad: "fora"}
	relay := NewRelay(testDB, pub, nil, nil)
	relay.batchSize = 1 // o lote só cabe a mensagem mais antiga, a que falha

	if n, _ := relay.Flush(context.Background()); n != 0 {
		t.Fatal("a primeira varredura pega só a mensagem que falha")
	}
	if n, _ := relay.Flush(context.Background()); n != 1 {
		t.Fatal("em espera, a mensagem que falhou não pode ocupar o lote da próxima varredura")
	}
	if msgs := pub.Messages(); len(msgs) != 1 || msgs[0].Envelope.MessageID != boa.MessageID {
		t.Fatalf("publicou %+v", msgs)
	}
}

func TestMensagemTravadaViraErro(t *testing.T) {
	clean(t)
	_ = testDB.Transaction(func(tx *gorm.DB) error {
		return Enqueue(context.Background(), tx, messaging.Destination{Kind: messaging.Queue, Name: "q"}, newEnv(t, "x"))
	})
	testDB.Model(&Message{}).Where("1 = 1").Update("attempts", StuckAfter-1)

	var buf bytes.Buffer
	pub := messaging.NewMemoryPublisher()
	pub.Err = errors.New("sns fora")
	relay := NewRelay(testDB, pub, nil, slog.New(slog.NewJSONHandler(&buf, nil)))
	if _, err := relay.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"msg":"outbox_message_stuck"`) || !strings.Contains(out, `"level":"ERROR"`) {
		t.Fatalf("esperava log de erro outbox_message_stuck, veio %s", out)
	}
	var m Message
	testDB.First(&m)
	if m.Attempts != StuckAfter || m.NextAttemptAt == nil || time.Until(*m.NextAttemptAt) < 4*time.Minute {
		t.Fatalf("depois de %d falhas a espera devia estar perto do máximo: %+v", StuckAfter, m)
	}
}

func TestBackoff(t *testing.T) {
	cases := map[int]time.Duration{0: 0, 1: time.Second, 2: 2 * time.Second, 5: 16 * time.Second, 9: 256 * time.Second, 10: MaxBackoff, 1000: MaxBackoff}
	for attempts, want := range cases {
		if got := Backoff(attempts); got != want {
			t.Errorf("Backoff(%d) = %s, esperava %s", attempts, got, want)
		}
	}
}

func TestCleanupApagaSoOQueVenceu(t *testing.T) {
	clean(t)
	ctx := context.Background()
	now := time.Now().UTC()
	velha, recente, pendente := newEnv(t, "velha"), newEnv(t, "recente"), newEnv(t, "pendente")
	_ = testDB.Transaction(func(tx *gorm.DB) error {
		for _, e := range []contracts.Envelope{velha, recente, pendente} {
			if err := Enqueue(ctx, tx, messaging.Destination{Kind: messaging.Queue, Name: "q"}, e); err != nil {
				return err
			}
		}
		return nil
	})
	testDB.Model(&Message{}).Where("id = ?", velha.MessageID).Update("published_at", now.Add(-8*24*time.Hour))
	testDB.Model(&Message{}).Where("id = ?", recente.MessageID).Update("published_at", now.Add(-time.Hour))
	testDB.Model(&Message{}).Where("id = ?", pendente.MessageID).Update("created_at", now.Add(-30*24*time.Hour))

	inboxVelho, inboxRecente := newEnv(t, "a").MessageID, newEnv(t, "b").MessageID
	testDB.Create(&inbox.ProcessedMessage{MessageID: inboxVelho, Consumer: "c", Type: "a", ProcessedAt: now.Add(-22 * 24 * time.Hour)})
	testDB.Create(&inbox.ProcessedMessage{MessageID: inboxRecente, Consumer: "c", Type: "b", ProcessedAt: now.Add(-15 * 24 * time.Hour)})

	relay := NewRelay(testDB, messaging.NewMemoryPublisher(), nil, nil)
	outboxDeleted, inboxDeleted, err := relay.Cleanup(ctx)
	if err != nil || outboxDeleted != 1 || inboxDeleted < 1 {
		t.Fatalf("cleanup: outbox=%d inbox=%d err=%v", outboxDeleted, inboxDeleted, err)
	}
	var ids []string
	testDB.Model(&Message{}).Order("id").Pluck("id", &ids)
	if len(ids) != 2 || strings.Contains(strings.Join(ids, ","), velha.MessageID) {
		t.Fatalf("outbox depois da limpeza: %v (pendente nunca sai, publicada recente fica)", ids)
	}
	var left []string
	testDB.Model(&inbox.ProcessedMessage{}).Where("message_id IN ?", []string{inboxVelho, inboxRecente}).Pluck("message_id", &left)
	if len(left) != 1 || left[0] != inboxRecente {
		t.Fatalf("inbox depois da limpeza: %v (registro de 15 dias precisa ficar, a DLQ guarda 14)", left)
	}
}

func TestLimpezaRodaNoIntervalo(t *testing.T) {
	clean(t)
	velha := newEnv(t, "velha")
	_ = testDB.Transaction(func(tx *gorm.DB) error {
		return Enqueue(context.Background(), tx, messaging.Destination{Kind: messaging.Queue, Name: "q"}, velha)
	})
	testDB.Model(&Message{}).Where("1 = 1").Update("published_at", time.Now().UTC().Add(-8*24*time.Hour))

	relay := NewRelay(testDB, messaging.NewMemoryPublisher(), nil, nil).WithInterval(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	var n int64 = 1
	for n > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		testDB.Model(&Message{}).Count(&n)
	}
	cancel()
	<-done
	if n != 0 {
		t.Fatal("o relay não fez a limpeza")
	}
	first := relay.lastCleanup
	relay.maybeCleanup(context.Background())
	if relay.lastCleanup != first {
		t.Fatal("a limpeza não pode rodar de novo antes do intervalo")
	}
	relay.WithCleanup(time.Hour, time.Hour, 0).maybeCleanup(context.Background())
	if relay.lastCleanup != first {
		t.Fatal("com intervalo 0 a limpeza fica desligada")
	}
}

func TestRunParaComContexto(t *testing.T) {
	clean(t)
	_ = testDB.Transaction(func(tx *gorm.DB) error {
		return Enqueue(context.Background(), tx, messaging.Destination{Kind: messaging.Queue, Name: "q"}, newEnv(t, "x"))
	})
	pub := messaging.NewMemoryPublisher()
	relay := NewRelay(testDB, pub, nil, nil).WithInterval(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(pub.Messages()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if len(pub.Messages()) != 1 {
		t.Fatal("relay em loop não publicou")
	}
}

func TestEnqueueEnvelopeInvalido(t *testing.T) {
	bad := contracts.Envelope{MessageID: "not-uuid", Type: "x", Payload: []byte("{")}
	if err := Enqueue(context.Background(), testDB, messaging.Destination{}, bad); err == nil {
		t.Fatal("esperava erro com payload inválido")
	}
}
