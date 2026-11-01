# RFCs (Request for Comments)

Propostas técnicas discutidas pelo time antes de virarem decisão. Diferente das [ADRs](../adr/README.md), que registram uma decisão fechada, a RFC carrega a comparação de alternativas, o impacto no custo, os riscos e as questões em aberto. Quando a proposta estabiliza, a conclusão vira ADR e a RFC aponta para ela.

## Convenções

- Numeração sequencial, sem buracos.
- Cabeçalho com **Status**, **Data**, **Autor** e decisões relacionadas.
- Seções: **Resumo**, **Motivação** (o problema), **Proposta** (a recomendação), **Alternativas avaliadas** (com o motivo do descarte), **Impacto no custo**, **Riscos**, **Questões em aberto**.
- Status: `Rascunho`, `Em discussão`, `Aceita`, `Aceita, revisada em AAAA-MM`, `Rejeitada`, `Substituída pela RFC-XXXX`. Revisões de uma fase posterior entram como nota no topo, sem reescrever a análise original.

## Índice

| # | Tema | Fase | Status |
|---|---|---|---|
| [0001](0001-escolha-da-nuvem.md) | Escolha da nuvem: por que continuar na AWS | 3 | Aceita, revisada em 2026-10 |
| [0002](0002-banco-gerenciado.md) | Banco de dados gerenciado: RDS PostgreSQL | 3 | Aceita, revisada em 2026-10 |
| [0003](0003-autenticacao-por-cpf.md) | Autenticação por CPF: Lambda própria, não Cognito | 3 | Aceita, revisada em 2026-10 |
| [0004](0004-decomposicao-em-microsservicos.md) | Decomposição do monolito em microsserviços (Strangler Fig) | 4 | Aceita |
