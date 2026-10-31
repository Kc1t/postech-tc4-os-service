package serviceorderuc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports/mocks"
	"github.com/Kc1t/postech-tc4-os-service/pkg/httpclient"
	"go.uber.org/mock/gomock"
)

type detailsMocks struct {
	orders    *mocks.MockServiceOrderRepository
	sagas     *mocks.MockSagaRepository
	history   *mocks.MockStatusHistoryRepository
	billing   *mocks.MockBudgetQuery
	execution *mocks.MockExecutionQuery
	uc        *GetServiceOrderDetails
}

func newDetails(t *testing.T) detailsMocks {
	ctrl := gomock.NewController(t)
	m := detailsMocks{
		orders: mocks.NewMockServiceOrderRepository(ctrl), sagas: mocks.NewMockSagaRepository(ctrl),
		history: mocks.NewMockStatusHistoryRepository(ctrl), billing: mocks.NewMockBudgetQuery(ctrl),
		execution: mocks.NewMockExecutionQuery(ctrl),
	}
	m.uc = NewGetServiceOrderDetails(m.orders, m.sagas, m.history, m.billing, m.execution)
	return m
}

func (m detailsMocks) local(id string) {
	m.orders.EXPECT().FindByID(gomock.Any(), id).Return(entities.NewServiceOrder("c", "v"), nil)
	m.sagas.EXPECT().FindByOSID(gomock.Any(), id).Return(&entities.Saga{ID: "s1"}, nil)
	m.history.EXPECT().ListByOS(gomock.Any(), id).Return([]*entities.StatusHistoryEntry{{To: entities.StatusReceived}}, nil)
}

func TestDetails_ComposicaoCompleta(t *testing.T) {
	m := newDetails(t)
	m.local("os-1")
	m.billing.EXPECT().BudgetByOS(gomock.Any(), "os-1").Return(json.RawMessage(`{"id":"b1"}`), nil)
	m.execution.EXPECT().ExecutionByOS(gomock.Any(), "os-1").Return(json.RawMessage(`{"status":"in_repair"}`), nil)
	d, err := m.uc.Execute(context.Background(), "os-1")
	if err != nil || string(d.Budget) != `{"id":"b1"}` || string(d.Execution) != `{"status":"in_repair"}` ||
		len(d.Unavailable) != 0 || d.Saga.ID != "s1" || len(d.History) != 1 {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestDetails_RespostaParcial(t *testing.T) {
	m := newDetails(t)
	m.local("os-1")
	m.billing.EXPECT().BudgetByOS(gomock.Any(), "os-1").Return(nil, httpclient.ErrCircuitOpen)
	m.execution.EXPECT().ExecutionByOS(gomock.Any(), "os-1").Return(nil, domainerrors.ErrNotFound)
	d, err := m.uc.Execute(context.Background(), "os-1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Budget != nil || d.Execution != nil || len(d.Unavailable) != 1 || d.Unavailable[0] != SourceBilling {
		t.Fatalf("parcial: %+v", d)
	}
}

func TestDetails_ErrosLocais(t *testing.T) {
	m := newDetails(t)
	m.orders.EXPECT().FindByID(gomock.Any(), "x").Return(nil, domainerrors.ErrNotFound)
	if _, err := m.uc.Execute(context.Background(), "x"); !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatal(err)
	}

	boom := errors.New("db fora")
	m.orders.EXPECT().FindByID(gomock.Any(), "y").Return(entities.NewServiceOrder("c", "v"), nil).Times(2)
	m.sagas.EXPECT().FindByOSID(gomock.Any(), "y").Return(nil, boom)
	if _, err := m.uc.Execute(context.Background(), "y"); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	m.sagas.EXPECT().FindByOSID(gomock.Any(), "y").Return(nil, domainerrors.ErrNotFound)
	m.history.EXPECT().ListByOS(gomock.Any(), "y").Return(nil, boom)
	if _, err := m.uc.Execute(context.Background(), "y"); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestHistory(t *testing.T) {
	ctrl := gomock.NewController(t)
	orders := mocks.NewMockServiceOrderRepository(ctrl)
	history := mocks.NewMockStatusHistoryRepository(ctrl)
	uc := NewGetServiceOrderHistory(orders, history)

	orders.EXPECT().FindByID(gomock.Any(), "os-1").Return(entities.NewServiceOrder("c", "v"), nil)
	history.EXPECT().ListByOS(gomock.Any(), "os-1").Return([]*entities.StatusHistoryEntry{{}, {}}, nil)
	if got, err := uc.Execute(context.Background(), "os-1"); err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	orders.EXPECT().FindByID(gomock.Any(), "x").Return(nil, domainerrors.ErrNotFound)
	if _, err := uc.Execute(context.Background(), "x"); !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatal(err)
	}
}
