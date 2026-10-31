# Roteiro do vídeo: Tech Challenge Fase 4

Limite de **15 minutos**; o roteiro fecha em cerca de 14. O enunciado pede quatro demonstrações:

| Item exigido | Bloco |
|---|---|
| Fluxo completo de uma OS passando pelos microsserviços | 3 |
| Execução do Saga Pattern e tratamento de falhas | 4 |
| Deploy automatizado de pelo menos um microsserviço, com validação de testes | 2 |
| Monitoramento e rastreamento dos fluxos distribuídos | 5 |

Na descrição do vídeo no YouTube, colar os timestamps dos blocos (o avaliador da fase 3 elogiou isso).

## Antes de gravar

**Ambiente (no dia, com o Learner Lab iniciado):**
- [ ] `scripts/setup-github.sh aws` no infra-k8s (credenciais da sessão) e `enable`
- [ ] Pipelines aplicados na ordem do README do infra-k8s: infra-k8s → os, billing, execution → lambda-auth e notification → infra-k8s de novo (rotas do gateway)
- [ ] `kubectl get pods -n postech` com os três serviços `Running` (2 réplicas cada)
- [ ] `GATEWAY=... scripts/demo.sh tudo` rodado **pelo menos duas vezes, uma hora antes**, para o New Relic ter dados no dashboard e no service map
- [ ] Caixa de e-mail do cliente da demo aberta (o e-mail cadastrado no `demo.sh`, variável `EMAIL`)
- [ ] Se o Mercado Pago estiver ligado (`PAYMENT_GATEWAY=mercadopago`): usuário comprador de teste logado numa aba anônima

**Pipeline para o bloco 2:**
- [ ] Branch `demo/deploy` no execution-service com uma mudança pequena e visível (por exemplo, um texto no Swagger ou uma mensagem de log)
- [ ] PR aberto **uns 15 minutos antes** de gravar, para os checks já estarem verdes. Gravar o merge e o job de deploy; se o deploy demorar, cortar a espera na edição

**Telas:**
- [ ] Terminal com fonte grande, dois painéis: `scripts/demo.sh` e `kubectl get pods -n postech -w`
- [ ] Abas: diagrama `docs/diagrams/arquitetura-fase4.svg`, GitHub Actions do execution-service, regras de proteção da `main`, New Relic (dashboard "Ordem de Serviço", Distributed tracing, service map, alertas), console SQS (DLQs)

## Bloco 1: contexto e arquitetura (0:00 a 1:45)

- Grupo e objetivo: a oficina saiu do monolito da fase 3 para microsserviços com saga.
- Diagrama: três serviços no EKS, cada um com o próprio banco (PostgreSQL no os e no billing, DynamoDB no execution), duas Lambdas (autenticação e notificação), API Gateway na frente.
- Comunicação: REST na borda e na leitura composta; SNS/SQS na saga, com DLQ em toda fila. "Nenhum serviço lê o banco do outro: até a Lambda de login, que na fase 3 abria o banco, agora pergunta ao os-service."
- Por que **orquestração**: fluxo longo com espera do cliente, dinheiro e prazos; a compensação depende da etapa; os outros serviços só obedecem comandos (ADR-0017).

## Bloco 2: deploy automatizado com testes (1:45 a 4:30)

- Mostrar a proteção da `main` (Settings → Branches): PR obrigatório e checks exigidos.
- No PR do execution-service, abrir os checks: lint e SAST, testes com cobertura (95%+), teste de componente com LocalStack, SonarQube com quality gate, Terraform com tfsec, imagem com Trivy.
- Abrir o resumo do job SonarQube: tabela de cobertura, bugs, vulnerabilidades e duplicação.
- Fazer o merge. No job `deploy`: `terraform apply` da tabela DynamoDB, push no ECR, `kubectl apply`, `rollout status` e smoke test pelo NLB.
- No painel do `kubectl get pods -w`: os pods novos sobem antes dos antigos saírem (`maxUnavailable: 0`).

