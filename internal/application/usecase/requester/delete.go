package requesteruc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type DeleteRequester struct {
	repo ports.RequesterRepository
}

func NewDeleteRequester(repo ports.RequesterRepository) *DeleteRequester {
	return &DeleteRequester{repo: repo}
}

func (uc *DeleteRequester) Execute(ctx context.Context, id string) error {
	return uc.repo.Delete(ctx, id)
}
