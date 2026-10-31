# ADR-0017: Saga orquestrada, com o orquestrador dentro do os-service

- **Status:** Accepted
- **Data:** 2026-10
- **Decisão por:** Reservoir Devs

## Contexto

Na Fase 3 o ciclo da OS era uma transação local: abrir a OS gravava os itens e baixava o estoque, e a equipe e o cliente moviam o status por `PUT /service-orders/:id/status` e `PUT /service-orders/code/:code/status`. Com a decomposição ([RFC-0004](../rfc/0004-decomposicao-em-microsservicos.md)), o mesmo ciclo atravessa três bancos: o diagnóstico e o reparo vivem no execution-service, o orçamento e o pagamento no billing-service. Não existe mais uma transação que cubra tudo.

O enunciado pede o Saga Pattern com compensação em qualquer etapa, orquestrado ou coreografado, com a escolha justificada. O fluxo tem características que pesam na escolha:

- **É longo e tem espera humana.** O mecânico diagnostica e repara; o cliente aprova o orçamento e paga. Entre um passo e outro passam horas ou dias.
- **A compensação depende do que já aconteceu.** Se houve pagamento, é preciso estornar; se já havia peças reservadas, devolver ao estoque; se o orçamento existe, cancelar.
- **Tem prazos.** Orçamento sem resposta e pagamento não confirmado não podem prender a OS (e o estoque) para sempre.

## Decisão

**Saga orquestrada. O orquestrador é o os-service**, que guarda o estado da saga na tabela `saga_instances` (uma linha por OS), manda comandos para `billing-commands` e `execution-commands` e reage às respostas que chegam por `os-saga-replies`.

Cada passo é **uma transação local**. Nas respostas, no cancelamento e na expiração, ela trava a linha da saga (`SELECT ... FOR UPDATE`) e então atualiza OS, saga, histórico (`service_order_status_history`) e estoque e grava os comandos e eventos no outbox ([ADR-0018](./0018-outbox-inbox-e-lock-otimista.md)).

### Passos

| # | Gatilho | O orquestrador faz | Estado da saga → status da OS |
|---|---|---|---|
| 1 | `POST /service-orders` (equipe) | grava OS e saga; envia `execution.StartDiagnosis` | `awaiting_diagnosis` → `received` |
| — | `execution.DiagnosisStarted` | só avança a OS | → `in_diagnosis` |
| 2 | `execution.DiagnosisCompleted` | resolve serviços e peças **com o preço do catálogo**, reserva estoque, envia `billing.CreateBudget` | `awaiting_budget` → `awaiting_budget` |
| 3 | `billing.BudgetCreated` | guarda o `budget_id` e abre o prazo de aprovação | `awaiting_approval` → `awaiting_approval` |
| 4 | `billing.BudgetApproved` (cliente, no billing) | abre o prazo de pagamento; envia `billing.RequestPayment` | `awaiting_payment` → `awaiting_payment` |
| — | `billing.PaymentLinkCreated` | guarda o link e publica o status com `checkout_url` | — |
| 5 | `billing.PaymentApproved` | envia `execution.StartRepair` | `executing` → `in_execution` |
| — | `execution.RepairStarted` | registra o início do reparo | — |
| 6 | `execution.RepairFinished` | encerra a saga | `completed` → `finished` |
| 7 | `POST /service-orders/{id}/deliver` (equipe) | entrega o veículo | → `delivered` |

### Compensação

Falhas de negócio disparam a compensação: item fora do catálogo, estoque insuficiente, `BudgetCreationFailed`, `BudgetRejected`, `PaymentFailed`, `RepairRejected`, prazo vencido ou `POST /service-orders/{id}/cancel` pela equipe. O plano sai do estado da saga (`Saga.Compensate`) e vale para as etapas já concluídas, da mais recente para a mais antiga:

| Etapa concluída | Compensação | Onde |
|---|---|---|
| Pagamento aprovado | `billing.RefundPayment` | billing (estorno no Mercado Pago, [ADR-0020](./0020-mercado-pago-checkout-pro.md)) |
| Orçamento criado | `billing.CancelBudget` | billing |
| Peças reservadas | devolução ao estoque | local, na mesma transação |
| Execução aberta | `execution.CancelExecution` | execution |

Os comandos de compensação saem todos na mesma transação, pelo outbox: não se perdem, mas são processados em paralelo pelos destinatários. Sem pagamento, a saga termina em `cancelled` na hora. Com pagamento, fica em `compensating` até o `billing.PaymentRefunded`: o estorno depende do provedor e é a única compensação cuja confirmação a saga espera. A OS termina `cancelled` com o motivo. `BudgetCancelled` e `ExecutionCancelled` são só confirmação e vão para o log.

