# ADR-0004: GORM como ORM

- **Status:** Aceita, revisada em 2026-10
- **Data:** 2026-04
- **Decisão por:** Reservoir Devs

> **Revisão na fase 4:** o GORM continua no os-service e passou a abrir a conexão pelo `pgx` com o tracer `nrpgx5` do New Relic (`internal/adapters/outbound/postgresql/open.go`), para cada consulta virar segmento de datastore no trace. O outbox e o inbox comuns (`pkg/outbox`, `pkg/inbox`) também usam GORM. O billing-service usa migrations SQL versionadas; o `AutoMigrate` ficou só no os-service.

## Contexto

O os-service tem um modelo de dados relacional não trivial (ver [ADR-0003](0003-postgresql.md)) e precisa de:

- Mapeamento entre entidades de domínio e tabelas.
- Transações para operações atômicas (ex.: criação de OS com baixa de estoque).
- Migrations do schema durante o desenvolvimento.
- Consultas com filtros, joins e paginação.
- Desempenho adequado com pouco overhead.

O ecossistema Go oferece GORM, `database/sql` puro, `sqlx`, `sqlc`, `ent` e `bun`.

## Decisão

Adotar **GORM v2** (`gorm.io/gorm`) com o driver `gorm.io/driver/postgres`.

Motivos:

- **Produtividade para CRUDs.** Boa parte das operações (solicitantes, veículos, peças, serviços) é CRUD. `Create`, `First`, `Find`, `Save`, `Delete` e `Updates` prontos reduzem o boilerplate.
- **Suporte a Postgres.** UUID, timestamps com fuso, serializer JSON e `AutoMigrate`.
- **Conversão domínio ↔ persistência isolada.** Os modelos em `internal/adapters/outbound/postgresql/model/` têm `FromX()` e `ToDomain()`; o domínio não conhece tags do GORM.
- **Transações simples.** O `TxManager` (`internal/adapters/outbound/postgresql/tx.go`) abre a transação com `db.Transaction` e a leva no `context`; os repositórios chamados com esse contexto usam a mesma transação, e transações aninhadas viram uma só.
- **Comunidade.** É o ORM Go mais usado, com documentação extensa.

## Alternativas consideradas

1. **`database/sql` puro.** Máximo controle e desempenho. Descartado porque exige escrever toda a conversão à mão, o que é verboso e propenso a erro num projeto com muitos CRUDs.
2. **`sqlx`.** Camada fina sobre `database/sql`. Descartado porque ainda exige SQL escrito à mão para todos os CRUDs.
3. **`sqlc`.** Gera código a partir de SQL e tem ótimo desempenho. Descartado porque o ciclo "edita SQL, roda o gerador, ajusta tipos" não compensa para CRUDs simples.
4. **`ent`.** Schema-first com tipagem forte. Descartado pela curva de aprendizado e pela DSL própria.
5. **`bun`.** ORM moderno e rápido. Descartado pela comunidade menor.

## Consequências

### Positivas

- Repositórios enxutos, com poucas linhas por método CRUD.
- `AutoMigrate` na subida do serviço acelera a iteração.
- Conversão domínio ↔ modelo isolada em `model/`.
- Tradução de erros do banco centralizada em `internal/adapters/outbound/postgresql/errors.go` (`gorm.ErrRecordNotFound` → `ErrNotFound`, violação de unicidade `23505` → `ErrAlreadyExists`).

### Negativas

- **Desempenho:** mais lento que SQL puro em consultas muito otimizadas. Aceitável para o volume esperado; `EXPLAIN ANALYZE` se for preciso.
- **N+1:** pode ocorrer sem aviso. Mitigado mantendo as consultas explícitas nos repositórios.
- **Comportamento implícito:** callbacks, hooks e scopes podem surpreender. Mitigado mantendo os repositórios simples.
- **`AutoMigrate` só adiciona:** cria tabelas, colunas e índices, mas não remove nem migra dados. Uma mudança destrutiva de schema exigirá migrations versionadas (`goose` ou `migrate`), como o billing-service já faz.

## Referências

- GORM v2: https://gorm.io/docs/
- nrpgx5: https://pkg.go.dev/github.com/newrelic/go-agent/v3/integrations/nrpgx5
