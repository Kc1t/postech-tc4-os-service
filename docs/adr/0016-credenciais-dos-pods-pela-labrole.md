# ADR-0016: Credenciais AWS dos pods pela LabRole, via IMDS com hop limit 2

- **Status:** Aceita
- **Data:** 2026-10
- **Decisão por:** Reservoir Devs

## Contexto

Na Fase 3 os pods só falavam com o PostgreSQL, por usuário e senha. Na Fase 4 os três serviços chamam APIs da AWS: SNS e SQS na mensageria ([ADR-0013](./0013-mensageria-sns-sqs-dlq.md)) e, no execution, o DynamoDB ([ADR-0012](./0012-banco-por-servico.md)). Cada pod precisa de credencial AWS.

Fora do Learner Lab, o caminho é dar a cada serviço uma role IAM própria (IRSA ou EKS Pod Identity). O Learner Lab **não deixa criar roles**: o cluster e os nós usam roles que o lab já entrega (`LabEksClusterRole`, `LabEksNodeRole`, `LabRole`). A `LabEksNodeRole` só tem as policies de worker do EKS, sem SQS, SNS ou DynamoDB.

As credenciais do usuário do lab são temporárias e expiram com a sessão (~4 h, RFC-0001).

Um detalhe do EKS fecha o problema: o node group gerenciado com AMI AL2023 e **sem launch template** sobe com IMDSv2 e `http_put_response_hop_limit = 1`. A resposta do IMDS não atravessa o salto extra da rede do container, então a SDK de dentro do pod não obtém a credencial do nó.

## Decisão

**Os nós usam a `LabRole`, e os pods herdam a credencial dela pelo IMDS.**

- O node group usa `node_role_arn` da `LabRole` (variável `lab_eks_node_role_regex = "^LabRole$"` no `postech-tc4-infra-k8s`).
- O node group ganhou um **launch template** (`aws_launch_template.workers`) com:

  ```hcl
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"   # só IMDSv2
    http_put_response_hop_limit = 2            # alcança o container
  }
  ```

- Os serviços não recebem chave nenhuma. A SDK usa a cadeia padrão (`config.LoadDefaultConfig` em `pkg/awsx`): sem `AWS_ACCESS_KEY_ID` no ambiente, cai no IMDS do nó. Os ConfigMaps só trazem `AWS_REGION` e os endereços das filas, dos tópicos e, no execution, o nome da tabela.
- No ambiente local o mesmo código lê `AWS_ENDPOINT_URL` e as credenciais fixas do LocalStack. Nada no código distingue os dois casos.

O launch template não declara security group, para o EKS continuar aplicando o security group do cluster nos nós. É por ele que os dois RDS liberam a porta 5432 ([ADR-0012](./0012-banco-por-servico.md)).

## Alternativas consideradas

1. **IRSA (IAM Roles for Service Accounts).** É o padrão do EKS: um provedor OIDC do cluster e uma role por service account, com escopo mínimo por serviço. Descartado por ser inviável no lab, que não permite criar a role nem a relação de confiança com o OIDC.

2. **EKS Pod Identity.** Mais simples que o IRSA (um addon e uma associação por service account) e é o que o comentário do `eks.tf` aponta como certo fora do lab. Descartado pelo mesmo bloqueio: exige criar uma role por serviço.

3. **Chave de acesso em Secret Kubernetes.** Funcionaria sem mexer nos nós. Descartada porque as credenciais do lab são de sessão e expiram em horas, então os pods parariam de consumir as filas no meio da demonstração até alguém atualizar o Secret e reiniciar tudo. Seria ainda uma credencial de usuário humano guardada no cluster.

4. **`hostNetwork: true` nos pods.** Com a rede do nó, o hop limit 1 bastaria. Descartado porque o pod passa a disputar portas com o nó e fica fora do modelo de rede do cluster, o que esvazia a `NetworkPolicy` de cada serviço.

5. **Manter a `LabEksNodeRole`.** É a role "certa" para worker. Descartada porque os pods ficariam sem SQS, SNS e DynamoDB, e o lab não deixa criar uma role nova com essas permissões.

## Consequências

### Positivas

- Nenhuma credencial AWS em Secret, variável de ambiente ou repositório. A renovação é do IMDS, transparente para a SDK.
- O mesmo binário roda no LocalStack e na AWS, mudando só o ambiente.
- IMDSv2 obrigatório (`http_tokens = "required"`): o IMDSv1, explorável por SSRF com um GET simples, fica desligado.

### Negativas

- **Não há menor privilégio por serviço.** Todo container em qualquer nó (os três serviços, o agente do New Relic, os pods do `kube-system`) obtém a credencial da `LabRole`, que é ampla. O billing consegue ler a tabela do execution, e o IAM não impede.
- **O hop limit 2 é o oposto do endurecimento recomendado.** A recomendação para EKS com IRSA é hop limit 1, justamente para o container não alcançar o IMDS. Aqui o acesso ao IMDS é a credencial.
- **Sem identidade por serviço no CloudTrail.** As chamadas aparecem como a sessão da role do nó, e a auditoria não distingue qual serviço publicou ou leu o quê.
- **O launch template é do cluster, não do serviço.** Mudar o template recicla os nós (`max_unavailable = 1`), e quem altera o `eks.tf` precisa saber que o hop limit não é detalhe.

Em produção real, a troca é por EKS Pod Identity com uma role por serviço (SQS e SNS só das próprias filas e tópicos; DynamoDB só no execution) e a volta do hop limit para 1. O código dos serviços não muda, porque a SDK lê a credencial do Pod Identity pela mesma cadeia padrão.

## Referências

- Implementação: [`postech-tc4-infra-k8s/eks.tf`](https://github.com/Kc1t/postech-tc4-infra-k8s/blob/main/eks.tf) (launch template e node group), [`pkg/awsx/awsx.go`](../../pkg/awsx/awsx.go)
- AWS: [Configure the instance metadata options](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/configuring-IMDS-existing-instances.html)
- AWS: [EKS Pod Identity](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html)
