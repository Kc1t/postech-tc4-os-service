package partuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type UpdatePart struct {
	repo ports.PartRepository
}

func NewUpdatePart(repo ports.PartRepository) *UpdatePart {
	return &UpdatePart{repo: repo}
}

func (uc *UpdatePart) Execute(ctx context.Context, p *entities.Part) error {
	return uc.repo.Update(ctx, p)
}
