# ADR-0020: Mercado Pago Checkout Pro atrás da porta `PaymentGateway`, com gateway fake

- **Status:** Aceita
- **Data:** 2026-10
- **Decisão por:** Reservoir Devs

## Contexto

O enunciado pede que o billing-service registre e verifique pagamentos **integrado ao Mercado Pago**. O pagamento é o passo 5 da saga ([ADR-0017](./0017-saga-orquestrada-no-os-service.md)): depois que o cliente aprova o orçamento, o orquestrador manda `billing.RequestPayment`, e a saga só avança com `billing.PaymentApproved`.

O que pesa na escolha:

- **O cliente paga fora do sistema.** O resultado chega depois, por notificação do provedor, e essa notificação vem da internet sem JWT.
- **Uma notificação forjada aprovaria um reparo sem pagamento.** A rota que recebe o aviso precisa ser pública e, ao mesmo tempo, confiável.
- **Testes, e2e local e a demonstração** não podem depender de conta, credencial e rede do Mercado Pago. As credenciais de teste são pessoais e o webhook exige URL pública.
- **A compensação da saga inclui estorno**, que precisa acontecer uma vez só mesmo com retry.

## Decisão

**Checkout Pro (Preferences API), atrás da porta `PaymentGateway`** do billing-service:

```go
type PaymentGateway interface {
    Provider() entities.Provider
    CreateCheckout(ctx context.Context, req CheckoutRequest) (preferenceID, checkoutURL string, err error)
    GetPayment(ctx context.Context, providerPaymentID string) (ProviderPayment, error)
    Refund(ctx context.Context, providerPaymentID, idempotencyKey string) error
    Cancel(ctx context.Context, providerPaymentID string) error
}
```

O adapter é escolhido por `PAYMENT_GATEWAY` (`mercadopago` ou `fake`).

### Adapter Mercado Pago

- **HTTP direto, sem SDK**, pelo `pkg/httpclient` (timeout `MP_TIMEOUT` de 5 s, retry só em método idempotente, circuit breaker).
- **Cobrança:** `POST /checkout/preferences` com o orçamento como um item só, pelo total (evita divergência de arredondamento entre itens), `external_reference` = ID do nosso pagamento, `notification_url` = `MP_NOTIFICATION_URL` e expiração de `MP_PREFERENCE_TTL` (24 h). O link devolvido ao cliente é o `init_point`.
- **Webhook:** `POST /api/v1/webhooks/mercadopago`, rota **pública** no API Gateway (sem authorizer) e no serviço. A autenticidade vem do header `x-signature`:
  1. separa `ts` e `v1`;
  2. monta o manifest `id:{data.id};request-id:{x-request-id};ts:{ts};`, com o `data.id` da query string;
  3. calcula HMAC-SHA256 com `MP_WEBHOOK_SECRET` e compara com `v1` em tempo constante.

  Assinatura inválida vira 401 `INVALID_SIGNATURE`. Sem segredo configurado, toda notificação é recusada (falha fechada). `MP_WEBHOOK_MAX_AGE` limita a idade do `ts` quando maior que zero. Notificação sem `data.id` ou de tipo diferente de `payment` recebe 200 `ignored` antes da checagem, sem efeito nenhum.
- **O corpo do webhook nunca é a fonte da verdade.** Com a assinatura válida, o billing relê o pagamento em `GET /v1/payments/{id}` e chega ao nosso registro pelo `external_reference`. `approved` vira `PaymentApproved` se o valor pago for o cobrado; com valor diferente, vira `PaymentFailed` e o pagamento é estornado. Uma aprovação que chega para um pagamento já encerrado também é estornada. `rejected` e `cancelled` viram `PaymentFailed`; `pending`, `in_process` e `authorized` só guardam o ID. Falha transitória responde 5xx e conflito de concorrência responde 409, e o Mercado Pago reentrega.
- **Estorno:** `POST /v1/payments/{id}/refunds` com `X-Idempotency-Key`. A mesma chave nas novas tentativas garante um estorno só.
- **401 e 403 do provedor são transitórios**, não recusa: credencial errada é problema de configuração. A mensagem vai para a DLQ em vez de cancelar a OS do cliente.

### Gateway fake

