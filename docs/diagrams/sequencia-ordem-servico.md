# Diagrama de sequência: ordem de serviço e saga

Da abertura da OS até a entrega, com as mensagens da saga entre os serviços. A versão da fase 3 (monolito, status alterado por `PUT`) está no [`postech-tc3-app`](https://github.com/Kc1t/postech-tc3-app/blob/main/docs/diagrams/sequencia-ordem-servico.md).

## Caminho feliz

```mermaid
sequenceDiagram
    autonumber
    actor E as Equipe (admin)
    actor C as Cliente
    participant OS as os-service
    participant DB as Postgres do os-service
    participant EX as execution-service
    participant BI as billing-service
    participant MP as Mercado Pago
    participant NT as Lambda notifier

    E->>OS: POST /api/v1/service-orders {requester_document, vehicle_plate}
    OS->>OS: cliente existe? veículo é dele? (404 / 422)
    OS->>DB: uma transação: OS received, saga awaiting_diagnosis,<br/>histórico, outbox StartDiagnosis e ServiceOrderStatusChanged
    OS-->>E: 201 {id, code, status: received}
    Note over OS: o relay publica o outbox
    OS-)EX: execution.StartDiagnosis (fila execution-commands)
    OS-)NT: os.ServiceOrderStatusChanged (tópico os-events)
    NT-->>C: e-mail "OS recebida"

    E->>EX: POST /api/v1/executions/{id}/diagnosis/start
    EX-)OS: DiagnosisStarted → OS in_diagnosis
    E->>EX: POST /api/v1/executions/{id}/diagnosis/complete {services, parts}
    EX-)OS: DiagnosisCompleted
    OS->>DB: preço do catálogo, reserva das peças, OS awaiting_budget,<br/>outbox CreateBudget
    OS-)BI: billing.CreateBudget (fila billing-commands)
    BI-)OS: BudgetCreated → OS awaiting_approval

    C->>BI: POST /api/v1/budgets/{id}/approve (JWT do CPF, só o dono)
    BI-)OS: BudgetApproved → OS awaiting_payment
    OS-)BI: billing.RequestPayment
    BI->>MP: cria a preferência do Checkout Pro
    BI-)OS: PaymentLinkCreated
    C->>MP: paga
    MP->>BI: webhook com x-signature
    BI->>MP: consulta o pagamento (GET /v1/payments/{id})
    BI-)OS: PaymentApproved → OS in_execution
    OS-)EX: execution.StartRepair
    E->>EX: POST /api/v1/executions/{id}/repair/start
    EX-)OS: RepairStarted (grava started_at)
    E->>EX: POST /api/v1/executions/{id}/repair/finish
    EX-)OS: RepairFinished → OS finished, saga completed
    E->>OS: POST /api/v1/service-orders/{id}/deliver → delivered
```

Cada mudança de status da OS grava uma linha em `service_order_status_history` e publica `os.ServiceOrderStatusChanged`, que vira um e-mail para o cliente. A exceção é `awaiting_payment`: o evento sai no `PaymentLinkCreated`, já com o link de pagamento. Cada passo é uma transação local com o outbox; o consumidor registra o `message_id` no inbox na mesma transação.

## Compensação: reparo recusado depois do pagamento

```mermaid
sequenceDiagram
    autonumber
    participant OS as os-service
    participant EX as execution-service
    participant BI as billing-service
    participant MP as Mercado Pago

    EX-)OS: RepairRejected
    OS->>OS: uma transação: saga compensating, peças de volta ao estoque,<br/>OS cancelled com o motivo, outbox RefundPayment e CancelBudget
    OS-)BI: billing.RefundPayment
    OS-)BI: billing.CancelBudget
    BI->>MP: POST /v1/payments/{id}/refunds (X-Idempotency-Key)
    BI-)OS: PaymentRefunded
    OS->>OS: saga cancelled (log saga_refund_confirmed)
```

A execução não recebe `CancelExecution` aqui, porque foi ela que recusou. Sem pagamento aprovado não há estorno, e a saga vai direto para `cancelled`. As outras compensações seguem o mesmo padrão, conforme a etapa em que a saga estava: tabela em [Compensações](../../README.md#compensações) no README.

## Máquina de estados da OS

```mermaid
stateDiagram-v2
    [*] --> received
    received --> in_diagnosis: DiagnosisStarted
    received --> awaiting_budget: DiagnosisCompleted antes do DiagnosisStarted
    in_diagnosis --> awaiting_budget: DiagnosisCompleted
    awaiting_budget --> awaiting_approval: BudgetCreated
    awaiting_approval --> awaiting_payment: BudgetApproved
    awaiting_payment --> in_execution: PaymentApproved
    in_execution --> finished: RepairFinished
    finished --> delivered: POST /deliver
    received --> cancelled
    in_diagnosis --> cancelled
    awaiting_budget --> cancelled
    awaiting_approval --> cancelled
    awaiting_payment --> cancelled
    in_execution --> cancelled
    delivered --> [*]
    cancelled --> [*]
```

As transições ficam em `validTransitions` (`internal/domain/entities/service_order.go`). Fora delas, o domínio devolve `ErrInvalidStatus` (409). `received → awaiting_budget` existe porque o SNS/SQS não garante ordem: o `DiagnosisCompleted` pode chegar antes do `DiagnosisStarted`.

## Máquina de estados da saga

```mermaid
stateDiagram-v2
    [*] --> awaiting_diagnosis: OS aberta, StartDiagnosis enviado
    awaiting_diagnosis --> awaiting_budget: peças reservadas, CreateBudget enviado
    awaiting_budget --> awaiting_approval: BudgetCreated
    awaiting_approval --> awaiting_payment: BudgetApproved, RequestPayment enviado
    awaiting_payment --> executing: PaymentApproved, StartRepair enviado
    executing --> completed: RepairFinished
    executing --> compensating: RepairRejected ou cancelamento, RefundPayment enviado
    compensating --> cancelled: PaymentRefunded
    cancelled --> compensating: PaymentApproved depois do cancelamento
    awaiting_diagnosis --> cancelled: diagnóstico inválido ou cancelamento
    awaiting_budget --> cancelled: BudgetCreationFailed ou cancelamento
    awaiting_approval --> cancelled: BudgetRejected, prazo de 72 h ou cancelamento
    awaiting_payment --> cancelled: PaymentFailed, prazo de 24 h ou cancelamento
    completed --> [*]
    cancelled --> [*]
```

Os estados ficam em `internal/domain/entities/saga.go`. Resposta que não corresponde ao estado atual é registrada no inbox e ignorada (`ErrSagaOutOfStep`).

## Origem dos números do dashboard

| Painel | Fonte |
|---|---|
| Funil da saga e passos por estado | logs `saga_started`, `saga_step` (com `saga_state` e `os_status`) e `saga_completed` |
| Concluídas, compensadas e expiradas | logs `saga_completed`, `saga_compensated` (com `reason`) e `saga_expired` |
| Estornos confirmados | log `saga_refund_confirmed` |
| Latência e erros por serviço | transações do APM (`nrgin`) |
| Mensageria | transações `OutboxRelay/<tipo>` e `Consume/<tipo>`, logs `message_failed` e `outbox_message_stuck` |

Os logs são JSON com o texto em `message` e `trace.id`, `saga_id`, `os_id` e `correlation_id`, enviados pelo agente do New Relic. Dashboard e alertas em Terraform no [`postech-tc4-infra-k8s/observability`](https://github.com/Kc1t/postech-tc4-infra-k8s/tree/main/observability).
