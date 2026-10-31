// Package contracts define o contrato das mensagens trocadas entre os microsserviços da oficina.
//
// Este pacote é a fonte canônica: billing-service e execution-service carregam uma cópia literal
// dele (e dos JSON Schemas em ./schemas). A documentação navegável está em docs/asyncapi.yaml.
// Qualquer mudança aqui é uma mudança de contrato: incremente Version e atualize as cópias.
package contracts

import (
	"encoding/json"
	"time"
)

// Version é a versão do envelope e dos payloads. Consumidores rejeitam versões maiores que a sua.
const Version = 1

// Tipos de mensagem. O prefixo indica o serviço dono do contrato:
// comandos são nomeados pelo destinatário; eventos, pelo publicador.
const (
	// Comandos para o execution-service (fila execution-commands).
	CmdStartDiagnosis  = "execution.StartDiagnosis"
	CmdStartRepair     = "execution.StartRepair"
	CmdCancelExecution = "execution.CancelExecution"

	// Comandos para o billing-service (fila billing-commands).
	CmdCreateBudget   = "billing.CreateBudget"
	CmdRequestPayment = "billing.RequestPayment"
	CmdCancelBudget   = "billing.CancelBudget"
	CmdRefundPayment  = "billing.RefundPayment"

	// Eventos do execution-service (tópico execution-events).
	EvtDiagnosisStarted   = "execution.DiagnosisStarted"
	EvtDiagnosisCompleted = "execution.DiagnosisCompleted"
	EvtRepairStarted      = "execution.RepairStarted"
	EvtRepairFinished     = "execution.RepairFinished"
	EvtRepairRejected     = "execution.RepairRejected"
	EvtExecutionCancelled = "execution.ExecutionCancelled"

	// Eventos do billing-service (tópico billing-events).
	EvtBudgetCreated        = "billing.BudgetCreated"
	EvtBudgetCreationFailed = "billing.BudgetCreationFailed"
	EvtBudgetApproved       = "billing.BudgetApproved"
	EvtBudgetRejected       = "billing.BudgetRejected"
	EvtBudgetCancelled      = "billing.BudgetCancelled"
	EvtPaymentLinkCreated   = "billing.PaymentLinkCreated"
	EvtPaymentApproved      = "billing.PaymentApproved"
	EvtPaymentFailed        = "billing.PaymentFailed"
	EvtPaymentRefunded      = "billing.PaymentRefunded"

	// Eventos do os-service (tópico os-events).
	EvtServiceOrderStatusChanged = "os.ServiceOrderStatusChanged"
)

// Falhas simuladas aceitas quando FAULT_INJECTION_ENABLED=true. Servem para demonstrar as
// compensações da saga; em produção a flag fica desligada e o campo é ignorado.
const (
	FailBudget  = "budget"
	FailPayment = "payment"
	FailRepair  = "repair"
)

// Envelope é o formato de toda mensagem. O corpo SNS/SQS é o JSON do envelope; os campos de
// roteamento e o contexto de trace também vão como MessageAttributes.
type Envelope struct {
	MessageID     string    `json:"message_id"`
	Type          string    `json:"type"`
	Version       int       `json:"version"`
	SagaID        string    `json:"saga_id"`
	OSID          string    `json:"os_id"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	CausationID   string    `json:"causation_id,omitempty"`
	Producer      string    `json:"producer"`
	OccurredAt    time.Time `json:"occurred_at"`
	Payload       json.RawMessage `json:"payload"`
}

// Item de orçamento e de execução.
type Item struct {
	Kind        string  `json:"kind"` // "service" | "part"
	Ref         string  `json:"ref"`  // código do serviço ou código do fabricante da peça
	Description string  `json:"description"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

const (
	ItemKindService = "service"
	ItemKindPart    = "part"
)

type Vehicle struct {
	Plate string `json:"plate"`
	Brand string `json:"brand,omitempty"`
	Model string `json:"model,omitempty"`
	Year  int    `json:"year,omitempty"`
}

type Requester struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Document string `json:"document"`
}

