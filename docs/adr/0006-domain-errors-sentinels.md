# ADR-0006: Erros sentinela centralizados em `domainerrors`

- **Status:** Aceita, revisada em 2026-10
- **Data:** 2026-04
- **Decisão por:** Reservoir Devs

> **Revisão na fase 4:** o exemplo e a regra de mapeamento foram atualizados para o código atual. O handler não faz mais `switch`; o mapeamento é uma tabela única em `internal/adapters/inbound/http/errors.go`, e o corpo de erro (`{"error": {"code", "message"}}`) é o mesmo nos três microsserviços. A saga trouxe `ErrSagaOutOfStep`, `ErrUnknownCatalogItem` e `ErrInvalidSimulatedFailure`.

## Contexto

Na arquitetura hexagonal ([ADR-0002](0002-hexagonal-architecture.md)), o domínio precisa expressar falhas de negócio sem vazar detalhes de infraestrutura:

- O repositório recebe `gorm.ErrRecordNotFound` ao buscar um solicitante inexistente. Esse erro não pode subir até o handler, senão caso de uso e handler ficam acoplados ao GORM.
- Casos de uso precisam sinalizar regras violadas (estoque insuficiente, transição de status inválida, documento inválido) sem `errors.New()` espalhados.
- Handlers HTTP precisam traduzir esses erros para status (`404`, `409`, `422`) de forma consistente.

Sem uma estratégia, o código tende a repetir literais de erro, vazar `gorm.ErrRecordNotFound` e mapear status de forma diferente em cada handler.

## Decisão

Centralizar os erros de domínio num pacote único, `internal/domain/errors/` (pacote `domainerrors`).

### Padrão

Erros sentinela exportados (trecho de `internal/domain/errors/errors.go`):

```go
var (
    ErrNotFound            = errors.New("not found")
    ErrAlreadyExists       = errors.New("already exists")
    ErrInvalidDocument     = errors.New("invalid CPF/CNPJ")
    ErrInvalidStatus       = errors.New("invalid status transition")
    ErrInsufficientStock   = errors.New("insufficient stock")
    ErrDocumentMismatch    = errors.New("document does not match the authenticated requester")
    ErrInvalidCredentials  = errors.New("invalid credentials")
    ErrConcurrentUpdate    = errors.New("resource was modified concurrently")
    // ...
)
```

### Regras

1. Casos de uso devolvem erros de `domainerrors`, direto ou embrulhados com `fmt.Errorf("...: %w", domainerrors.ErrX)`.
2. Repositórios traduzem os erros de infraestrutura (`internal/adapters/outbound/postgresql/errors.go`): `gorm.ErrRecordNotFound` vira `ErrNotFound` e a violação de unicidade (`23505`) vira `ErrAlreadyExists`.
3. Handlers chamam `httputil.HandleError(c, err)`, que percorre uma tabela `erro → status, código` com `errors.Is`. Erro fora da tabela vira 500 `INTERNAL_ERROR` sem expor a mensagem. Exemplos da tabela:

   | Erro | Status | `code` |
   |---|---|---|
   | `ErrNotFound` | 404 | `NOT_FOUND` |
   | `ErrAlreadyExists` | 409 | `ALREADY_EXISTS` |
   | `ErrConcurrentUpdate` | 409 | `CONCURRENT_UPDATE` |
   | `ErrInvalidStatus` | 409 | `INVALID_STATUS_TRANSITION` |
   | `ErrInsufficientStock` | 422 | `INSUFFICIENT_STOCK` |
   | `ErrDocumentMismatch` | 403 | `FORBIDDEN` |
   | `ErrInvalidCredentials` | 401 | `UNAUTHORIZED` |
   | `ErrInvalidSimulatedFailure` | 400 | `INVALID_SIMULATE_FAILURE` |

4. Nada de `errors.New("...")` solto em casos de uso ou handlers. Erro novo entra em `domainerrors` e, se chega ao HTTP, na tabela.

## Alternativas consideradas

1. **Tipos de erro estruturados (structs com `Error()`).** Carregam dados, mas trazem boilerplate. Descartados porque sentinelas com `errors.Is` bastam para o escopo.
2. **Um pacote de erros por contexto** (`requester/errors`, `vehicle/errors`). Mais granular. Descartado porque duplicaria `ErrNotFound` em cada pacote.
3. **Caso de uso devolvendo status HTTP.** Descartado porque acopla o domínio ao transporte e viola a regra de dependência da arquitetura hexagonal.
4. **`panic`/`recover`.** Descartado por não ser idiomático em Go para fluxos esperados de negócio.

## Consequências

### Positivas

- Mensagens e códigos de erro consistentes no sistema e entre os serviços.
- Mapeamento HTTP num lugar só.
- Testes conferem `errors.Is(err, domainerrors.ErrX)` sem comparar strings.
- Erro de infraestrutura não sai do adapter de persistência.

### Negativas

- Sentinelas não carregam contexto (ex.: qual ID faltou). Mitigado com `fmt.Errorf("...: %w", ...)` quando preciso.
- Mensagens em vários idiomas exigiriam trocar sentinelas por structs com código.
- Erro de banco que o adapter não traduz cai no 500 genérico. Hoje a violação de chave estrangeira (`23503`, ex.: excluir um solicitante que ainda tem veículos) não é traduzida.

## Referências

- Dave Cheney, *Don't just check errors, handle them gracefully* (2016).
- `errors.Is` e `errors.As`: https://pkg.go.dev/errors
