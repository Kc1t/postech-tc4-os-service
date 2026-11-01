# ADR-0005: Autenticação JWT com refresh token rotativo

- **Status:** Aceita, revisada em 2026-10
- **Data:** 2026-04
- **Decisão por:** Reservoir Devs

> **Revisão nas fases 3 e 4:** o login por e-mail e senha descrito aqui é o da equipe da oficina. Na fase 3 o cliente passou a se autenticar por CPF numa Lambda própria ([RFC-0003](../rfc/0003-autenticacao-por-cpf.md)), e o "portal público" do item 5 deixou de existir: as rotas do cliente foram para trás do middleware `Auth` ([ADR-0007](0007-api-gateway-comunicacao.md)) e conferem se o `document` do token é o dono da OS (403 se não for). Na fase 4, a aprovação do orçamento saiu do os-service para o billing-service (`POST /api/v1/budgets/{id}/approve` e `/reject`). As notas de implementação no fim corrigem três pontos em que o texto original não batia com o código.

## Contexto

O Tech Challenge exige autenticação JWT para as APIs administrativas. Era preciso decidir:

- A estrutura do token (claims, expiração).
- Como renovar sem forçar login frequente.
- Como revogar o acesso de um usuário comprometido.
- Como tratar o portal público de consulta de OS (que, na fase 1, não exigia autenticação).

## Decisão

Autenticação em duas peças, mais bloqueio de conta.

### 1. Access token (JWT)

- `Bearer` no header `Authorization`.
- Validade de **15 minutos** (`ACCESS_TOKEN_EXP_MIN`).
- `HS256` com o segredo em `JWT_SECRET`.
- Claims: `sub` (id do usuário), `email`, `role` (`admin` ou `client`), `iat`, `exp`.

### 2. Refresh token rotativo

- Validade de **7 dias** (`REFRESH_TOKEN_EXP_DAYS`).
- Guardado na tabela `refresh_tokens` só como hash, com o usuário, a expiração e a marca de revogado.
- A cada uso, o refresh antigo é revogado e um novo é emitido, na mesma transação (`RotateToken`, que só revoga se o token ainda não estava revogado).
- Rota pública: `POST /api/v1/auth/refresh`.

### 3. Logout

`POST /api/v1/auth/logout` revoga os refresh tokens do usuário. Access tokens não são revogáveis e valem até o `exp`; o prazo curto limita a exposição.

### 4. Bloqueio de conta

Depois de **5 tentativas de login erradas** seguidas (`MAX_FAILED_LOGINS`), a conta fica bloqueada por **15 minutos** (`LOGIN_LOCK_MIN`), com os campos `failed_attempts` e `locked_until` do usuário. A regra fica no domínio (`User.RegisterFailedLogin`), não em middleware. Conta bloqueada recebe o mesmo 401 de credencial errada, e a senha é comparada mesmo quando o e-mail não existe (contra um hash fictício), para que nem a resposta nem o tempo revelem quais e-mails existem ou estão bloqueados.

### 5. Portal público (fase 1)

As rotas de consulta do cliente (`GET /api/v1/service-orders/code/:code` e a busca por solicitante) e a aprovação do orçamento eram públicas, identificadas pelo código da OS. Substituído na fase 3, como diz a nota no topo.

## Alternativas consideradas

1. **Sessão com cookie HttpOnly.** Mais simples para um SPA no mesmo domínio. Descartada porque o sistema pode ter outros clientes (mobile, parceiros), e o JWT serve a todos.
2. **Só access token de longa duração.** Comum em protótipos. Descartada porque um token vazado valeria por dias.
3. **OAuth2/OIDC com provedor externo (Auth0, Keycloak).** Bom para produção. Descartado por trazer uma dependência externa fora do escopo do MVP.
4. **Refresh token sem rotação.** Mais simples. Descartado porque um refresh roubado poderia ser usado várias vezes durante toda a validade.

## Consequências

### Positivas

- O prazo curto do access token reduz a janela de uso de um token vazado.
- Com a rotação, um refresh só funciona uma vez: o reuso de um refresh já revogado é recusado com 401.
- O bloqueio de conta protege contra força bruta de senha sem depender de WAF.
- A role no claim simplifica a autorização (`RequireRole("admin")`).

### Negativas

- O access token não pode ser revogado antes do `exp`; aceito pelo prazo de 15 minutos.
- Duas requisições com o mesmo refresh ao mesmo tempo: só uma passa, a outra recebe 401. É o comportamento esperado da rotação, garantido pela transação do `RotateToken`.
- O bloqueio pode ser usado para travar contas legítimas. CAPTCHA ou bloqueio por IP ficam para depois.

## Notas de implementação

- `TokenService` em `internal/adapters/outbound/jwt/service.go`. Os casos de uso recebem a interface `ports.TokenService`, nunca a biblioteca de JWT ([ADR-0002](0002-hexagonal-architecture.md)).
- Senhas com bcrypt, `BCRYPT_COST=12` (configurável).
- `POST /api/v1/auth/register` cria usuário com role `client`. O usuário `admin` é criado na subida a partir de `ADMIN_EMAIL` e `ADMIN_PASSWORD`.
- O middleware `Auth` também aceita o token emitido pela Lambda de CPF (mesmo segredo) e, quando há claim `document`, a guarda no contexto para a checagem de posse.
- Diferenças em relação ao texto original desta ADR:
  - A tabela `refresh_tokens` não guarda o IP de emissão.
  - O reuso de um refresh revogado só é recusado (401); não revoga os demais tokens do usuário.
  - O logout revoga todos os refresh tokens do usuário (`RevokeByUserID`), não só o da sessão atual.

## Referências

- RFC 7519 (JSON Web Token).
- Auth0, *Refresh Token Rotation*: https://auth0.com/docs/secure/tokens/refresh-tokens/refresh-token-rotation
