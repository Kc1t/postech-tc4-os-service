# RFC-0004: Decomposição do monolito em microsserviços (Strangler Fig)

- **Status:** Aceita
- **Data:** 2026-10
- **Autor:** Reservoir Devs
- **Decisões relacionadas:** ADR-0012 a ADR-0022

## Resumo

Propõe decompor a `workshop-api` da Fase 3 em **três microsserviços por capacidade de negócio**: `os-service` (atendimento e orquestração), `billing-service` (orçamento e pagamento) e `execution-service` (diagnóstico e reparo). Autenticação e notificação ficam em duas Lambdas. A migração segue o padrão **Strangler Fig**: o monolito vira o os-service, as capacidades saem dele uma por vez, e o API Gateway é a fachada que troca o destino de cada rota.

## Motivação

O enunciado da Fase 4 pede no mínimo três microsserviços, cada um com repositório, infraestrutura e banco próprios; proíbe um serviço de acessar o banco de outro; e pede o Saga Pattern para a abertura da OS, a aprovação e o pagamento. Sugere como exemplo os serviços de Ordem de Serviço, Orçamento e Pagamento e Execução e Produção.

A pergunta desta RFC não é se decompor, que o enunciado já decide. É **por onde cortar** e **em que ordem**.

O ponto de partida é o monolito da Fase 3:

| Capacidade | Como era na Fase 3 |
|---|---|
| Autenticação da equipe | e-mail e senha com refresh token (`users`, `refresh_tokens`) |
| Clientes e veículos | `requesters`, `vehicles` |
| Catálogo e estoque | `services`, `parts` |
| Ordem de serviço | `service_orders`; a equipe movia o status à mão (`PUT /service-orders/:id/status`) |
| Aprovação do orçamento | o cliente movia o status (`PUT /service-orders/code/:code/status`); o estoque baixava nessa transação |
| Pagamento | não existia |
| Notificação | SMTP síncrono dentro do pod ([ADR-0011](../adr/0011-notificacoes-smtp-no-pod.md)) |
| Autenticação por CPF | Lambda lendo `requesters` direto no RDS |

Tudo num banco, com uma transação local para cada mudança.

## Proposta

### Critério de corte: capacidade de negócio

O enunciado descreve a oficina por áreas: atendimento, diagnóstico, execução e cobrança. O corte segue os pontos em que a OS **muda de mãos** e em que os dados têm ciclo de vida e exigência de consistência próprios.

| Serviço | Capacidade | Dados | Banco |
|---|---|---|---|
| **os-service** | Atendimento: abertura, consulta e histórico da OS; clientes e veículos; catálogo e estoque; autenticação da equipe; **orquestração da saga** | `service_orders`, `saga_instances`, `service_order_status_history`, `requesters`, `vehicles`, `services`, `parts`, `users`, `refresh_tokens` | RDS PostgreSQL |
| **billing-service** | Cobrança: orçamento, aprovação ou recusa pelo cliente, pagamento no Mercado Pago, webhook, estorno | `budgets`, `payments` | RDS PostgreSQL |
| **execution-service** | Produção: fila do mecânico, diagnóstico, reparo, linha do tempo | itens `EXEC#{os_id}` da tabela `executions` | DynamoDB |
| `postech-tc4-lambda-auth` | CPF → JWT e authorizer do gateway | nenhum; consulta o os-service | — |
| `postech-tc4-notification-lambda` | E-mail a cada mudança de status | dedup por `message_id` | DynamoDB |

Os três serviços têm `outbox_messages` e `processed_messages` (ou os itens equivalentes no DynamoDB) para a mensageria ([ADR-0018](../adr/0018-outbox-inbox-e-lock-otimista.md)).

**Por que o catálogo e o estoque ficam no os-service.** Quando o diagnóstico chega, o orquestrador resolve os itens com o preço do catálogo e reserva as peças **na mesma transação** que avança a saga ([ADR-0017](../adr/0017-saga-orquestrada-no-os-service.md)). Num serviço à parte, a reserva viraria um passo remoto da saga, com comando, resposta e compensação próprios, e o preço exigiria uma chamada síncrona ou uma cópia do catálogo. O catálogo não tem, aqui, equipe nem ciclo de vida que justifiquem esse custo.

