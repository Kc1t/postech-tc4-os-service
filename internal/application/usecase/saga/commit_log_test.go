package sagauc

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/Kc1t/postech-tc4-os-service/pkg/logger"
)

// commitFails roda o passo e falha no fim, como um commit recusado pelo banco: a transação
// interna é desfeita e o passo não pode ter logado o que o dashboard conta.
type commitFails struct{ inner ports.TxManager }

var errCommit = errors.New("commit recusado")

func (c commitFails) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return c.inner.WithinTx(ctx, func(ctx context.Context) error {
		if err := fn(ctx); err != nil {
			return err
		}
		return errCommit
	})
}

func TestSaga_CompensacaoLogaSoDepoisDoCommit(t *testing.T) {
	f := newFx(t, Config{})
	var buf bytes.Buffer
	ctx := logger.ContextWith(f.ctx, logger.NewWriter(&buf, "info"))

	falha := f.open("")
	tx := f.o.d.Tx
	f.o.d.Tx = commitFails{inner: tx}
	if err := f.cancel.Execute(ctx, falha.ID(), ""); !errors.Is(err, errCommit) {
		t.Fatalf("esperava a falha do commit: %v", err)
	}
	f.o.d.Tx = tx
	if strings.Contains(buf.String(), "saga_compensated") {
		t.Fatalf("logou compensação de transação desfeita: %s", buf.String())
	}
	if f.order(falha.ID()).Status() == entities.StatusCancelled {
		t.Fatal("a OS não pode ficar cancelada depois do rollback")
	}

	must(t, f.cancel.Execute(ctx, falha.ID(), ""))
	if n := strings.Count(buf.String(), `"message":"saga_compensated"`); n != 1 {
		t.Fatalf("esperava 1 saga_compensated depois do commit, veio %d: %s", n, buf.String())
	}
}

func TestStep_CommittedRodaUmaVez(t *testing.T) {
	var nilStep *step
	nilStep.committed() // passo que não chegou a ser montado

	st := &step{}
	n := 0
	st.onCommit(func() { n++ })
	st.committed()
	st.committed()
	if n != 1 {
		t.Fatalf("rodou %d vezes", n)
	}
}
