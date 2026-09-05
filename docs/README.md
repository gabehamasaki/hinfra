# Documentação da infra

Documentação completa do cluster que roda em `hamasakis.cloud` (plataforma) e `hamasakis.dev` (projetos).

Para um resumo visual de tudo, abra [`overview.html`](overview.html) no navegador.

## Índice

| Documento | O que cobre |
| --- | --- |
| [01 - Arquitetura](01-arquitetura.md) | Visão geral, as quatro camadas, e o porquê de cada decisão estrutural |
| [02 - Servidor e acesso](02-servidor-e-acesso.md) | VPS, usuário `deploy`, SSH, firewall, Tailscale, DNS |
| [03 - Kubernetes](03-kubernetes.md) | k3s, storage, backup do datastore, adicionar workers |
| [04 - Ansible](04-ansible.md) | Provisionamento, roles, inventário, como rodar |
| [05 - Segredos](05-segredos.md) | ansible-vault, Sealed Secrets, inventário de credenciais |
| [06 - TLS e Ingress](06-tls-e-ingress.md) | cert-manager, Let's Encrypt DNS-01, Traefik, restrição por tailnet |
| [07 - ArgoCD e GitOps](07-argocd-gitops.md) | App of Apps, divisão Ansible × ArgoCD, acesso |
| [08 - Serviços de dados](08-data-services.md) | Postgres, Valkey, RustFS, KEDA |
| [09 - CI/CD](09-cicd.md) | GitHub Actions, GHCR, onboarding de projeto novo |
| [10 - Runbooks](10-runbooks.md) | Operações do dia a dia, passo a passo |
| [11 - Armadilhas](11-armadilhas.md) | Problemas reais já enfrentados e como foram resolvidos |
| [12 - MCP](12-mcp.md) | Servidor MCP local para deploy, diagnóstico e scaffold a partir de qualquer repo de projeto |

## Referência rápida

```bash
ssh vps                                            # usuário deploy, root desabilitado
export KUBECONFIG=.secrets/vps-1.kubeconfig        # exige estar na tailnet
kubectl get applications -n argocd                 # estado do GitOps
```

| Endereço | Alcance | O que é |
| --- | --- | --- |
| `https://hamasakis.dev` | Público | Portfólio |
| `https://argocd.hamasakis.cloud` | Só tailnet | Console do ArgoCD |
| `https://s3.hamasakis.cloud` | Só tailnet | Console do RustFS |
| `postgres-rw.postgres.svc.cluster.local:5432` | Só cluster | Postgres (escrita) |
| `valkey.valkey.svc.cluster.local:6379` | Só cluster | Valkey |
| `rustfs-svc.rustfs.svc.cluster.local:9000` | Só cluster | API S3 |