- `PAYMENT_GATEWAY=fake` (padrão local e no deploy, enquanto a variável `PAYMENT_GATEWAY` do repositório não muda): o checkout é uma página do próprio billing, `GET /fake-checkout/{payment_id}`, com rota pública no gateway.
- O resultado é simulado por `POST /api/v1/payments/{id}/simulate` (só `admin`), que passa pelo **mesmo aplicador de resultado** do webhook real: as mesmas regras, os mesmos eventos. Fora do fake a rota responde 403 `SIMULATION_DISABLED`.
- O webhook continua exigindo a assinatura com `MP_WEBHOOK_SECRET`; com o fake e sem esse segredo, toda notificação é recusada.

## Alternativas consideradas

1. **Checkout Transparente (Payments API) ou Checkout Bricks.** O formulário de cartão ficaria na nossa página. Descartado porque o billing passaria a coletar dados de pagamento, com o escopo de segurança que isso traz. No Checkout Pro o cliente paga na página do Mercado Pago, e o sistema só recebe o resultado.

2. **SDK oficial do Mercado Pago.** Menos código de integração. Descartado porque o adapter usa quatro chamadas, e o SDK ficaria fora do `pkg/httpclient`: sem o circuit breaker, o retry e a instrumentação do New Relic que as outras integrações já têm.

3. **Confiar no corpo do webhook, sem reler o pagamento.** Uma chamada a menos. Descartado porque a assinatura prova a origem, não que o status no corpo é o atual. O Mercado Pago manda `payment.created` e `payment.updated` e reentrega notificações, e a releitura resolve as duas questões.

4. **Consulta periódica do pagamento, sem webhook.** Dispensaria a rota pública. Descartada porque atrasa a aprovação até a próxima consulta e gasta chamadas com pagamentos que não mudaram. A releitura no webhook dá o mesmo resultado sem polling.

5. **Só o adapter real, com sandbox nos testes.** Descartado porque o teste de componente e o e2e dependeriam de rede, credencial pessoal e de uma URL pública para o webhook. O fake roda em qualquer máquina e no CI.

## Consequências

### Positivas

- Requisito atendido com o meio de pagamento que o enunciado pede, sem o sistema tocar em dado de cartão.
- O domínio do billing não conhece o Mercado Pago. Trocar de provedor é escrever outro adapter da porta.
- Webhook forjado não passa: falta a assinatura, e mesmo uma assinatura válida não aprova nada sem a releitura no provedor.
- O fake exercita o mesmo caminho do real a partir do resultado do provedor, então o teste de componente e o e2e cobrem as regras de pagamento sem rede.
- O estorno é idempotente do lado do provedor, não só do nosso.

### Negativas

- **O deploy sai com o fake por padrão.** Usar o Mercado Pago no EKS depende de trocar `PAYMENT_GATEWAY` e de configurar `MP_ACCESS_TOKEN` e `MP_WEBHOOK_SECRET`.
- **O fake não testa a integração.** Assinatura, formato da API e conta de teste só são exercitados com o adapter real. Os testes do adapter cobrem o manifest e as respostas com servidor HTTP simulado, não com o provedor.
- **A janela de idade do webhook está desligada** (`MP_WEBHOOK_MAX_AGE` = 0). Uma notificação capturada pode ser reenviada. O efeito é limitado, porque a releitura devolve o estado real e a máquina de estados não repete transições.
- **Pagamento recusado é definitivo.** No Checkout Pro o cliente poderia tentar outro meio na mesma preferência; para a saga, `PaymentFailed` cancela a OS.
- A rota do webhook é pública e não tem limite próprio além do throttling padrão do stage (100 req/s, rajada de 200).

## Referências

- Implementação: [`internal/ports/gateway.go`](https://github.com/Kc1t/postech-tc4-billing-service/blob/main/internal/ports/gateway.go), [`internal/adapters/outbound/mercadopago/`](https://github.com/Kc1t/postech-tc4-billing-service/tree/main/internal/adapters/outbound/mercadopago), [`internal/adapters/outbound/fakegateway/`](https://github.com/Kc1t/postech-tc4-billing-service/tree/main/internal/adapters/outbound/fakegateway)
- Mercado Pago: [Checkout Pro](https://www.mercadopago.com.br/developers/pt/docs/checkout-pro/landing) e [Webhooks](https://www.mercadopago.com.br/developers/pt/docs/your-integrations/notifications/webhooks)
