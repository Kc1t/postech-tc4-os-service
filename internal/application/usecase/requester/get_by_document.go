package requesteruc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type GetRequesterByDocument struct {
	repo ports.RequesterRepository
}

func NewGetRequesterByDocument(repo ports.RequesterRepository) *GetRequesterByDocument {
	return &GetRequesterByDocument{repo: repo}
}

func (uc *GetRequesterByDocument) Execute(ctx context.Context, document string) (*entities.Requester, error) {
	return uc.repo.FindByDocument(ctx, document)
}
