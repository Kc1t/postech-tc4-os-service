package postgresql

import (
	"context"
	"errors"
	"testing"
	"time"

	pgmodel "github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql/model"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
)

func TestUserRepository_CicloDoBloqueio(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepository(testDB)
	u := entities.NewUser("Equipe", "user-repo@test.com", "hash", entities.RoleAdmin)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { testDB.Delete(&pgmodel.User{}, "id = ?", u.ID()) })

	byEmail, err := repo.FindByEmail(ctx, "user-repo@test.com")
	if err != nil || byEmail.ID() != u.ID() || byEmail.Role() != entities.RoleAdmin || byEmail.PasswordHash() != "hash" {
		t.Fatalf("FindByEmail: %+v, %v", byEmail, err)
	}

	// Cinco senhas erradas bloqueiam; o bloqueio precisa sobreviver à leitura do banco.
	for range 5 {
		byEmail.RegisterFailedLogin(5, 15*time.Minute)
	}
	if err := repo.Update(ctx, byEmail); err != nil {
		t.Fatalf("update: %v", err)
	}
	byID, err := repo.FindByID(ctx, u.ID())
	if err != nil || byID.FailedAttempts() != 5 || !byID.IsLocked() || byID.Email() != "user-repo@test.com" {
		t.Fatalf("FindByID depois do bloqueio: tentativas %d, bloqueado %v, %v", byID.FailedAttempts(), byID.IsLocked(), err)
	}

	byID.ResetFailedLogins()
	if err := repo.Update(ctx, byID); err != nil {
		t.Fatalf("update: %v", err)
	}
	again, _ := repo.FindByID(ctx, u.ID())
	if again.FailedAttempts() != 0 || again.LockedUntil() != nil {
		t.Errorf("login certo limpa o bloqueio no banco: %d, %v", again.FailedAttempts(), again.LockedUntil())
	}
}

func TestUserRepository_Erros(t *testing.T) {
	ctx := context.Background()
	repo := NewUserRepository(testDB)

	if _, err := repo.FindByEmail(ctx, "ninguem@test.com"); !errors.Is(err, domainerrors.ErrNotFound) {
		t.Errorf("e-mail inexistente: esperava ErrNotFound, veio %v", err)
	}
	if _, err := repo.FindByID(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, domainerrors.ErrNotFound) {
		t.Errorf("id inexistente: esperava ErrNotFound, veio %v", err)
	}
	if _, err := repo.FindByID(ctx, "nao-e-uuid"); err == nil {
		t.Error("id que não é UUID deve falhar")
	}

	u := entities.NewUser("Equipe", "user-dup@test.com", "hash", entities.RoleAdmin)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { testDB.Delete(&pgmodel.User{}, "id = ?", u.ID()) })
	if err := repo.Create(ctx, entities.NewUser("Outra", "user-dup@test.com", "h", entities.RoleAdmin)); err == nil {
		t.Error("e-mail repetido deve violar o índice único")
	}
}
