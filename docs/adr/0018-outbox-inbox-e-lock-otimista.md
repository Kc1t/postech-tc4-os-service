# ADR-0018: Transactional outbox, inbox idempotente e lock otimista

- **Status:** Aceita
- **Data:** 2026-10
- **Decisão por:** Reservoir Devs

## Contexto

Cada passo da saga ([ADR-0017](./0017-saga-orquestrada-no-os-service.md)) faz duas coisas que precisam acontecer juntas: **mudar o estado** no banco do serviço e **avisar** os outros por mensagem ([ADR-0013](./0013-mensageria-sns-sqs-dlq.md)). O banco e o SNS/SQS não participam da mesma transação.

As duas ordens ingênuas falham:

- **Gravar e depois publicar:** se o processo cai entre o commit e o `Publish`, o estado mudou e ninguém fica sabendo. A saga para sem erro visível.
- **Publicar e depois gravar:** se o commit falha, o outro serviço age sobre uma mudança que não existe.

Do lado de quem consome, o SQS entrega **pelo menos uma vez**. A mesma mensagem chega de novo quando a visibilidade expira antes do `DeleteMessage`, ou quando o relay publica e cai antes de marcar a mensagem como publicada. Aplicar duas vezes um `PaymentApproved` mandaria o veículo duas vezes para o reparo.

Há ainda escritas concorrentes na mesma OS: duas réplicas processando respostas, o worker de prazos e a equipe pela API.

## Decisão

### Transactional outbox

O caso de uso grava a mensagem **na mesma transação** da mudança de estado. Um relay publica depois.

- **PostgreSQL (os e billing, `pkg/outbox`):** tabela `outbox_messages`, com o ID da mensagem, destino (tópico ou fila), tipo, `saga_id`, corpo, headers de trace, `created_at`, `published_at`, `next_attempt_at`, `attempts` e `last_error`. O relay varre a cada `OUTBOX_POLL_INTERVAL` (500 ms) um lote de 50 pendentes cuja espera já passou, em ordem de criação, com `FOR UPDATE SKIP LOCKED`: várias réplicas rodam o relay sem publicar a mesma linha em paralelo.
- **Falha de publicação espera antes de tentar de novo** (os três serviços): `attempts` sobe e a próxima tentativa fica para 1 s, 2 s, 4 s... até 5 min depois (`next_attempt_at` no PostgreSQL, `lease_until` no DynamoDB). Enquanto espera, a mensagem não entra no lote, então uma mensagem que não publica não trava as outras. A partir da 10ª tentativa (uns 8 min falhando), cada falha é logada como erro `outbox_message_stuck`, e a falha também aparece como erro na transação `OutboxRelay/<tipo>` do New Relic.
- **Limpeza:** o relay do PostgreSQL apaga, uma vez por hora, o outbox publicado há mais de 7 dias e o inbox com mais de 21 dias. A retenção do inbox passa a das DLQs (14 dias) de propósito: um redrive reentrega mensagens antigas, e sem o registro elas reaplicariam o efeito.
- **No os-service, enviar fora de transação é erro** (`ErrNoTransaction`). O `outboxMessenger` só grava no outbox da transação corrente, então não existe caminho que publique direto.
- **DynamoDB (execution):** o item `OUTBOX` entra na mesma `TransactWriteItems` do `STATE` ([ADR-0012](./0012-banco-por-servico.md)). Sem `SKIP LOCKED`, cada réplica reserva a mensagem com um `UpdateItem` condicional (lease de 30 s). Publicada, a mensagem ganha TTL de 24 h. Na falha, o lease vira a hora da próxima tentativa.
- **O trace atravessa o outbox.** Os headers de distributed tracing do New Relic são capturados na gravação e reaceitos pelo relay antes de publicar, então o trace da requisição HTTP continua no consumidor do outro serviço.

### Inbox (consumidor idempotente)

O consumidor registra o `message_id` **na mesma transação** dos efeitos do handler.

- **PostgreSQL (`pkg/inbox`):** tabela `processed_messages`, chave (`message_id`, `consumer`). `inbox.Once` faz `INSERT ... ON CONFLICT DO NOTHING`. Se nenhuma linha entrou, a mensagem é duplicata e o handler não roda.
- **DynamoDB:** item `MSG#{consumer}#{message_id}` com `attribute_not_exists(pk)` dentro da mesma `TransactWriteItems`. O TTL de 15 dias é maior que a retenção máxima do SQS (14 dias), então nenhuma reentrega encontra o registro já expirado.
- **Lambda de notificação:** tabela de dedup própria, com reserva condicional por `message_id` ([ADR-0022](./0022-notificacao-por-lambda.md)).
- **Resposta fora de etapa** (`ErrSagaOutOfStep`) é registrada no inbox e ignorada: tardia, duplicada com outro ID ou fora de ordem. Qualquer outro erro desfaz a transação inteira, inbox incluído, e a mensagem volta para a fila.

### Lock otimista

