package requesteruc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type UpdateRequester struct {
	repo ports.RequesterRepository
}

func NewUpdateRequester(repo ports.RequesterRepository) *UpdateRequester {
	return &UpdateRequester{repo: repo}
}

func (uc *UpdateRequester) Execute(ctx context.Context, c *entities.Requester) error {
	return uc.repo.Update(ctx, c)
}
