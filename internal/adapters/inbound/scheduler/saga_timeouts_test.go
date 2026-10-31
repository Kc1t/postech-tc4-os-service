package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/internal/ports/mocks"
	"github.com/newrelic/go-agent/v3/newrelic"
	"go.uber.org/mock/gomock"
)

func TestSagaTimeouts_TickERun(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := mocks.NewMockExpireSagasUseCase(ctrl)
	app, _ := newrelic.NewApplication(newrelic.ConfigAppName("t"), newrelic.ConfigEnabled(false))
	s := NewSagaTimeouts(uc, 10*time.Millisecond, app, nil)
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }

	uc.EXPECT().Execute(gomock.Any(), fixed).Return(2, nil)
	s.Tick(context.Background())
	uc.EXPECT().Execute(gomock.Any(), fixed).Return(0, errors.New("db"))
	s.Tick(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan struct{}, 10)
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, time.Time) (int, error) {
		ran <- struct{}{}
		return 0, nil
	}).MinTimes(1)
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	<-ran
	cancel()
	<-done

	if NewSagaTimeouts(uc, 0, nil, nil).interval != time.Minute {
		t.Fatal("intervalo padrão")
	}
}
