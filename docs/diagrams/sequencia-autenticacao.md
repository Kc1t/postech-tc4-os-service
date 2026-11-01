# Diagrama de sequência: autenticação por CPF

O cliente troca o CPF por um JWT e usa o token numa rota protegida. Na fase 4 a Lambda issuer não abre mais o banco: consulta o os-service pela rota interna ([ADR-0014](../adr/0014-lambdas-fora-da-vpc.md)). A versão da fase 3 está no [`postech-tc3-app`](https://github.com/Kc1t/postech-tc3-app/blob/main/docs/diagrams/sequencia-autenticacao.md).

```mermaid
sequenceDiagram
    autonumber
    actor C as Cliente
    participant GW as API Gateway
    participant IS as Lambda auth-issuer
    participant AZ as Lambda auth-authorizer
    participant OS as os-service
    participant BI as billing-service

    rect rgb(238, 245, 255)
    note over C,OS: Emissão do token (rota aberta, 10 req/s)
    C->>GW: POST /auth {"cpf": "529.982.247-25"}
    GW->>IS: invoca (AWS_PROXY)
    IS->>IS: normaliza e valida os dígitos verificadores
    alt CPF inválido
        IS-->>C: 400
    else CPF válido
        IS->>OS: GET /internal/requesters/52998224725<br/>X-Internal-Token
        alt não encontrado
            OS-->>IS: 404
            IS-->>C: 404
        else status diferente de active
            OS-->>IS: 200, status inactive
            IS-->>C: 403
        else os-service fora (5xx ou timeout de 3 s, depois de 2 tentativas)
            IS-->>C: 503
        else ativo
            OS-->>IS: 200 {id, name, email, status}
            IS->>IS: assina HS256: sub, role=client, name, email, document, iat, exp (15 min)
            IS-->>C: 200 {access_token, token_type, expires_at, expires_in}
        end
    end
    end

    rect rgb(240, 250, 240)
    note over C,BI: Rota protegida: o JWT é validado duas vezes
    C->>GW: POST /api/v1/budgets/{id}/approve<br/>Authorization: Bearer <jwt>
    alt sem header Authorization
        GW-->>C: 401 (o authorizer nem é chamado)
    else com header
        GW->>AZ: invoca o authorizer (TTL de cache 0)
        AZ->>AZ: valida assinatura HS256, exp, sub e role
        alt token inválido ou expirado
            AZ-->>GW: isAuthorized = false
            GW-->>C: 403
        else token válido
            AZ-->>GW: isAuthorized = true<br/>context: subject, role, document
            GW->>BI: HTTP_PROXY pelo NLB, com o header Authorization
            BI->>BI: valida o mesmo JWT de novo
            BI->>BI: document do token é o dono do orçamento?
            alt outro CPF
                BI-->>C: 403
            else dono
                BI-->>C: 200, orçamento aprovado (evento BudgetApproved para a saga)
            end
        end
    end
    end
```

## Pontos de decisão

| Ponto | Regra |
|---|---|
| Validação do CPF | dígitos verificadores calculados na Lambda, antes de qualquer chamada ao os-service |
| Consulta do cliente | `GET /internal/requesters/{document}` no os-service, com `X-Internal-Token`; a rota não existe no gateway |
| Status | só `active` recebe token; qualquer outro valor resulta em 403 |
| Falha do os-service | timeout de 3 s por tentativa, uma nova tentativa em erro de rede ou 5xx, e então 503 |
| Cache do authorizer | zero: desativar um cliente vale na requisição seguinte |
| Segunda validação | cada serviço repete a verificação do JWT, o que cobre o acesso direto ao NLB |
| Posse | o os-service e o billing conferem se o `document` do token é o dono do recurso (403 se não for) |

A resposta separa "não existe" (404) de "existe mas está inativo" (403). Isso ajuda o cliente legítimo e permite enumerar CPFs cadastrados; a mitigação é o limite próprio de `POST /auth` no gateway (10 req/s, rajada de 20), registrada como risco aceito na [RFC-0003](../rfc/0003-autenticacao-por-cpf.md). Controles e testes de cada camada em [`../SEGURANCA.md`](../SEGURANCA.md).
