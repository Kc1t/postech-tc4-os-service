package serviceuc

import (
	"context"
	"errors"
	"testing"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports/mocks"
	"go.uber.org/mock/gomock"
)

func newDeleteServiceUC(t *testing.T) (*DeleteService, *mocks.MockServiceRepository, *mocks.MockServiceOrderRepository) {
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockServiceRepository(ctrl)
	orders := mocks.NewMockServiceOrderRepository(ctrl)
	return NewDeleteService(repo, orders), repo, orders
}

func TestDeleteService_Execute_Success(t *testing.T) {
	uc, repo, orders := newDeleteServiceUC(t)
	orders.EXPECT().ExistsActiveWithService(gomock.Any(), "svc-1").Return(false, nil)
	repo.EXPECT().Delete(gomock.Any(), "svc-1").Return(nil)

	if err := uc.Execute(context.Background(), "svc-1"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDeleteService_Execute_EmUsoPorOSAtiva(t *testing.T) {
	uc, _, orders := newDeleteServiceUC(t)
	orders.EXPECT().ExistsActiveWithService(gomock.Any(), "svc-1").Return(true, nil)
	// Delete não pode ser chamado: o mock falha se for.

	if err := uc.Execute(context.Background(), "svc-1"); !errors.Is(err, domainerrors.ErrInUse) {
		t.Fatalf("expected ErrInUse, got %v", err)
	}
}

func TestDeleteService_Execute_ErroAoConsultarOS(t *testing.T) {
	uc, _, orders := newDeleteServiceUC(t)
	dbErr := errors.New("db error")
	orders.EXPECT().ExistsActiveWithService(gomock.Any(), "svc-1").Return(false, dbErr)

	if err := uc.Execute(context.Background(), "svc-1"); !errors.Is(err, dbErr) {
		t.Fatalf("expected %v, got %v", dbErr, err)
	}
}

func TestDeleteService_Execute_RepoError(t *testing.T) {
	uc, repo, orders := newDeleteServiceUC(t)
	repoErr := errors.New("db error")
	orders.EXPECT().ExistsActiveWithService(gomock.Any(), "svc-1").Return(false, nil)
	repo.EXPECT().Delete(gomock.Any(), "svc-1").Return(repoErr)

	if err := uc.Execute(context.Background(), "svc-1"); !errors.Is(err, repoErr) {
		t.Fatalf("expected %v, got %v", repoErr, err)
	}
}
