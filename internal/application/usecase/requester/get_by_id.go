package requesteruc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type GetRequester struct {
	repo ports.RequesterRepository
}

func NewGetRequester(repo ports.RequesterRepository) *GetRequester {
	return &GetRequester{repo: repo}
}

func (uc *GetRequester) Execute(ctx context.Context, id string) (*entities.Requester, error) {
	return uc.repo.FindByID(ctx, id)
}
