# ADR-0021: Ambiente único no EKS; homologação no ambiente e2e local

- **Status:** Aceita
- **Data:** 2026-10
- **Decisão por:** Reservoir Devs

## Contexto

Esta ADR substitui parcialmente a [ADR-0010](./0010-cluster-unico-dois-namespaces.md), que resolveu a homologação da Fase 3 com um cluster e dois namespaces (`postech-homolog` e `postech`), dividindo o mesmo RDS. Era um serviço só.

A Fase 4 multiplica o que um ambiente contém:

- **Três serviços**, cada um com HPA de 2 a 6 réplicas.
- **Dois RDS**, um por serviço relacional ([ADR-0012](./0012-banco-por-servico.md)), e a tabela DynamoDB do execution.
- **Quatro filas, quatro DLQs e três tópicos** ([ADR-0013](./0013-mensageria-sns-sqs-dlq.md)), mais a Lambda de notificação e a tabela de dedup dela.

Repetir o desenho da ADR-0010 significaria, na mesma conta do Learner Lab, um segundo par de RDS, filas e tópicos com prefixo de ambiente e um namespace de homologação disputando os mesmos nós. Com os preços da RFC-0001, só as duas `db.t3.micro` e os dois `t3.medium` a mais para caber a cota de outro namespace somam ~US$ 0,12/hora: mais da metade do custo do ambiente inteiro da Fase 3.

Ao mesmo tempo, a Fase 4 trouxe uma forma melhor de homologar: um ambiente e2e local que sobe os três serviços reais, cada um com o próprio banco, as Lambdas e a mensageria no LocalStack, e roda a saga de ponta a ponta.

## Decisão

**Na AWS existe um ambiente só, produção: o cluster `postech-tc4-prod` e o namespace `postech`.** A homologação é o ambiente e2e local.

### Produção (EKS)

- O namespace `postech` e o teto dele ficam no `postech-tc4-infra-k8s` (`k8s/namespace.yaml`): `ResourceQuota` com 4 CPU / 4Gi de requests, 10 CPU / 10Gi de limits e 30 pods (três serviços com até seis réplicas, mais folga para o rolling update), e `LimitRange` com requests e limits padrão por container. A quota é do namespace inteiro, então fica na plataforma, e não num serviço.
- Os três serviços fazem deploy no mesmo namespace (`NAMESPACE: postech` no CI de cada um), e o API Gateway procura os NLBs `<serviço>-public` nele.
- Tópicos e filas não têm prefixo de ambiente: só produção existe na AWS.

### Homologação (e2e local, `postech-tc4-infra-k8s/local/`)

- `docker compose` com os três serviços construídos dos repositórios vizinhos, um PostgreSQL 16 para o os e outro para o billing, LocalStack 3.8 (SNS, SQS com DLQ e `maxReceiveCount` 5, DynamoDB e Lambda) com a mesma topologia do Terraform, e Mailpit no lugar do SMTP da Brevo.
- As Lambdas reais (issuer e notifier) são compiladas para `linux/arm64` e publicadas no LocalStack.
- `make e2e` roda quatro cenários: caminho feliz até a entrega (com o JWT emitido pela Lambda e sete e-mails no Mailpit), pagamento recusado, reparo recusado depois do pagamento (com estorno) e orçamento recusado pelo cliente. Ao final, confere que as quatro DLQs estão vazias.

### Branches

| Repositório | Push em `homolog` | Push em `main` |
|---|---|---|
| os, billing, execution | CI completo (lint, gosec, govulncheck, testes com cobertura, BDD no os e teste de componente no billing e no execution, SonarQube, Terraform validate, imagem com Trivy), sem deploy | o mesmo CI e deploy no EKS (`postech`) |
| `postech-tc4-notification-lambda` | CI, sem deploy | `terraform apply` da function de produção |
| `postech-tc4-lambda-auth` | CI, sem deploy | `terraform apply` das functions de produção |
| `postech-tc4-infra-k8s` | `terraform plan` contra o state de produção | `terraform apply` |

O deploy só roda com a variável `AWS_DEPLOY_ENABLED=true`: as credenciais do lab expiram a cada sessão, e sem elas o push não deve ficar vermelho.

### O que segue valendo da ADR-0010

Um cluster só; `plan` na homolog e `apply` só na `main` para a infraestrutura de cluster; teto de recursos por namespace com `ResourceQuota` e `LimitRange`; `NetworkPolicy` por serviço. O que deixa de valer: o namespace `postech-homolog`, o deploy da homolog no cluster e o RDS compartilhado entre ambientes.

## Alternativas consideradas

1. **Manter dois namespaces, como na ADR-0010.** Daria um homolog na mesma infraestrutura de produção. Descartado pelo custo (~US$ 0,12/hora a mais, contando só os bancos e os nós) e pela duplicação de filas e tópicos com prefixo de ambiente em três repositórios. O ganho sobre o e2e local seria testar o IAM e a rede da AWS, que o ambiente único de produção já exercita.

2. **Dois namespaces compartilhando bancos e filas.** Sairia barato. Descartado porque a homologação consumiria as mesmas filas que produção: uma mensagem de teste poderia ser processada pelo serviço de produção, e vice-versa. Um banco compartilhado entre ambientes já era a consequência mais grave da ADR-0010; aqui seriam dois.

3. **Ambiente efêmero por PR no EKS.** É o modelo mais completo. Descartado porque cria e destrói dois RDS a cada PR, cada um com vários minutos de provisionamento, em sessões de lab de quatro horas.

4. **`kind` no CI para o e2e.** Rodaria a homologação a cada push. Descartado nesta fase pelo tempo de execução (três imagens, dois bancos, LocalStack e build das Lambdas) e porque o docker compose já reproduz o desenho de serviços, bancos e mensageria, que é o que o e2e verifica.

## Consequências

### Positivas

- O custo de AWS fica em um ambiente: um cluster, dois RDS e a mensageria.
- A homologação roda os três serviços reais, as duas Lambdas e a saga com compensações, inclusive o estorno, sem custo e sem credencial do lab.
- Uma topologia só para manter: o `init-aws.sh` do LocalStack e o `messaging.tf` criam as mesmas filas e assinaturas.

### Negativas

- **A homologação não é automática.** O `make e2e` roda na máquina de quem desenvolve, não a cada push na `homolog`. O CI da homolog garante testes, BDD, componente e qualidade de cada serviço, não a integração entre eles.
- **O LocalStack não é a AWS.** IAM, rede do EKS, IMDS ([ADR-0016](./0016-credenciais-dos-pods-pela-labrole.md)), NLB e API Gateway só são exercitados em produção.
- **O primeiro deploy de uma mudança na AWS já é o de produção.** O `plan` da homolog mostra o efeito na infraestrutura de cluster, mas não o exercita.

Como na ADR-0010, nada disso seria aceitável em produção real: lá, homologação é um ambiente na mesma nuvem, com a mesma infraestrutura e dados próprios. Aqui é dívida deliberada, para caber no orçamento do lab.

## Referências

- Ambiente e2e: [`postech-tc4-infra-k8s/local/`](https://github.com/Kc1t/postech-tc4-infra-k8s/tree/main/local)
- Namespace e quota: [`postech-tc4-infra-k8s/k8s/namespace.yaml`](https://github.com/Kc1t/postech-tc4-infra-k8s/blob/main/k8s/namespace.yaml)
- Pipeline: [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml), job `deploy`
- LocalStack: [Documentation](https://docs.localstack.cloud/)
