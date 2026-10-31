#!/usr/bin/env bash
# SonarQube Community num container do próprio job: os repositórios são privados, então o
# SonarCloud gratuito não serve. O script sobe o servidor, cria o projeto e um quality gate
# com condições sobre o código todo (não só o "new code", que na primeira análise é vazio),
# roda o scanner com a cobertura e os relatórios do golangci-lint e do gosec, e falha se o
# gate reprovar.
#
# O SCM fica desligado: o checkout pertence a outro uid e o git blame dentro do container
# trava ou é recusado ("dubious ownership"). O gate avalia o código todo, então o blame
# (que serve para separar o "new code") não faz falta.
#
# Uso: SONAR_PROJECT_KEY=postech-tc4-lambda-auth .github/scripts/sonar.sh
# Espera coverage.out, golangci-report.xml e gosec-report.json já gerados na raiz.
set -euo pipefail

PROJECT_KEY=${SONAR_PROJECT_KEY:?defina SONAR_PROJECT_KEY}

# Com -quiet, o gosec não grava o arquivo quando não acha nada; relatório vazio é válido.
[[ -f gosec-report.json ]] || echo '{"rules":[],"issues":[]}' > gosec-report.json
[[ -f coverage.out ]] || { echo "coverage.out não encontrado: rode os testes antes" >&2; exit 1; }
SONAR_IMAGE=${SONAR_IMAGE:-sonarqube:community}
SCANNER_IMAGE=${SCANNER_IMAGE:-sonarsource/sonar-scanner-cli:latest}
SONAR_PORT=${SONAR_PORT:-9000}
MIN_COVERAGE=${MIN_COVERAGE:-80}
MAX_DUPLICATION=${MAX_DUPLICATION:-3}
NETWORK=sonar-ci
SERVER=sonarqube-ci
API="http://localhost:${SONAR_PORT}/api"
GATE="tc4-gate"
PASSWORD="Ci-$(openssl rand -hex 12)-Aa1!"

cleanup() {
  if [[ "${KEEP_SONAR:-false}" != "true" ]]; then
    docker rm -f "$SERVER" >/dev/null 2>&1 || true
    docker network rm "$NETWORK" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

docker network create "$NETWORK" >/dev/null 2>&1 || true
docker rm -f "$SERVER" >/dev/null 2>&1 || true
docker run -d --name "$SERVER" --network "$NETWORK" -p "${SONAR_PORT}:9000" \
  -e SONAR_ES_BOOTSTRAP_CHECKS_DISABLE=true -e SONAR_TELEMETRY_ENABLE=false "$SONAR_IMAGE" >/dev/null

echo "Aguardando o SonarQube subir..."
for _ in $(seq 1 120); do
  status=$(curl -s "$API/system/status" | sed -n 's/.*"status":"\([A-Z_]*\)".*/\1/p' || true)
  [[ "$status" == "UP" ]] && break
  sleep 3
done
[[ "${status:-}" == "UP" ]] || { echo "SonarQube não subiu"; docker logs --tail 50 "$SERVER"; exit 1; }

admin() { curl -sf -u "admin:${PASSWORD}" -X POST "$API/$1" "${@:2}"; }

# A imagem nova pede troca da senha padrão antes de liberar a API.
curl -sf -u admin:admin -X POST "$API/users/change_password" \
  --data-urlencode login=admin --data-urlencode previousPassword=admin \
  --data-urlencode "password=${PASSWORD}" >/dev/null

TOKEN=$(admin user_tokens/generate --data-urlencode name=ci | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
[[ -n "$TOKEN" ]] || { echo "falha ao gerar o token"; exit 1; }

admin projects/create --data-urlencode "project=${PROJECT_KEY}" --data-urlencode "name=${PROJECT_KEY}" >/dev/null
admin qualitygates/create --data-urlencode "name=${GATE}" >/dev/null
condition() {
  admin qualitygates/create_condition --data-urlencode "gateName=${GATE}" \
    --data-urlencode "metric=$1" --data-urlencode "op=$2" --data-urlencode "error=$3" >/dev/null
}
condition coverage LT "$MIN_COVERAGE"
condition duplicated_lines_density GT "$MAX_DUPLICATION"
condition reliability_rating GT 1  # nota A em bugs
condition security_rating GT 1     # nota A em vulnerabilidades
condition sqale_rating GT 1        # nota A em manutenibilidade
admin qualitygates/select --data-urlencode "gateName=${GATE}" --data-urlencode "projectKey=${PROJECT_KEY}" >/dev/null

set +e
docker run --rm --network "$NETWORK" \
  -e SONAR_HOST_URL="http://${SERVER}:9000" -e SONAR_TOKEN="$TOKEN" \
  -v "$PWD:/usr/src" "$SCANNER_IMAGE" \
  -Dsonar.projectKey="$PROJECT_KEY" \
  -Dsonar.working.directory=/tmp/scannerwork \
  -Dsonar.scm.disabled=true \
  -Dsonar.qualitygate.wait=true -Dsonar.qualitygate.timeout=300
scan_status=$?
set -e

measures=$(curl -s -u "admin:${PASSWORD}" "$API/measures/component?component=${PROJECT_KEY}&metricKeys=coverage,bugs,vulnerabilities,code_smells,security_hotspots,duplicated_lines_density,ncloc")
gate=$(curl -s -u "admin:${PASSWORD}" "$API/qualitygates/project_status?projectKey=${PROJECT_KEY}" | sed -n 's/.*"projectStatus":{"status":"\([A-Z]*\)".*/\1/p')

summary() {
  echo "### SonarQube: ${PROJECT_KEY}"
  echo
  echo "Quality gate **${gate:-desconhecido}** (cobertura ≥ ${MIN_COVERAGE}%, duplicação ≤ ${MAX_DUPLICATION}%, notas A em confiabilidade, segurança e manutenibilidade)"
  echo
  echo "| Métrica | Valor |"
  echo "|---|---|"
  echo "$measures" | grep -o '"metric":"[a-z_]*","value":"[^"]*"' | sed 's/"metric":"\([a-z_]*\)","value":"\([^"]*\)"/| \1 | \2 |/'
}
summary
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then summary >> "$GITHUB_STEP_SUMMARY"; fi

exit "$scan_status"
