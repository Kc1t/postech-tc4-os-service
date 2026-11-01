# ADR-0001: Go como linguagem do back-end

- **Status:** Aceita, revisada em 2026-10
- **Data:** 2026-04
- **Decisão por:** Reservoir Devs

> **Revisão na fase 4:** a decisão vale para os seis repositórios `postech-tc4-*`, todos em Go 1.26 (`go 1.26.10` no `go.mod`), inclusive as duas Lambdas.

## Contexto

O Tech Challenge pede um back-end monolítico para o MVP do sistema da oficina, com foco em qualidade, segurança e DDD. A escolha de linguagem é livre, mas impacta diretamente:

- Tempo de boot e footprint do container (relevante para `docker compose` em ambiente local de avaliação).
- Maturidade do ORM e do driver de Postgres.
- Curva de adoção pelo time (todos os integrantes têm experiência nas duas linguagens consideradas).
- Distribuição: binário único ou JVM com artefato.

A discussão se restringiu a duas opções viáveis dentro do que o time domina: **Go** e **Java**.

## Decisão

Adotar **Go** (1.25 na época, 1.26 hoje) como linguagem do back-end, com **Gin** como framework HTTP e **GORM** como ORM.

### Footprint e startup

Binário único compilado estaticamente, imagem final `alpine` com poucos MB e startup em centenas de milissegundos. No ambiente de avaliação (`docker compose up`), o avaliador tem o ambiente de pé em segundos, sem esperar a JVM aquecer.

### Concorrência nativa

Goroutines e channels são parte da linguagem, sem pool ou executor configurado por fora. Para um serviço com requisições HTTP concorrentes acessando o mesmo Postgres, isso reduz boilerplate e classes inteiras de bug de threading.

### Tipagem forte e simplicidade

Sistema de tipos suficiente para modelar Value Objects (CPF/CNPJ, placa) e estados da OS sem framework por trás. A linguagem é deliberadamente pequena: menos jeitos de fazer a mesma coisa, código mais homogêneo entre os quatro integrantes.

### Hexagonal natural

Interfaces implícitas (*structural typing*) casam com Ports & Adapters: o domínio define a interface, e qualquer struct que satisfaça os métodos vira adapter sem `implements` explícito. Reduz acoplamento sem framework de DI.

### Ecossistema suficiente

- `gin-gonic/gin` para HTTP.
- `gorm.io/gorm` com driver `pgx` para Postgres.
- `golang-jwt/jwt` para tokens.
- `swaggo/swag` para gerar Swagger a partir de anotações.
- `gosec`, `govulncheck` e `trivy` para análise de segurança.

### Familiaridade do time

Todos os integrantes já trabalharam com Go. A escolha não introduz curva de aprendizado dentro do prazo do TC1.

## Alternativas consideradas

1. **Java com Spring Boot.** Stack madura, ecossistema grande, ORM consolidado (Hibernate/JPA) e familiar ao mercado. Descartada pelo boot mais lento, pelo footprint de memória maior no container, pelo build mais pesado e pela verbosidade, que não agrega num MVP enxuto.

## Consequências

### Positivas

- Imagem pequena, deploy e CI rápidos.
- Concorrência de primeira classe sem configuração extra.
- Hexagonal expresso de forma idiomática por interfaces.
- Ferramental de segurança maduro (`gosec`, `govulncheck`).

### Negativas

- Ecossistema de bibliotecas menor que o de Java/Spring para casos muito específicos (integrações enterprise). Não impacta o escopo.
- Tratamento explícito de erros (`if err != nil`) gera verbosidade, mitigada por guard clauses e erros de domínio centralizados ([ADR-0006](./0006-domain-errors-sentinels.md)).
- Código mais antigo do ecossistema não usa generics; desde o Go 1.18 esse ponto perdeu relevância.

## Referências

- Go release notes: https://go.dev/doc/devel/release
- Effective Go: https://go.dev/doc/effective_go
