# ── Build ─────────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.3 && swag init -g cmd/api/main.go -o docs --parseInternal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/os-service ./cmd/api

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata \
 && addgroup -S app -g 10001 \
 && adduser -S app -u 10001 -G app
ENV TZ=America/Sao_Paulo
WORKDIR /app
COPY --from=builder --chown=app:app /out/os-service /app/os-service

# Usuário sem privilégios (o Deployment também força runAsNonRoot).
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health/live >/dev/null || exit 1
ENTRYPOINT ["/app/os-service"]
