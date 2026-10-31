package partuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type CreatePart struct {
	repo ports.PartRepository
}

func NewCreatePart(repo ports.PartRepository) *CreatePart {
	return &CreatePart{repo: repo}
}

func (uc *CreatePart) Execute(ctx context.Context, p *entities.Part) error {
	return uc.repo.Create(ctx, p)
}
