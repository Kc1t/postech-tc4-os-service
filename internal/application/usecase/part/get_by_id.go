package partuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type GetPart struct {
	repo ports.PartRepository
}

func NewGetPart(repo ports.PartRepository) *GetPart {
	return &GetPart{repo: repo}
}

func (uc *GetPart) Execute(ctx context.Context, id string) (*entities.Part, error) {
	return uc.repo.FindByID(ctx, id)
}
