# Roteiro do vídeo: Tech Challenge Fase 4

Limite de **15 minutos**; o roteiro fecha em cerca de 14 min 30 s. O enunciado pede quatro demonstrações:

| Item exigido pelo enunciado | Bloco | Trecho |
|---|---|---|
| Fluxo completo de uma OS passando pelos microsserviços | 3 | 4:30 a 8:30 |
| Execução do Saga Pattern e tratamento de falhas | 4 | 8:30 a 11:30 |
| Deploy automatizado de pelo menos um microsserviço, com validação de testes | 2 | 1:45 a 4:30 |
| Monitoramento e rastreamento dos fluxos distribuídos | 5 | 11:30 a 14:00 |

Na descrição do vídeo no YouTube, colar os timestamps abaixo, ajustados aos tempos reais depois da edição (o avaliador da fase 3 elogiou isso):

```text
0:00 Arquitetura: microsserviços, bancos e comunicação
1:45 Deploy automatizado com testes (execution-service)
4:30 Fluxo completo de uma OS
8:30 Saga: pagamento recusado e reparo recusado com estorno
11:30 Monitoramento e rastreamento no New Relic
14:00 Onde está cada coisa e limitações
```

## Antes de gravar

**Ambiente (no dia, com o Learner Lab iniciado):**

- [ ] `scripts/setup-github.sh aws` no infra-k8s (credenciais da sessão) e `scripts/setup-github.sh enable`
- [ ] Pipelines aplicados na ordem do README do infra-k8s: infra-k8s → os, billing e execution → lambda-auth e notification-lambda → infra-k8s de novo (liga as Lambdas e os NLBs às rotas do gateway)
- [ ] Workflow `observability` do infra-k8s aplicado (dashboard, alertas e SLOs)
- [ ] `kubectl get pods -n postech` com os três serviços `Running` (2 réplicas cada)
- [ ] `GATEWAY=... scripts/demo.sh tudo` rodado **pelo menos duas vezes, uma hora antes**, para o New Relic ter dados no dashboard e no service map
- [ ] Caixa de e-mail do cliente da demo aberta (o e-mail cadastrado pelo `demo.sh`, variável `EMAIL`)
- [ ] Se o Mercado Pago estiver ligado (`scripts/setup-github.sh mercadopago`, que põe `PAYMENT_GATEWAY=mercadopago`): usuário comprador de teste logado numa aba anônima

**Pipeline para o bloco 2:**

- [ ] Branch `demo/deploy` no execution-service com uma mudança pequena e visível (por exemplo, um texto no Swagger ou uma mensagem de log)
- [ ] PR aberto **uns 15 minutos antes** de gravar, para os checks já estarem verdes. Gravar o merge e o job de deploy; se o deploy demorar, cortar a espera na edição

**Telas:**

- [ ] Terminal com fonte grande, dois painéis: `scripts/demo.sh` e `kubectl get pods -n postech -w`
- [ ] Abas: diagrama `docs/diagrams/arquitetura-fase4.svg`, GitHub Actions do execution-service, regras de proteção da `main`, New Relic (dashboard "Ordem de Serviço (prod)", Distributed tracing, service map, alertas e SLOs), console do SQS (DLQs)

## Bloco 1: contexto e arquitetura (0:00 a 1:45)

- Grupo e objetivo: a oficina saiu do monolito da fase 3 para microsserviços com saga.
- Diagrama: três serviços no EKS, cada um com o próprio banco (PostgreSQL no os e no billing, DynamoDB no execution), duas Lambdas (autenticação e notificação) e o API Gateway na frente.
- Comunicação: REST na borda, na leitura composta e na rota interna; SNS/SQS em todos os passos da saga, com DLQ em toda fila. "Nenhum serviço lê o banco do outro: até a Lambda de login, que na fase 3 abria o banco, agora pergunta ao os-service."
- Por que **orquestração**: fluxo longo com espera do cliente, dinheiro e prazos; a compensação depende da etapa; os outros serviços só obedecem comandos ([ADR-0017](adr/0017-saga-orquestrada-no-os-service.md)).

## Bloco 2: deploy automatizado com testes (1:45 a 4:30)

- Mostrar a proteção da `main` (*Settings > Branches*): PR obrigatório, checks exigidos, branch atualizada, valendo para administradores.
- No PR do execution-service, abrir os checks: *Lint, SAST e dependências*, *Testes e cobertura* (95,8%), *Teste de componente (LocalStack)*, *SonarQube (quality gate)*, *Terraform do banco* (fmt, validate e tfsec), *Imagem* (build e Trivy) e *Manifestos K8s (kubeconform)*.
- Abrir o resumo do job: a linha "Cobertura do execution-service" e a tabela do SonarQube com o gate, a cobertura, bugs, vulnerabilidades e duplicação.
- Fazer o merge. No job *Deploy no EKS*: `terraform apply` da tabela DynamoDB, push da imagem no ECR, Secret e manifests no cluster, `rollout status` e smoke test pelo NLB.
- No painel do `kubectl get pods -w`: os pods novos sobem antes dos antigos saírem (`maxSurge: 1`, `maxUnavailable: 0`).

