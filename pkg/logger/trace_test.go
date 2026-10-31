package logger

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/newrelic/go-agent/v3/newrelic"
)

func TestWithTrace_SemTransacao(t *testing.T) {
	l := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	if WithTrace(context.Background(), l) != l {
		t.Fatal("sem transação o logger deve voltar intacto")
	}
	if WithTrace(context.Background(), nil) == nil {
		t.Fatal("logger nil deve virar o default")
	}
}

func TestWithTrace_TransacaoSemTrace(t *testing.T) {
	app, _ := newrelic.NewApplication(newrelic.ConfigAppName("t"), newrelic.ConfigEnabled(false))
	txn := app.StartTransaction("x")
	defer txn.End()
	l := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	if WithTrace(newrelic.NewContext(context.Background(), txn), l) == nil {
		t.Fatal("logger nil")
	}
}
