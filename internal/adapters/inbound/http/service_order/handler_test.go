package serviceorderhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports/mocks"
	"github.com/gin-gonic/gin"
	"go.uber.org/mock/gomock"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// Simula o middleware Auth com um token de admin: as regras de posse do cliente
// ficam cobertas em ownership_test.go.
func newTestRouter(h *ServiceOrderHandler) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_role", string(entities.RoleAdmin))
		c.Next()
	})
	h.SetupRoutes(r.Group(""))
	h.SetupRequesterRoutes(r.Group(""))
	return r
}

type handlerMocks struct {
	handler              *ServiceOrderHandler
	create               *mocks.MockCreateServiceOrderUseCase
	getByID              *mocks.MockGetServiceOrderUseCase
	getByCode            *mocks.MockGetServiceOrderByCodeUseCase
	listAll              *mocks.MockListServiceOrdersUseCase
	listByDocument       *mocks.MockListServiceOrdersByDocumentUseCase
	cancel               *mocks.MockCancelServiceOrderUseCase
	deliver              *mocks.MockDeliverServiceOrderUseCase
	history              *mocks.MockGetServiceOrderHistoryUseCase
	details              *mocks.MockGetServiceOrderDetailsUseCase
	averageExecutionTime *mocks.MockGetAverageExecutionTimeUseCase
}

func newHandler(ctrl *gomock.Controller) handlerMocks {
	m := handlerMocks{
		create:               mocks.NewMockCreateServiceOrderUseCase(ctrl),
		getByID:              mocks.NewMockGetServiceOrderUseCase(ctrl),
		getByCode:            mocks.NewMockGetServiceOrderByCodeUseCase(ctrl),
		listAll:              mocks.NewMockListServiceOrdersUseCase(ctrl),
		listByDocument:       mocks.NewMockListServiceOrdersByDocumentUseCase(ctrl),
		cancel:               mocks.NewMockCancelServiceOrderUseCase(ctrl),
		deliver:              mocks.NewMockDeliverServiceOrderUseCase(ctrl),
		history:              mocks.NewMockGetServiceOrderHistoryUseCase(ctrl),
		details:              mocks.NewMockGetServiceOrderDetailsUseCase(ctrl),
		averageExecutionTime: mocks.NewMockGetAverageExecutionTimeUseCase(ctrl),
	}
	m.handler = NewServiceOrderHandler(Deps{
		Create: m.create, GetByID: m.getByID, GetByCode: m.getByCode, ListAll: m.listAll,
		ListByDocument: m.listByDocument, Cancel: m.cancel, Deliver: m.deliver, History: m.history,
		Details: m.details, AverageExecutionTime: m.averageExecutionTime,
	})
	return m
}

// =============================================================================
// Create
// =============================================================================

