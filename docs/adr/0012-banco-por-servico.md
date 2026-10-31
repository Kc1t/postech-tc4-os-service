# ADR-0012: Banco de dados por serviço — PostgreSQL no os e no billing, DynamoDB no execution

- **Status:** Accepted
- **Data:** 2026-10
- **Decisão por:** Reservoir Devs

## Contexto

Na Fase 3 havia um banco só: a instância RDS `workshop`, compartilhada pela aplicação e lida direto pela Lambda de autenticação. A Fase 4 exige que cada microsserviço tenha **banco próprio**, que nenhum serviço acesse o banco de outro e que a solução use **pelo menos um banco relacional e um não relacional**.

Os três serviços (ver [RFC-0004](../rfc/0004-decomposicao-em-microsservicos.md)) têm padrões de acesso diferentes:

- **os-service:** OS, saga, clientes, veículos, catálogo e estoque. Cada passo da saga atualiza OS, saga, histórico, estoque e outbox numa transação só ([ADR-0017](./0017-saga-orquestrada-no-os-service.md)). CPF/CNPJ, placa e código da OS são únicos.
- **billing-service:** orçamento e pagamento. É dinheiro: um orçamento por OS, um pagamento do provedor registrado uma vez, transições que não podem se perder.
- **execution-service:** a fila de trabalho do mecânico e a linha do tempo de cada execução. Os acessos são sempre por chave: "a execução da OS X", "a fila de diagnóstico em ordem de chegada". Não há junção nem agregação.

## Decisão

| Serviço | Banco | Recurso |
|---|---|---|
| os-service | PostgreSQL 16 (RDS) | instância `postech-tc4-prod-os-service-postgres`, database `workshop` |
| billing-service | PostgreSQL 16 (RDS) | instância `postech-tc4-prod-billing-service-postgres`, database `billing` |
| execution-service | DynamoDB | tabela única `executions`, `PAY_PER_REQUEST` |

Cada RDS fica no Terraform do próprio serviço (`infra/`), com security group que só libera a porta 5432 para o security group do cluster EKS, `rds.force_ssl = 1` e a credencial gerada por `random_password` e gravada no Secrets Manager (`postech-tc4-prod-<serviço>-db-credentials`). O job de deploy lê a DSN desse segredo e a grava no Secret Kubernetes do serviço; nenhum outro repositório conhece a senha.

O que um serviço precisa do outro chega por **mensagem** (o comando `billing.CreateBudget` leva nome, documento e e-mail do cliente e os itens com preço) ou pela **API** (a composição do `/details`, [ADR-0019](./0019-api-composition-no-details.md); a Lambda de autenticação consulta `GET /internal/requesters/{doc}`, [ADR-0014](./0014-lambdas-fora-da-vpc.md)).

### Tabela única do execution-service

| Item | `pk` | `sk` | `gsi1pk` | `gsi1sk` |
|---|---|---|---|---|
| Estado atual da execução | `EXEC#{os_id}` | `STATE` | `QUEUE#{status}` | `{prioridade}#{enfileirada_em}` |
| Linha do tempo | `EXEC#{os_id}` | `EVT#{ts}#{id}` | — | — |
| Outbox | `OUTBOX` | `{ts}#{message_id}` | — | — |
| Inbox | `MSG#{consumer}#{message_id}` | `MSG` | — | — |

- **Uma `Query` em `EXEC#{os_id}`** devolve o estado e a linha do tempo já ordenada. Os timestamps têm largura fixa (nove casas de fração), então a ordem lexicográfica da `sk` é a cronológica.
- **O `gsi1` é esparso.** Só o item `STATE` de uma execução em status ativo recebe `gsi1pk`. Quando a execução termina (`finished`, `rejected`, `cancelled`), os atributos saem e o item sai do índice sozinho. O índice contém apenas o trabalho em andamento.
- **A fila do mecânico** (`GET /api/v1/executions/queue`) consulta a partição `QUEUE#queued_repair` e depois `QUEUE#queued_diagnosis`, cada uma em ordem de chegada: reparo vem antes porque o orçamento já foi pago.
- **Toda mudança de estado é uma `TransactWriteItems`**: `STATE` com condição na `version`, o `EVT#`, o `OUTBOX` e, quando veio de mensagem, o `MSG#` com `attribute_not_exists(pk)` ([ADR-0018](./0018-outbox-inbox-e-lock-otimista.md)).
- O atributo `ttl` limpa sozinho os itens de inbox (15 dias) e as mensagens de outbox já publicadas (24 h).

