# Roteamento

Toda requisição externa passa por duas decisões: o **API Gateway** escolhe o serviço e decide se chama o authorizer; depois o **roteador do serviço** escolhe o grupo de middlewares. Este documento desenha as duas para a fase 4, com o os-service como exemplo do lado do serviço. A versão da fase 3 (um backend só) está no [`postech-tc3-app`](https://github.com/Kc1t/postech-tc3-app/blob/main/docs/diagrams/roteamento.md).

## 1. O caminho de uma requisição

```mermaid
flowchart LR
    req(["Requisição"]) --> gw{{"API Gateway<br/>casamento de rota"}}

    gw -->|"POST /auth"| iss["Lambda auth-issuer"]
    gw -->|"rota aberta"| nlb["NLB do serviço"]
    gw -->|"rota protegida"| az{{"Lambda auth-authorizer"}}
    gw -->|"sem rota"| r404(["404"])

    az -->|"isAuthorized: false"| r403(["403"])
    az -->|"isAuthorized: true"| nlb

    iss -->|"GET /internal/requesters/{document}"| os
    nlb --> os["os-service"] & bi["billing-service"] & ex["execution-service"]
    os & bi & ex --> mw{{"middlewares do serviço<br/>JWT de novo, papel, posse"}}

    style r403 fill:#ffe4e1,stroke:#c66
    style r404 fill:#ffe4e1,stroke:#c66
    style az fill:#fff4e0,stroke:#d90
    style gw fill:#e8f0fe,stroke:#4a7
```

Passar pelo authorizer não dispensa os middlewares do serviço. São duas decisões independentes sobre o mesmo token, e a segunda ainda confere papel e posse. O NLB de cada serviço é público, então essa segunda validação também cobre quem chama o NLB direto ([ADR-0007](../adr/0007-api-gateway-comunicacao.md)).

## 2. Rotas do API Gateway

Declaradas em [`postech-tc4-infra-k8s/api_gateway.tf`](https://github.com/Kc1t/postech-tc4-infra-k8s/blob/main/api_gateway.tf). A integração com os serviços é `HTTP_PROXY` para o NLB do Service `<serviço>-public`, com o caminho original repassado inteiro e timeout de 10 s.

| Rota | Destino | Authorizer |
|---|---|---|
| `POST /auth` | Lambda auth-issuer | não (limite próprio: 10 req/s, rajada de 20) |
| `GET /health` | os-service | não |
| `POST /api/v1/auth/{proxy+}` | os-service (login, refresh, logout e register da equipe) | não; o logout e o register exigem JWT no serviço, e o register, papel `admin` |
| `ANY /api/v1/service-orders` e `/{proxy+}` | os-service | sim |
| `ANY /api/v1/requesters`, `/vehicles`, `/services`, `/parts` e `/{proxy+}` de cada um | os-service | sim |
| `ANY /api/v1/budgets/{proxy+}` | billing-service | sim |
| `ANY /api/v1/payments/{proxy+}` | billing-service | sim |
| `POST /api/v1/webhooks/mercadopago` | billing-service | não (a autenticidade vem da assinatura `x-signature`) |
| `GET /fake-checkout/{proxy+}` | billing-service | não (página do gateway fake de pagamento) |
| `ANY /api/v1/executions/{proxy+}` | execution-service | sim |

- O stage `$default` limita a 100 req/s com rajada de 200 e grava access log no CloudWatch.
- O authorizer é do tipo `REQUEST`, com resposta simples e cache desligado. Sem o header `Authorization` o gateway responde 401 sem chamar a Lambda; com token recusado, 403.
- Uma rota protegida só é criada quando o authorizer existe, e as rotas de um serviço só são criadas quando o NLB dele existe. O Terraform acha as Lambdas pelo nome e os NLBs pela tag `kubernetes.io/service-name`, sem valores copiados à mão entre repositórios.
- `/internal/*` não tem rota no gateway.

## 3. Roteamento no os-service

```mermaid
flowchart TD
    pod(["requisição no pod"]) --> base["Recovery · nrgin · CorrelationID · RequestLogger · CORS"]
    base --> grp{{"grupo da rota"}}

    grp -->|"/health, /swagger, /api/v1/auth/login e refresh"| open["aberta"]
    grp -->|"/internal/*"| tok{{"X-Internal-Token"}}
    grp -->|"/api/v1/auth/logout,<br/>/service-orders/code/{code}, /service-orders/requester"| auth1{{"Auth: JWT HS256 com exp"}}
    grp -->|"demais /api/v1,<br/>inclusive /api/v1/auth/register"| auth2{{"Auth: JWT HS256 com exp"}}

    tok -->|"sem token configurado"| r503(["503"])
    tok -->|"diferente"| r401a(["401"])
    tok -->|"igual"| hint["handler interno"]

    auth1 -->|"ausente ou inválido"| r401b(["401"])
    auth1 -->|"ok"| posse{{"document do token<br/>= document pedido?"}}
    posse -->|"não"| r403a(["403"])
    posse -->|"sim"| hcli["handler do cliente<br/>(OS de outro cliente: 404)"]

    auth2 -->|"ausente ou inválido"| r401c(["401"])
    auth2 -->|"ok"| role{{"RequireRole(admin)"}}
    role -->|"outro papel"| r403b(["403"])
    role -->|"admin"| uuid{{"UUIDParams: {id} é UUID?"}}
    uuid -->|"não"| r400(["400"])
    uuid -->|"sim"| hadm["handler da equipe"]

    style r401a fill:#ffe4e1,stroke:#c66
    style r401b fill:#ffe4e1,stroke:#c66
    style r401c fill:#ffe4e1,stroke:#c66
    style r403a fill:#ffe0b2,stroke:#e80
    style r403b fill:#ffe0b2,stroke:#e80
    style r503 fill:#ffe4e1,stroke:#c66
    style r400 fill:#ffe4e1,stroke:#c66
```

O `UUIDParams` roda nos grupos `service-orders`, `requesters`, `vehicles`, `services` e `parts`, depois da autenticação e do papel. Na consulta do cliente por código, o handler recusa código não numérico ou ≤ 0 e CPF/CNPJ inválido com 400 antes de conferir a posse. O `logout` usa o mesmo grupo autenticado das rotas do cliente, sem a checagem de posse. As rotas estão em [`cmd/api/routes/routes.go`](../../cmd/api/routes/routes.go) e em cada `handler.go` de `internal/adapters/inbound/http`.

### Inventário do os-service

| Grupo | Rotas | Acesso |
|---|---|---|
| Saúde e documentação | `GET /health`, `/health/live`, `/health/ready`, `/swagger/*` | aberta |
| Autenticação da equipe | `POST /api/v1/auth/login` e `/refresh`; `POST /api/v1/auth/logout`; `POST /api/v1/auth/register` | login e refresh abertas; logout com qualquer JWT; register só `admin` ([ADR-0023](../adr/0023-registro-restrito-a-equipe.md)) |
| Interna | `GET /internal/requesters/{document}` | `X-Internal-Token` |
| Cliente | `GET /api/v1/service-orders/code/{code}?document=`, `GET /api/v1/service-orders/requester?document=` | JWT com o `document` pedido |
| Solicitantes | `POST`, `GET` em `/api/v1/requesters`; `GET /document/{document}`; `GET`, `PUT`, `DELETE /{id}`; `GET /{id}/vehicles` | `admin` |
| Veículos, serviços | `POST`, `GET` na coleção; `GET`, `PUT`, `DELETE /{id}` | `admin` |
| Peças | as mesmas e `PATCH /api/v1/parts/{id}/stock` | `admin` |
| Ordens de serviço | `POST`, `GET /api/v1/service-orders`; `GET /{id}`, `/{id}/details`, `/{id}/history`; `POST /{id}/cancel`, `/{id}/deliver`; `GET /metrics/execution-time` | `admin` |

Contratos completos no [Swagger](../swagger.yaml). Papéis e posse dos outros serviços em [`../SEGURANCA.md`](../SEGURANCA.md).

## 4. Onde nasce cada código de resposta

```mermaid
flowchart LR
    subgraph gwl["API Gateway"]
        g404["404<br/>rota não declarada"]
        g401["401<br/>sem header Authorization"]
        g403["403<br/>authorizer recusou"]
        g429["429<br/>throttling"]
    end

    subgraph lam["Lambda auth-issuer"]
        l400["400<br/>payload ou CPF inválido"]
        l404["404<br/>cliente não encontrado"]
        l403["403<br/>cliente inativo"]
        l503["503<br/>os-service fora"]
    end

    subgraph appl["os-service"]
        a400["400<br/>id que não é UUID, código ou<br/>CPF/CNPJ inválido, corpo inválido,<br/>X-Simulate-Failure inválido"]
        a401["401<br/>token ausente ou inválido,<br/>credenciais erradas"]
        a403["403<br/>papel insuficiente,<br/>documento alheio"]
        a404["404<br/>recurso inexistente,<br/>inclusive no DELETE"]
        a409["409<br/>já existe, transição inválida,<br/>atualização concorrente,<br/>cadastro em uso por OS"]
        a422["422<br/>CPF ou placa inválidos no cadastro,<br/>estoque insuficiente,<br/>veículo de outro cliente"]
        a500["500<br/>erro não mapeado"]
    end

    style g401 fill:#ffe4e1,stroke:#c66
    style g403 fill:#ffe0b2,stroke:#e80
    style a401 fill:#ffe4e1,stroke:#c66
    style a403 fill:#ffe0b2,stroke:#e80
    style l403 fill:#ffe0b2,stroke:#e80
```

O mesmo código pode vir de camadas diferentes, e isso importa ao depurar. Um 401 ou 403 do gateway não aparece nos logs do serviço, porque a requisição nunca chega ao pod: a evidência está no access log do gateway (campo `authorizer`) e nos logs da Lambda authorizer. O corpo de erro dos serviços é sempre `{"error": {"code", "message"}}`; a tabela erro → status está em [`internal/adapters/inbound/http/errors.go`](../../internal/adapters/inbound/http/errors.go) e na [ADR-0006](../adr/0006-domain-errors-sentinels.md).
