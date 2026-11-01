## O que foi feito

<!-- O que este PR implementa, corrige ou refatora. -->

## Por que

<!-- Contexto e motivação: por que a mudança é necessária agora. -->

## Como testar

<!-- Passos para validar o comportamento esperado. -->

- [ ] ...

## Checklist

- [ ] `make test` e `make bdd` passam (Postgres de teste com `make test-db`)
- [ ] Cobertura continua acima de 80% (`make cover`)
- [ ] `make lint` sem achados
- [ ] Swagger regenerado (`make swagger`) se a API mudou
- [ ] Contrato de mensagem alterado também em `pkg/contracts` dos outros repositórios e no AsyncAPI (`make asyncapi` valida)
- [ ] Sem segredo ou credencial no código
- [ ] Sem `TODO` novo sem issue

## Tipo de mudança

- [ ] Nova funcionalidade
- [ ] Refatoração
- [ ] Correção de bug
- [ ] Documentação
- [ ] Infra ou CI