Um pagamento aprovado **depois** do cancelamento (o prazo venceu enquanto o cliente pagava) leva a saga de volta a `compensating` e dispara o estorno.

### Prazos — `ExpireSagas`

O `BudgetCreated` grava `deadline_at = agora + SAGA_APPROVAL_TIMEOUT` (72 h), e o `BudgetApproved` grava `agora + SAGA_PAYMENT_TIMEOUT` (24 h). Um worker do os-service roda o caso de uso `ExpireSagas` a cada `SAGA_TIMEOUT_CHECK_INTERVAL` (1 min): lista até 50 sagas em `awaiting_approval` ou `awaiting_payment` com prazo vencido, trava cada uma, **revalida** o prazo (outra réplica ou uma resposta pode ter chegado antes) e compensa com o motivo "prazo de aprovação do orçamento expirado" ou "prazo de pagamento expirado".

### Demonstração de falhas

Com `FAULT_INJECTION_ENABLED=true`, o header `X-Simulate-Failure: budget|payment|repair` na abertura da OS grava a falha na saga, que a repassa no comando da etapa correspondente. Desligado, o header é ignorado.

## Alternativas consideradas

1. **Coreografia.** Cada serviço reagiria aos eventos dos outros, sem coordenador. Funciona bem para sagas curtas e lineares, que é o caso em que a literatura (Richardson, aula SAGA 2) a recomenda. Aqui o plano de compensação depende de quais etapas já aconteceram, e esse conhecimento ficaria espalhado: o billing precisaria saber se a execução foi recusada para estornar, e o os-service precisaria juntar eventos de dois serviços para saber se devolve peças. Os prazos não teriam dono, e "em que passo está a OS X" exigiria correlacionar eventos de três serviços.

2. **Orquestrador num serviço próprio (`saga-service`).** Separaria a coordenação do domínio da OS. Descartado: o orquestrador precisa mudar a OS, o histórico e o estoque a cada passo. Num serviço à parte, cada uma dessas mudanças viraria mais um comando, mais uma resposta e mais uma compensação. Dentro do os-service, é uma transação local.

3. **AWS Step Functions.** Orquestração gerenciada, com visualização do fluxo. Descartado na [ADR-0013](./0013-mensageria-sns-sqs-dlq.md): a saga sairia do código Go e dos testes, e cada espera humana viraria um callback com task token.

4. **Motor de workflow (Temporal, Camunda).** Resolve sagas longas com prazos de forma madura. Descartado pelo custo de operar um cluster a mais no Learner Lab, para uma única saga.

5. **Transação distribuída (2PC).** SQS e DynamoDB não participam de XA, e travar recursos de três serviços durante dias de espera humana é inviável.

## Consequências

### Positivas

- O estado da saga é uma linha consultável. O `/details` mostra o estado, o prazo e o motivo da falha ([ADR-0019](./0019-api-composition-no-details.md)), e os logs `saga_started`, `saga_step`, `saga_compensated`, `saga_expired` e `saga_completed` carregam `saga_id` e `os_id`.
- A compensação é decidida num lugar só, a partir do que foi concluído. As respostas tardias, duplicadas ou fora de ordem são ignoradas: todo handler confere a etapa antes de escrever (`ErrSagaOutOfStep`).
- O BDD (`features/saga.feature`) cobre o caminho feliz, as três falhas simuladas, a recusa do orçamento, o prazo vencido e a mensagem reentregue. O e2e local roda o caminho feliz e três compensações com os três serviços reais.
- O preço vem do catálogo do os-service no momento do diagnóstico. O mecânico informa só códigos e quantidades.

### Negativas

- **O os-service conhece o contrato do billing e do execution.** Um comando novo ou uma mudança de payload mexe no orquestrador. É o acoplamento que a coreografia evitaria.
- **Com o os-service fora, a saga para.** As respostas se acumulam em `os-saga-replies` sem se perder, mas nenhum passo avança até ele voltar.
- **A compensação é semântica, não um desfazer.** O estorno é uma operação nova no Mercado Pago, e o cliente vê a cobrança e a devolução. As peças voltam ao estoque, mas a reserva ocupou o item enquanto durou.
- **A trava na linha da saga serializa as respostas da mesma OS.** É o que dá consistência, mas duas respostas simultâneas para a mesma OS esperam uma pela outra.
- **Pagamento recusado encerra a OS.** No Checkout Pro o cliente poderia tentar outro cartão; para a saga, `PaymentFailed` é definitivo.

## Referências

- Implementação: [`internal/application/usecase/saga/`](../../internal/application/usecase/saga/) e [`internal/domain/entities/saga.go`](../../internal/domain/entities/saga.go)
- BDD: [`features/saga.feature`](../../features/saga.feature)
- Chris Richardson — [Pattern: Saga](https://microservices.io/patterns/data/saga.html)
