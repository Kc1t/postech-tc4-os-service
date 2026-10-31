# Architecture Decision Records (ADRs)

Esta pasta contém o registro das decisões arquiteturais relevantes do projeto **Workshop API**. Cada ADR documenta o **contexto**, a **decisão** tomada e suas **consequências**.

A partir da [ADR-0012](./0012-banco-por-servico.md), as decisões são da Fase 4 (microsserviços) e valem para os seis repositórios `postech-tc4-*`. Ficam aqui, no os-service, que é o orquestrador da saga e o repositório de origem do monolito.

## Por que ADRs?

ADRs (formato proposto por [Michael Nygard](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)) são arquivos curtos, versionados junto com o código, que registram decisões importantes no momento em que são tomadas. Servem como memória arquitetural — quem chega depois entende **por que** o sistema está como está, não apenas **como**.

## Convenções

- Numeração sequencial (`0001`, `0002`, …) sem buracos.
- Formato fixo: **Status**, **Contexto**, **Decisão**, **Alternativas consideradas**, **Consequências**.
- Status possíveis: `Proposed`, `Accepted`, `Deprecated`, `Superseded by ADR-XXXX`, `Partially superseded by ADR-XXXX`.
- Decisão registrada **uma vez** — se mudar, cria-se um novo ADR que supersede o anterior.

## Índice

| #    | Decisão                                           | Status   |
| ---- | ------------------------------------------------- | -------- |
| [0001](./0001-go-language.md)            | Go como linguagem do back-end                    | Accepted |
| [0002](./0002-hexagonal-architecture.md) | Arquitetura hexagonal (Ports & Adapters)         | Accepted |
| [0003](./0003-postgresql.md)             | PostgreSQL como banco de dados relacional        | Accepted |
| [0004](./0004-gorm-orm.md)               | GORM como ORM                                    | Accepted |
| [0005](./0005-jwt-refresh-token.md)      | Autenticação JWT com refresh token rotativo     | Accepted |
| [0006](./0006-domain-errors-sentinels.md)| Erros sentinela centralizados em `domainerrors` | Accepted |
| [0007](./0007-api-gateway-comunicacao.md) | Comunicação entre API Gateway e aplicação        | Accepted |
| [0008](./0008-hpa.md)                    | Escalabilidade de pods via HPA                   | Accepted |
| [0009](./0009-lambda-terraform-em-vez-de-sam.md) | Deploy da Lambda por Terraform, não por SAM | Accepted |
| [0010](./0010-cluster-unico-dois-namespaces.md) | Um cluster com dois namespaces           | Partially superseded by ADR-0021 |
| [0011](./0011-notificacoes-smtp-no-pod.md) | Notificações por SMTP no pod, com caminho para serverless | Superseded by ADR-0022 |
| [0012](./0012-banco-por-servico.md) | Banco por serviço: PostgreSQL no os e no billing, DynamoDB no execution | Accepted |
| [0013](./0013-mensageria-sns-sqs-dlq.md) | Mensageria com SNS, SQS e DLQ | Accepted |
| [0014](./0014-lambdas-fora-da-vpc.md) | Lambdas fora da VPC; a autenticação consulta o os-service | Accepted |
| [0015](./0015-rds-single-az-sem-deletion-protection.md) | RDS Single-AZ e sem deletion protection no Learner Lab | Accepted |
| [0016](./0016-credenciais-dos-pods-pela-labrole.md) | Credenciais dos pods pela LabRole, via IMDS com hop limit 2 | Accepted |
| [0017](./0017-saga-orquestrada-no-os-service.md) | Saga orquestrada, com o orquestrador no os-service | Accepted |
| [0018](./0018-outbox-inbox-e-lock-otimista.md) | Transactional outbox, inbox idempotente e lock otimista | Accepted |
| [0019](./0019-api-composition-no-details.md) | API composition no `/details`, com resposta parcial | Accepted |
| [0020](./0020-mercado-pago-checkout-pro.md) | Mercado Pago Checkout Pro atrás da porta `PaymentGateway` | Accepted |
| [0021](./0021-ambiente-unico-no-eks.md) | Ambiente único no EKS; homologação no e2e local | Accepted |
| [0022](./0022-notificacao-por-lambda.md) | Notificação por Lambda consumindo a fila de eventos da OS | Accepted |
