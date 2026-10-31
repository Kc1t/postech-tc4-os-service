package requesteruc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type ListRequesters struct {
	repo ports.RequesterRepository
}

func NewListRequesters(repo ports.RequesterRepository) *ListRequesters {
	return &ListRequesters{repo: repo}
}

func (uc *ListRequesters) Execute(ctx context.Context) ([]*entities.Requester, error) {
	return uc.repo.FindAll(ctx)
}
