# ADR-0014: Lambdas fora da VPC — a autenticação consulta o os-service, não o banco

- **Status:** Aceita
- **Data:** 2026-10
- **Decisão por:** Reservoir Devs

## Contexto

Na Fase 3 a Lambda emissora de token rodava **dentro da VPC** para ler a tabela `requesters` direto no RDS da aplicação. Como só falava com o banco, não havia NAT Gateway, e a string de conexão chegava por variável de ambiente em vez do Secrets Manager. Era a forma de evitar os US$ 32,85/mês do NAT (RFC-0002, risco "Senha em variável de ambiente da Lambda").

A Fase 4 muda duas premissas:

1. **Nenhum serviço pode acessar o banco de outro.** A tabela `requesters` é do os-service ([ADR-0012](./0012-banco-por-servico.md)), e uma Lambda lendo o banco dele é exatamente o acoplamento proibido.
2. **As Lambdas precisam de New Relic** (lacuna apontada na avaliação da Fase 3). A extension do New Relic envia telemetria para a internet. Uma Lambda na VPC sem NAT não tem essa saída.

A Lambda de notificação ([ADR-0022](./0022-notificacao-por-lambda.md)) tem a mesma necessidade: fala com o relay SMTP da Brevo, com SQS e DynamoDB pelos endpoints públicos, e com o New Relic.

## Decisão

**As duas Lambdas rodam fora da VPC** (sem `vpc_config`).

A emissora (`postech-tc4-prod-auth-issuer`) consulta o cliente na rota interna do os-service:

```
GET /internal/requesters/{doc}
X-Internal-Token: <INTERNAL_API_TOKEN>
```

- **O caminho é o NLB do Service `os-service-public`**, o mesmo que o API Gateway usa. O Terraform da Lambda acha o NLB pela tag `kubernetes.io/service-name = postech/os-service-public`, ou usa a variável `os_service_url` se ela vier preenchida.
- **`/internal/*` não tem rota no API Gateway.** Só é alcançável pelo NLB ou de dentro do cluster.
- **O token é compartilhado** entre a Lambda e o os-service (`INTERNAL_API_TOKEN`), comparado em tempo constante (`crypto/subtle`). Sem token configurado, o os-service responde 503 em vez de liberar a rota.
- **A resposta tem só o que a emissão precisa:** `id`, `name`, `email`, `document` e `status`. Inexistente vira 404 e documento inválido vira 400 no os-service. A Lambda valida o CPF antes de chamar (400 se inválido), devolve 404 para cliente inexistente e 403 para `status` diferente de `active`.
- **Falha do os-service vira 503 na emissão.** O cliente HTTP da Lambda faz até duas tentativas com 150 ms de intervalo, cada uma com timeout de 3 s (`UPSTREAM_TIMEOUT`), e só repete em erro de rede e 5xx. Qualquer outra resposta inesperada vira 500.

O authorizer não muda: valida a assinatura HS256 com o `JWT_SECRET` e não precisa de rede.

## Alternativas consideradas

1. **Manter a Lambda na VPC lendo o RDS.** Descartada pelo requisito: viola "nenhum serviço acessa o banco de outro" e amarra a Lambda ao schema do os-service. Além disso, a extension do New Relic ficaria sem saída para a internet.

2. **Lambda na VPC com NAT Gateway, chamando o os-service por dentro.** Resolveria a saída para o New Relic e permitiria falar com um endereço privado. Descartada pelo custo: o NAT cobra por hora (os mesmos US$ 32,85/mês que a Fase 3 evitou) mais o tráfego processado. A ENI na VPC também pesa no cold start de uma rota que o cliente espera síncrona.

3. **Lambda na VPC com VPC endpoints, sem NAT.** Cobriria SQS, DynamoDB e Secrets Manager. Descartada porque não cobre o New Relic nem o SMTP da Brevo, que são destinos na internet. Cada endpoint de interface também cobra por hora e por AZ.

4. **Rota de consulta de cliente no API Gateway.** Daria TLS e throttling ao caminho. Descartada porque criaria uma rota pública de busca de cliente por CPF, que é a superfície de enumeração que o limite reduzido em `POST /auth` tenta conter, e exigiria uma terceira forma de autenticação no gateway só para a Lambda.

5. **Réplica dos clientes numa tabela da Lambda, alimentada por eventos.** Tiraria a dependência síncrona do os-service. Descartada porque desativar um cliente passaria a valer só quando o evento chegasse, e a emissão de token é justamente o lugar onde essa demora importa ([ADR-0007](./0007-api-gateway-comunicacao.md) zerou o cache do authorizer pelo mesmo motivo). Seriam ainda uma tabela, um evento e um consumidor a mais para uma consulta de baixo volume.

## Consequências

### Positivas

- A Lambda não tem mais credencial de banco. O `DATABASE_URL` em variável de ambiente, risco aceito na Fase 3, deixou de existir.
- O contrato entre a Lambda e os dados do cliente é um JSON de cinco campos, não o schema da tabela `requesters`.
- O New Relic funciona nas duas functions sem NAT, e o cold start não espera ENI.
- A emissão continua checando existência **e** status do cliente a cada token, sem cache.

### Negativas

- **O token interno trafega sem TLS.** O NLB é público e escuta HTTP na porta 80. A chamada sai da Lambda para o IP público do NLB com o `X-Internal-Token` em claro. A comparação em tempo constante protege contra ataque de tempo, não contra interceptação nem contra tentativa por força bruta, já que o NLB não tem rate limit. O passo seguinte é TLS no NLB (certificado no ACM) ou NLB interno com VPC Link, o mesmo caminho registrado na [ADR-0007](./0007-api-gateway-comunicacao.md).
- **A emissão de token depende do os-service.** Na Fase 3 dependia só do RDS. Agora, os-service fora significa cliente sem token (503), mesmo com o banco no ar.
- **Rotacionar o `INTERNAL_API_TOKEN` exige coordenar dois repositórios**, como já acontece com o `JWT_SECRET`.
- O token interno é um segredo estático, sem identidade de quem chama. Uma assinatura por requisição (IAM SigV4 ou mTLS) seria o desenho correto fora do Learner Lab.

## Referências

- Lambda: [`postech-tc4-lambda-auth/infra/main.tf`](https://github.com/Kc1t/postech-tc4-lambda-auth/blob/main/infra/main.tf) e [`internal/requester/requester.go`](https://github.com/Kc1t/postech-tc4-lambda-auth/blob/main/internal/requester/requester.go)
- Rota interna: [`internal/adapters/inbound/http/internalapi/`](../../internal/adapters/inbound/http/internalapi/)
- AWS: [Giving Lambda functions access to resources in an Amazon VPC](https://docs.aws.amazon.com/lambda/latest/dg/configuration-vpc.html)
