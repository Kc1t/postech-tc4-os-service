# Manifestos Kubernetes do os-service

Deploy do os-service no EKS, namespace `postech` ([ADR-0021](../docs/adr/0021-ambiente-unico-no-eks.md)). O namespace, a `ResourceQuota` e o `LimitRange` são da plataforma e ficam em [`postech-tc4-infra-k8s/k8s/namespace.yaml`](https://github.com/Kc1t/postech-tc4-infra-k8s/blob/main/k8s/namespace.yaml), porque a cota vale para os três serviços somados.

Quem aplica é o job `deploy` do [CI](../.github/workflows/ci.yml), na `main`. Os manifests não declaram namespace e trazem marcadores (`*_PLACEHOLDER`) que o job substitui; o job falha se sobrar algum. Antes disso, o job `manifests` valida todos os arquivos desta pasta com o kubeconform em modo strict, contra o schema do Kubernetes 1.35 (a versão do EKS), com valores de exemplo nos marcadores; o deploy depende dele.

## Manifestos

| Arquivo | Recurso | O que define |
|---|---|---|
| `configmap.yaml` | `ConfigMap os-service-config` | Configuração não sensível: porta, nível de log, nome da app no New Relic, política de login, CORS, URLs das filas `os-saga-replies`, `billing-commands` e `execution-commands` e o ARN do tópico `os-events`, intervalo do relay do outbox, prazos da saga (`SAGA_APPROVAL_TIMEOUT`, `SAGA_PAYMENT_TIMEOUT`), URLs internas do billing e do execution para a API composition, timeouts e a chave `FAULT_INJECTION_ENABLED` do header `X-Simulate-Failure`. |
| `secret.yaml` | `Secret os-service-secret` | Só documenta as chaves (`JWT_SECRET`, `POSTGRES_DSN`, `ADMIN_PASSWORD`, `INTERNAL_API_TOKEN`, `NEW_RELIC_LICENSE_KEY`). **Não é aplicado**: o CI cria o Secret a partir dos GitHub Secrets e lê a DSN do Secrets Manager, onde o Terraform do `infra/` a gravou. |
| `deployment.yaml` | `Deployment os-service` | 2 réplicas, rolling update com `maxSurge: 1` e `maxUnavailable: 0`, usuário não root e sistema de arquivos só leitura, requests de 100m/128Mi e limits de 500m/384Mi, `preStop` de 5 s para o pod sair dos endpoints antes do SIGTERM. |
| `service.yaml` | `Service os-service` (ClusterIP) e `os-service-public` (NLB) | O ClusterIP fica para acesso de dentro do cluster: hoje nenhum outro serviço chama o os-service, porque a API composition parte dele. O NLB é o destino do API Gateway, que o encontra pela tag `kubernetes.io/service-name=postech/os-service-public`, e da Lambda issuer, que chama `GET /internal/requesters/{doc}` com `X-Internal-Token`. |
| `hpa.yaml` | `HorizontalPodAutoscaler` | De 2 a 6 réplicas, com alvo de 70% de CPU e 80% de memória. |
| `pdb.yaml` | `PodDisruptionBudget` | Mantém pelo menos 1 pod durante drain e atualização de nó. |
| `networkpolicy.yaml` | `NetworkPolicy os-service` | Libera a entrada só na porta 8080/TCP e nega o resto. |

## Probes

| Probe | Caminho | Falha significa |
|---|---|---|
| startup | `/health/live` (até 60 s) | O processo ainda está subindo; as outras probes esperam. |
| readiness | `/health/ready` (ping no Postgres, 2 s) | O banco está fora: o pod sai do Service, mas não reinicia. |
| liveness | `/health/live` | O processo travou: o kubelet reinicia o pod. |

Separar as duas impede que uma queda do banco faça o kubelet reiniciar todos os pods em cascata: eles só saem de rotação até o banco voltar.

## Política de rede

A porta 8080 precisa aceitar origens de fora do namespace, porque quem chega nela não é um pod: o NLB, em modo `instance`, entra com o IP do nó, e as probes partem do kubelet. Restringir por `podSelector` derrubaria o serviço. O efeito da política é fechar todas as outras portas.

> No EKS, a `NetworkPolicy` só filtra com o addon `vpc-cni` configurado com `enableNetworkPolicy = "true"` ([`addons.tf`](https://github.com/Kc1t/postech-tc4-infra-k8s/blob/main/addons.tf)). Sem isso, o objeto é aceito e não faz nada.

## Aplicar à mão

O fluxo abaixo replica o que o CI faz, com valores de exemplo.

```bash
NAMESPACE=postech
IMAGE=<conta>.dkr.ecr.us-east-1.amazonaws.com/os-service:<sha>

kubectl create secret generic os-service-secret -n "$NAMESPACE" \
  --from-literal=JWT_SECRET=... --from-literal=POSTGRES_DSN=... \
  --from-literal=ADMIN_PASSWORD=... --from-literal=INTERNAL_API_TOKEN=... \
  --from-literal=NEW_RELIC_LICENSE_KEY=... \
  --dry-run=client -o yaml | kubectl apply -f -

mkdir -p rendered && cp configmap.yaml deployment.yaml service.yaml hpa.yaml pdb.yaml networkpolicy.yaml rendered/
sed -i "s|IMAGE_PLACEHOLDER|$IMAGE|g; s|NAMESPACE_PLACEHOLDER|$NAMESPACE|g; ..." rendered/*.yaml
kubectl apply -n "$NAMESPACE" -f rendered/
kubectl rollout status deployment/os-service -n "$NAMESPACE"
kubectl get svc os-service-public -n "$NAMESPACE"   # EXTERNAL-IP = DNS do NLB
```

## Ver a escala

```bash
kubectl get hpa os-service -n postech -w
kubectl get pods -l app=os-service -n postech -w
```

O HPA depende do addon `metrics-server`, instalado pelo infra-k8s. Sem ele, os alvos ficam em `<unknown>` e nada escala.
