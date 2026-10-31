package logger_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/Kc1t/postech-tc4-os-service/pkg/logger"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":    slog.LevelDebug,
		"DEBUG":    slog.LevelDebug,
		"  warn  ": slog.LevelWarn,
		"warning":  slog.LevelWarn,
		"error":    slog.LevelError,
		"info":     slog.LevelInfo,
		"":         slog.LevelInfo,
		"qualquer": slog.LevelInfo,
	}

	for input, expected := range cases {
		if got := logger.ParseLevel(input); got != expected {
			t.Errorf("ParseLevel(%q): esperava %v, veio %v", input, expected, got)
		}
	}
}

func TestNew(t *testing.T) {
	if logger.New("debug") == nil {
		t.Fatal("esperava um logger, veio nil")
	}
}

func TestNewWriter_TextoEmMessage(t *testing.T) {
	var buf bytes.Buffer
	logger.NewWriter(&buf, "info").Info("saga_started", "saga_id", "s-1")
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	if line[logger.MessageKey] != "saga_started" || line["saga_id"] != "s-1" {
		t.Fatalf("linha sem o texto em %q: %v", logger.MessageKey, line)
	}
	if _, ok := line["msg"]; ok {
		t.Fatalf("a chave msg não pode sair: %v", line)
	}
	if logger.NewWriter(&buf, "warn").Enabled(t.Context(), slog.LevelInfo) {
		t.Fatal("nível warn não loga info")
	}
}
