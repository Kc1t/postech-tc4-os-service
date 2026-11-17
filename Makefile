.PHONY: help run build up down logs tidy swagger asyncapi mocks test cover lint test-db bdd

APP_NAME=os-service
MAIN=./cmd/api
COVER_PKGS=$$(go list ./... | grep -v -E '/cmd/|/docs|/mocks|/scripts')
TEST_DB_ENV=TEST_DB_HOST=localhost TEST_DB_PORT=55432 TEST_DB_USER=test TEST_DB_PASSWORD=test TEST_DB_NAME=testdb

help:
	@echo "run | build | up | down | logs | swagger | asyncapi | mocks | test-db | test | cover | bdd | lint | tidy"

run:
	go run $(MAIN)

build:
	go build -o $(APP_NAME) $(MAIN)

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f os-service

swagger:
	swag init -g $(MAIN)/main.go -o docs --parseInternal

# Valida docs/asyncapi/asyncapi.yaml (e os schemas de pkg/contracts que ele referencia) com o CLI
# oficial do AsyncAPI, em contêiner e sem rede.
asyncapi:
	docker run --rm --network none -v "$(CURDIR)":/work -w /work asyncapi/cli:6.2.2 validate docs/asyncapi/asyncapi.yaml

mocks:
	cd internal/ports && for f in usecases repositories transactions clients security; do \
		mockgen -source=./$$f.go -destination=./mocks/$$f.go -package=mocks; done

# Postgres descartável para os testes de repositório e do orquestrador.
test-db:
	docker run -d --rm --name tc4-pg-test -p 55432:5432 -e POSTGRES_USER=test \
		-e POSTGRES_PASSWORD=test -e POSTGRES_DB=testdb postgres:16-alpine

# -p 1: os pacotes de repositório, outbox e inbox dividem o mesmo Postgres de teste e contam
# linhas das mesmas tabelas; em paralelo um vê o que o outro gravou.
test:
	$(TEST_DB_ENV) go test -p 1 ./... -count=1

cover:
	$(TEST_DB_ENV) go test -p 1 -count=1 -coverprofile=coverage.out $(COVER_PKGS)
	go tool cover -func=coverage.out | tail -1

# BDD da saga (godog): features/saga.feature contra o orquestrador real e o Postgres de teste.
bdd:
	$(TEST_DB_ENV) go test -tags bdd -count=1 -v -run TestFeatures ./internal/application/usecase/saga/

lint:
	golangci-lint run ./...

tidy:
	go mod tidy
