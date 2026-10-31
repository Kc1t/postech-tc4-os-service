package pgmodel

import (
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
)

type ServiceItem struct {
	ServiceID   string  `json:"service_id"`
	Code        int     `json:"code,omitempty"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
}

type PartItem struct {
	PartID           string  `json:"part_id"`
	ManufacturerCode string  `json:"manufacturer_code,omitempty"`
	Description      string  `json:"description"`
	Quantity         int     `json:"quantity"`
	UnitPrice        float64 `json:"unit_price"`
}

type ServiceOrder struct {
	ID           string               `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Code         int                  `gorm:"not null;uniqueIndex;autoIncrement"`
	RequesterID  string               `gorm:"type:uuid;not null;index;constraint:OnDelete:RESTRICT"`
	Requester    Requester            `gorm:"foreignKey:RequesterID"`
	VehicleID    string               `gorm:"type:uuid;not null;constraint:OnDelete:RESTRICT"`
	Vehicle      Vehicle              `gorm:"foreignKey:VehicleID"`
	Status       entities.OrderStatus `gorm:"not null;index"`
	Services     []ServiceItem        `gorm:"serializer:json"`
	Parts        []PartItem           `gorm:"serializer:json"`
	TotalAmount  float64              `gorm:"not null"`
	Notes        string
	CancelReason string
	Version      int `gorm:"not null;default:1"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

func FromServiceOrder(so *entities.ServiceOrder) *ServiceOrder {
	services := make([]ServiceItem, 0, len(so.Services()))
	for _, s := range so.Services() {
		services = append(services, ServiceItem{ServiceID: s.ServiceID, Code: s.Code, Description: s.Description, Price: s.Price})
	}

	parts := make([]PartItem, 0, len(so.Parts()))
	for _, p := range so.Parts() {
		parts = append(parts, PartItem{
			PartID: p.PartID, ManufacturerCode: p.ManufacturerCode, Description: p.Description,
			Quantity: p.Quantity, UnitPrice: p.UnitPrice,
		})
	}

	version := so.Version()
	if version == 0 {
		version = 1
	}

	return &ServiceOrder{
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
		Version:      version,
		CreatedAt:    so.CreatedAt(),
		UpdatedAt:    so.UpdatedAt(),
		StartedAt:    so.StartedAt(),
		FinishedAt:   so.FinishedAt(),
	}
}

func (m *ServiceOrder) ToDomain() *entities.ServiceOrder {
	services := make([]entities.ServiceItem, 0, len(m.Services))
	for _, s := range m.Services {
		services = append(services, entities.ServiceItem{ServiceID: s.ServiceID, Code: s.Code, Description: s.Description, Price: s.Price})
	}

	parts := make([]entities.PartItem, 0, len(m.Parts))
	for _, p := range m.Parts {
		parts = append(parts, entities.PartItem{
			PartID: p.PartID, ManufacturerCode: p.ManufacturerCode, Description: p.Description,
			Quantity: p.Quantity, UnitPrice: p.UnitPrice,
		})
	}

	return entities.ReconstituteServiceOrder(entities.ServiceOrderSnapshot{
		ID:           m.ID,
		Code:         m.Code,
		RequesterID:  m.RequesterID,
		VehicleID:    m.VehicleID,
		Status:       m.Status,
		Services:     services,
		Parts:        parts,
		TotalAmount:  m.TotalAmount,
		Notes:        m.Notes,
		CancelReason: m.CancelReason,
		Version:      m.Version,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		StartedAt:    m.StartedAt,
		FinishedAt:   m.FinishedAt,
	})
}
