package entities

import (
	"testing"
	"time"
)

func TestNewUser(t *testing.T) {
	u := NewUser("Equipe", "equipe@oficina.com", "hash", RoleAdmin)
	if u.Name() != "Equipe" || u.Email() != "equipe@oficina.com" || u.PasswordHash() != "hash" || u.Role() != RoleAdmin {
		t.Fatalf("campos inesperados: %+v", u)
	}
	if u.ID() != "" || u.FailedAttempts() != 0 || u.LockedUntil() != nil || u.IsLocked() {
		t.Error("usuário novo começa sem id, sem tentativas e desbloqueado")
	}
	if u.CreatedAt().IsZero() || !u.CreatedAt().Equal(u.UpdatedAt()) {
		t.Error("createdAt e updatedAt começam iguais")
	}
	u.SetID("u-1")
	if u.ID() != "u-1" {
		t.Errorf("SetID: veio %q", u.ID())
	}
}

func TestReconstituteUser(t *testing.T) {
	until := time.Now().Add(time.Minute)
	created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	u := ReconstituteUser("u-1", "Equipe", "e@o.com", "hash", RoleClient, 3, &until, created, created)
	if u.ID() != "u-1" || u.Role() != RoleClient || u.FailedAttempts() != 3 || u.LockedUntil() != &until {
		t.Fatalf("campos inesperados: %+v", u)
	}
	if !u.CreatedAt().Equal(created) || !u.UpdatedAt().Equal(created) {
		t.Error("datas devem vir do banco")
	}
}

func TestUser_SetNameAtualizaUpdatedAt(t *testing.T) {
	u := ReconstituteUser("u-1", "Antigo", "e@o.com", "h", RoleAdmin, 0, nil, time.Time{}, time.Time{})
	u.SetName("Novo")
	if u.Name() != "Novo" || u.UpdatedAt().IsZero() {
		t.Errorf("SetName: nome %q, updatedAt %v", u.Name(), u.UpdatedAt())
	}
}

func TestUser_BloqueioPorTentativas(t *testing.T) {
	u := NewUser("Equipe", "e@o.com", "h", RoleAdmin)
	for i := 1; i < 5; i++ {
		u.RegisterFailedLogin(5, 15*time.Minute)
		if u.IsLocked() {
			t.Fatalf("bloqueou na tentativa %d, antes do limite", i)
		}
	}
	u.RegisterFailedLogin(5, 15*time.Minute)
	if !u.IsLocked() || u.FailedAttempts() != 5 {
		t.Fatalf("esperava bloqueio na 5ª tentativa: tentativas %d, bloqueado %v", u.FailedAttempts(), u.IsLocked())
	}
	if d := time.Until(*u.LockedUntil()); d <= 14*time.Minute || d > 15*time.Minute {
		t.Errorf("bloqueio deveria durar 15 min, restam %v", d)
	}

	u.ResetFailedLogins()
	if u.IsLocked() || u.FailedAttempts() != 0 || u.LockedUntil() != nil {
		t.Error("login bem-sucedido limpa tentativas e bloqueio")
	}
}

func TestUser_BloqueioVencidoLiberaSozinho(t *testing.T) {
	past := time.Now().Add(-time.Second)
	u := ReconstituteUser("u-1", "Equipe", "e@o.com", "h", RoleAdmin, 5, &past, time.Now(), time.Now())
	if u.IsLocked() {
		t.Error("bloqueio vencido não pode valer")
	}
}