- **`service_orders.version` e `saga_instances.version`:** o `Update` grava com `WHERE id = ? AND version = ?` e incrementa a versão. Nenhuma linha afetada vira `ErrConcurrentUpdate`: 409 (`CONCURRENT_UPDATE`) na API e rollback com nova tentativa pela fila no consumidor.
- **No billing,** `budgets` e `payments` seguem a mesma regra.
- **No DynamoDB,** o `STATE` é criado com `attribute_not_exists(pk)` e atualizado com a condição `version = :esperada`.
- **Nos passos da saga,** o lock otimista se soma a um lock pessimista: o handler trava a linha da saga com `FOR UPDATE` antes de ler a OS, sempre nessa ordem (saga, depois OS) para não haver deadlock com o cancelamento e o worker de prazos.

## Alternativas consideradas

1. **Publicar depois do commit, com retry em memória.** Descartado porque perde a mensagem quando o pod morre entre o commit e a publicação, e o HPA e o rolling update reciclam pods o tempo todo.

2. **CDC com Debezium.** Lê o WAL do PostgreSQL e publica sem tabela de outbox. Descartado porque exige Kafka Connect rodando e replicação lógica habilitada no RDS. É infraestrutura para manter de pé maior que o próprio relay.

3. **DynamoDB Streams + Lambda como relay no execution.** Seria o outbox nativo do DynamoDB. Descartado para manter uma semântica só nos três serviços (mesmo envelope, mesma propagação de trace, mesma contagem de pendentes) e não criar uma terceira Lambda para filtrar o stream.

4. **Deduplicação do SQS FIFO no lugar do inbox.** Descartada porque a janela é de cinco minutos e só cobre o envio. Não protege contra reentrega por visibilidade expirada nem contra a mesma mensagem publicada duas vezes pelo relay depois de cinco minutos.

5. **Só lock pessimista.** Bastaria nos passos da saga. Descartado porque não serve à API: as escritas da equipe na OS seriam serializadas por trava, e um handler lento seguraria a linha.

6. **Sem controle de concorrência (última escrita vence).** Descartado porque uma resposta da saga e um cancelamento simultâneo poderiam se sobrescrever, deixando OS cancelada com execução em andamento.

## Consequências

### Positivas

- Não existe estado salvo sem mensagem nem mensagem sem estado salvo, em nenhum dos três serviços.
- A reentrega é segura: o redrive da DLQ é uma operação de rotina, não um risco ([runbook](https://github.com/Kc1t/postech-tc4-infra-k8s/blob/main/docs/runbooks/dlq.md)).
- Conflito de concorrência vira 409 ou retry, nunca escrita perdida.
- O relay escala com as réplicas sem coordenação extra.

### Negativas

- **Latência de até um intervalo de varredura** (500 ms) entre o commit e a publicação, somada em cada passo da saga.
- **Uma mensagem que nunca publica** (destino inexistente, por exemplo) continua sendo tentada a cada 5 min, para sempre: não há DLQ do lado do outbox. Ela não trava as outras, mas alguém precisa agir quando o `outbox_message_stuck` aparecer (corrigir o destino ou apagar a linha).
- **A ordem entre mensagens não é garantida depois de uma falha.** A mensagem que espera é ultrapassada pelas mais novas. A saga já trata resposta fora de ordem (`ErrSagaOutOfStep`); onde isso aparece é na notificação, que pode mandar dois e-mails de status fora de ordem.
- **A limpeza é um `DELETE` por data**, sem lotes. No volume de uma oficina é instantâneo; em escala maior, apagaria em lotes ou particionaria a tabela por data.
- **A entrega continua "pelo menos uma vez".** O outbox garante que a mensagem sai, não que sai uma vez só. Todo consumidor novo precisa de inbox, ou repete efeitos.
- **O lock pessimista na saga e o otimista na OS convivem.** Quem mexer no orquestrador precisa respeitar a ordem de travamento.

## Referências

- Implementação: [`pkg/outbox/outbox.go`](../../pkg/outbox/outbox.go), [`pkg/inbox/inbox.go`](../../pkg/inbox/inbox.go), [`internal/adapters/outbound/postgresql/messenger.go`](../../internal/adapters/outbound/postgresql/messenger.go), [`internal/adapters/outbound/postgresql/serviceorder_repository.go`](../../internal/adapters/outbound/postgresql/serviceorder_repository.go)
- execution-service: [`internal/adapters/outbound/dynamodb/repository.go`](https://github.com/Kc1t/postech-tc4-execution-service/blob/main/internal/adapters/outbound/dynamodb/repository.go) e [`outbox_relay.go`](https://github.com/Kc1t/postech-tc4-execution-service/blob/main/internal/adapters/outbound/dynamodb/outbox_relay.go)
- Chris Richardson: [Pattern: Transactional outbox](https://microservices.io/patterns/data/transactional-outbox.html) e [Idempotent Consumer](https://microservices.io/patterns/communication-style/idempotent-consumer.html)
