package serviceorderuc

import (
	"context"
	"errors"
	"testing"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports/mocks"
	"go.uber.org/mock/gomock"
)

func TestGetServiceOrderByCode_Sucesso(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	requester := entities.ReconstituteRequester("cust-1", "Diego", "52998224725", "d@e.com", "", ft(), ft())
	expected := entities.ReconstituteServiceOrder(entities.ServiceOrderSnapshot{ID: "order-1", Code: 100, RequesterID: "cust-1", VehicleID: "veh-1", Status: entities.StatusReceived, CreatedAt: ft(), UpdatedAt: ft()})

	soRepo := mocks.NewMockServiceOrderRepository(ctrl)
	custRepo := mocks.NewMockRequesterRepository(ctrl)

	custRepo.EXPECT().FindByDocument(gomock.Any(), "52998224725").Return(requester, nil)
	soRepo.EXPECT().FindByCode(gomock.Any(), 100).Return(expected, nil)

	uc := NewGetServiceOrderByCode(soRepo, custRepo)
	got, err := uc.Execute(context.Background(), 100, "52998224725")
	if err != nil {
		t.Fatalf("esperava sucesso, obteve: %v", err)
	}
	if got.ID() != "order-1" {
		t.Errorf("ID = %q, esperava %q", got.ID(), "order-1")
	}
}

func TestGetServiceOrderByCode_ClienteNaoEncontrado(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	soRepo := mocks.NewMockServiceOrderRepository(ctrl)
	custRepo := mocks.NewMockRequesterRepository(ctrl)

	custRepo.EXPECT().FindByDocument(gomock.Any(), "00000000000").Return(nil, domainerrors.ErrNotFound)

	uc := NewGetServiceOrderByCode(soRepo, custRepo)
	_, err := uc.Execute(context.Background(), 100, "00000000000")
	if !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatalf("erro = %v, esperava ErrNotFound", err)
	}
}

func TestGetServiceOrderByCode_OSNaoEncontrada(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	requester := entities.ReconstituteRequester("cust-1", "Diego", "52998224725", "d@e.com", "", ft(), ft())

	soRepo := mocks.NewMockServiceOrderRepository(ctrl)
	custRepo := mocks.NewMockRequesterRepository(ctrl)

	custRepo.EXPECT().FindByDocument(gomock.Any(), "52998224725").Return(requester, nil)
	soRepo.EXPECT().FindByCode(gomock.Any(), 999).Return(nil, domainerrors.ErrNotFound)

	uc := NewGetServiceOrderByCode(soRepo, custRepo)
	_, err := uc.Execute(context.Background(), 999, "52998224725")
	if !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatalf("erro = %v, esperava ErrNotFound", err)
	}
}

func TestGetServiceOrderByCode_OSDeOutroCliente(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	requester := entities.ReconstituteRequester("cust-1", "Diego", "52998224725", "d@e.com", "", ft(), ft())
	so := entities.ReconstituteServiceOrder(entities.ServiceOrderSnapshot{ID: "order-1", Code: 100, RequesterID: "outro-cliente", VehicleID: "veh-1", Status: entities.StatusReceived, CreatedAt: ft(), UpdatedAt: ft()})

	soRepo := mocks.NewMockServiceOrderRepository(ctrl)
	custRepo := mocks.NewMockRequesterRepository(ctrl)

	custRepo.EXPECT().FindByDocument(gomock.Any(), "52998224725").Return(requester, nil)
	soRepo.EXPECT().FindByCode(gomock.Any(), 100).Return(so, nil)

	uc := NewGetServiceOrderByCode(soRepo, custRepo)
	_, err := uc.Execute(context.Background(), 100, "52998224725")
	if !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatalf("erro = %v, esperava ErrNotFound (ownership)", err)
	}
}

func TestGetServiceOrderByCode_ErroInfraCliente(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	infraErr := errors.New("timeout")

	soRepo := mocks.NewMockServiceOrderRepository(ctrl)
	custRepo := mocks.NewMockRequesterRepository(ctrl)

	custRepo.EXPECT().FindByDocument(gomock.Any(), "52998224725").Return(nil, infraErr)

	uc := NewGetServiceOrderByCode(soRepo, custRepo)
	_, err := uc.Execute(context.Background(), 100, "52998224725")
	if !errors.Is(err, infraErr) {
		t.Fatalf("erro = %v, esperava erro de infra propagado", err)
	}
}
