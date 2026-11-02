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
		case "22P02": // invalid_text_representation: id que não é UUID
			return domainerrors.ErrInvalidID
		}
	}
	return err
}

// deleteByID apaga o registro pela chave e devolve ErrNotFound quando nada foi apagado, para a
// API responder 404 em vez de 204 a um id que não existe.
func deleteByID(db *gorm.DB, model any, id string) error {
	res := db.Delete(model, "id = ?", id)
	if res.Error != nil {
		return mapError(res.Error)
	}
	if res.RowsAffected == 0 {
		return domainerrors.ErrNotFound
	}
	return nil
}
