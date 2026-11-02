package postgresql

import (
	"context"
	"errors"
	"testing"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
)

// deleter e finder cobrem o que os cinco repositórios de cadastro e de OS têm em comum.
type deleter interface {
	Delete(ctx context.Context, id string) error
}

func deleters() map[string]deleter {
	return map[string]deleter{
		"part":          NewPartRepository(testDB),
		"service":       NewServiceRepository(testDB),
		"vehicle":       NewVehicleRepository(testDB),
		"requester":     NewRequesterRepository(testDB),
		"service order": NewServiceOrderRepository(testDB),
	}
}

func TestRepositories_DeleteInexistenteDevolveNotFound(t *testing.T) {
	for name, repo := range deleters() {
		err := repo.Delete(context.Background(), "00000000-0000-4000-8000-000000000000")
		if !errors.Is(err, domainerrors.ErrNotFound) {
			t.Errorf("%s: expected ErrNotFound, got %v", name, err)
		}
	}
}

func TestRepositories_IDQueNaoEUUIDDevolveInvalidID(t *testing.T) {
	for name, repo := range deleters() {
		err := repo.Delete(context.Background(), "nao-e-uuid")
		if !errors.Is(err, domainerrors.ErrInvalidID) {
			t.Errorf("%s: expected ErrInvalidID on delete, got %v", name, err)
		}
	}
	if _, err := NewPartRepository(testDB).FindByID(context.Background(), "nao-e-uuid"); !errors.Is(err, domainerrors.ErrInvalidID) {
		t.Errorf("part: expected ErrInvalidID on find, got %v", err)
	}
}
