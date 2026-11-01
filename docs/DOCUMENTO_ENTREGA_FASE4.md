# Tech Challenge Fase 4 · Reservoir Devs

## Documento de entrega

**Grupo 112** · Pós Tech FIAP, Software Architecture (13SOAT) · Prazo: 17/11/2026

> Base do PDF submetido no Portal do Aluno, gerado com `scripts/entrega-pdf.sh`. Os itens marcados com ⟨…⟩ são preenchidos depois do deploy final e da gravação do vídeo.

## 1. Participantes

| Nome | E-mail | GitHub |
|---|---|---|
| Kauã Miguel da Cunha | kauamigueldev@gmail.com | [@Kc1t](https://github.com/Kc1t) |
| Pedro Henrique Carlini de Oliveira Ribeiro | phcarlini@gmail.com | [@PedroHCarlini](https://github.com/PedroHCarlini) |
| Yuri Lopes Padlipskas | yupadlipskas@gmail.com | [@yuriLpadlipskas](https://github.com/yuriLpadlipskas) |
| Diego Oliveira Andrade Rafael | diegoliveiraa1@gmail.com | [@diegoliveiraa](https://github.com/diegoliveiraa) |

## 2. Links da entrega

| Entregável | Link |
|---|---|
| Vídeo de demonstração (até 15 min) | ⟨link do YouTube, não listado⟩ |
| API Gateway (entrada única) | ⟨endpoint do gateway⟩ |
| Swagger | `docs/swagger.yaml` em cada repositório de serviço |
| Postman | `postman_collection.json` em cada repositório de serviço |
| Dashboard do New Relic | ⟨links dos snapshots⟩ |
| Decisões de arquitetura | [ADRs](https://github.com/Kc1t/postech-tc4-os-service/blob/main/docs/adr/README.md) e [RFCs](https://github.com/Kc1t/postech-tc4-os-service/blob/main/docs/rfc/README.md) |
| Segurança | [docs/SEGURANCA.md](https://github.com/Kc1t/postech-tc4-os-service/blob/main/docs/SEGURANCA.md) |
| Contratos das mensagens | [AsyncAPI](https://github.com/Kc1t/postech-tc4-os-service/blob/main/docs/asyncapi/asyncapi.yaml) |

### Repositórios (privados, com acesso para `soat-architecture`)

| Repositório | Responsabilidade | Banco |
|---|---|---|
| [postech-tc4-os-service](https://github.com/Kc1t/postech-tc4-os-service) | ordem de serviço (OS), clientes, veículos, catálogo e estoque; **orquestrador da saga**; ADRs, RFCs e AsyncAPI | PostgreSQL (RDS) |
| [postech-tc4-billing-service](https://github.com/Kc1t/postech-tc4-billing-service) | orçamento e pagamento com Mercado Pago (Checkout Pro e webhook) | PostgreSQL (RDS) |
| [postech-tc4-execution-service](https://github.com/Kc1t/postech-tc4-execution-service) | fila de execução: diagnóstico e reparo | DynamoDB |
| [postech-tc4-notification-lambda](https://github.com/Kc1t/postech-tc4-notification-lambda) | e-mail ao cliente a cada mudança de status (serverless) | DynamoDB (deduplicação) |
| [postech-tc4-lambda-auth](https://github.com/Kc1t/postech-tc4-lambda-auth) | autenticação por CPF (issuer) e authorizer do API Gateway | nenhum (consulta o os-service) |
| [postech-tc4-infra-k8s](https://github.com/Kc1t/postech-tc4-infra-k8s) | EKS, API Gateway, SNS/SQS/DLQ, observabilidade, ambiente e2e local | nenhum |

## 3. Arquitetura final

![Arquitetura da fase 4: microsserviços, bancos e comunicação](diagrams/arquitetura-fase4.svg)

- **Síncrono (REST):** entrada pelo API Gateway HTTP, com Lambda authorizer; leitura composta da OS (`GET /service-orders/{id}/details`), em que o os-service consulta billing e execution com timeout, retry e circuit breaker; consulta do cliente pela Lambda issuer na rota interna `GET /internal/requesters/{document}`.
- **Assíncrono (SNS e SQS):** todos os passos da saga. Comandos vão para a fila do serviço que executa (`billing-commands`, `execution-commands`); eventos saem pelo tópico do serviço que publica (`os-events`, `billing-events`, `execution-events`) e chegam à fila de quem consome (`os-saga-replies`, `notification-os-events`). Toda fila tem DLQ (`<fila>-dlq`), alerta no New Relic, alarme no CloudWatch e runbook.
- **Nenhum serviço acessa o banco de outro.** A Lambda de autenticação, que na fase 3 abria o banco, agora consulta o os-service.

## 4. Saga: orquestração

**Escolha:** saga **orquestrada**, com o orquestrador dentro do os-service, dono da OS ([ADR-0017](https://github.com/Kc1t/postech-tc4-os-service/blob/main/docs/adr/0017-saga-orquestrada-no-os-service.md); justificativa também no [README do os-service](https://github.com/Kc1t/postech-tc4-os-service#saga-orquestrada)).

**Por quê:**

- O fluxo é longo, com espera humana (aprovação do cliente), dinheiro e prazos. O estado persistido num só lugar (`saga_instances`) responde em que etapa a OS está e o que precisa ser desfeito.
- A compensação depende da etapa: estornar só se pagou, devolver peças só se reservou. No orquestrador, essa regra é código testado (unitário e BDD); na coreografia, ficaria espalhada em três serviços.
- Billing e execution só recebem comandos e respondem eventos, sem conhecer a ordem das etapas.
- O orquestrador é dono dos prazos: expira a aprovação em 72 h e o pagamento em 24 h e compensa sozinho.

**Alternativas descartadas:** **coreografia**, porque o plano de compensação e os prazos ficariam espalhados e saber em que passo está a OS exigiria correlacionar eventos de três serviços; **orquestrador num serviço próprio**, porque cada passo mexe na OS, no histórico e no estoque, e cada mudança dessas viraria mais um comando e mais uma compensação; **AWS Step Functions**, porque a regra sairia do código Go e do BDD e cada espera humana viraria callback com task token; **Temporal ou Camunda**, pelo custo de operar mais um cluster para uma saga só; **2PC**, porque SQS e DynamoDB não participam de transação distribuída.

**Fluxo:** abrir a OS → `StartDiagnosis` (execution) → `DiagnosisCompleted` → reserva das peças (transação local) → `CreateBudget` (billing) → aprovação do cliente → `RequestPayment` (Mercado Pago) → `PaymentApproved` → `StartRepair` (execution) → `RepairFinished` → entrega.

**Compensações:**

| Falha | Compensação |
|---|---|
| Diagnóstico inválido (sem itens, item fora do catálogo, quantidade inválida) ou estoque insuficiente | devolve o que já foi reservado e cancela a execução |
| Orçamento não criado ou recusado pelo cliente | devolve as peças e cancela a execução |
| Pagamento recusado | cancela o orçamento, devolve as peças e cancela a execução |
| Reparo recusado depois de pago | **estorna o pagamento**, cancela o orçamento e devolve as peças |
| Prazo vencido (aprovação em 72 h, pagamento em 24 h) ou cancelamento pela oficina | as compensações da etapa em que a saga parou |
| Pagamento aprovado depois que a saga expirou | estorna o valor |

**Confiabilidade:**

- **Transactional outbox:** estado e mensagem na mesma transação; o relay publica depois, com espera exponencial entre tentativas (até 5 min) e log `outbox_message_stuck`, com alerta, a partir da 10ª tentativa.
- **Inbox idempotente:** cada consumidor registra o `message_id` na mesma transação do efeito e guarda o registro por mais tempo que a DLQ (21 dias contra 14), então nem um redrive repete efeito.
- **Concorrência:** `SELECT … FOR UPDATE` na saga e lock otimista (`version`) na saga e na OS.
- **Contratos:** os 23 tipos de mensagem (7 comandos e 16 eventos) têm JSON Schema validado nos testes de contrato de produtor e consumidor, idêntico nos quatro repositórios Go.
- **Demonstração:** o header `X-Simulate-Failure: budget|payment|repair` na abertura da OS força a falha de uma etapa.

## 5. Divisão dos microsserviços e tecnologias

A divisão segue as **capacidades de negócio** do enunciado (atendimento, cobrança e execução), aplicada ao monolito da fase 3 com Strangler Fig ([RFC-0004](https://github.com/Kc1t/postech-tc4-os-service/blob/main/docs/rfc/0004-decomposicao-em-microsservicos.md)):

| Serviço | Por que existe sozinho | Por que esse banco |
|---|---|---|
| os-service | É o dono do ciclo de vida da OS e do estoque reservado, e coordena os outros. Ficou com o código e o histórico do monolito. | **PostgreSQL**: OS, estoque e saga mudam juntos em transações ACID, com o outbox na mesma transação. |
| billing-service | Dinheiro tem regras, integração externa (Mercado Pago) e auditoria próprias, e muda num ritmo diferente da oficina. | **PostgreSQL**: orçamento e pagamento exigem consistência forte e consultas relacionais. |
| execution-service | A fila da oficina é operacional, com muita escrita e leitura por chave (OS) e por status. | **DynamoDB** (tabela única com GSI esparso da fila): acesso por chave e por status, sem servidor para operar, e o TTL limpa o outbox e o inbox. |
| notification-lambda | Notificar é efeito colateral sem estado de negócio; serverless escala a zero e isola falhas de SMTP. | **DynamoDB** para deduplicar o envio. |
| lambda-auth | Autenticação por CPF na borda, antes de qualquer serviço. | Nenhum: consulta o os-service. |

| Camada | Tecnologia |
|---|---|
| Linguagem e HTTP | Go 1.26, Gin, arquitetura hexagonal |
| Mensageria | Amazon SNS, SQS e DLQ (LocalStack no ambiente local) |
| Bancos | Amazon RDS PostgreSQL 16 (dois), Amazon DynamoDB (dois) |
| Pagamento | Mercado Pago Checkout Pro com webhook assinado (`x-signature`); gateway fake para testes |
| Plataforma | Amazon EKS, API Gateway HTTP, Lambda, ECR, Secrets Manager, tudo em Terraform |
| CI/CD | GitHub Actions por repositório, SonarQube, golangci-lint, gosec, govulncheck, tfsec, Trivy |
| Testes | `testing` e gomock, godog (BDD), testes de componente com LocalStack, testes de contrato com JSON Schema |
| Observabilidade | New Relic: APM, trace distribuído pelas mensagens, Postgres no trace, logs em contexto, dashboard, alertas e SLOs em Terraform |

## 6. Requisitos do enunciado

| Requisito | Como foi atendido |
|---|---|
| ≥ 3 microsserviços com repositório, infraestrutura e banco próprios | 3 serviços no EKS e 2 Lambdas; cada serviço com `infra/` (Terraform do banco), `k8s/` e `Dockerfile` |
| Pelo menos um banco SQL e um NoSQL | PostgreSQL (os, billing) e DynamoDB (execution, notification) |
| Integração com o Mercado Pago | Checkout Pro, webhook com assinatura conferida e consulta do pagamento na API antes de decidir |
| REST síncrono e mensageria assíncrona | REST na borda, na API composition e na rota interna; SNS/SQS na saga (contratos em AsyncAPI) |
| Nenhum acesso ao banco de outro serviço | cada banco só é acessível pelo serviço dono; a Lambda issuer consulta o os-service por HTTP |
| Saga com rollback e compensação, escolha documentada no README | orquestrada, com compensação por etapa, prazos e estorno; justificativa no README do os-service e na ADR-0017 |
| Testes unitários, BDD e cobertura ≥ 80% | unitários em todos os repositórios; BDD godog da saga no os-service; cobertura por repositório na seção 7 |
| SonarQube no CI | SonarQube em container em cada pipeline, com quality gate que reprova o build |
| Pipeline independente com build, testes, qualidade e deploy em Kubernetes | um workflow por repositório; deploy no EKS com rollout e smoke test; as Lambdas por Terraform |
| `main` protegida com PR e checagens | branch protection nos seis repositórios: PR obrigatório, checagens obrigatórias, branch atualizada e conversas resolvidas, inclusive para administradores |
| Observabilidade da fase 3 | New Relic estendido aos novos serviços e às Lambdas, com o trace atravessando as filas |
| Swagger ou Postman atualizados | os dois, em cada serviço |

## 7. Qualidade

| Repositório | Cobertura (`go test`) | SonarQube | Evidência (resumo de cada execução) |
|---|---|---|---|
| os-service | 86,3% | 83,6%, gate aprovado | [CI](https://github.com/Kc1t/postech-tc4-os-service/actions/workflows/ci.yml) |
| billing-service | 95,7% | 94,7%, gate aprovado | [CI](https://github.com/Kc1t/postech-tc4-billing-service/actions/workflows/ci.yml) |
| execution-service | 95,6% | 95,3%, gate aprovado | [CI](https://github.com/Kc1t/postech-tc4-execution-service/actions/workflows/ci.yml) |
| lambda-auth | 91,4% | 90,6%, gate aprovado | [CI](https://github.com/Kc1t/postech-tc4-lambda-auth/actions/workflows/ci.yml) |
| notification-lambda | 92,0% | 89,0%, gate aprovado | [CI](https://github.com/Kc1t/postech-tc4-notification-lambda/actions/workflows/ci.yml) |

Em todos: 0 bugs, 0 vulnerabilidades, 0 code smells e 0% de duplicação. O quality gate é próprio (cobertura ≥ 80%, duplicação ≤ 3% e nota A em confiabilidade, segurança e manutenibilidade), e cada pipeline também roda golangci-lint, gosec, govulncheck, tfsec e Trivy. Cada execução do CI publica no resumo do job a cobertura do `go test` e a tabela do SonarQube.

## 8. Segurança

Autenticação em camadas: o API Gateway aplica throttling e o Lambda authorizer valida o JWT em toda rota protegida; cada serviço valida o JWT de novo, confere o papel (`admin` para a equipe) e a posse do recurso: o CPF do token tem de ser o do cliente da OS ou do orçamento, e o pedido de outro CPF é recusado com 403. A rota interna fica fora do gateway e exige token próprio. Segredos ficam no GitHub Secrets e no Secrets Manager, os contêineres rodam sem root e com sistema de arquivos só leitura, e o CI bloqueia o merge com gosec, govulncheck, Trivy, tfsec e o quality gate. Detalhes, modelo de ameaça e testes de cada controle em [docs/SEGURANCA.md](https://github.com/Kc1t/postech-tc4-os-service/blob/main/docs/SEGURANCA.md).

## 9. Limitações conhecidas

Nenhuma delas deixa um requisito do enunciado de fora. São escolhas de custo e de escopo, ou restrições do AWS Learner Lab.

- **Credenciais dos pods pela `LabRole`.** O Learner Lab não permite criar roles IAM, então os pods usam a `LabRole` pelo IMDS em vez de IRSA, sem menor privilégio por serviço.
- **Integração da conta AWS com o New Relic.** É a única peça do monitoramento fora, porque também exige criar uma role IAM. Ela traria as métricas nativas do SQS e do RDS para o New Relic. O que está coberto: os três serviços com APM, trace distribuído atravessando SNS/SQS e Postgres no trace; as três Lambdas instrumentadas; logs em contexto; dashboard, 16 alertas com runbook e SLOs. A DLQ tem alerta no New Relic pelo log `message_failed` da última entrega antes da DLQ, além do alarme do CloudWatch na própria fila.
- **RDS Single-AZ e sem proteção contra exclusão**, por custo e porque o ambiente é recriado a cada sessão do lab.
- **Um ambiente só na AWS.** A homologação é o ambiente e2e local, com os mesmos serviços e Lambdas no LocalStack, onde o caminho feliz e três compensações passam.
- **NLBs públicos e HTTP entre o gateway e os serviços.** A defesa é o JWT validado no serviço, a checagem de posse e o token interno; fechar exigiria NLB interno com VPC Link.
- **Leitura composta na hora.** O `/details` usa API composition, sem CQRS, com resposta parcial quando um serviço está fora.

## 10. Antes de submeter

- [x] Convites de colaborador enviados em 31/10 nos seis repositórios (`soat-architecture` e integrantes do grupo)
- [ ] Convites aceitos (conferir em *Settings > Collaborators* de cada repositório)
- [ ] Grupo trazido da fase anterior no portal
- [ ] Deploy final no Learner Lab (`AWS_DEPLOY_ENABLED=true`) e endpoint do gateway preenchido acima
- [ ] Vídeo no YouTube (não listado) e link acima
- [ ] Snapshots do New Relic e números de cobertura conferidos com a última execução do CI
- [ ] PDF gerado com `scripts/entrega-pdf.sh`
