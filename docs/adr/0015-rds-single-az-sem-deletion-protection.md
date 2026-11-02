# ADR-0015: RDS Single-AZ e sem deletion protection no Learner Lab

- **Status:** Aceita
- **Data:** 2026-10
- **Decisão por:** Reservoir Devs

## Contexto

A RFC-0002 propôs Multi-AZ, backup de 7 dias e deletion protection em produção. Na Fase 3 o Terraform seguia isso: `multi_az` e `deletion_protection` ligados quando `environment = "prod"`, e só o `prod.tfvars` desligava o Multi-AZ para caber no orçamento.

A Fase 4 tem **dois** RDS de produção, um no os-service e outro no billing ([ADR-0012](./0012-banco-por-servico.md)), e o ambiente roda no AWS Academy Learner Lab:

- O crédito é fechado (US$ 100 por aluno), e uma `db.t3.micro` custa ~US$ 0,018/hora. Multi-AZ dobra o valor de cada instância.
- A sessão do lab expira, e a prática do projeto é destruir a infraestrutura ao fim de cada sessão de trabalho e recriá-la pelo Terraform na seguinte (RFC-0001).
- O lab não permite criar chave KMS gerenciada pelo cliente.

Com deletion protection ligada, cada `terraform destroy` falha até alguém desligar a proteção num apply separado. Com snapshot final obrigatório, cada destruição deixa um snapshot cobrado e exige um identificador novo na próxima.

## Decisão

Nos dois RDS (`infra/main.tf` do os-service e do billing), para o ambiente do Learner Lab:

| Parâmetro | Valor | Por quê |
|---|---|---|
| `multi_az` | `false` (variável, padrão desligado) | Metade do custo de instância |
| `deletion_protection` | `false` | `terraform destroy` funciona em um passo |
| `skip_final_snapshot` | `true` | Destruir não deixa snapshot cobrado nem exige identificador novo |
| `backup_retention_period` | `1` | Backup automático e restauração pontual mantidos, com o mínimo de retenção |
| `apply_immediately` | `true` | Mudança aplicada no apply, sem esperar janela de manutenção |
| `recovery_window_in_days` (segredo) | `0` | O nome do segredo fica livre para o próximo apply |

O que **não** foi afrouxado: armazenamento gp3 criptografado, `publicly_accessible = false`, `rds.force_ssl = 1`, security group restrito ao cluster, Performance Insights com 7 dias e export do log do PostgreSQL para o CloudWatch (consultas acima de 500 ms).

A tabela DynamoDB do execution segue o mesmo raciocínio: `deletion_protection_enabled = false`, com point-in-time recovery e criptografia ligados.

Cada desligamento está marcado no código. O `tfsec:ignore` traz o motivo ao lado do recurso, e a descrição da variável `multi_az` aponta para esta ADR.

## Alternativas consideradas

1. **Multi-AZ nos dois bancos.** É o certo em produção: failover automático para a standby em outra AZ. Descartado pelo custo: dobra a linha de banco de ~US$ 0,036/hora para ~US$ 0,072/hora, num ambiente sem usuário real para proteger.

2. **Deletion protection ligada, como na Fase 3.** Protege contra o `destroy` por engano, que é o risco que ela existe para cobrir. Descartada porque o `destroy` aqui é rotina: com dois bancos, cada fim de sessão exigiria dois applies de desligamento antes da destruição, e um esquecimento deixaria o banco cobrando com o lab encerrado.

3. **Snapshot final e retenção de 7 dias.** Preservaria os dados entre sessões. Descartado porque os dados do projeto são de demonstração e voltam pelo seed. Snapshots acumulados por sessão consomem crédito sem trazer nada de volta.

4. **Um RDS só, com um database por serviço.** Resolveria o custo do Multi-AZ de outro jeito. Descartado na [ADR-0012](./0012-banco-por-servico.md): o enunciado pede banco e infraestrutura próprios por serviço.

5. **Aurora Serverless v2.** Descartada na [RFC-0002](../rfc/0002-banco-gerenciado.md): o piso de capacidade cobrado é maior que uma `db.t3.micro` ociosa.

## Consequências

### Positivas

- O custo de banco relacional fica em ~US$ 0,036/hora para os dois serviços.
- Destruir e recriar o ambiente é um `terraform destroy` e um `terraform apply`, sem passo manual.
- As decisões de produção continuam a uma variável de distância: `multi_az` já existe, e os demais parâmetros são uma linha cada.

### Negativas

- **Uma falha de AZ derruba o banco** até a recuperação manual. Com a instância Single-AZ, manutenção e patch também causam indisponibilidade.
- **Um `destroy` por engano apaga os dados** sem snapshot para restaurar. Com retenção de 1 dia, o backup automático cobre só as últimas 24 horas, e só enquanto a instância existir.
- **`apply_immediately` reinicia a instância no meio do dia** quando a mudança exige reboot.
- **Criptografia com a chave gerenciada da AWS**, não com CMK. Não dá para revogar a chave nem auditar o uso por chave própria.

### O que muda em produção real

| Item | Learner Lab | Produção |
|---|---|---|
| `multi_az` | `false` | `true` |
| `deletion_protection` | `false` | `true` |
| `skip_final_snapshot` | `true` | `false`, com `final_snapshot_identifier` |
| `backup_retention_period` | 1 dia | 7 a 35 dias, conforme a política de retenção |
| `apply_immediately` | `true` | `false`, com janela de manutenção definida |
| Segredo | `recovery_window_in_days = 0` | 7 a 30 dias |
| Criptografia | chave gerenciada da AWS | CMK própria (KMS) |
| Autenticação | senha na DSN (a autenticação IAM já está habilitada na instância, mas os pods não têm role própria) | IAM database authentication com uma role por serviço ([ADR-0016](./0016-credenciais-dos-pods-pela-labrole.md)) |
| DynamoDB | `deletion_protection_enabled = false` | `true` |

Nenhuma dessas configurações é aceitável em produção real. São aceitáveis aqui porque o ambiente é acadêmico e recriável. A decisão fica registrada para que quem herdar o projeto saiba que é dívida deliberada, não descuido.

## Referências

- Implementação: [`infra/main.tf`](../../infra/main.tf) e [`infra/variables.tf`](../../infra/variables.tf) (os-service), [`postech-tc4-billing-service/infra/main.tf`](https://github.com/Kc1t/postech-tc4-billing-service/blob/main/infra/main.tf)
- AWS: [Multi-AZ DB instance deployments](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.MultiAZSingleStandby.html)
- AWS: [Deleting a DB instance](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_DeleteInstance.html)
