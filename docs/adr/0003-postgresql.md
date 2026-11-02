# ADR-0003: PostgreSQL como banco de dados relacional

- **Status:** Parcialmente substituída pela [ADR-0012](0012-banco-por-servico.md)
- **Data:** 2026-04
- **Decisão por:** Reservoir Devs

> **Revisão na fase 4:** a escolha do PostgreSQL continua valendo para o os-service e para o billing-service. O que mudou foi o "banco único": cada microsserviço passou a ter o próprio banco ([ADR-0012](0012-banco-por-servico.md)), e o execution-service usa DynamoDB.

## Contexto

O domínio da oficina mecânica é fortemente relacional:

- **Hierarquia clara:** `Requester (1) → Vehicle (N) → ServiceOrder (N) → ServiceItem / PartItem (N)`.
- **Transações críticas:** a criação da OS baixa estoque, calcula o total e grava os itens, e tudo precisa ser atômico.
- **Buscas frequentes por chave de negócio:** CPF/CNPJ, placa, código da OS.
- **Histórico imutável:** ordens finalizadas não podem ser alteradas sem rastro.

O Tech Challenge permite escolha livre de banco, desde que justificada.

## Decisão

Adotar **PostgreSQL 16** como banco de dados único da aplicação: em container (`postgres:16-alpine`) no `docker-compose` para desenvolvimento e no RDS em produção.

Motivos:

- **ACID e integridade transacional.** A OS agrega serviços e peças e baixa o estoque numa única operação. Se uma parte falha, a transação inteira é revertida, sem compensação no código.
- **Integridade referencial.** Chaves estrangeiras com `ON DELETE RESTRICT` fazem o banco recusar a remoção de um solicitante que ainda tem veículos e impedem OS sem solicitante ou veículo válido.
- **UUID nativo.** `gen_random_uuid()` gera os IDs no próprio banco. UUID evita a enumeração de recursos pela API, que um `serial` permitiria. O código sequencial da OS (`code`) existe só para atendimento.
- **Snapshot dos itens da OS.** Serviços e peças da OS são gravados como JSON na própria linha (`serializer:json` do GORM, coluna `text`), com o preço do momento da criação. Uma mudança posterior no catálogo não altera OS já abertas.
- **Buscas indexadas.** Índices únicos em `requesters.document` e `vehicles.plate`.
- **Ecossistema.** Driver `pgx` maduro, suporte de primeira classe no GORM, `psql` e `pg_dump`.
- **Escala suficiente.** Para o volume de uma oficina (centenas a milhares de OS por mês), um nó basta. Se crescer, há réplicas de leitura, particionamento e pool de conexões (`pgbouncer`) sem trocar de tecnologia.

## Alternativas consideradas

1. **MySQL 8.** Maduro e comum. Descartado porque o suporte do Postgres a tipos (UUID, JSONB, arrays) e a *check constraints* é melhor.
2. **MongoDB.** Rápido para começar. Descartado porque o domínio é relacional: seria preciso simular chaves estrangeiras na aplicação e perder a integridade nativa.
3. **SQLite.** Bom para protótipo. Descartado porque não atende ao uso concorrente esperado e não gera UUID nativamente.

## Consequências

### Positivas

- Garantias ACID sem código extra na aplicação.
- Modelo de dados expresso no schema (chaves estrangeiras, índices únicos, tipos).
- Ferramental conhecido (`psql`, `pg_dump`) e driver e ORM Go maduros.

### Negativas

- É preciso gerenciar migrations. O os-service usa `gorm.AutoMigrate` ([ADR-0004](0004-gorm-orm.md)); uma ferramenta dedicada (`goose`, `migrate`) fica para quando o schema exigir migração de dados.
- Consome mais memória que SQLite.
- Algum acoplamento a recursos do Postgres (`gen_random_uuid()`, `ON CONFLICT`, `FOR UPDATE SKIP LOCKED` no outbox), aceito pelo ganho.

## Referências

- PostgreSQL 16 release notes: https://www.postgresql.org/docs/16/release-16.html
