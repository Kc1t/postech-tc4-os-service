package messaging

import (
	"context"
	"testing"

	"github.com/Kc1t/postech-tc4-os-service/internal/ports/mocks"
	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	pkgmessaging "github.com/Kc1t/postech-tc4-os-service/pkg/messaging"
	"go.uber.org/mock/gomock"
)

type fakeRegistrar map[string]pkgmessaging.Handler

func (f fakeRegistrar) Handle(t string, h pkgmessaging.Handler) { f[t] = h }

func TestRegisterSagaReplies(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := mocks.NewMockHandleSagaReplyUseCase(ctrl)
	uc.EXPECT().Types().Return([]string{contracts.EvtBudgetCreated, contracts.EvtRepairFinished})
	reg := fakeRegistrar{}
	RegisterSagaReplies(reg, uc)
	if len(reg) != 2 {
		t.Fatalf("registrados: %d", len(reg))
	}
	env := contracts.Envelope{Type: contracts.EvtBudgetCreated}
	uc.EXPECT().Execute(gomock.Any(), env).Return(nil)
	if err := reg[contracts.EvtBudgetCreated](context.Background(), env); err != nil {
		t.Fatal(err)
	}
}
