# ADR-0023: Registro de usuário restrito à equipe

- **Status:** Aceita
- **Data:** 2026-11
- **Decisão por:** Reservoir Devs

## Contexto

Desde a fase 1, `POST /api/v1/auth/register` era pública e criava um usuário com papel `client` ([ADR-0005](0005-jwt-refresh-token.md)). Na fase 3 o cliente passou a se autenticar pelo CPF na Lambda auth-issuer ([RFC-0003](../rfc/0003-autenticacao-por-cpf.md)), e esse usuário perdeu a função:

- o token dele não tem `document`, então não passa nas rotas do cliente, que conferem a posse pelo CPF;
- não tem papel `admin`, então não passa nas rotas da equipe.

A rota continuava aberta, e o API Gateway deixa `POST /api/v1/auth/{proxy+}` passar sem authorizer, porque o login e o refresh da equipe precisam ser abertos. O resultado era escrita anônima no banco do os-service: qualquer um criava linhas em `users`, com e-mail arbitrário, limitado só pelo throttling do stage (100 req/s). Por outro lado, a oficina precisa cadastrar novos atendentes e mecânicos sem depender de um novo deploy.

## Decisão

`POST /api/v1/auth/register` passa para o grupo de rotas da equipe no os-service, com `Auth` e `RequireRole("admin")`, e cria o usuário com papel `admin`.

- Só quem já é da equipe cadastra outra pessoa da equipe. O primeiro `admin` continua vindo do seed na subida (`ADMIN_EMAIL` e `ADMIN_PASSWORD`; em produção, sem os dois, o seed não roda).
- O cliente não se cadastra no os-service: ele é cadastrado pela equipe (`POST /api/v1/requesters`) e se autentica pelo CPF na Lambda auth-issuer.
- No gateway nada muda: a rota segue sob `POST /api/v1/auth/{proxy+}`, sem authorizer, e a barreira é o JWT de `admin` validado no serviço (só HS256, com `exp`).

## Alternativas consideradas

1. **Manter a rota pública, com papel `client`.** É o comportamento herdado. Descartada porque o usuário criado não acessa nada e a rota só servia de vetor de escrita anônima.
2. **Remover a rota e criar a equipe só pelo seed.** Fecha o vetor de escrita, mas cada novo funcionário exigiria trocar variáveis e reiniciar o serviço. Descartada pelo custo operacional.
3. **Rota própria no gateway com o authorizer (`POST /api/v1/auth/register` com `auth = true`).** Seria mais uma camada, mas o authorizer só confere se o token é válido, não o papel: um token de cliente passaria por ele. A regra de papel continuaria no serviço. Não adotada agora; pode ser somada depois sem mudar esta decisão.
4. **Papel próprio para quem é criado (`mechanic`, `attendant`).** Daria menor privilégio dentro da equipe, mas nenhuma rota do os-service distingue esses papéis hoje. Descartada por ora.

## Consequências

### Positivas

- Nenhuma escrita anônima no banco do os-service: as únicas rotas abertas que gravam são o login (contador de tentativas e refresh token) e o refresh.
- O papel criado é coerente com o uso: a equipe opera com `admin`, e o cliente com o JWT do CPF.
- A decisão não depende do gateway: quem chama o NLB direto encontra a mesma barreira.

### Negativas

- Todo usuário da equipe é `admin`. Não há papel menor no os-service.
- Sem o seed, não há como criar o primeiro `admin` pela API.
- A rota fica numa posição incomum: dentro de `/auth`, que no gateway é aberta, mas protegida no serviço. A [tabela de rotas](../diagrams/roteamento.md#2-rotas-do-api-gateway) registra isso.

## Referências

- Rotas: `cmd/api/routes/routes.go`; handler: `internal/adapters/inbound/http/auth/register.go`; teste: `TestRegister_CriaUsuarioDaEquipe` em `internal/adapters/inbound/http/auth/register_test.go`.
- [`../SEGURANCA.md`](../SEGURANCA.md), seção 2.4.
