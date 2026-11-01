# Architecture Decision Records (ADRs)

Registro das decisões de arquitetura da oficina mecânica. Cada ADR documenta o contexto, a decisão, as alternativas descartadas e as consequências.

As ADRs 0001 a 0006 são da fase 1 (monolito), 0007 a 0011 da fase 3 (nuvem, gateway e Lambdas) e, a partir da [ADR-0012](0012-banco-por-servico.md), da fase 4 (microsserviços). As da fase 4 valem para os seis repositórios `postech-tc4-*` e ficam aqui, no os-service, que é o orquestrador da saga e o repositório que nasceu do monolito.

## Por que ADRs

ADRs ([formato de Michael Nygard](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)) são arquivos curtos, versionados com o código, que registram uma decisão no momento em que ela é tomada. Quem chega depois entende por que o sistema está como está, não só como.

## Convenções

- Numeração sequencial (`0001`, `0002`...), sem buracos.
- Cabeçalho com **Status**, **Data** e **Decisão por**; seções **Contexto**, **Decisão**, **Alternativas consideradas** (cada uma com o motivo do descarte) e **Consequências** (positivas e negativas).
- Status possíveis: `Proposta`, `Aceita`, `Aceita, revisada em AAAA-MM`, `Parcialmente substituída pela ADR-XXXX`, `Substituída pela ADR-XXXX`.
- Uma decisão não é reescrita. Se uma fase posterior muda a decisão, a ADR antiga recebe o status novo e uma nota de revisão no topo, e a decisão nova vira outra ADR. Revisões que só atualizam fatos (versão, nome de arquivo, número) ficam na nota.

## Índice

| # | Decisão | Fase | Status |
|---|---|---|---|
| [0001](0001-go-language.md) | Go como linguagem do back-end | 1 | Aceita, revisada em 2026-10 |
| [0002](0002-hexagonal-architecture.md) | Arquitetura hexagonal (Ports & Adapters) | 1 | Aceita, revisada em 2026-10 |
| [0003](0003-postgresql.md) | PostgreSQL como banco de dados relacional | 1 | Parcialmente substituída pela [ADR-0012](0012-banco-por-servico.md) |
| [0004](0004-gorm-orm.md) | GORM como ORM | 1 | Aceita, revisada em 2026-10 |
| [0005](0005-jwt-refresh-token.md) | Autenticação JWT com refresh token rotativo | 1 | Aceita, revisada em 2026-10 |
| [0006](0006-domain-errors-sentinels.md) | Erros sentinela centralizados em `domainerrors` | 1 | Aceita, revisada em 2026-11 |
| [0007](0007-api-gateway-comunicacao.md) | Comunicação entre API Gateway e aplicação | 3 | Aceita, revisada em 2026-10 |
| [0008](0008-hpa.md) | Escalabilidade de pods via HPA | 3 | Aceita, revisada em 2026-10 |
| [0009](0009-lambda-terraform-em-vez-de-sam.md) | Deploy da Lambda por Terraform, não por SAM | 3 | Aceita, revisada em 2026-10 |
| [0010](0010-cluster-unico-dois-namespaces.md) | Um cluster com dois namespaces | 3 | Parcialmente substituída pela [ADR-0021](0021-ambiente-unico-no-eks.md) |
| [0011](0011-notificacoes-smtp-no-pod.md) | Notificações por SMTP no pod, com caminho para serverless | 3 | Substituída pela [ADR-0022](0022-notificacao-por-lambda.md) |
| [0012](0012-banco-por-servico.md) | Banco por serviço: PostgreSQL no os e no billing, DynamoDB no execution | 4 | Aceita |
| [0013](0013-mensageria-sns-sqs-dlq.md) | Mensageria com SNS, SQS e DLQ | 4 | Aceita |
| [0014](0014-lambdas-fora-da-vpc.md) | Lambdas fora da VPC; a autenticação consulta o os-service | 4 | Aceita |
| [0015](0015-rds-single-az-sem-deletion-protection.md) | RDS Single-AZ e sem deletion protection no Learner Lab | 4 | Aceita |
| [0016](0016-credenciais-dos-pods-pela-labrole.md) | Credenciais dos pods pela LabRole, via IMDS com hop limit 2 | 4 | Aceita |
| [0017](0017-saga-orquestrada-no-os-service.md) | Saga orquestrada, com o orquestrador no os-service | 4 | Aceita |
| [0018](0018-outbox-inbox-e-lock-otimista.md) | Transactional outbox, inbox idempotente e lock otimista | 4 | Aceita |
| [0019](0019-api-composition-no-details.md) | API composition no `/details`, com resposta parcial | 4 | Aceita |
| [0020](0020-mercado-pago-checkout-pro.md) | Mercado Pago Checkout Pro atrás da porta `PaymentGateway` | 4 | Aceita |
| [0021](0021-ambiente-unico-no-eks.md) | Ambiente único no EKS; homologação no e2e local | 4 | Aceita |
| [0022](0022-notificacao-por-lambda.md) | Notificação por Lambda consumindo a fila de eventos da OS | 4 | Aceita |
| [0023](0023-registro-restrito-a-equipe.md) | Registro de usuário restrito à equipe | 4 | Aceita |

A fase 2 não gerou ADR nova: as escolhas dela (EKS, RDS, Terraform, HPA e o pipeline) estão no [documento de entrega da fase 2](../DOCUMENTO_ENTREGA_FASE2.md).
