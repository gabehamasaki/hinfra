# 13 — Observabilidade

Alertas 24/7 e dashboards na **camada de plataforma**, visíveis no **ArgoCD** (`platform-root` → Applications `monitoring` e `monitoring-extras`). Não fica no repo `hinfra-workloads`.

## O que roda no cluster

| Componente | Namespace | Função |
| --- | --- | --- |
| kube-prometheus-stack | `monitoring` | Prometheus, Alertmanager, Grafana, exporters |
| alertmanager-discord | `monitoring` | Converte webhooks do Alertmanager para Discord |
| Argo CD notifications | `argocd` | Degraded / sync failed / Unknown → Discord |

**Grafana:** `https://grafana.hamasakis.cloud` — só tailnet (middleware Traefik, mesmo modelo do ArgoCD).

**ArgoCD** continua em `https://argocd.hamasakis.cloud`.

## Pré-requisitos no vault

Em `ansible/group_vars/all/vault.yml` (ver `vault.yml.example`):

- `vault_alertmanager_discord_webhook_url` — obrigatório (Alertmanager + ArgoCD + falha de backup)
- `vault_grafana_admin_password` — obrigatório para o Grafana
- `vault_alertmanager_critical_webhook_url` — opcional (rota `severity=critical` no Alertmanager)
- `vault_k3s_backup_healthcheck_url` — opcional (ping após backup diário bem-sucedido)

DNS: registro `grafana` na zona `hamasakis.cloud` apontando para o IP Tailscale do node (igual `argocd`).

## Ver no ArgoCD

No console (`argocd.hamasakis.cloud`), filtre por label **`hinfra.layer=platform`** ou abra o App of Apps **`platform-root`**. Filhos típicos:

| Application | O que sincroniza |
| --- | --- |
| `monitoring` | Kustomize em `platform/observability/` (Helm chart + middleware, Discord, PodMonitor) |

Cert-manager, Sealed Secrets, KEDA e o próprio ArgoCD **continuam só no Ansible** (dependências do GitOps) — não aparecem como Applications.

## Instalar ou atualizar

1. **Secrets** (uma vez ou ao rotacionar): role `observability_secrets` no playbook Ansible.
2. **Manifestos e Helm values**: commit no repo `hinfra` → o ArgoCD sincroniza (~3 min).

```bash
cd ansible
script -qec "ansible-playbook -i inventory/hosts.ini site.yml \
  --private-key ../.secrets/vps-1_deploy_ed25519" /dev/null
```

Alterar thresholds ou recursos: edite [`platform/observability/values.yaml`](../platform/observability/values.yaml) e deixe o Application `monitoring` sincronizar.

### Migração do Ansible Helm (instalação antiga)

Se o stack foi instalado pelo role `observability` (Helm via Ansible), antes do primeiro sync do ArgoCD:

```bash
helm uninstall kube-prometheus-stack -n monitoring   # na VPS, com KUBECONFIG
```

Depois commit + push do `platform-root` e sync. O ArgoCD recria o release com o mesmo nome.

## Canais de alerta

| Origem | Exemplo |
| --- | --- |
| Prometheus / Alertmanager | Node com disco cheio, pod em CrashLoop, certificado expirando |
| Argo CD | Application Degraded, sync Failed, status Unknown |
| Backup k3s (systemd) | Falha no script; sucesso opcional via healthchecks.io |
| Externo (recomendado) | Uptime em URLs públicas `*.hamasakis.dev` — não substitui alertas in-cluster |

Prometheus usa scrape de **60s** e retention de **7 dias** para caber em 2 vCPU.

## Testar alertas

**Alertmanager (Discord):**

```bash
kubectl port-forward -n monitoring svc/kube-prometheus-stack-alertmanager 9093:9093
# Em outro terminal, dispare um alerta de teste (amtool ou UI em :9093)
```

**Argo CD:** force um Application Unhealthy ou use o trigger de teste documentado no [upstream](https://argo-cd.readthedocs.io/en/stable/operator-manual/notifications/).

**Backup:**

```bash
ssh vps 'sudo /usr/local/bin/k3s-backup.sh'   # deve pingar healthcheck se configurado
```

## Silenciar ruído

No Grafana (Alerting) ou via `kubectl edit alertmanager` no namespace `monitoring` — prefira ajustar thresholds em `platform/observability/values.yaml` e sync do Application `monitoring` em vez de editar à mão (o ArgoCD reconcilia).

## Worker novo

O `node-exporter` é DaemonSet: ao adicionar um agent k3s, confira:

```bash
kubectl get pods -n monitoring -l app.kubernetes.io/name=prometheus-node-exporter -o wide
```

## Verificação pós-deploy

```bash
kubectl get pods -n monitoring
kubectl get prometheusrule -n monitoring
kubectl top pods -n monitoring
curl -sk -o /dev/null -w "%{http_code}\n" https://grafana.hamasakis.cloud  # na tailnet, 200 ou 302
```

## Plano B (CPU apertada)

Se o node passar a recusar pods por requests de CPU: aumentar `scrapeInterval` / reduzir `retention` em `values.yaml.j2`, ou migrar para Grafana Alloy + Grafana Cloud (remote_write) mantendo só exporters no cluster — ver comentário no plano de arquitetura.
