package postgresql

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/newrelic/go-agent/v3/integrations/nrpgx5"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open conecta ao Postgres pelo pgx com o tracer do New Relic: cada consulta feita com um contexto
// que carrega transação (os repositórios usam db.WithContext) vira um segmento de datastore, e o
// banco aparece no trace distribuído e no service map.
func Open(dsn string) (*gorm.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgresql: dsn inválida: %w", err)
	}
	cfg.Tracer = nrpgx5.NewTracer()
	db, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: stdlib.OpenDB(*cfg)}), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("postgresql: conectar: %w", err)
	}
	return db, nil
}
