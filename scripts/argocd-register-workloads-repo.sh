#!/usr/bin/env bash
# Registra credencial do ArgoCD para https://github.com/gabehamasaki/hinfra-workloads.git
# Uso: HINFRA_WORKLOADS_TOKEN=ghp_... ./scripts/argocd-register-workloads-repo.sh
set -euo pipefail

REPO_URL="${HINFRA_WORKLOADS_REPO_URL:-https://github.com/gabehamasaki/hinfra-workloads.git}"
NS="${ARGOCD_NAMESPACE:-argocd}"
SECRET_NAME="${ARGOCD_WORKLOADS_SECRET:-workloads-repo-creds}"
USER="${HINFRA_WORKLOADS_GIT_USER:-git}"

if [[ -z "${KUBECONFIG:-}" ]]; then
  ROOT="$(cd "$(dirname "$0")/.." && pwd)"
  export KUBECONFIG="${ROOT}/.secrets/vps-1.kubeconfig"
fi

if [[ -z "${HINFRA_WORKLOADS_TOKEN:-}" ]]; then
  echo "Defina HINFRA_WORKLOADS_TOKEN (PAT fine-grained: Contents read no repo hinfra-workloads)." >&2
  exit 1
fi

kubectl create secret generic "$SECRET_NAME" -n "$NS" \
  --from-literal=type=git \
  --from-literal=url="$REPO_URL" \
  --from-literal=username="$USER" \
  --from-literal=password="$HINFRA_WORKLOADS_TOKEN" \
  --dry-run=client -o yaml | \
kubectl label --local -f - -o yaml \
  argocd.argoproj.io/secret-type=repository | \
kubectl apply -f -

kubectl annotate application workloads-root -n "$NS" \
  argocd.argoproj.io/refresh=hard --overwrite

echo "OK: secret $SECRET_NAME aplicado; workloads-root com refresh hard."
