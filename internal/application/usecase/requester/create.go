package requesteruc

import (
	"context"
	"errors"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type CreateRequester struct {
	repo ports.RequesterRepository
}

func NewCreateRequester(repo ports.RequesterRepository) *CreateRequester {
	return &CreateRequester{repo: repo}
}

func (uc *CreateRequester) Execute(ctx context.Context, c *entities.Requester) error {
	// Documento ja validado na factory NewRequester (fast-fail no dominio)

	// Verificar duplicidade de documento
	existing, err := uc.repo.FindByDocument(ctx, c.Document())
	if err != nil && !errors.Is(err, domainerrors.ErrNotFound) {
		return err
	}
	if existing != nil {
		return domainerrors.ErrAlreadyExists
	}

	return uc.repo.Create(ctx, c)
}
