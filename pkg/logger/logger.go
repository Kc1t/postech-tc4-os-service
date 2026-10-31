package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// MessageKey é a chave do texto do log no JSON. O New Relic usa o atributo message como texto da
// linha (busca, agrupamento e os alertas por log em postech-tc4-infra-k8s/observability); com o
// "msg" padrão do slog, isso dependeria de um remapeamento na ingestão.
const MessageKey = "message"

// New devolve o logger JSON da aplicação, no stdout.
func New(level string) *slog.Logger { return NewWriter(os.Stdout, level) }

// NewWriter é o New escrevendo em w (testes).
func NewWriter(w io.Writer, level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: ParseLevel(level), ReplaceAttr: renameMessage}))
}

func renameMessage(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.MessageKey {
		a.Key = MessageKey
	}
	return a
}

func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