## Alternativas consideradas

1. **Uma instância RDS com um schema e um usuário por serviço.** Corta pela metade o custo de banco relacional (uma `db.t3.micro` em vez de duas). Descartada: o enunciado pede infraestrutura e banco próprios por serviço, e uma instância só volta a ser ponto único de falha e de contenção de I/O entre serviços. É o recuo natural se o crédito apertar.

2. **DynamoDB também no billing.** Teria um banco a menos para operar. Descartada: o billing depende de unicidade (`os_id` no orçamento, ID do pagamento no provedor) e de transições de valor que precisam ser atômicas com o outbox. O PostgreSQL entrega isso com índice único e transação; no DynamoDB seriam itens auxiliares de unicidade e condições escritas à mão.

3. **PostgreSQL no execution.** Funcionaria, mas o requisito de NoSQL ficaria sem dono. O execution é o melhor candidato: nenhuma consulta dele precisa de junção, e o volume cresce com o número de OS, não com relatórios.

4. **DocumentDB ou MongoDB no execution.** Também atenderia o NoSQL. Descartado pelo custo: cobra por instância-hora, ligada ou ociosa. O DynamoDB sob demanda cobra por requisição e fica perto de zero fora da demonstração.

5. **Uma tabela DynamoDB por tipo de item** (execuções, linha do tempo, outbox, inbox). A `TransactWriteItems` também atravessa tabelas, então a atomicidade não exigia tabela única. Descartada porque ler a execução com o histórico viraria duas chamadas, e seriam quatro tabelas para provisionar, com TTL e PITR em cada.

6. **GSI comum em vez de esparso**, com o status como atributo filtrado. O índice carregaria todas as execuções já encerradas, e a fila pagaria leitura de itens que nunca aparecem nela.

## Consequências

### Positivas

- Requisito atendido: três bancos, dois relacionais e um NoSQL, e nenhum serviço com credencial do banco de outro.
- Uma falha ou migration num banco não derruba o outro serviço.
- O execution não tem instância para dimensionar nem conexão para esgotar com o HPA subindo réplicas.
- A fila de trabalho é uma `Query` num índice que só contém trabalho em andamento.

### Negativas

- **Dois RDS custam o dobro de um.** São ~US$ 0,036/hora de instância em vez de ~US$ 0,018.
- **O isolamento de rede é por cluster, não por serviço.** O security group dos dois RDS libera o cluster inteiro: qualquer pod alcança as duas portas 5432, e quem separa os bancos é a credencial. No DynamoDB é pior: todos os pods usam a `LabRole` ([ADR-0016](./0016-credenciais-dos-pods-pela-labrole.md)), então o IAM não impede outro serviço de ler a tabela `executions`.
- **Dados de outro serviço viram cópia.** O billing guarda nome e e-mail do cliente do jeito que vieram no `CreateBudget`; uma correção no cadastro não chega aos orçamentos já criados.
- **A partição `OUTBOX` é uma chave só.** Todas as mensagens a publicar caem na mesma partição do DynamoDB. No volume de uma oficina é irrelevante; em escala nacional seria preciso espalhar a chave (`OUTBOX#{n}`).
- O os-service segue com `AutoMigrate` (limite já registrado na RFC-0002), enquanto o billing usa migrations SQL versionadas. São duas práticas convivendo.

## Referências

- Terraform: [`infra/main.tf`](../../infra/main.tf) (os-service), [`postech-tc4-billing-service/infra/main.tf`](https://github.com/Kc1t/postech-tc4-billing-service/blob/main/infra/main.tf), [`postech-tc4-execution-service/infra/main.tf`](https://github.com/Kc1t/postech-tc4-execution-service/blob/main/infra/main.tf)
- Chaves da tabela única: [`internal/adapters/outbound/dynamodb/keys.go`](https://github.com/Kc1t/postech-tc4-execution-service/blob/main/internal/adapters/outbound/dynamodb/keys.go)
- AWS — [Using global secondary indexes in DynamoDB: sparse indexes](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/bp-indexes-general-sparse-indexes.html)
