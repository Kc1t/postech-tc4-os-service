.PHONY: help run build up down logs tidy swagger mocks test cover lint test-db

APP_NAME=os-service
MAIN=./cmd/api
COVER_PKGS=$$(go list ./... | grep -v -E '/cmd/|/docs|/mocks|/scripts')
TEST_DB_ENV=TEST_DB_HOST=localhost TEST_DB_PORT=55432 TEST_DB_USER=test TEST_DB_PASSWORD=test TEST_DB_NAME=testdb

help:
	@echo "run | build | up | down | logs | swagger | mocks | test-db | test | cover | lint | tidy"

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

mocks:
	cd internal/ports && for f in usecases repositories transactions clients security; do \
		mockgen -source=./$$f.go -destination=./mocks/$$f.go -package=mocks; done

# Postgres descartável para os testes de repositório e do orquestrador.
test-db:
	docker run -d --rm --name tc4-pg-test -p 55432:5432 -e POSTGRES_USER=test \
		-e POSTGRES_PASSWORD=test -e POSTGRES_DB=testdb postgres:16-alpine

test:
	$(TEST_DB_ENV) go test ./... -count=1

cover:
	$(TEST_DB_ENV) go test -count=1 -coverprofile=coverage.out $(COVER_PKGS)
	go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run ./...

tidy:
	go mod tidy
