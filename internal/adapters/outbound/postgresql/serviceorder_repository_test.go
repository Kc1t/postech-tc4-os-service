package postgresql

import (
	"context"
	"errors"
	"testing"
	"time"

	pgmodel "github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql/model"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
)

func seedVehicle(t *testing.T, plate string) (*entities.Requester, *entities.Vehicle) {
	t.Helper()
	crepo := NewRequesterRepository(testDB)
	doc := "99999999909"
	now := time.Now()
	c := entities.ReconstituteRequester("", "Cliente OS", doc, "os@test.com", "11999990010", now, now)
	if err := crepo.Create(context.Background(), c); err != nil {
		existing, ferr := crepo.FindByDocument(context.Background(), doc)
		if ferr != nil {
			t.Fatalf("seedVehicle: requester create failed: %v", err)
		}
		c = existing
	} else {
		t.Cleanup(func() { testDB.Delete(&pgmodel.Requester{}, "id = ?", c.ID()) })
	}

	vrepo := NewVehicleRepository(testDB)
	v := entities.ReconstituteVehicle("", c.ID(), plate, "Toyota", "Hilux", 2023, now, now)
	if err := vrepo.Create(context.Background(), v); err != nil {
		t.Fatalf("seedVehicle: vehicle create failed: %v", err)
	}
	t.Cleanup(func() { testDB.Delete(&pgmodel.Vehicle{}, "id = ?", v.ID()) })

	return c, v
}

func newTestOrder(requesterID, vehicleID string) *entities.ServiceOrder {
	so := entities.NewServiceOrder(requesterID, vehicleID)
	return so
}

func TestServiceOrderRepository_Create(t *testing.T) {
	c, v := seedVehicle(t, "OS00001")
	repo := NewServiceOrderRepository(testDB)
	so := newTestOrder(c.ID(), v.ID())

	if err := repo.Create(context.Background(), so); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if so.ID() == "" {
		t.Fatal("expected ID to be set after create")
	}
	t.Cleanup(func() { testDB.Delete(&pgmodel.ServiceOrder{}, "id = ?", so.ID()) })
}

func TestServiceOrderRepository_FindByID(t *testing.T) {
	c, v := seedVehicle(t, "OS00002")
	repo := NewServiceOrderRepository(testDB)
	so := newTestOrder(c.ID(), v.ID())
	if err := repo.Create(context.Background(), so); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Cleanup(func() { testDB.Delete(&pgmodel.ServiceOrder{}, "id = ?", so.ID()) })

	found, err := repo.FindByID(context.Background(), so.ID())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID() != so.ID() {
		t.Errorf("expected ID %s, got %s", so.ID(), found.ID())
	}
}

