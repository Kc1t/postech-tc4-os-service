package commands

import (
	"encoding/json"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type CreateServiceOrderRequest struct {
	RequesterDocument string `json:"requester_document" binding:"required"`
	VehiclePlate      string `json:"vehicle_plate"     binding:"required"`
	Notes             string `json:"notes"`
}

type CancelServiceOrderRequest struct {
	Reason string `json:"reason" example:"cliente desistiu do serviço"`
}

type ServiceItemResponse struct {
	ServiceID   string  `json:"service_id"`
	Code        int     `json:"code"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
}

type PartItemResponse struct {
	PartID           string  `json:"part_id"`
	ManufacturerCode string  `json:"manufacturer_code"`
	Description      string  `json:"description"`
	Quantity         int     `json:"quantity"`
	UnitPrice        float64 `json:"unit_price"`
}

type ServiceOrderResponse struct {
	ID           string                `json:"id"`
	Code         int                   `json:"code"`
	RequesterID  string                `json:"requester_id"`
	VehicleID    string                `json:"vehicle_id"`
	Status       entities.OrderStatus  `json:"status"`
	Services     []ServiceItemResponse `json:"services"`
	Parts        []PartItemResponse    `json:"parts"`
	TotalAmount  float64               `json:"total_amount"`
	Notes        string                `json:"notes"`
	CancelReason string                `json:"cancel_reason,omitempty"`
	Version      int                   `json:"version"`
	CreatedAt    string                `json:"created_at"`
	UpdatedAt    string                `json:"updated_at"`
	StartedAt    *string               `json:"started_at,omitempty"`
	FinishedAt   *string               `json:"finished_at,omitempty"`
}

type AverageExecutionTimeResponse struct {
	AverageSeconds float64 `json:"average_seconds" example:"5400"`
	AverageHuman   string  `json:"average_human"   example:"1h30m0s"`
}

func ToServiceOrderResponse(so *entities.ServiceOrder) ServiceOrderResponse {
	services := make([]ServiceItemResponse, 0, len(so.Services()))
	for _, s := range so.Services() {
		services = append(services, ServiceItemResponse{
			ServiceID:   s.ServiceID,
			Code:        s.Code,
			Description: s.Description,
			Price:       s.Price,
		})
	}

	parts := make([]PartItemResponse, 0, len(so.Parts()))
	for _, p := range so.Parts() {
		parts = append(parts, PartItemResponse{
			PartID:           p.PartID,
			ManufacturerCode: p.ManufacturerCode,
			Description:      p.Description,
			Quantity:         p.Quantity,
			UnitPrice:        p.UnitPrice,
		})
	}

	var startedAt, finishedAt *string
	if t := so.StartedAt(); t != nil {
		s := t.Format(time.RFC3339)
		startedAt = &s
	}
	if t := so.FinishedAt(); t != nil {
		s := t.Format(time.RFC3339)
		finishedAt = &s
	}

	return ServiceOrderResponse{
		ID:           so.ID(),
		Code:         so.Code(),
		RequesterID:  so.RequesterID(),
		VehicleID:    so.VehicleID(),
		Status:       so.Status(),
		Services:     services,
		Parts:        parts,
		TotalAmount:  so.TotalAmount(),
		Notes:        so.Notes(),
		CancelReason: so.CancelReason(),
		Version:      so.Version(),
		CreatedAt:    so.CreatedAt().Format(time.RFC3339),
		UpdatedAt:    so.UpdatedAt().Format(time.RFC3339),
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
	}
}

func ToServiceOrderListResponse(orders []*entities.ServiceOrder) []ServiceOrderResponse {
	resp := make([]ServiceOrderResponse, 0, len(orders))
	for _, so := range orders {
		resp = append(resp, ToServiceOrderResponse(so))
	}
	return resp
}

type StatusHistoryResponse struct {
	From      entities.OrderStatus `json:"from,omitempty"`
	To        entities.OrderStatus `json:"to"`
	Actor     string               `json:"actor"   example:"saga"`
	Reason    string               `json:"reason,omitempty"`
	MessageID string               `json:"message_id,omitempty"`
	At        string               `json:"at"`
}

func ToStatusHistoryResponse(entries []*entities.StatusHistoryEntry) []StatusHistoryResponse {
	out := make([]StatusHistoryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, StatusHistoryResponse{
			From: e.From, To: e.To, Actor: e.Actor, Reason: e.Reason, MessageID: e.MessageID,
			At: e.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}

type SagaResponse struct {
	ID            string             `json:"id"`
	State         entities.SagaState `json:"state"           example:"awaiting_approval"`
	BudgetID      string             `json:"budget_id,omitempty"`
	PaymentID     string             `json:"payment_id,omitempty"`
	CheckoutURL   string             `json:"checkout_url,omitempty"`
	DeadlineAt    *string            `json:"deadline_at,omitempty"`
	FailureReason string             `json:"failure_reason,omitempty"`
}

type ServiceOrderDetailsResponse struct {
	ServiceOrder ServiceOrderResponse    `json:"service_order"`
	Saga         *SagaResponse           `json:"saga"`
	History      []StatusHistoryResponse `json:"history"`
	Budget       json.RawMessage         `json:"budget"      swaggertype:"object"`
	Execution    json.RawMessage         `json:"execution"   swaggertype:"object"`
	Partial      bool                    `json:"partial"`
	Unavailable  []string                `json:"unavailable"`
}

func ToServiceOrderDetailsResponse(d *ports.ServiceOrderDetails) ServiceOrderDetailsResponse {
	resp := ServiceOrderDetailsResponse{
		ServiceOrder: ToServiceOrderResponse(d.Order),
		History:      ToStatusHistoryResponse(d.History),
		Budget:       nullIfEmpty(d.Budget),
		Execution:    nullIfEmpty(d.Execution),
		Partial:      len(d.Unavailable) > 0,
		Unavailable:  append([]string{}, d.Unavailable...),
	}
	if s := d.Saga; s != nil {
		resp.Saga = &SagaResponse{
			ID: s.ID, State: s.State, BudgetID: s.BudgetID, PaymentID: s.PaymentID,
			CheckoutURL: s.CheckoutURL, FailureReason: s.FailureReason,
		}
		if s.DeadlineAt != nil {
			v := s.DeadlineAt.UTC().Format(time.RFC3339)
			resp.Saga.DeadlineAt = &v
		}
	}
	return resp
}

func nullIfEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	return raw
}
