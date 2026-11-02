# ADR-0002: Arquitetura hexagonal (Ports & Adapters)

- **Status:** Aceita, revisada em 2026-10
- **Data:** 2026-04
- **Decisão por:** Reservoir Devs

> **Revisão na fase 4:** os três microsserviços seguem a mesma estrutura. No os-service entraram adapters de entrada para a fila (`inbound/messaging`) e para o worker de prazos (`inbound/scheduler`), e um adapter de saída para os outros serviços (`outbound/services`); o orquestrador da saga é um caso de uso (`application/usecase/saga`). O código comum aos serviços (contratos, mensageria, outbox, inbox, cliente HTTP) fica em `pkg/`.

## Contexto

O Tech Challenge da Fase 1 da FIAP SOAT permite, explicitamente, a adoção de **arquitetura em camadas tradicional** (controller → service → repository) por se tratar de um MVP em monolito. Apesar disso, identificamos os seguintes desafios:

- O domínio (oficina mecânica) tem regras de negócio claras: máquina de estados da Ordem de Serviço, cálculo de orçamento, controle de estoque com transações, validação de CPF/CNPJ e placa.
- A escolha de framework HTTP (Gin), ORM (GORM) e biblioteca de JWT são detalhes de infraestrutura que podem mudar.
- Queremos cobertura de testes mínima de 80% nos domínios críticos — o que exige isolamento do domínio para permitir mocks de portas externas.
- O grupo prevê evolução do sistema em fases futuras (talvez microserviços, talvez troca de banco). Quanto menos a regra de negócio depender de tecnologia, mais barata é a evolução.

## Decisão

Adotar **arquitetura hexagonal (Ports & Adapters)** dentro do monolito (na fase 4, dentro de cada microsserviço). O domínio fica no centro e expõe interfaces (ports). Frameworks e infraestrutura são adapters que dependem do domínio, nunca o contrário.

### Camadas

```
internal/
  domain/                  → núcleo (entidades + erros). Zero dependências externas.
  ports/                   → interfaces puras (repositories, usecases, security).
  application/usecase/     → lógica de negócio (depende só de ports + entities).
  adapters/
    inbound/http/          → adapters Gin (driving adapters).
    outbound/postgresql/   → adapters GORM (driven adapters).
    outbound/jwt/          → adapter de TokenService.
```

### Regra de dependência

Dependências apontam **sempre para dentro**. O único componente que conhece todas as camadas é o container de DI manual em `cmd/api/bootstrap/container.go`.

## Alternativas consideradas

1. **Camadas tradicionais (controller → service → repository).** Mais simples. Descartada porque o `service` acabaria importando GORM e Gin diretamente, o que dificulta testes e troca de tecnologia.
2. **Clean Architecture.** Muito parecida com a hexagonal, com terminologia mais carregada (entities, use cases, interface adapters, frameworks). Descartada por ser menos explícita sobre o que é porta e o que é adapter.
3. **Vertical Slice Architecture.** Boa para features muito independentes. Descartada porque exigiria mais boilerplate e é incomum em Go.

## Consequências

### Positivas

- Domínio testável sem subir banco nem servidor HTTP, com mocks das portas gerados pelo `mockgen` (gomock, `make mocks`).
- Substituição de frameworks localizada: trocar Gin por gRPC ou GORM por `sqlc` mexe só nos adapters.
- Separação clara: handler é "burro", use case é regra de negócio, adapter é tradução.

### Negativas

- Boilerplate maior comparado a camadas tradicionais (interfaces explícitas em `ports/`, adapters de tradução `FromDomain`/`ToDomain`).
- Curva de aprendizado para quem nunca trabalhou com hexagonal — é necessário disciplina para não vazar dependências.
- Risco de "anemia" no domínio se os use cases acabarem fazendo todo o trabalho — mitigado pela regra de comportamentos no agregado (ex.: `ServiceOrder.UpdateStatus`).

## Referências

- Alistair Cockburn, *Hexagonal Architecture* (2005).
- Vaughn Vernon, *Implementing Domain-Driven Design* (2013).
