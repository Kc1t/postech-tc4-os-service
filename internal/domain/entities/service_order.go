package entities

import (
	"time"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
)

type OrderStatus string

// Status da Ordem de Serviço. Na Fase 4 as transições entre recebimento e finalização são
// dirigidas pela saga (eventos do execution-service e do billing-service); a oficina só entrega
// ou cancela diretamente.
const (
	StatusReceived         OrderStatus = "received"          // Recebida
	StatusInDiagnosis      OrderStatus = "in_diagnosis"      // Em diagnóstico
	StatusAwaitingBudget   OrderStatus = "awaiting_budget"   // Diagnosticada, orçamento em geração
	StatusAwaitingApproval OrderStatus = "awaiting_approval" // Aguardando aprovação do cliente
	StatusAwaitingPayment  OrderStatus = "awaiting_payment"  // Aprovada, aguardando pagamento
	StatusInExecution      OrderStatus = "in_execution"      // Paga, em execução
	StatusFinished         OrderStatus = "finished"          // Finalizada
	StatusDelivered        OrderStatus = "delivered"         // Entregue
	StatusCancelled        OrderStatus = "cancelled"         // Cancelada (compensação da saga ou oficina)
)

// validTransitions define a máquina de estados da OS. received → awaiting_budget é aceita
// porque o SNS/SQS não garante ordem: DiagnosisCompleted pode chegar antes de DiagnosisStarted.
var validTransitions = map[OrderStatus][]OrderStatus{
	StatusReceived:         {StatusInDiagnosis, StatusAwaitingBudget, StatusCancelled},
	StatusInDiagnosis:      {StatusAwaitingBudget, StatusCancelled},
	StatusAwaitingBudget:   {StatusAwaitingApproval, StatusCancelled},
	StatusAwaitingApproval: {StatusAwaitingPayment, StatusCancelled},
	StatusAwaitingPayment:  {StatusInExecution, StatusCancelled},
	StatusInExecution:      {StatusFinished, StatusCancelled},
	StatusFinished:         {StatusDelivered},
	StatusDelivered:        {},
	StatusCancelled:        {},
}

// IsValidStatus verifica se o status informado é um valor conhecido.
func IsValidStatus(s OrderStatus) bool {
	_, ok := validTransitions[s]
	return ok
}

// IsFinal indica status que não mudam mais.
func (s OrderStatus) IsFinal() bool {
	return s == StatusDelivered || s == StatusCancelled
}

// ServiceOrderInput carrega os dados necessários para abrir uma nova OS.
type ServiceOrderInput struct {
	RequesterDocument string
	VehiclePlate      string
	Notes             string
	// SimulateFailure força uma falha numa etapa da saga (demonstração); ignorado quando a
	// injeção de falhas está desligada.
	SimulateFailure string
}

// ServiceItem e PartItem são value objects — identificados por valor, sem identidade própria.
type ServiceItem struct {
	ServiceID   string
	Code        int
	Description string
	Price       float64
}

type PartItem struct {
	PartID           string
	ManufacturerCode string
	Description      string
	Quantity         int
	UnitPrice        float64
}

type ServiceOrder struct {
	id           string
	code         int
	requesterID  string
	vehicleID    string
	status       OrderStatus
	services     []ServiceItem
	parts        []PartItem
	totalAmount  float64
	notes        string
	cancelReason string
	version      int
	createdAt    time.Time
	updatedAt    time.Time
	startedAt    *time.Time
	finishedAt   *time.Time
}

func NewServiceOrder(requesterID, vehicleID string) *ServiceOrder {
	now := time.Now()
	return &ServiceOrder{
		requesterID: requesterID,
		vehicleID:   vehicleID,
		status:      StatusReceived,
		services:    []ServiceItem{},
		parts:       []PartItem{},
		createdAt:   now,
		updatedAt:   now,
	}
}

