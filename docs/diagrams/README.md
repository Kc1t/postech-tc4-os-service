# Diagramas

Diagramas da fase 4. Quase todos são Mermaid embutido em Markdown: renderizam direto no GitHub, ficam versionados com o código e aparecem no diff quando a arquitetura muda. As versões da fase 3 (monolito) ficam no repositório [`postech-tc3-app`](https://github.com/Kc1t/postech-tc3-app/tree/main/docs/diagrams).

## Visão geral

| Documento | O que responde |
|---|---|
| [`arquitetura-fase4.svg`](./arquitetura-fase4.svg) | Diagrama geral do documento de entrega: microsserviços, bancos e comunicação |
| [`componentes.md`](./componentes.md) | Como as peças se encaixam na AWS, quem fala com quem e o que cada repositório provisiona |
| [`roteamento.md`](./roteamento.md) | Por onde uma requisição passa, quem a barra e onde nasce cada código de resposta |
| [`pipelines.md`](./pipelines.md) | O que cada um dos seis pipelines faz, a ordem do primeiro deploy e os secrets de cada repositório |

## Fluxos

| Documento | O que responde |
|---|---|
| [`sequencia-ordem-servico.md`](./sequencia-ordem-servico.md) | A saga da OS da abertura à entrega, uma compensação com estorno e as máquinas de estado da OS e da saga |
| [`sequencia-autenticacao.md`](./sequencia-autenticacao.md) | Como o CPF vira JWT e como o token é aceito numa rota protegida |

A arquitetura interna do os-service (hexagonal) está no [README](../../README.md#arquitetura-do-serviço), e as camadas de autenticação em [`../SEGURANCA.md`](../SEGURANCA.md).

## Dados

| Documento | O que responde |
|---|---|
| [`../MODELAGEM_DE_DADOS.md`](../MODELAGEM_DE_DADOS.md) | Diagrama ER do banco do os-service, justificativa e relacionamentos |
| [`../database-model.dbml`](../database-model.dbml) | Modelo em DBML das tabelas de cadastro e da OS, para <https://dbdiagram.io/d>; as tabelas da saga, do outbox e do inbox estão só na modelagem |
| [`../documentation-diagram.drawio`](../documentation-diagram.drawio) | Diagrama das fases 1 e 2, mantido como referência histórica |

## Por onde começar

`arquitetura-fase4.svg` para a visão geral, `sequencia-ordem-servico.md` para a saga e `roteamento.md` para as camadas de proteção.
