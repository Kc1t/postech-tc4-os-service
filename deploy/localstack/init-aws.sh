#!/bin/bash
# Cria no LocalStack a mesma topologia de mensageria que o Terraform cria na AWS.
# Executado automaticamente pelo LocalStack (montado em /etc/localstack/init/ready.d).
#
#   Tópicos SNS (eventos, pub/sub)     Filas SQS (consumidores)            Assinaturas (raw delivery)
#   os-events                          os-saga-replies (+dlq)              billing-events   -> os-saga-replies
#   billing-events                     billing-commands (+dlq)             execution-events -> os-saga-replies
#   execution-events                   execution-commands (+dlq)           os-events        -> notification-os-events
#                                      notification-os-events (+dlq)
set -euo pipefail

REGION="${AWS_DEFAULT_REGION:-us-east-1}"
ACCOUNT="000000000000"
MAX_RECEIVE="${MAX_RECEIVE_COUNT:-5}"

queue() {
  local name="$1"
  awslocal sqs create-queue --queue-name "${name}-dlq" >/dev/null
  local dlq_arn="arn:aws:sqs:${REGION}:${ACCOUNT}:${name}-dlq"
  awslocal sqs create-queue --queue-name "$name" --attributes "{
    \"VisibilityTimeout\": \"30\",
    \"ReceiveMessageWaitTimeSeconds\": \"20\",
    \"RedrivePolicy\": \"{\\\"deadLetterTargetArn\\\":\\\"${dlq_arn}\\\",\\\"maxReceiveCount\\\":\\\"${MAX_RECEIVE}\\\"}\"
  }" >/dev/null
  echo "fila: ${name} (+dlq)"
}

topic() {
  awslocal sns create-topic --name "$1" >/dev/null
  echo "tópico: $1"
}

subscribe() {
  local topic="$1" queue="$2"
  local sub_arn
  sub_arn=$(awslocal sns subscribe \
    --topic-arn "arn:aws:sns:${REGION}:${ACCOUNT}:${topic}" \
    --protocol sqs \
    --notification-endpoint "arn:aws:sqs:${REGION}:${ACCOUNT}:${queue}" \
    --attributes RawMessageDelivery=true \
    --query SubscriptionArn --output text)
  echo "assinatura: ${topic} -> ${queue} (${sub_arn##*:})"
}

for t in os-events billing-events execution-events; do topic "$t"; done
for q in os-saga-replies billing-commands execution-commands notification-os-events; do queue "$q"; done
subscribe billing-events os-saga-replies
subscribe execution-events os-saga-replies
subscribe os-events notification-os-events

# Tabela do execution-service (no EKS ela vem do Terraform do próprio serviço).
awslocal dynamodb create-table \
  --table-name executions \
  --attribute-definitions AttributeName=pk,AttributeType=S AttributeName=sk,AttributeType=S \
    AttributeName=gsi1pk,AttributeType=S AttributeName=gsi1sk,AttributeType=S \
  --key-schema AttributeName=pk,KeyType=HASH AttributeName=sk,KeyType=RANGE \
  --global-secondary-indexes '[{"IndexName":"gsi1","KeySchema":[{"AttributeName":"gsi1pk","KeyType":"HASH"},{"AttributeName":"gsi1sk","KeyType":"RANGE"}],"Projection":{"ProjectionType":"ALL"}}]' \
  --billing-mode PAY_PER_REQUEST >/dev/null
echo "tabela dynamodb: executions"

echo "localstack pronto"
