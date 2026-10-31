package serviceuc

import (
	"context"
	"errors"
	"testing"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports/mocks"
	"go.uber.org/mock/gomock"
)

func TestCreateService_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockServiceRepository(ctrl)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	uc := NewCreateService(repo)
	if err := uc.Execute(context.Background(), entities.NewService(1, "Troca de óleo", "Desc", 150.0, 60)); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCreateService_Execute_RepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoErr := errors.New("db error")
	repo := mocks.NewMockServiceRepository(ctrl)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(repoErr)

	uc := NewCreateService(repo)
	if err := uc.Execute(context.Background(), entities.NewService(1, "Troca de óleo", "Desc", 150.0, 60)); !errors.Is(err, repoErr) {
		t.Fatalf("expected %v, got %v", repoErr, err)
	}
}
