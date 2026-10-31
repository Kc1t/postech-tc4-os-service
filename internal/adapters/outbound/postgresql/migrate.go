package postgresql

import (
	pgmodel "github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql/model"
	"github.com/Kc1t/postech-tc4-os-service/pkg/inbox"
	"github.com/Kc1t/postech-tc4-os-service/pkg/outbox"
	"gorm.io/gorm"
)

// Migrate cria ou atualiza o schema do os-service: modelos de domínio, estado da saga,
// histórico de status, outbox e inbox. AutoMigrate só adiciona colunas e índices.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&pgmodel.User{},
		&pgmodel.RefreshToken{},
		&pgmodel.Requester{},
		&pgmodel.Vehicle{},
		&pgmodel.Service{},
		&pgmodel.Part{},
		&pgmodel.ServiceOrder{},
		&pgmodel.Saga{},
		&pgmodel.StatusHistory{},
	); err != nil {
		return err
	}
	if err := outbox.Migrate(db); err != nil {
		return err
	}
	return inbox.Migrate(db)
}
