package entities

import (
	"testing"
	"time"
)

func TestNewRefreshToken(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	rt := NewRefreshToken("u-1", "sha256", exp)
	if rt.ID() != "" || rt.UserID() != "u-1" || rt.TokenHash() != "sha256" || !rt.ExpiresAt().Equal(exp) {
		t.Fatalf("campos inesperados: %+v", rt)
	}
	if rt.Revoked() || rt.CreatedAt().IsZero() || !rt.IsValid() {
		t.Error("token novo começa válido e não revogado")
	}
	rt.SetID("rt-1")
	if rt.ID() != "rt-1" {
		t.Errorf("SetID: veio %q", rt.ID())
	}
}

func TestRefreshToken_Validade(t *testing.T) {
	created := time.Now().Add(-2 * time.Hour)
	cases := []struct {
		name    string
		exp     time.Time
		revoked bool
		expired bool
		valid   bool
	}{
		{"vigente", time.Now().Add(time.Hour), false, false, true},
		{"expirado", time.Now().Add(-time.Second), false, true, false},
		{"revogado", time.Now().Add(time.Hour), true, false, false},
	}
	for _, tc := range cases {
		rt := ReconstituteRefreshToken("rt-1", "u-1", "h", tc.exp, tc.revoked, created)
		if rt.IsExpired() != tc.expired || rt.IsValid() != tc.valid || rt.Revoked() != tc.revoked {
			t.Errorf("%s: expirado %v, válido %v", tc.name, rt.IsExpired(), rt.IsValid())
		}
		if !rt.CreatedAt().Equal(created) {
			t.Errorf("%s: createdAt deve vir do banco", tc.name)
		}
	}
}

func TestRefreshToken_Revoke(t *testing.T) {
	rt := NewRefreshToken("u-1", "h", time.Now().Add(time.Hour))
	rt.Revoke()
	if !rt.Revoked() || rt.IsValid() {
		t.Error("token revogado não pode ser válido")
	}
}
