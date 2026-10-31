package postgresql

import (
	"context"
	"errors"
	"testing"
	"time"

	pgmodel "github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql/model"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/outbox"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedOrderWithSaga(t *testing.T, plate string) (*entities.ServiceOrder, *entities.Saga) {
	t.Helper()
	c, v := seedVehicle(t, plate)
	so := newTestOrder(c.ID(), v.ID())
	if err := NewServiceOrderRepository(testDB).Create(context.Background(), so); err != nil {
		t.Fatal(err)
	}
	saga := entities.NewSaga(uuid.NewString(), so.ID(), "")
	if err := NewSagaRepository(testDB).Create(context.Background(), saga); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testDB.Delete(&pgmodel.Saga{}, "id = ?", saga.ID)
		testDB.Delete(&pgmodel.StatusHistory{}, "os_id = ?", so.ID())
		testDB.Delete(&pgmodel.ServiceOrder{}, "id = ?", so.ID())
	})
	return so, saga
}

func TestSagaRepository_CicloCompleto(t *testing.T) {
	_, saga := seedOrderWithSaga(t, "SAG0001")
	repo := NewSagaRepository(testDB)
	ctx := context.Background()

	got, err := repo.FindByOSID(ctx, saga.OSID)
	if err != nil || got.State != entities.SagaAwaitingDiagnosis || got.Version != 1 {
		t.Fatalf("FindByOSID: %+v %v", got, err)
	}
	got.State = entities.SagaAwaitingApproval
	got.BudgetID = "b-1"
	got.Reservations = []entities.PartReservation{{PartID: "p1", Quantity: 2}}
	got.WaitUntil(time.Now().Add(-time.Minute))
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	stale := *saga
	if err := repo.Update(ctx, &stale); !errors.Is(err, domainerrors.ErrConcurrentUpdate) {
		t.Fatalf("versão velha: esperava ErrConcurrentUpdate, veio %v", err)
	}

	reread, _ := repo.FindByOSID(ctx, saga.OSID)
	if reread.BudgetID != "b-1" || len(reread.Reservations) != 1 || reread.Version != 2 {
		t.Fatalf("estado persistido: %+v", reread)
	}
	expired, err := repo.ListExpired(ctx, time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range expired {
		found = found || s.ID == saga.ID
	}
	if !found {
		t.Fatal("saga com prazo vencido deveria ser listada")
	}

	if _, err := repo.FindByOSID(ctx, uuid.NewString()); !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatalf("esperava ErrNotFound, veio %v", err)
	}
}

