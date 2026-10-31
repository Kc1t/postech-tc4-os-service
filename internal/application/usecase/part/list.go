package partuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type ListParts struct {
	repo ports.PartRepository
}

func NewListParts(repo ports.PartRepository) *ListParts {
	return &ListParts{repo: repo}
}

func (uc *ListParts) Execute(ctx context.Context) ([]*entities.Part, error) {
	return uc.repo.FindAll(ctx)
}
