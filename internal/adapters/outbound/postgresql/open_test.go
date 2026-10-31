package postgresql

import (
	"context"
	"testing"

	"github.com/newrelic/go-agent/v3/newrelic"
)

func TestOpen_ComTracerDoNewRelic(t *testing.T) {
	db, err := Open(testDSN)
	if err != nil {
		t.Fatal(err)
	}
	app, err := newrelic.NewApplication(newrelic.ConfigAppName("test"), newrelic.ConfigEnabled(false))
	if err != nil {
		t.Fatal(err)
	}
	txn := app.StartTransaction("test")
	defer txn.End()
	var n int
	if err := db.WithContext(newrelic.NewContext(context.Background(), txn)).Raw("SELECT 1").Scan(&n).Error; err != nil || n != 1 {
		t.Fatalf("consulta pelo pgx instrumentado: n=%d err=%v", n, err)
	}
}

func TestOpen_Erros(t *testing.T) {
	if _, err := Open("postgres://%zz"); err == nil {
		t.Fatal("DSN inválida deveria falhar")
	}
	if _, err := Open("host=127.0.0.1 port=1 user=x dbname=x sslmode=disable connect_timeout=1"); err == nil {
		t.Fatal("banco inacessível deveria falhar")
	}
}
