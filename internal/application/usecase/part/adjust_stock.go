package partuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type AdjustPartStock struct {
	repo ports.PartRepository
}

func NewAdjustPartStock(repo ports.PartRepository) *AdjustPartStock {
	return &AdjustPartStock{repo: repo}
}

func (uc *AdjustPartStock) Execute(ctx context.Context, id string, delta int) error {
	return uc.repo.UpdateStock(ctx, id, delta)
}