// ---- Comandos para o execution-service ----

type StartDiagnosis struct {
	OSCode          int     `json:"os_code"`
	Vehicle         Vehicle `json:"vehicle"`
	Notes           string  `json:"notes,omitempty"`
	SimulateFailure string  `json:"simulate_failure,omitempty"`
}

type StartRepair struct {
	OSCode          int    `json:"os_code"`
	Items           []Item `json:"items"`
	SimulateFailure string `json:"simulate_failure,omitempty"`
}

type CancelExecution struct {
	Reason string `json:"reason"`
}

// ---- Comandos para o billing-service ----

type CreateBudget struct {
	OSCode          int       `json:"os_code"`
	Requester       Requester `json:"requester"`
	Items           []Item    `json:"items"`
	Total           float64   `json:"total"`
	SimulateFailure string    `json:"simulate_failure,omitempty"`
}

type RequestPayment struct {
	BudgetID        string `json:"budget_id"`
	SimulateFailure string `json:"simulate_failure,omitempty"`
}

type CancelBudget struct {
	BudgetID string `json:"budget_id"`
	Reason   string `json:"reason"`
}

type RefundPayment struct {
	BudgetID string `json:"budget_id"`
	Reason   string `json:"reason"`
}

// ---- Eventos do execution-service ----

type DiagnosisStarted struct {
	StartedAt time.Time `json:"started_at"`
}

type DiagnosedService struct {
	Code int `json:"code"`
}

type DiagnosedPart struct {
	ManufacturerCode string `json:"manufacturer_code"`
	Quantity         int    `json:"quantity"`
}

type DiagnosisCompleted struct {
	Services []DiagnosedService `json:"services"`
	Parts    []DiagnosedPart    `json:"parts"`
	Notes    string             `json:"notes,omitempty"`
}

type RepairStarted struct {
	StartedAt time.Time `json:"started_at"`
}

type RepairFinished struct {
	FinishedAt time.Time `json:"finished_at"`
}

type RepairRejected struct {
	Reason string `json:"reason"`
}

type ExecutionCancelled struct {
	Reason string `json:"reason"`
}

// ---- Eventos do billing-service ----

type BudgetCreated struct {
	BudgetID string  `json:"budget_id"`
	Total    float64 `json:"total"`
}

type BudgetCreationFailed struct {
	Reason string `json:"reason"`
}

type BudgetApproved struct {
	BudgetID   string    `json:"budget_id"`
	ApprovedAt time.Time `json:"approved_at"`
}

type BudgetRejected struct {
	BudgetID string `json:"budget_id"`
	Reason   string `json:"reason"`
}

type BudgetCancelled struct {
	BudgetID string `json:"budget_id"`
	Reason   string `json:"reason"`
}

type PaymentLinkCreated struct {
	BudgetID    string `json:"budget_id"`
	PaymentID   string `json:"payment_id"`
	CheckoutURL string `json:"checkout_url"`
}

type PaymentApproved struct {
	BudgetID          string  `json:"budget_id"`
	PaymentID         string  `json:"payment_id"`
	ProviderPaymentID string  `json:"provider_payment_id"`
	Amount            float64 `json:"amount"`
}

type PaymentFailed struct {
	BudgetID  string `json:"budget_id"`
	PaymentID string `json:"payment_id"`
	Reason    string `json:"reason"`
}

type PaymentRefunded struct {
	BudgetID  string  `json:"budget_id"`
	PaymentID string  `json:"payment_id"`
	Amount    float64 `json:"amount"`
}

// ---- Eventos do os-service ----

type ServiceOrderStatusChanged struct {
	OSCode         int       `json:"os_code"`
	Status         string    `json:"status"`
	PreviousStatus string    `json:"previous_status"`
	Requester      Requester `json:"requester"`
	TotalAmount    float64   `json:"total_amount"`
	CheckoutURL    string    `json:"checkout_url,omitempty"`
	Reason         string    `json:"reason,omitempty"`
}