## Bloco 3: fluxo completo de uma OS (4:30 a 8:30)

`PAUSE=1 GATEWAY=... scripts/demo.sh feliz`, narrando cada etapa:

1. Login da equipe e cadastros. O cliente troca o CPF por um token (`POST /auth` → Lambda issuer → `GET /internal/requesters/{document}` no os-service). Sem token, o gateway responde 401.
2. Abrir a OS: o os-service manda `StartDiagnosis` pela fila `execution-commands`; a OS aparece na fila do execution-service.
3. O mecânico conclui o diagnóstico no execution → `DiagnosisCompleted` → o orquestrador reserva as peças e manda `CreateBudget` ao billing.
4. O cliente aprova o orçamento no billing com o próprio token (outro CPF recebe 403).
5. Pagamento: com o Mercado Pago, abrir o link do Checkout Pro e pagar com o comprador de teste; mostrar o webhook chegando. Com o gateway fake, o script chama `POST /payments/{id}/simulate`.
6. Reparo e entrega. Mostrar o `/details`: OS, saga, orçamento, pagamento, execução e histórico de status numa resposta só (API composition).
7. Caixa de e-mail: um e-mail por mudança de status, enviado pela Lambda de notificação.

## Bloco 4: saga com falhas e compensação (8:30 a 11:30)

`PAUSE=1 GATEWAY=... scripts/demo.sh pagamento` e depois `PAUSE=1 GATEWAY=... scripts/demo.sh reparo`:

- **Pagamento recusado** (`X-Simulate-Failure: payment`): mostrar o estoque com as peças reservadas, a recusa e a compensação: orçamento cancelado, peças de volta, execução cancelada, OS `cancelled` com o motivo no `/details`.
- **Reparo recusado depois de pago** (`X-Simulate-Failure: repair`): o orquestrador manda estornar; a saga fica em `compensating` até o `PaymentRefunded` e então cancela. Pagamento `refunded`.
- Uma frase para cada garantia: outbox (estado e mensagem na mesma transação), inbox (reentrega não repete efeito), DLQ com alerta e runbook. Mostrar as DLQs vazias no console do SQS.
- Citar os prazos: orçamento sem resposta em 72 h ou pagamento sem confirmação em 24 h expira e compensa sozinho.

## Bloco 5: monitoramento e rastreamento (11:30 a 14:00)

- **Distributed tracing** no New Relic, filtrando pelo `os_id` da OS do bloco 3 (o `demo.sh` imprime o filtro): um trace que começa no `POST /service-orders`, passa pelo relay do outbox (`OutboxRelay/<tipo>`) e segue pelas filas até o execution e o billing (`Consume/<tipo>`), com as consultas ao Postgres como segmentos.
- **Service map**: os três serviços, as Lambdas e o Postgres, ligados pelo trace.
- **Logs em contexto**: abrir um log do trace e mostrar `saga_id`, `os_id` e `trace.id`; num log da Lambda issuer, o campo `document` com o CPF mascarado.
- **Dashboard** "Ordem de Serviço (prod)": página Saga (funil por etapa, concluídas, compensadas e expiradas), Serviços (latência e erros), Mensageria e trace, Lambdas.
- **Alertas e SLOs**: os 16 alertas, cada um com link para o runbook, e os SLOs de disponibilidade por serviço, latência do os-service e OS encerradas sem compensação técnica.

## Bloco 6: encerramento (14:00 a 14:30)

- Onde está cada coisa: READMEs com arquitetura e evidência de cobertura, ADRs, RFCs, AsyncAPI, Swagger, Postman e o documento de segurança.
- Limitações, sem tocar requisito: o Learner Lab não permite criar roles, então os pods usam a `LabRole` em vez de IRSA e a integração da conta AWS com o New Relic (métricas nativas de SQS e RDS) fica de fora; serviços, Lambdas, trace, logs e DLQ já estão no New Relic. RDS Single-AZ e um ambiente só na AWS, com a homologação no e2e local.

## Plano B

- Se a sessão do lab cair: o ambiente e2e local (`postech-tc4-infra-k8s/local`) roda os mesmos serviços e Lambdas com LocalStack e Mailpit. `make all` confere os contratos, sobe tudo, publica as Lambdas e roda o `e2e.sh` (caminho feliz e três compensações). Para gravar os blocos 3 e 4 com o mesmo roteiro, `make demo` sobe um nginx com as rotas do API Gateway na porta 8090 e roda `GATEWAY=http://localhost:8090 ADMIN_PASSWORD=admin123 ../scripts/demo.sh tudo` (validado em 31/10). O gateway local não emula o authorizer: quem recusa a requisição sem token são os próprios serviços. Os e-mails aparecem no Mailpit, em http://localhost:8026.
- Se o deploy demorar: gravar o merge, cortar a espera e voltar no resumo do job concluído.