// ServiceOrderSnapshot agrupa os campos persistidos, para reconstituir a entidade.
type ServiceOrderSnapshot struct {
	ID           string
	Code         int
	RequesterID  string
	VehicleID    string
	Status       OrderStatus
	Services     []ServiceItem
	Parts        []PartItem
	TotalAmount  float64
	Notes        string
	CancelReason string
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

// ReconstituteServiceOrder restaura uma entidade a partir de dados persistidos (uso exclusivo de repositories).
func ReconstituteServiceOrder(s ServiceOrderSnapshot) *ServiceOrder {
	return &ServiceOrder{
		id:           s.ID,
		code:         s.Code,
		requesterID:  s.RequesterID,
		vehicleID:    s.VehicleID,
		status:       s.Status,
		services:     s.Services,
		parts:        s.Parts,
		totalAmount:  s.TotalAmount,
		notes:        s.Notes,
		cancelReason: s.CancelReason,
		version:      s.Version,
		createdAt:    s.CreatedAt,
		updatedAt:    s.UpdatedAt,
		startedAt:    s.StartedAt,
		finishedAt:   s.FinishedAt,
	}
}

func (so *ServiceOrder) ID() string              { return so.id }
func (so *ServiceOrder) Code() int               { return so.code }
func (so *ServiceOrder) RequesterID() string     { return so.requesterID }
func (so *ServiceOrder) VehicleID() string       { return so.vehicleID }
func (so *ServiceOrder) Status() OrderStatus     { return so.status }
func (so *ServiceOrder) Services() []ServiceItem { return so.services }
func (so *ServiceOrder) Parts() []PartItem       { return so.parts }
func (so *ServiceOrder) TotalAmount() float64    { return so.totalAmount }
func (so *ServiceOrder) Notes() string           { return so.notes }
func (so *ServiceOrder) CancelReason() string    { return so.cancelReason }
func (so *ServiceOrder) Version() int            { return so.version }
func (so *ServiceOrder) CreatedAt() time.Time    { return so.createdAt }
func (so *ServiceOrder) UpdatedAt() time.Time    { return so.updatedAt }
func (so *ServiceOrder) StartedAt() *time.Time   { return so.startedAt }
func (so *ServiceOrder) FinishedAt() *time.Time  { return so.finishedAt }

func (so *ServiceOrder) SetID(id string)   { so.id = id }
func (so *ServiceOrder) SetCode(code int)  { so.code = code }
func (so *ServiceOrder) SetVersion(v int)  { so.version = v }
func (so *ServiceOrder) SetNotes(n string) { so.notes = n; so.touch() }

// TransitionTo muda o status respeitando a máquina de estados.
func (so *ServiceOrder) TransitionTo(s OrderStatus) error {
	if !IsValidStatus(s) {
		return domainerrors.ErrInvalidStatusValue
	}
	for _, allowed := range validTransitions[so.status] {
		if allowed == s {
			so.status = s
			so.touch()
			return nil
		}
	}
	return domainerrors.ErrInvalidStatus
}

// Cancel encerra a OS com o motivo informado. Só OS ainda em andamento podem ser canceladas.
func (so *ServiceOrder) Cancel(reason string) error {
	if so.status == StatusFinished || so.status.IsFinal() {
		return domainerrors.ErrOrderNotCancellable
	}
	if err := so.TransitionTo(StatusCancelled); err != nil {
		return err
	}
	so.cancelReason = reason
	return nil
}

// MarkRepairStarted registra o início do reparo (para a métrica de tempo de execução).
func (so *ServiceOrder) MarkRepairStarted(at time.Time) {
	if so.startedAt == nil {
		so.startedAt = &at
		so.touch()
	}
}

// Finish conclui o reparo.
func (so *ServiceOrder) Finish(at time.Time) error {
	if err := so.TransitionTo(StatusFinished); err != nil {
		return err
	}
	if so.startedAt == nil {
		so.startedAt = &at
	}
	so.finishedAt = &at
	return nil
}

// SetServices substitui a lista de serviços e recalcula o total.
func (so *ServiceOrder) SetServices(items []ServiceItem) {
	so.services = items
	so.recalcTotal()
}

// SetParts substitui a lista de peças e recalcula o total.
func (so *ServiceOrder) SetParts(items []PartItem) {
	so.parts = items
	so.recalcTotal()
}

func (so *ServiceOrder) recalcTotal() {
	total := 0.0
	for _, s := range so.services {
		total += s.Price
	}
	for _, p := range so.parts {
		total += float64(p.Quantity) * p.UnitPrice
	}
	so.totalAmount = total
	so.touch()
}

func (so *ServiceOrder) touch() { so.updatedAt = time.Now() }
