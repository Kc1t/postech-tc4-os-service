# Diagrama de componentes

Visão de nuvem da fase 4: entrada, serviços, bancos, mensageria e monitoramento. A versão da fase 3 (monolito `workshop-api`) está no [`postech-tc3-app`](https://github.com/Kc1t/postech-tc3-app/blob/main/docs/diagrams/componentes.md).

```mermaid
flowchart TB
    cliente["Cliente da oficina"]
    equipe["Equipe da oficina<br/>(admin)"]
    mp["Mercado Pago<br/>Checkout Pro"]

    subgraph aws["AWS · us-east-1"]
        gw["API Gateway HTTP<br/>throttling · authorizer sem cache"]

        subgraph auth["Lambda · postech-tc4-lambda-auth"]
            issuer["auth-issuer<br/>CPF → JWT"]
            authz["auth-authorizer<br/>valida o JWT"]
        end

        subgraph eks["EKS · namespace postech"]
            os["os-service<br/>OS, cadastros, estoque<br/>orquestrador da saga"]
            bi["billing-service<br/>orçamento e pagamento"]
            ex["execution-service<br/>diagnóstico e reparo"]
            nri["nri-bundle<br/>infra e logs dos pods"]
        end

        subgraph msg["Mensageria"]
            sns{{"SNS<br/>os-events · billing-events · execution-events"}}
            sqs[["SQS<br/>os-saga-replies · billing-commands<br/>execution-commands · notification-os-events<br/>(cada uma com DLQ)"]]
        end

        notifier["notifier<br/>postech-tc4-notification-lambda"]

        rdsos[("RDS PostgreSQL 16<br/>workshop")]
        rdsbi[("RDS PostgreSQL 16<br/>billing")]
        ddbex[("DynamoDB<br/>executions")]
        ddbnt[("DynamoDB<br/>notifier-dedup")]
        sm["Secrets Manager<br/>credenciais dos bancos"]
        ecr["ECR<br/>imagens dos serviços"]
    end

    smtp["SMTP (Brevo)"]
    nr["New Relic<br/>APM · trace · logs · dashboard · alertas · SLOs"]
    gha["GitHub Actions<br/>um pipeline por repositório"]

    cliente -->|"POST /auth {cpf}<br/>Bearer JWT"| gw
    equipe -->|"POST /api/v1/auth/login<br/>Bearer JWT"| gw
    mp -->|"webhook x-signature"| gw

    gw -->|"POST /auth"| issuer
    gw -.->|"rotas protegidas"| authz
    gw -->|"HTTP_PROXY pelo NLB<br/>de cada serviço"| os & bi & ex

    issuer -->|"GET /internal/requesters/{document}<br/>X-Internal-Token"| os
    os -.->|"GET /details<br/>(API composition)"| bi & ex
    bi -->|"preferência, consulta, estorno"| mp

    os -->|"comandos"| sqs
    os & bi & ex -->|"eventos"| sns
    sns --> sqs
    sqs --> os & bi & ex
    sqs --> notifier
    notifier --> smtp
    notifier --> ddbnt

    os --> rdsos
    bi --> rdsbi
    ex --> ddbex
    rdsos & rdsbi -.->|"senha gerada pelo Terraform"| sm

    os & bi & ex -->|"agente Go"| nr
    issuer & authz & notifier -->|"layer do New Relic"| nr
    nri --> nr

    gha -->|"build e push"| ecr
    gha -->|"terraform apply e kubectl apply"| eks
    gha -->|"terraform apply"| auth & notifier
```

## Quem fala com quem

| Origem | Destino | Como | Por quê |
|---|---|---|---|
| Cliente, equipe, Mercado Pago | serviços | API Gateway → NLB `<serviço>-public` → pod | entrada única, authorizer na borda |
| Lambda issuer | os-service | `GET /internal/requesters/{document}` com `X-Internal-Token` | emitir o token sem abrir o banco do os-service ([ADR-0014](../adr/0014-lambdas-fora-da-vpc.md)) |
| os-service | billing, execution | REST no Service do cluster, com JWT de serviço | leitura composta do `/details` ([ADR-0019](../adr/0019-api-composition-no-details.md)) |
| os-service | billing, execution | comandos nas filas `billing-commands` e `execution-commands` | passos da saga ([ADR-0013](../adr/0013-mensageria-sns-sqs-dlq.md)) |
| billing, execution | os-service | eventos nos tópicos `billing-events` e `execution-events`, fila `os-saga-replies` | respostas da saga |
| os-service | notifier | evento `os.ServiceOrderStatusChanged` no tópico `os-events`, fila `notification-os-events` | e-mail a cada mudança de status ([ADR-0022](../adr/0022-notificacao-por-lambda.md)) |

Nenhum serviço acessa o banco de outro ([ADR-0012](../adr/0012-banco-por-servico.md)).

## Fronteiras de repositório

| Repositório | Provisiona |
|---|---|
| `postech-tc4-infra-k8s` | EKS e node group, namespace com quota, API Gateway, rotas e authorizer, tópicos, filas, DLQs e alarmes, agente do New Relic no cluster, dashboard, alertas e SLOs |
| `postech-tc4-os-service` | RDS `workshop` e o secret da credencial, imagem no ECR, manifests do serviço |
| `postech-tc4-billing-service` | RDS `billing` e o secret da credencial, imagem no ECR, manifests do serviço |
| `postech-tc4-execution-service` | tabela DynamoDB `executions`, imagem no ECR, manifests do serviço |
| `postech-tc4-lambda-auth` | Lambdas `auth-issuer` e `auth-authorizer` e os log groups |
| `postech-tc4-notification-lambda` | Lambda `notifier`, a tabela de deduplicação e o gatilho na fila `notification-os-events` |
