package outbox

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
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
	if m.Attempts != 1 || m.LastError == "" || m.PublishedAt != nil {
		t.Fatalf("falha não registrada: %+v", m)
	}
	pub.Err = nil
	if n, _ := relay.Flush(context.Background()); n != 1 {
		t.Fatal("deveria publicar na tentativa seguinte")
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
