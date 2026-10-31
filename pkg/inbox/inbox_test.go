package inbox

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
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

func TestOnce_Idempotente(t *testing.T) {
	e, _ := contracts.NewEnvelope(contracts.EvtBudgetCreated, "billing", contracts.Meta{}, struct{}{})
	calls := 0
	fn := func(*gorm.DB) error { calls++; return nil }

	dup, err := Once(context.Background(), testDB, "os-saga", e, fn)
	if err != nil || dup {
		t.Fatalf("primeira entrega: dup=%v err=%v", dup, err)
	}
	dup, err = Once(context.Background(), testDB, "os-saga", e, fn)
	if err != nil || !dup {
		t.Fatalf("reentrega: dup=%v err=%v", dup, err)
	}
	if calls != 1 {
		t.Fatalf("efeitos aplicados %d vezes", calls)
	}
	// Outro consumidor pode processar a mesma mensagem.
	if dup, _ := Once(context.Background(), testDB, "outro", e, fn); dup || calls != 2 {
		t.Fatal("consumidores diferentes têm registros independentes")
	}
}

func TestOnce_FalhaPermiteNovaTentativa(t *testing.T) {
	e, _ := contracts.NewEnvelope(contracts.EvtPaymentApproved, "billing", contracts.Meta{}, struct{}{})
	boom := errors.New("falhou")
	if _, err := Once(context.Background(), testDB, "os-saga", e, func(*gorm.DB) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do handler, veio %v", err)
	}
	ran := false
	dup, err := Once(context.Background(), testDB, "os-saga", e, func(*gorm.DB) error { ran = true; return nil })
	if err != nil || dup || !ran {
		t.Fatal("após falha (rollback) a mensagem precisa ser reprocessada")
	}
}

func TestPurge(t *testing.T) {
	ctx := context.Background()
	velha, _ := contracts.NewEnvelope(contracts.EvtBudgetCreated, "billing", contracts.Meta{}, struct{}{})
	nova, _ := contracts.NewEnvelope(contracts.EvtBudgetCreated, "billing", contracts.Meta{}, struct{}{})
	now := time.Now().UTC()
	consumer := "purge-" + velha.MessageID // único por execução: o banco de teste persiste
	testDB.Create(&ProcessedMessage{MessageID: velha.MessageID, Consumer: consumer, Type: velha.Type, ProcessedAt: now.Add(-30 * 24 * time.Hour)})
	testDB.Create(&ProcessedMessage{MessageID: nova.MessageID, Consumer: consumer, Type: nova.Type, ProcessedAt: now})

	n, err := Purge(ctx, testDB, now.Add(-21*24*time.Hour))
	if err != nil || n < 1 {
		t.Fatalf("purge: n=%d err=%v", n, err)
	}
	var left []string
	testDB.Model(&ProcessedMessage{}).Where("consumer = ?", consumer).Pluck("message_id", &left)
	if len(left) != 1 || left[0] != nova.MessageID {
		t.Fatalf("depois do purge ficou %v", left)
	}
	// Mensagem apagada do inbox volta a ser processável: por isso a retenção passa a da DLQ.
	if dup, err := Once(ctx, testDB, consumer, velha, func(*gorm.DB) error { return nil }); err != nil || dup {
		t.Fatalf("dup=%v err=%v", dup, err)
	}
}

func TestPurgeBancoFechado(t *testing.T) {
	db, err := gorm.Open(gormpostgres.Open("host=127.0.0.1 port=1 user=x dbname=x sslmode=disable connect_timeout=1"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	if err != nil {
		t.Skip(err)
	}
	if _, err := Purge(context.Background(), db, time.Now()); err == nil {
		t.Fatal("esperava erro sem banco")
	}
}
