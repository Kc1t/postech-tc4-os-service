#!/usr/bin/env bash
# Teste de fumaça da saga só com o os-service: os serviços billing e execution são simulados
# publicando os eventos deles nos tópicos SNS do LocalStack. Útil para validar o orquestrador
# isoladamente (o fluxo completo com os três serviços fica no e2e do repositório de infra).
#
#   docker compose -p tc4-os up -d && ./scripts/smoke-saga.sh
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
LS="${LOCALSTACK_CONTAINER:-os-localstack}"
ACC=000000000000
REGION=us-east-1
aws() { docker exec "$LS" awslocal "$@"; }
step() { printf '\n\033[1;34m▶ %s\033[0m\n' "$*"; }

purge() { for q in execution-commands billing-commands notification-os-events; do
  aws sqs purge-queue --queue-url "http://localhost:4566/$ACC/$q" >/dev/null 2>&1 || true; done; }

# Lê mensagens da fila e devolve os tipos, consumindo-as.
types_in() {
  local q="http://localhost:4566/$ACC/$1" out=""
  for _ in 1 2 3 4 5 6; do
    local batch
    batch=$(aws sqs receive-message --queue-url "$q" --max-number-of-messages 10 --wait-time-seconds 1 \
      --query 'Messages[].[ReceiptHandle,Body]' --output json)
    [ "$batch" = "null" ] && { [ -n "$out" ] && break || continue; }
    out+=$(echo "$batch" | jq -r '.[][1] | fromjson | .type')$'\n'
    echo "$batch" | jq -r '.[][0]' | while read -r rh; do
      aws sqs delete-message --queue-url "$q" --receipt-handle "$rh"; done
  done
  echo "$out" | sed '/^$/d'
}

publish() { # tópico tipo payload
  local env
  env=$(jq -nc --arg id "$(uuidgen | tr 'A-Z' 'a-z')" --arg t "$2" --arg saga "$SAGA" --arg os "$OS" \
    --argjson p "$3" '{message_id:$id,type:$t,version:1,saga_id:$saga,os_id:$os,producer:"smoke",occurred_at:(now|todate),payload:$p}')
  aws sns publish --topic-arn "arn:aws:sns:$REGION:$ACC:$1" --message "$env" >/dev/null
}

status() { curl -sf "$API/service-orders/$OS" -H "Authorization: Bearer $TOKEN" | jq -r .status; }
wait_status() {
  for _ in $(seq 1 30); do [ "$(status)" = "$1" ] && { echo "  OS: $1"; return; }; sleep 0.5; done
  echo "  ✗ esperava $1, está $(status)"; exit 1
}

purge
step "login e cadastro"
TOKEN=$(curl -sf "$API/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"admin@workshop.com","password":"admin123"}' | jq -r .access_token)
SUF=$RANDOM
H=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')
curl -s "$API/requesters" "${H[@]}" -d '{"name":"Ana Smoke","document":"52998224725","email":"ana@example.com"}' >/dev/null || true
PLATE="SMK$(printf '%04d' $((SUF % 10000)))"
curl -sf "$API/vehicles" "${H[@]}" -d "{\"requester_document\":\"52998224725\",\"plate\":\"$PLATE\",\"brand\":\"Fiat\",\"model\":\"Argo\",\"year\":2022}" >/dev/null
curl -sf "$API/services" "${H[@]}" -d "{\"code\":$SUF,\"name\":\"Troca de pastilhas\",\"price\":150,\"duration_min\":60}" >/dev/null
curl -sf "$API/parts" "${H[@]}" -d "{\"manufacturer_code\":\"PAS-$SUF\",\"name\":\"Pastilha\",\"unit\":\"par\",\"price\":90,\"stock\":5}" >/dev/null

step "abre a OS → StartDiagnosis na fila do execution-service"
OS=$(curl -sf "$API/service-orders" "${H[@]}" -d "{\"requester_document\":\"52998224725\",\"vehicle_plate\":\"$PLATE\",\"notes\":\"barulho\"}" | jq -r .id)
SAGA=$(curl -sf "$API/service-orders/$OS/details" -H "Authorization: Bearer $TOKEN" | jq -r .saga.id)
echo "  OS=$OS saga=$SAGA"
types_in execution-commands | sed 's/^/  fila execution-commands: /'

step "execution-service: diagnóstico iniciado e concluído"
publish execution-events execution.DiagnosisStarted '{"started_at":"2026-11-03T12:00:00Z"}'
publish execution-events execution.DiagnosisCompleted "{\"services\":[{\"code\":$SUF}],\"parts\":[{\"manufacturer_code\":\"PAS-$SUF\",\"quantity\":2}]}"
wait_status awaiting_budget
types_in billing-commands | sed 's/^/  fila billing-commands: /'

step "billing-service: orçamento criado e aprovado, link de pagamento, pagamento aprovado"
publish billing-events billing.BudgetCreated '{"budget_id":"b-smoke","total":330}'
wait_status awaiting_approval
publish billing-events billing.BudgetApproved '{"budget_id":"b-smoke","approved_at":"2026-11-03T12:10:00Z"}'
wait_status awaiting_payment
publish billing-events billing.PaymentLinkCreated '{"budget_id":"b-smoke","payment_id":"p-smoke","checkout_url":"https://example.test/checkout"}'
publish billing-events billing.PaymentApproved '{"budget_id":"b-smoke","payment_id":"p-smoke","provider_payment_id":"1","amount":330}'
wait_status in_execution
types_in billing-commands | sed 's/^/  fila billing-commands: /'
types_in execution-commands | sed 's/^/  fila execution-commands: /'

step "execution-service: reparo iniciado e finalizado"
publish execution-events execution.RepairStarted '{"started_at":"2026-11-03T13:00:00Z"}'
publish execution-events execution.RepairFinished '{"finished_at":"2026-11-03T15:00:00Z"}'
wait_status finished

step "entrega"
curl -sf -X POST "$API/service-orders/$OS/deliver" -H "Authorization: Bearer $TOKEN"
wait_status delivered

step "eventos para a notificação (fila notification-os-events)"
types_in notification-os-events | sort | uniq -c | sed 's/^/  /'

step "histórico"
curl -sf "$API/service-orders/$OS/history" -H "Authorization: Bearer $TOKEN" | jq -r '.[] | "  \(.from // "-") → \(.to) (\(.actor))"'
echo; echo "✔ saga completa"