**Por que a aprovação vai para o billing.** O cliente aprova um **orçamento**, não um status de OS. O orçamento tem dono, valor e itens congelados no momento da criação, e a aprovação dispara o pagamento, que também é do billing. O os-service só fica sabendo por evento.

### O que muda no modelo

- **A OS abre sem itens.** Serviços e peças vêm do diagnóstico do mecânico (códigos e quantidades), e o os-service põe o preço.
- **Novos status:** `awaiting_budget`, `awaiting_payment` e `cancelled`. O ciclo completo é `received → in_diagnosis → awaiting_budget → awaiting_approval → awaiting_payment → in_execution → finished → delivered`.
- **O status deixa de ser movido à mão.** `PUT /service-orders/:id/status` e `PUT /service-orders/code/:code/status` saem; quem move é a saga, a partir das ações do mecânico no execution e do cliente no billing.
- **O estoque é reservado depois do diagnóstico** e devolvido na compensação. Na Fase 3 baixava na aprovação.

### Strangler Fig: a fachada

O API Gateway HTTP já existia na frente do monolito ([ADR-0007](../adr/0007-api-gateway-comunicacao.md)). Ele vira a fachada do estrangulamento: cada rota aponta para o NLB de um serviço, e trocar o dono de uma rota é mudar uma linha do mapa de rotas no `api_gateway.tf`.

| Rotas | Serviço |
|---|---|
| `/api/v1/service-orders`, `/api/v1/requesters`, `/api/v1/vehicles`, `/api/v1/services`, `/api/v1/parts`, `POST /api/v1/auth/*` (equipe), `GET /health` | os-service |
| `/api/v1/budgets/*`, `/api/v1/payments/*`, `/fake-checkout/*`, `POST /api/v1/webhooks/mercadopago` | billing-service |
| `/api/v1/executions/*` | execution-service |
| `POST /auth` | Lambda issuer |

O Terraform só cria as rotas de um serviço quando o NLB dele existe. Um serviço novo entra no ar sem quebrar as rotas que já funcionam.

### Ordem de migração

Cada passo deixa o sistema funcionando e testável sozinho.

1. **Preparar o monolito.** Renomear o módulo para `postech-tc4-os-service`, mantendo o histórico do repositório, e criar o chassis em `pkg/`: contratos das mensagens com versão no envelope, publicador e consumidor SNS/SQS, outbox e inbox. Nenhum comportamento muda.
2. **Orquestrador e contratos.** Escrever a saga no os-service contra os contratos de `pkg/contracts`, antes de os outros serviços existirem. O BDD da saga roda com o orquestrador real e o PostgreSQL real, com os passos do teste fazendo o papel do billing e do execution. Os contratos ficam definidos antes de qualquer extração.
3. **Extrair o execution-service e o billing-service, em paralelo.** Os dois só dependem do contrato com o os-service, não um do outro. Cada um nasce com a cópia do chassis, o próprio banco e o teste de componente contra LocalStack.
4. **Tirar as Lambdas do banco.** A notificação passa a consumir `os-events` ([ADR-0022](../adr/0022-notificacao-por-lambda.md)), e a Lambda de autenticação passa a chamar `GET /internal/requesters/{doc}` ([ADR-0014](../adr/0014-lambdas-fora-da-vpc.md)). Só depois disso o banco do os-service pode ficar fechado ao cluster.
5. **Plataforma e corte.** Criar a topologia de filas e tópicos no Terraform ([ADR-0013](../adr/0013-mensageria-sns-sqs-dlq.md)), ligar as rotas do billing e do execution no gateway, validar o fluxo inteiro no e2e local e remover do os-service o que foi estrangulado: as rotas de status manual e o adapter SMTP.

## Alternativas avaliadas