func TestServiceOrderRepository_FindByID_NotFound(t *testing.T) {
	repo := NewServiceOrderRepository(testDB)

	_, err := repo.FindByID(context.Background(), "00000000-0000-0000-0000-000000000000")
	if !isDomainNotFound(err) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestServiceOrderRepository_FindAll(t *testing.T) {
	c, v := seedVehicle(t, "OS00003")
	repo := NewServiceOrderRepository(testDB)

	soReceived := newTestOrder(c.ID(), v.ID())
	soInDiagnosis := newTestOrder(c.ID(), v.ID())
	soAwaiting := newTestOrder(c.ID(), v.ID())
	soInExecution := newTestOrder(c.ID(), v.ID())
	soFinished := newTestOrder(c.ID(), v.ID())
	soDelivered := newTestOrder(c.ID(), v.ID())

	for _, so := range []*entities.ServiceOrder{soReceived, soInDiagnosis, soAwaiting, soInExecution, soFinished, soDelivered} {
		if err := repo.Create(context.Background(), so); err != nil {
			t.Fatalf("setup Create: %v", err)
		}
		id := so.ID()
		t.Cleanup(func() { testDB.Delete(&pgmodel.ServiceOrder{}, "id = ?", id) })
	}

	testDB.Model(&pgmodel.ServiceOrder{}).Where("id = ?", soInDiagnosis.ID()).Update("status", entities.StatusInDiagnosis)
	testDB.Model(&pgmodel.ServiceOrder{}).Where("id = ?", soAwaiting.ID()).Update("status", entities.StatusAwaitingApproval)
	testDB.Model(&pgmodel.ServiceOrder{}).Where("id = ?", soInExecution.ID()).Update("status", entities.StatusInExecution)
	testDB.Model(&pgmodel.ServiceOrder{}).Where("id = ?", soFinished.ID()).Update("status", entities.StatusFinished)
	testDB.Model(&pgmodel.ServiceOrder{}).Where("id = ?", soDelivered.ID()).Update("status", entities.StatusDelivered)

	all, err := repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	for _, so := range all {
		if so.Status() == entities.StatusFinished || so.Status() == entities.StatusDelivered {
			t.Errorf("FindAll retornou OS com status %s — deveria ser excluida", so.Status())
		}
	}

	indexOf := func(id string) int {
		for i, so := range all {
			if so.ID() == id {
				return i
			}
		}
		return -1
	}

	idxExec := indexOf(soInExecution.ID())
	idxAwait := indexOf(soAwaiting.ID())
	idxDiag := indexOf(soInDiagnosis.ID())
	idxRecv := indexOf(soReceived.ID())

	if idxExec == -1 || idxAwait == -1 || idxDiag == -1 || idxRecv == -1 {
		t.Fatal("uma ou mais OS ativas nao foram retornadas")
	}

	if idxExec >= idxAwait {
		t.Errorf("in_execution (pos %d) deveria vir antes de awaiting_approval (pos %d)", idxExec, idxAwait)
	}
	if idxAwait >= idxDiag {
		t.Errorf("awaiting_approval (pos %d) deveria vir antes de in_diagnosis (pos %d)", idxAwait, idxDiag)
	}
	if idxDiag >= idxRecv {
		t.Errorf("in_diagnosis (pos %d) deveria vir antes de received (pos %d)", idxDiag, idxRecv)
	}
}

func TestServiceOrderRepository_FindByRequesterID(t *testing.T) {
	c, v := seedVehicle(t, "OS00004")
	repo := NewServiceOrderRepository(testDB)
	so := newTestOrder(c.ID(), v.ID())
	if err := repo.Create(context.Background(), so); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Cleanup(func() { testDB.Delete(&pgmodel.ServiceOrder{}, "id = ?", so.ID()) })

	orders, err := repo.FindByRequesterID(context.Background(), c.ID())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(orders) == 0 {
		t.Fatal("expected at least one service order for this requester")
	}
}

func TestServiceOrderRepository_Update(t *testing.T) {
	c, v := seedVehicle(t, "OS00006")
	repo := NewServiceOrderRepository(testDB)
	so := newTestOrder(c.ID(), v.ID())
	if err := repo.Create(context.Background(), so); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Cleanup(func() { testDB.Delete(&pgmodel.ServiceOrder{}, "id = ?", so.ID()) })

	so.SetNotes("Observação atualizada")
	if err := repo.Update(context.Background(), so); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	found, _ := repo.FindByID(context.Background(), so.ID())
	if found.Notes() != "Observação atualizada" {
		t.Errorf("expected updated notes, got %s", found.Notes())
	}
}

func TestServiceOrderRepository_Delete(t *testing.T) {
	c, v := seedVehicle(t, "OS00007")
	repo := NewServiceOrderRepository(testDB)
	so := newTestOrder(c.ID(), v.ID())
	if err := repo.Create(context.Background(), so); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := repo.Delete(context.Background(), so.ID()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err := repo.FindByID(context.Background(), so.ID())
	if !isDomainNotFound(err) {
		t.Fatal("expected record to be deleted")
	}
}

func TestServiceOrderRepository_AverageExecutionTime(t *testing.T) {
	c, v := seedVehicle(t, "OS00008")
	repo := NewServiceOrderRepository(testDB)

	now := time.Now().UTC().Truncate(time.Second)
	started1 := now.Add(-2 * time.Hour)
	finished1 := now.Add(-1 * time.Hour) // duracao 1h
	started2 := now.Add(-4 * time.Hour)
	finished2 := now.Add(-1 * time.Hour) // duracao 3h

	so1 := entities.ReconstituteServiceOrder(entities.ServiceOrderSnapshot{RequesterID: c.ID(), VehicleID: v.ID(), Status: entities.StatusFinished, CreatedAt: now, UpdatedAt: now, StartedAt: &started1, FinishedAt: &finished1})
	so2 := entities.ReconstituteServiceOrder(entities.ServiceOrderSnapshot{RequesterID: c.ID(), VehicleID: v.ID(), Status: entities.StatusFinished, CreatedAt: now, UpdatedAt: now, StartedAt: &started2, FinishedAt: &finished2})
	so3 := entities.ReconstituteServiceOrder(entities.ServiceOrderSnapshot{RequesterID: c.ID(), VehicleID: v.ID(), Status: entities.StatusReceived, CreatedAt: now, UpdatedAt: now})

	for _, so := range []*entities.ServiceOrder{so1, so2, so3} {
		if err := repo.Create(context.Background(), so); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		t.Cleanup(func() { testDB.Delete(&pgmodel.ServiceOrder{}, "id = ?", so.ID()) })
	}

	avg, err := repo.AverageExecutionTime(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := 2 * time.Hour
	delta := avg - expected
	if delta < 0 {
		delta = -delta
	}
	if delta > time.Second {
		t.Errorf("media = %v, esperava ~%v (delta %v)", avg, expected, delta)
	}
}

func TestServiceOrderRepository_FindByCode(t *testing.T) {
	c, v := seedVehicle(t, "OS00010")
	repo := NewServiceOrderRepository(testDB)
	so := newTestOrder(c.ID(), v.ID())
	if err := repo.Create(context.Background(), so); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Cleanup(func() { testDB.Delete(&pgmodel.ServiceOrder{}, "id = ?", so.ID()) })

	found, err := repo.FindByCode(context.Background(), so.Code())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID() != so.ID() {
		t.Errorf("expected ID %s, got %s", so.ID(), found.ID())
	}
}

func TestServiceOrderRepository_FindByCode_NotFound(t *testing.T) {
	repo := NewServiceOrderRepository(testDB)
	_, err := repo.FindByCode(context.Background(), 999999999)
	if !isDomainNotFound(err) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestServiceOrderRepository_AverageExecutionTime_SemAmostras(t *testing.T) {
	repo := NewServiceOrderRepository(testDB)

	testDB.Exec("DELETE FROM service_orders WHERE started_at IS NOT NULL AND finished_at IS NOT NULL")

	avg, err := repo.AverageExecutionTime(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if avg != 0 {
		t.Errorf("esperava 0 sem amostras, obteve %v", avg)
	}
}

// seedPart cria uma peca com o estoque informado e cadastra cleanup.
func seedPart(t *testing.T, code string, stock int) *entities.Part {
	t.Helper()
	repo := NewPartRepository(testDB)
	p := entities.NewPart(code, "Peca de teste "+code, "Desc", "un", 10.00, stock)
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("seedPart(%s): %v", code, err)
	}
	t.Cleanup(func() { testDB.Delete(&pgmodel.Part{}, "id = ?", p.ID()) })
	return p
}

func partStock(t *testing.T, id string) int {
	t.Helper()
	var m pgmodel.Part
	if err := testDB.First(&m, "id = ?", id).Error; err != nil {
		t.Fatalf("partStock(%s): %v", id, err)
	}
	return m.Stock
}

func TestServiceOrderRepository_Update_OptimisticLock(t *testing.T) {
	c, v := seedVehicle(t, "OS00011")
	repo := NewServiceOrderRepository(testDB)
	so := newTestOrder(c.ID(), v.ID())
	if err := repo.Create(context.Background(), so); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Delete(&pgmodel.ServiceOrder{}, "id = ?", so.ID()) })

	a, _ := repo.FindByID(context.Background(), so.ID())
	b, _ := repo.FindByID(context.Background(), so.ID())
	if err := a.TransitionTo(entities.StatusInDiagnosis); err != nil {
		t.Fatal(err)
	}
	a.SetServices([]entities.ServiceItem{{ServiceID: "s1", Code: 7, Description: "Alinhamento", Price: 80}})
	if err := repo.Update(context.Background(), a); err != nil {
		t.Fatalf("primeira escrita: %v", err)
	}
	if a.Version() != b.Version()+1 {
		t.Fatalf("versão não incrementada: %d", a.Version())
	}
	if err := b.Cancel("x"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(context.Background(), b); !errors.Is(err, domainerrors.ErrConcurrentUpdate) {
		t.Fatalf("escrita com versão velha deveria falhar com ErrConcurrentUpdate, veio %v", err)
	}
	got, _ := repo.FindByID(context.Background(), so.ID())
	if got.Status() != entities.StatusInDiagnosis || got.TotalAmount() != 80 || got.Services()[0].Code != 7 {
		t.Fatalf("estado persistido incorreto: %s %.2f", got.Status(), got.TotalAmount())
	}

	ghost := entities.ReconstituteServiceOrder(entities.ServiceOrderSnapshot{ID: "00000000-0000-0000-0000-000000000000", Version: 1})
	if err := repo.Update(context.Background(), ghost); !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatalf("OS inexistente: esperava ErrNotFound, veio %v", err)
	}
}

func TestPartRepository_ReserveStock(t *testing.T) {
	p := seedPart(t, "RSV-001", 3)
	repo := NewPartRepository(testDB)
	if err := repo.ReserveStock(context.Background(), p.ID(), 2); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReserveStock(context.Background(), p.ID(), 2); !errors.Is(err, domainerrors.ErrInsufficientStock) {
		t.Fatalf("esperava ErrInsufficientStock, veio %v", err)
	}
	if partStock(t, p.ID()) != 1 {
		t.Fatalf("estoque = %d", partStock(t, p.ID()))
	}
}
