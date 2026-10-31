package inbox

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"

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