| Alternativa | Por que não |
|---|---|
| **Manter o monolito, modularizado** | Para o tamanho real de uma oficina, é a opção mais simples e provavelmente suficiente: um deploy, transações locais, nenhuma saga. A arquitetura hexagonal ([ADR-0002](../adr/0002-hexagonal-architecture.md)) já separa portas e adapters. Descartada porque o enunciado exige três serviços com repositório, infraestrutura e banco próprios. Seria o passo intermediário natural num projeto sem esse requisito. |
| **Decompor por entidade** (cliente, veículo, peça, serviço, OS) | Cada caso de uso atravessaria a rede: abrir uma OS chamaria três serviços para validar cliente, veículo e posse; reservar peças viraria transação distribuída. Os serviços seriam tabelas com API, sem regra própria, e conversariam o tempo todo. |
| **Mais serviços: catálogo e estoque separados** | Exigiria a reserva de peças como passo remoto da saga, com compensação própria, e o preço por chamada síncrona ou cópia do catálogo. Seriam mais um repositório, um banco e um pipeline, sem ganho de autonomia. |
| **Mais serviços: clientes e veículos separados** | A abertura da OS e a emissão de token passariam a depender de mais um serviço síncrono. O cadastro é pequeno e só o atendimento escreve nele. |
| **Reescrever do zero (big bang)** | Três serviços novos e o monolito desligado no fim. Descartado: perderia o código testado e o histórico da Fase 3, que seguem no os-service, e não teria ponto intermediário estável para validar. |

## Impacto no custo

Estimativa `us-east-1`, on-demand, com os preços da RFC-0001:

| Item | Fase 3 (US$/hora) | Fase 4 (US$/hora) |
|---|---|---|
| EKS control plane | 0,100 | 0,100 |
| Nós `t3.medium` | 2× = 0,083 | 3× = 0,125 (`node_desired = 3`) |
| RDS `db.t3.micro` | 1× = 0,018 | 2× = 0,036 |
| Armazenamento (EBS + RDS) | 0,008 | ~0,012 |
| DynamoDB, SNS, SQS, Lambda | ~0 | ~0 (por requisição) |
| **Total** | **~0,21** | **~0,27** |

A decomposição custa ~US$ 0,06/hora a mais, quase tudo do terceiro nó e do segundo RDS. As contenções estão nas ADRs: Single-AZ e destruição por sessão ([ADR-0015](../adr/0015-rds-single-az-sem-deletion-protection.md)), um ambiente só na AWS ([ADR-0021](../adr/0021-ambiente-unico-no-eks.md)) e banco NoSQL sob demanda no execution ([ADR-0012](../adr/0012-banco-por-servico.md)).

## Riscos

| Risco | Mitigação |
|---|---|
| Perder a consistência que a transação local dava | Saga com compensação em todas as etapas ([ADR-0017](../adr/0017-saga-orquestrada-no-os-service.md)); outbox, inbox e lock otimista ([ADR-0018](../adr/0018-outbox-inbox-e-lock-otimista.md)) |
| Um serviço fora parar o fluxo | Comunicação assíncrona: a mensagem espera na fila. A consulta síncrona (`/details`) devolve resposta parcial, com circuit breaker ([ADR-0019](../adr/0019-api-composition-no-details.md)) |
| Contratos divergirem entre serviços | Versão no envelope: o consumidor rejeita versão maior que a sua. `pkg/contracts` é a fonte canônica, copiada literalmente nos outros serviços, e o `make contracts` do e2e local (`postech-tc4-infra-k8s/local/check-contracts.sh`) falha se a cópia de algum dos quatro repositórios divergir |
| Cópias do chassis divergirem | Cópia explícita e sincronizada a cada mudança. O próximo passo é um módulo Go versionado compartilhado |
| Rastrear uma OS por três serviços | `saga_id`, `os_id` e `correlation_id` em todo log; o trace do New Relic atravessa as filas e o outbox |
| A migração não ter sido exercitada com tráfego real | Registrado: o sistema não tinha usuários, e o corte das rotas foi feito de uma vez. O padrão foi seguido na estrutura (um passo por vez, cada um testável), não validado sob carga |

## Questões em aberto

- Com várias filiais, cada uma com estoque próprio (o cenário do enunciado), o estoque deveria sair do os-service? A reserva passaria a depender da filial, e talvez justificasse o serviço que esta RFC descartou.
- O chassis em `pkg/` deveria virar um módulo Go compartilhado, versionado, em vez de cópia em cada repositório?
- O `/details` deveria virar uma visão materializada (CQRS) se a consulta crescer ([ADR-0019](../adr/0019-api-composition-no-details.md))?