func TestSagaRepository_ForUpdateSerializa(t *testing.T) {
	_, saga := seedOrderWithSaga(t, "SAG0002")
	repo := NewSagaRepository(testDB)
	txm := NewTxManager(testDB)

	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan time.Time, 1)
	go func() {
		_ = txm.WithinTx(context.Background(), func(ctx context.Context) error {
			if _, err := repo.FindByOSIDForUpdate(ctx, saga.OSID); err != nil {
				t.Error(err)
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	go func() {
		_ = txm.WithinTx(context.Background(), func(ctx context.Context) error {
			_, err := repo.FindByOSIDForUpdate(ctx, saga.OSID)
			done <- time.Now()
			return err
		})
	}()
	time.Sleep(150 * time.Millisecond)
	releasedAt := time.Now()
	close(release)
	if acquired := <-done; acquired.Before(releasedAt) {
		t.Fatal("segunda transação obteve o lock antes da primeira terminar")
	}
}

func TestStatusHistoryRepository(t *testing.T) {
	so, _ := seedOrderWithSaga(t, "SAG0003")
	repo := NewStatusHistoryRepository(testDB)
	ctx := context.Background()
	t0 := time.Now().UTC().Add(-time.Minute)
	entries := []*entities.StatusHistoryEntry{
		{ID: uuid.NewString(), OSID: so.ID(), To: entities.StatusReceived, Actor: entities.ActorStaff, CreatedAt: t0},
		{OSID: so.ID(), From: entities.StatusReceived, To: entities.StatusInDiagnosis, Actor: entities.ActorSaga, MessageID: "m-1"},
	}
	for _, e := range entries {
		if err := repo.Add(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListByOS(ctx, so.ID())
	if err != nil || len(got) != 2 {
		t.Fatalf("ListByOS: %d %v", len(got), err)
	}
	if got[0].To != entities.StatusReceived || got[1].MessageID != "m-1" || got[1].ID == "" {
		t.Fatalf("ordem ou campos incorretos: %+v %+v", got[0], got[1])
	}
}

func TestTxManager_RollbackDesfazTudo(t *testing.T) {
	so, saga := seedOrderWithSaga(t, "SAG0004")
	txm := NewTxManager(testDB)
	orders := NewServiceOrderRepository(testDB)
	messenger := NewOutboxMessenger(Routes{ExecutionCommandsQueueURL: "http://q/execution-commands"})
	boom := errors.New("falha depois das escritas")

	err := txm.WithinTx(context.Background(), func(ctx context.Context) error {
		current, _ := orders.FindByID(ctx, so.ID())
		_ = current.TransitionTo(entities.StatusInDiagnosis)
		if err := orders.Update(ctx, current); err != nil {
			return err
		}
		// transação aninhada reaproveita a externa
		if err := txm.WithinTx(ctx, func(ctx context.Context) error {
			return messenger.Send(ctx, contracts.CmdCancelExecution, contracts.Meta{SagaID: saga.ID, OSID: so.ID()},
				contracts.CancelExecution{Reason: "x"})
		}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("esperava o erro do callback, veio %v", err)
	}
	got, _ := orders.FindByID(context.Background(), so.ID())
	if got.Status() != entities.StatusReceived {
		t.Fatal("rollback não desfez a OS")
	}
	var n int64
	testDB.Model(&outbox.Message{}).Where("saga_id = ?", saga.ID).Count(&n)
	if n != 0 {
		t.Fatal("rollback não desfez o outbox")
	}
}

func TestOutboxMessenger_RoteiaPorTipo(t *testing.T) {
	routes := Routes{
		BillingCommandsQueueURL:   "http://q/billing-commands",
		ExecutionCommandsQueueURL: "http://q/execution-commands",
		OSEventsTopicARN:          "arn:aws:sns:us-east-1:000000000000:os-events",
	}
	m := NewOutboxMessenger(routes)
	sagaID := uuid.NewString()
	t.Cleanup(func() { testDB.Exec("DELETE FROM outbox_messages WHERE saga_id = ?", sagaID) })

	if err := m.Send(context.Background(), contracts.CmdCreateBudget, contracts.Meta{}, struct{}{}); !errors.Is(err, ErrNoTransaction) {
		t.Fatalf("fora de transação deveria falhar, veio %v", err)
	}
	err := testDB.Transaction(func(tx *gorm.DB) error {
		ctx := WithTx(context.Background(), tx)
		meta := contracts.Meta{SagaID: sagaID, OSID: uuid.NewString(), CorrelationID: "corr-1"}
		for _, typ := range []string{contracts.CmdCreateBudget, contracts.CmdStartRepair, contracts.EvtServiceOrderStatusChanged} {
			if err := m.Send(ctx, typ, meta, map[string]string{"k": "v"}); err != nil {
				return err
			}
		}
		return m.Send(ctx, "desconhecido.X", meta, nil)
	})
	if err == nil {
		t.Fatal("tipo sem rota deveria falhar")
	}
	err = testDB.Transaction(func(tx *gorm.DB) error {
		ctx := WithTx(context.Background(), tx)
		meta := contracts.Meta{SagaID: sagaID, OSID: uuid.NewString(), CorrelationID: "corr-1"}
		for _, typ := range []string{contracts.CmdCreateBudget, contracts.CmdStartRepair, contracts.EvtServiceOrderStatusChanged} {
			if err := m.Send(ctx, typ, meta, map[string]string{"k": "v"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var msgs []outbox.Message
	testDB.Where("saga_id = ?", sagaID).Order("created_at").Find(&msgs)
	if len(msgs) != 3 {
		t.Fatalf("esperava 3 mensagens, veio %d", len(msgs))
	}
	want := map[string]string{
		contracts.CmdCreateBudget:              routes.BillingCommandsQueueURL,
		contracts.CmdStartRepair:               routes.ExecutionCommandsQueueURL,
		contracts.EvtServiceOrderStatusChanged: routes.OSEventsTopicARN,
	}
	for _, msg := range msgs {
		if msg.Destination != want[msg.Type] {
			t.Errorf("%s foi para %s", msg.Type, msg.Destination)
		}
		env, err := contracts.Parse([]byte(msg.Body))
		if err != nil || env.Producer != Producer || env.CorrelationID != "corr-1" {
			t.Errorf("envelope inválido: %+v %v", env, err)
		}
	}
}

func TestInbox_Adaptador(t *testing.T) {
	in := NewInbox(testDB, "os-saga-test")
	env, _ := contracts.NewEnvelope(contracts.EvtBudgetCreated, "billing-service", contracts.Meta{}, struct{}{})
	calls := 0
	for i := 0; i < 2; i++ {
		dup, err := in.Once(context.Background(), env, func(ctx context.Context) error {
			if _, ok := TxFrom(ctx); !ok {
				t.Error("callback do inbox deve receber a transação no ctx")
			}
			calls++
			return nil
		})
		if err != nil || dup != (i == 1) {
			t.Fatalf("entrega %d: dup=%v err=%v", i, dup, err)
		}
	}
	if calls != 1 {
		t.Fatalf("efeito aplicado %d vezes", calls)
	}
}