func TestServiceOrderHandler_Create_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	so := entities.NewServiceOrder("cust-id", "veh-id")
	m.create.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(so, nil)

	body, _ := json.Marshal(map[string]any{
		"requester_document": "52998224725",
		"vehicle_plate":      "ABC1234",
		"notes":              "trocar pastilhas",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/service-orders", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceOrderHandler_Create_BadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/service-orders", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServiceOrderHandler_Create_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	m.create.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, domainerrors.ErrNotFound)

	body, _ := json.Marshal(map[string]any{
		"requester_document": "52998224725",
		"vehicle_plate":      "ABC1234",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/service-orders", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestServiceOrderHandler_Create_VehicleNotFromRequester(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	m.create.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, domainerrors.ErrVehicleNotFromRequester)

	body, _ := json.Marshal(map[string]any{
		"requester_document": "52998224725",
		"vehicle_plate":      "ABC1234",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/service-orders", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestServiceOrderHandler_Create_InternalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	m.create.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))

	body, _ := json.Marshal(map[string]any{
		"requester_document": "52998224725",
		"vehicle_plate":      "ABC1234",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/service-orders", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// =============================================================================
// FindByID
// =============================================================================

func TestServiceOrderHandler_FindByID_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	so := entities.NewServiceOrder("cust-id", "veh-id")
	m.getByID.EXPECT().Execute(gomock.Any(), "so-1").Return(so, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders/so-1", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestServiceOrderHandler_FindByID_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	m.getByID.EXPECT().Execute(gomock.Any(), "so-x").Return(nil, domainerrors.ErrNotFound)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders/so-x", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// =============================================================================
// FindAll
// =============================================================================

func TestServiceOrderHandler_FindAll_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	orders := []*entities.ServiceOrder{entities.NewServiceOrder("c1", "v1")}
	m.listAll.EXPECT().Execute(gomock.Any()).Return(orders, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestServiceOrderHandler_FindAll_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	m.listAll.EXPECT().Execute(gomock.Any()).Return(nil, errors.New("db error"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// =============================================================================
// FindByCode (rota publica)
// =============================================================================

func TestServiceOrderHandler_FindByCode_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	so := entities.NewServiceOrder("cust-id", "veh-id")
	m.getByCode.EXPECT().Execute(gomock.Any(), 100, "52998224725").Return(so, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders/code/100?document=52998224725", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceOrderHandler_FindByCode_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	m.getByCode.EXPECT().Execute(gomock.Any(), 999, "52998224725").Return(nil, domainerrors.ErrNotFound)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders/code/999?document=52998224725", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestServiceOrderHandler_FindByCode_SemDocument(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders/code/100", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServiceOrderHandler_FindByCode_CodigoInvalido(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders/code/abc?document=52998224725", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// =============================================================================
// FindByDocument (rota publica)
// =============================================================================

func TestServiceOrderHandler_FindByDocument_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	orders := []*entities.ServiceOrder{entities.NewServiceOrder("c1", "v1")}
	m.listByDocument.EXPECT().Execute(gomock.Any(), "52998224725").Return(orders, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders/requester?document=52998224725", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceOrderHandler_FindByDocument_SemDocument(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders/requester", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestServiceOrderHandler_FindByDocument_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := newHandler(ctrl)
	m.listByDocument.EXPECT().Execute(gomock.Any(), "52998224725").Return(nil, domainerrors.ErrNotFound)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/service-orders/requester?document=52998224725", nil)
	newTestRouter(m.handler).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// =============================================================================
// Ciclo de vida dirigido pela saga
// =============================================================================

func TestServiceOrderHandler_Create_RepassaFalhaSimulada(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := newHandler(ctrl)
	m.create.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in entities.ServiceOrderInput) (*entities.ServiceOrder, error) {
			if in.SimulateFailure != "payment" {
				t.Errorf("SimulateFailure = %q", in.SimulateFailure)
			}
			return entities.NewServiceOrder("c", "v"), nil
		})
	body, _ := json.Marshal(map[string]any{"requester_document": "52998224725", "vehicle_plate": "ABC1234"})
	req := httptest.NewRequest(http.MethodPost, "/service-orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SimulateFailureHeader, "payment")
	w := httptest.NewRecorder()
	newTestRouter(m.handler).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("esperava 201, veio %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceOrderHandler_Cancel(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		err    error
		status int
	}{
		{"com motivo", `{"reason":"cliente desistiu"}`, nil, http.StatusNoContent},
		{"sem corpo", ``, nil, http.StatusNoContent},
		{"finalizada", `{"reason":"x"}`, domainerrors.ErrOrderNotCancellable, http.StatusConflict},
		{"inexistente", ``, domainerrors.ErrNotFound, http.StatusNotFound},
		{"json inválido", `{`, nil, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := newHandler(ctrl)
			if tc.status != http.StatusBadRequest {
				m.cancel.EXPECT().Execute(gomock.Any(), "os-1", gomock.Any()).Return(tc.err)
			}
			req := httptest.NewRequest(http.MethodPost, "/service-orders/os-1/cancel", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			newTestRouter(m.handler).ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("esperava %d, veio %d: %s", tc.status, w.Code, w.Body.String())
			}
		})
	}
}

func TestServiceOrderHandler_Deliver(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{nil, http.StatusNoContent},
		{domainerrors.ErrInvalidStatus, http.StatusConflict},
	} {
		ctrl := gomock.NewController(t)
		m := newHandler(ctrl)
		m.deliver.EXPECT().Execute(gomock.Any(), "os-1").Return(tc.err)
		w := httptest.NewRecorder()
		newTestRouter(m.handler).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/service-orders/os-1/deliver", nil))
		if w.Code != tc.status {
			t.Fatalf("esperava %d, veio %d", tc.status, w.Code)
		}
	}
}

func TestServiceOrderHandler_History(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := newHandler(ctrl)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	m.history.EXPECT().Execute(gomock.Any(), "os-1").Return([]*entities.StatusHistoryEntry{
		{To: entities.StatusReceived, Actor: entities.ActorStaff, CreatedAt: at},
		{From: entities.StatusReceived, To: entities.StatusInDiagnosis, Actor: entities.ActorSaga, MessageID: "m1", CreatedAt: at},
	}, nil)
	w := httptest.NewRecorder()
	newTestRouter(m.handler).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/service-orders/os-1/history", nil))
	var got []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != http.StatusOK || len(got) != 2 || got[1]["from"] != "received" || got[1]["at"] != "2026-01-02T03:04:05Z" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}

	m.history.EXPECT().Execute(gomock.Any(), "x").Return(nil, domainerrors.ErrNotFound)
	w = httptest.NewRecorder()
	newTestRouter(m.handler).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/service-orders/x/history", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("esperava 404, veio %d", w.Code)
	}
}

func TestServiceOrderHandler_Details(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := newHandler(ctrl)
	deadline := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	m.details.EXPECT().Execute(gomock.Any(), "os-1").Return(&ports.ServiceOrderDetails{
		Order:       entities.NewServiceOrder("c", "v"),
		Saga:        &entities.Saga{ID: "s1", State: entities.SagaAwaitingApproval, BudgetID: "b1", DeadlineAt: &deadline},
		Budget:      json.RawMessage(`{"id":"b1","status":"pending_approval"}`),
		Unavailable: []string{"execution"},
	}, nil)
	w := httptest.NewRecorder()
	newTestRouter(m.handler).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/service-orders/os-1/details", nil))
	var got struct {
		Saga        map[string]any `json:"saga"`
		Budget      map[string]any `json:"budget"`
		Execution   any            `json:"execution"`
		Partial     bool           `json:"partial"`
		Unavailable []string       `json:"unavailable"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if got.Budget["id"] != "b1" || got.Execution != nil || !got.Partial || got.Unavailable[0] != "execution" ||
		got.Saga["state"] != "awaiting_approval" || got.Saga["deadline_at"] != "2026-01-05T00:00:00Z" {
		t.Fatalf("composição inesperada: %s", w.Body.String())
	}

	m.details.EXPECT().Execute(gomock.Any(), "x").Return(nil, domainerrors.ErrNotFound)
	w = httptest.NewRecorder()
	newTestRouter(m.handler).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/service-orders/x/details", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("esperava 404, veio %d", w.Code)
	}
}
