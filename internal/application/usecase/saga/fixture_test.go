package sagauc

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/Kc1t/postech-tc4-os-service/pkg/outbox"
	"github.com/google/uuid"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Os testes do orquestrador rodam contra Postgres real (TEST_DB_*): o que está em jogo é a
// atomicidade de cada passo (saga + OS + histórico + estoque + outbox na mesma transação).

var testDB *gorm.DB

func TestMain(m *testing.M) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		env("TEST_DB_HOST", "localhost"), env("TEST_DB_PORT", "5432"), env("TEST_DB_USER", "postgres"),
		env("TEST_DB_PASSWORD", "postgres"), env("TEST_DB_NAME", "workshop"))
	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatalf("banco de teste: %v", err)
	}
	if err := postgresql.Migrate(db); err != nil {
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

var routes = postgresql.Routes{
	BillingCommandsQueueURL:   "http://sqs/billing-commands",
	ExecutionCommandsQueueURL: "http://sqs/execution-commands",
	OSEventsTopicARN:          "arn:aws:sns:us-east-1:000000000000:os-events",
}

// tb é o que o fixture usa do *testing.T. O BDD (godog) passa um adaptador do T do cenário.
type tb interface {
	Helper()
	Fatal(args ...any)
	Cleanup(func())
}

type fx struct {
	t       tb
	ctx     context.Context
	now     time.Time
	o       *Orchestrator
	create  *CreateServiceOrder
	reply   *HandleReply
	cancel  *CancelServiceOrder
	deliver *DeliverServiceOrder
	expire  *ExpireSagas

	requester *entities.Requester
	vehicle   *entities.Vehicle
	service   *entities.Service
	part      *entities.Part
	sagas     []string
}

func newFx(t tb, cfg Config) *fx {
	t.Helper()
	f := &fx{t: t, ctx: context.Background(), now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	f.o = NewOrchestrator(Deps{
		Tx:         postgresql.NewTxManager(testDB),
		Inbox:      postgresql.NewInbox(testDB, "os-saga-test-"+uuid.NewString()[:8]),
		Sagas:      postgresql.NewSagaRepository(testDB),
		Orders:     postgresql.NewServiceOrderRepository(testDB),
		History:    postgresql.NewStatusHistoryRepository(testDB),
		Requesters: postgresql.NewRequesterRepository(testDB),
		Vehicles:   postgresql.NewVehicleRepository(testDB),
		Services:   postgresql.NewServiceRepository(testDB),
		Parts:      postgresql.NewPartRepository(testDB),
		Messenger:  postgresql.NewOutboxMessenger(routes),
		Now:        func() time.Time { return f.now },
	}, cfg)
	f.create = NewCreateServiceOrder(f.o)
	f.reply = NewHandleReply(f.o)
	f.cancel = NewCancelServiceOrder(f.o)
	f.deliver = NewDeliverServiceOrder(f.o)
	f.expire = NewExpireSagas(f.o)
	f.seedCatalog(5)
	return f
}

func (f *fx) seedCatalog(stock int) {
	f.t.Helper()
	now := time.Now()
	suffix := rand.IntN(900000) + 100000
	f.requester = entities.ReconstituteRequester("", "Cliente Saga", fmt.Sprintf("%011d", suffix),
		"cliente@example.com", "11999990000", now, now)
	must(f.t, f.o.d.Requesters.Create(f.ctx, f.requester))
	f.vehicle = entities.ReconstituteVehicle("", f.requester.ID(), fmt.Sprintf("S%06d", suffix), "Fiat", "Argo", 2022, now, now)
	must(f.t, f.o.d.Vehicles.Create(f.ctx, f.vehicle))
	f.service = entities.NewService(suffix, "Troca de pastilhas", "", 150, 60)
	must(f.t, f.o.d.Services.Create(f.ctx, f.service))
	f.part = entities.NewPart(fmt.Sprintf("PAS-%d", suffix), "Pastilha dianteira", "", "par", 90, stock)
	must(f.t, f.o.d.Parts.Create(f.ctx, f.part))
	f.t.Cleanup(func() {
		for _, s := range f.sagas {
			testDB.Exec("DELETE FROM outbox_messages WHERE saga_id = ?", s)
		}
		testDB.Exec("DELETE FROM service_order_status_history WHERE os_id IN (SELECT id FROM service_orders WHERE vehicle_id = ?)", f.vehicle.ID())
		testDB.Exec("DELETE FROM saga_instances WHERE os_id IN (SELECT id FROM service_orders WHERE vehicle_id = ?)", f.vehicle.ID())
		testDB.Exec("DELETE FROM service_orders WHERE vehicle_id = ?", f.vehicle.ID())
		testDB.Exec("DELETE FROM vehicles WHERE id = ?", f.vehicle.ID())
		testDB.Exec("DELETE FROM requesters WHERE id = ?", f.requester.ID())
		testDB.Exec("DELETE FROM services WHERE id = ?", f.service.ID())
		testDB.Exec("DELETE FROM parts WHERE id = ?", f.part.ID())
	})
}

func must(t tb, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// open abre uma OS pelo caso de uso real.
func (f *fx) open(simulate string) *entities.ServiceOrder {
	f.t.Helper()
	so, err := f.create.Execute(f.ctx, entities.ServiceOrderInput{
		RequesterDocument: f.requester.Document(), VehiclePlate: f.vehicle.Plate(), Notes: "barulho ao frear",
		SimulateFailure: simulate,
	})
	must(f.t, err)
	f.sagas = append(f.sagas, f.saga(so.ID()).ID)
	return so
}

// send entrega uma resposta como se viesse da fila os-saga-replies.
func (f *fx) send(osID, msgType string, payload any) contracts.Envelope {
	f.t.Helper()
	saga := f.saga(osID)
	e, err := contracts.NewEnvelope(msgType, "teste", contracts.Meta{SagaID: saga.ID, OSID: osID}, payload)
	must(f.t, err)
	must(f.t, f.reply.Execute(f.ctx, e))
	return e
}

func (f *fx) saga(osID string) *entities.Saga {
	f.t.Helper()
	s, err := f.o.d.Sagas.FindByOSID(f.ctx, osID)
	must(f.t, err)
	return s
}

func (f *fx) order(osID string) *entities.ServiceOrder {
	f.t.Helper()
	so, err := f.o.d.Orders.FindByID(f.ctx, osID)
	must(f.t, err)
	return so
}

func (f *fx) stock() int {
	f.t.Helper()
	p, err := f.o.d.Parts.FindByID(f.ctx, f.part.ID())
	must(f.t, err)
	return p.Stock()
}

// sent devolve as mensagens gravadas no outbox para a saga da OS, do tipo indicado.
func (f *fx) sent(osID, msgType string) []contracts.Envelope {
	f.t.Helper()
	var rows []outbox.Message
	must(f.t, testDB.Where("saga_id = ? AND type = ?", f.saga(osID).ID, msgType).Order("created_at").Find(&rows).Error)
	out := make([]contracts.Envelope, 0, len(rows))
	for _, r := range rows {
		e, err := contracts.Parse([]byte(r.Body))
		must(f.t, err)
		out = append(out, e)
	}
	return out
}

func (f *fx) statuses(osID string) []entities.OrderStatus {
	f.t.Helper()
	h, err := f.o.d.History.ListByOS(f.ctx, osID)
	must(f.t, err)
	out := make([]entities.OrderStatus, 0, len(h))
	for _, e := range h {
		out = append(out, e.To)
	}
	return out
}

func decodeAs[T any](t tb, e contracts.Envelope) T {
	t.Helper()
	v, err := contracts.Decode[T](e)
	must(t, err)
	return v
}

func (f *fx) diagnose(osID string, partQty int) {
	f.t.Helper()
	f.send(osID, contracts.EvtDiagnosisStarted, contracts.DiagnosisStarted{StartedAt: f.now})
	f.send(osID, contracts.EvtDiagnosisCompleted, contracts.DiagnosisCompleted{
		Services: []contracts.DiagnosedService{{Code: f.service.Code()}},
		Parts:    []contracts.DiagnosedPart{{ManufacturerCode: f.part.ManufacturerCode(), Quantity: partQty}},
	})
}

func (f *fx) toAwaitingPayment(osID string) {
	f.t.Helper()
	f.diagnose(osID, 2)
	f.send(osID, contracts.EvtBudgetCreated, contracts.BudgetCreated{BudgetID: "budget-" + osID[:8], Total: 330})
	f.send(osID, contracts.EvtBudgetApproved, contracts.BudgetApproved{BudgetID: "budget-" + osID[:8], ApprovedAt: f.now})
}

func (f *fx) toExecuting(osID string) {
	f.t.Helper()
	f.toAwaitingPayment(osID)
	f.send(osID, contracts.EvtPaymentLinkCreated, contracts.PaymentLinkCreated{
		BudgetID: "budget-" + osID[:8], PaymentID: "pay-1", CheckoutURL: "https://mp.test/checkout/1"})
	f.send(osID, contracts.EvtPaymentApproved, contracts.PaymentApproved{
		BudgetID: "budget-" + osID[:8], PaymentID: "pay-1", ProviderPaymentID: "123", Amount: 330})
}
