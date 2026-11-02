# ADR-0019: API composition no `/details`, com resposta parcial

- **Status:** Aceita
- **Data:** 2026-10
- **Decisão por:** Reservoir Devs

## Contexto

Na Fase 3, ver uma OS completa era uma consulta só: OS, itens e status estavam na mesma base. Na Fase 4 a mesma visão está espalhada por três bancos ([ADR-0012](./0012-banco-por-servico.md)):

| Parte | Dono |
|---|---|
| OS, saga e histórico de status | os-service (PostgreSQL) |
| Orçamento e pagamento | billing-service (PostgreSQL) |
| Execução e linha do tempo do mecânico | execution-service (DynamoDB) |

A equipe precisa dessa visão num lugar só: é o que mostra onde a OS está, por que foi cancelada e o que falta. Nenhum serviço pode ler o banco do outro.

## Decisão

**API composition no os-service:** `GET /api/v1/service-orders/{id}/details` junta os dados locais com uma chamada ao billing e outra ao execution, feitas em paralelo.

| Fonte | Chamada | Destino no cluster |
|---|---|---|
| Local | OS, saga (`saga_instances`) e histórico | — |
| billing | `GET /api/v1/budgets/os/{os_id}` | `BILLING_SERVICE_URL` (`http://billing-service`) |
| execution | `GET /api/v1/executions/{os_id}` | `EXECUTION_SERVICE_URL` (`http://execution-service`) |

- **As chamadas não passam pelo API Gateway.** Vão direto ao Service ClusterIP de cada serviço, no mesmo namespace.
- **O os-service se autentica com um JWT de serviço** de curta duração (HS256, `sub = service:os-service`, papel `admin`, 2 minutos), assinado com o `JWT_SECRET` compartilhado. Os outros serviços validam com o mesmo middleware que usam para a equipe.
- **Cada chamada passa pelo `pkg/httpclient`:** timeout por tentativa (`UPSTREAM_TIMEOUT`, 2 s), até duas novas tentativas com backoff exponencial e jitter (só para método idempotente, como estes GET, e só em 5xx, 429 ou erro de rede) e um **circuit breaker por destino** (`sony/gobreaker`), que abre depois de 5 falhas seguidas e fica aberto 30 s. Com o circuito aberto, a chamada falha na hora.
- **Resposta parcial em vez de erro.** Se uma fonte falha (timeout, 5xx, circuito aberto), o campo dela vem `null`, `partial` vem `true` e o nome da fonte aparece em `unavailable` (`billing` ou `execution`). O status HTTP continua 200. O log `composition_source_unavailable` registra a fonte e o erro.
- **404 da fonte não é falha.** Uma OS recém-aberta ainda não tem orçamento: o campo vem `null` e a fonte não entra em `unavailable`.
- **A parte local sempre responde.** A saga já guarda `budget_id`, `payment_id`, `checkout_url`, o estado, o prazo e o motivo da falha. Mesmo com os dois serviços fora, a resposta diz em que passo a OS está.

```json
{
  "service_order": { "...": "..." },
  "saga": { "state": "awaiting_payment", "budget_id": "...", "checkout_url": "..." },
  "history": [ "..." ],
  "budget": null,
  "execution": { "...": "..." },
  "partial": true,
  "unavailable": ["billing"]
}
```

## Alternativas consideradas

1. **CQRS com réplica de leitura (visão materializada no os-service).** O os-service já recebe todos os eventos do billing e do execution em `os-saga-replies`; poderia projetá-los numa tabela de leitura e responder o `/details` sem chamar ninguém. Descartado nesta fase: a projeção copiaria o modelo de dois serviços e teria de mudar a cada mudança deles, os eventos teriam de carregar o estado completo (hoje carregam o necessário para a saga) e a visão seria eventualmente consistente por definição. O `/details` é leitura de tela, de baixo volume; não justifica o custo. Fica registrado como evolução se a consulta virar caminho quente ou relatório.

2. **Composição no cliente.** O front chamaria os três serviços pelo gateway. Descartada porque seriam três idas e voltas pelo gateway e pelo authorizer, e o acoplamento iria para quem consome. A API do execution só aceita `admin` e `mechanic`, então o cliente final nem teria acesso a uma das partes.

3. **Agregador fora dos serviços** (Lambda ou BFF atrás do API Gateway). O HTTP API do API Gateway não agrega respostas sozinho. Descartado porque seria mais um componente com deploy, credencial e observabilidade próprios, para fazer o que o os-service, dono da OS, já faz.

4. **Composição que falha inteira.** Mais simples, sem campo `unavailable`. Descartada porque a disponibilidade do `/details` viraria o produto da disponibilidade dos três serviços, e a parte mais útil (o estado da saga) é justamente a que não depende dos outros.

## Consequências

### Positivas

- A visão completa da OS existe sem nenhum serviço ler o banco de outro e sem cópia de dados.
- Os dados de orçamento e execução são os de agora, lidos do dono.
- Um serviço fora degrada a resposta, não a derruba. O circuit breaker evita que cada requisição espere o timeout de um serviço que já se sabe fora.
- As duas chamadas aparecem como *external* no trace do New Relic, ligadas à requisição de origem.

### Negativas

- **A latência é a da fonte mais lenta**, com retry. No pior caso são três tentativas de 2 s mais o backoff, ainda abaixo dos 10 s de timeout da integração no API Gateway, mas longe de uma consulta local.
- **As partes não são um retrato do mesmo instante.** O orçamento pode refletir uma aprovação que a saga ainda não processou.
- **O token de serviço tem papel `admin`.** O execution e o billing não distinguem o os-service de um administrador humano. E o `JWT_SECRET` é o mesmo em todos os serviços: vazou em um, vale em todos.
- **Cada requisição ao `/details` gera duas chamadas internas.** Se a tela for consultada em polling, a carga cai em três serviços.

## Referências

- Implementação: [`internal/application/usecase/service_order/details.go`](../../internal/application/usecase/service_order/details.go), [`internal/adapters/outbound/services/client.go`](../../internal/adapters/outbound/services/client.go), [`pkg/httpclient/httpclient.go`](../../pkg/httpclient/httpclient.go)
- Chris Richardson: [Pattern: API Composition](https://microservices.io/patterns/data/api-composition.html) e [Pattern: CQRS](https://microservices.io/patterns/data/cqrs.html)
