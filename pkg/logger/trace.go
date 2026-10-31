package logger

import (
	"context"
	"log/slog"

	"github.com/newrelic/go-agent/v3/newrelic"
)

// WithTrace acrescenta trace.id e span.id da transação New Relic presente em ctx. Com esses
// campos no log JSON, o New Relic liga cada linha de log ao trace distribuído (logs em contexto).
func WithTrace(ctx context.Context, l *slog.Logger) *slog.Logger {
	if l == nil {
		l = slog.Default()
	}
	txn := newrelic.FromContext(ctx)
	if txn == nil {
		return l
	}
	md := txn.GetTraceMetadata()
	if md.TraceID == "" {
		return l
	}
	return l.With("trace.id", md.TraceID, "span.id", md.SpanID)
}
