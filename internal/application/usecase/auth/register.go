package authuc

import (
	"context"
	"errors"

	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
)

type Register struct {
	userRepo ports.UserRepository
	hasher   ports.PasswordHasher
}

func NewRegister(userRepo ports.UserRepository, hasher ports.PasswordHasher) *Register {
	return &Register{userRepo: userRepo, hasher: hasher}
}

func (uc *Register) Execute(ctx context.Context, name, email, rawPassword string, role entities.Role) error {
	existing, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, domainerrors.ErrNotFound) {
		return err
	}
	if existing != nil {
		return domainerrors.ErrAlreadyExists
	}

	hash, err := uc.hasher.Hash(rawPassword)
	if err != nil {
		return err
	}

	u := entities.NewUser(name, email, hash, role)
	return uc.userRepo.Create(ctx, u)
}
