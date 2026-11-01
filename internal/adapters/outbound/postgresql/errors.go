package postgresql

import (
	"errors"

	domainerrors "github.com/Kc1t/postech-tc4-os-service/internal/domain/errors"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func mapError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainerrors.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return domainerrors.ErrAlreadyExists
		case "23503": // foreign_key_violation: OS ou veículo ainda referenciam o registro (ON DELETE RESTRICT)
			return domainerrors.ErrInUse
		}
	}
	return err
}