## Bloco 3: fluxo completo de uma OS (4:30 a 8:30)

`PAUSE=1 GATEWAY=... scripts/demo.sh feliz`, narrando cada etapa:

1. Login da equipe e cadastros. Cliente pede o token pelo CPF (`POST /auth` → Lambda issuer → os-service). Mostrar que sem token o gateway responde 401.
2. Abrir a OS: o os-service manda `StartDiagnosis` pela fila; a OS aparece na fila do execution-service.
3. Mecânico conclui o diagnóstico no execution → `DiagnosisCompleted` → o orquestrador reserva as peças e pede o orçamento ao billing.
4. Cliente aprova o orçamento no billing com o próprio token (só ele pode aprovar a dele).
5. Pagamento: com o Mercado Pago, abrir o link do Checkout Pro e pagar com o comprador de teste; mostrar o webhook chegando. Com o gateway fake, o `simulate`.
6. Reparo e entrega. Mostrar o `/details`: OS, saga, orçamento, pagamento e execução numa resposta só (API composition), e o histórico de status.
7. Caixa de e-mail: um e-mail por mudança de status, enviado pela Lambda de notificação.

## Bloco 4: saga com falhas e compensação (8:30 a 11:30)

`PAUSE=1 GATEWAY=... scripts/demo.sh pagamento` e depois `reparo`:

- **Pagamento recusado** (`X-Simulate-Failure: payment`): mostrar o estoque com as peças reservadas, a recusa e a compensação: orçamento cancelado, peças de volta, execução cancelada, OS `cancelled` com o motivo no `/details`.
- **Reparo recusado depois de pago** (`X-Simulate-Failure: repair`): o orquestrador manda estornar; a saga fica em `compensating` até o `PaymentRefunded` e então cancela. Pagamento `refunded`.
- Explicar em uma frase cada garantia: outbox (estado e mensagem na mesma transação), inbox (reentrega não repete efeito), DLQ com alarme e runbook. Mostrar as DLQs vazias no console.
- Citar o prazo: orçamento sem resposta em 72 h ou pagamento em 24 h expira e compensa sozinho.

## Bloco 5: monitoramento e rastreamento (11:30 a 14:00)

- **Distributed tracing** no New Relic, filtrando pelo `os_id` da OS do bloco 3: um trace que começa no `POST /service-orders` e segue pelas filas até o execution e o billing (`Consume/<tipo>`).
- **Service map**: os três serviços, as filas e as Lambdas ligados.
- **Logs em contexto**: abrir um log do trace; mostrar `saga_id`, `os_id`, `trace.id` e o CPF mascarado.
- **Dashboard**: funil da saga (`saga_step` por estado), concluídas × compensadas × expiradas, latência e erros por serviço, Lambdas.
- **Alertas e SLOs**: a política com um alerta por serviço e por sinal da saga, cada um com link para o runbook; os SLOs de disponibilidade e latência.

## Bloco 6: encerramento (14:00 a 14:30)

- Onde está cada coisa: READMEs com cobertura e arquitetura, ADRs, AsyncAPI, Swagger e Postman.
- Limitações honestas: Learner Lab sem IRSA e sem integração AWS no New Relic, RDS single-AZ, um ambiente só na AWS (homologação no e2e local).

## Plano B

- Se a sessão do lab cair: o ambiente e2e local (`postech-tc4-infra-k8s/local`, `make all`) roda os mesmos serviços e Lambdas com LocalStack e Mailpit, e o `e2e.sh` mostra o caminho feliz e as três compensações. Para gravar os blocos 3 e 4 com o mesmo roteiro, `./gateway.sh up` põe um nginx com as rotas do API Gateway na porta 8090 e `GATEWAY=http://localhost:8090 ADMIN_PASSWORD=admin123 ../scripts/demo.sh tudo` roda a demo inteira contra ele (validado em 31/10). Os e-mails aparecem no Mailpit, em http://localhost:8026.
- Se o deploy demorar: gravar o merge, cortar a espera e voltar no resumo do job concluído.
