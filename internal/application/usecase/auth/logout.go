package authuc

import (
	"context"

	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type Logout struct {
	refreshRepo ports.RefreshTokenRepository
}

func NewLogout(refreshRepo ports.RefreshTokenRepository) *Logout {
	return &Logout{refreshRepo: refreshRepo}
}

func (uc *Logout) Execute(ctx context.Context, userID string) error {
	return uc.refreshRepo.RevokeByUserID(ctx, userID)
}
