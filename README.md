# hinfra

Kit open-source de **provisionamento** (Ansible) + **GitOps** (ArgoCD) para uma VPS com k3s, Tailscale e Traefik.

Documentação completa em [`docs/`](docs/) (fonte de verdade). Resumo visual: [`docs/overview.html`](docs/overview.html).

## Dois repositórios

| Repositório | Visibilidade | O que versiona |
| --- | --- | --- |
| **[hinfra](https://github.com/gabehamasaki/hinfra)** (este) | Público | Ansible, plataforma, exemplos, CLI, templates ArgoCD para adotantes |
| **[hinfra-workloads](https://github.com/gabehamasaki/hinfra-workloads)** | Privado (seu fork/modelo) | Apps de produção, data-services reais, `clusters/production/apps/*` |

Não há dependência Git entre os dois: o **Ansible** registra o `workloads-root` no ArgoCD; o **CI** dos projetos faz bump de imagem direto no repo workloads.

## Quatro camadas

| Camada | Quem gerencia | Onde |
| --- | --- | --- |
| Provisionamento (hardening, Tailscale, k3s) | Ansible, 1× por servidor | `ansible/` |
| Plataforma (cert-manager, Sealed Secrets, ArgoCD, KEDA, operadores) | Ansible + Helm | `ansible/roles/`, `platform/` |
| Serviços de dados (Postgres, Valkey, RustFS) | ArgoCD | repo **workloads** `data-services/` |
| Projetos (imagem do CI) | ArgoCD | repo **workloads** `apps/` |

O ArgoCD **não** provisiona a plataforma: ele depende do cert-manager e do Git para existir.

## Quick start — adotante (só este repo)

```bash
git clone https://github.com/gabehamasaki/hinfra.git
cd hinfra
cp ansible/inventory/hosts.ini.example ansible/inventory/hosts.ini
cp ansible/group_vars/all/local.yml.example ansible/group_vars/all/local.yml
cp ansible/group_vars/all/vault.yml.example ansible/group_vars/all/vault.yml
# preencha vault.yml e: ansible-vault encrypt ansible/group_vars/all/vault.yml

# em group_vars/all/vars.yml: hinfra_deploy_public_gitops_examples: true
./scripts/setup-local-tools.sh
cd ansible && ansible-playbook -i inventory/hosts.ini site.yml
```

ArgoCD usa `root-app` → `clusters/production/apps/` (só `example-app`). Data-services: copie manifests de [`examples/argocd/`](examples/argocd/).

## Quick start — operador com workloads privado

Mesmo clone do hinfa, mais um clone local do workloads (não commitado — ver `.gitignore`):

```bash
git clone git@github.com:YOU/hinfra-workloads.git hinfra-workloads
```

Em `ansible/group_vars/all/vars.yml` (já é o padrão do autor):

- `hinfra_deploy_public_gitops_examples: false`
- `hinfra_deploy_workloads_root: true`

No vault: `vault_workloads_repo_token` (PAT **read** no repo workloads para o ArgoCD).

```bash
cd ansible && script -qec "ansible-playbook -i inventory/hosts.ini site.yml \
  --private-key ../.secrets/vps-1_deploy_ed25519" /dev/null
```

Isso aplica `workloads-root`, cria `workloads-repo-creds` e remove `root-app` / `example` do template público.

**Bootstrap rápido do secret** (sem Ansible), se o PAT já está no `gh`:

```bash
export KUBECONFIG=.secrets/vps-1.kubeconfig
HINFRA_WORKLOADS_TOKEN="$(gh auth token)" ./scripts/argocd-register-workloads-repo.sh
```

## Máquina local (CLI / TUI / MCP)

```bash
# Release (quando publicada) ou desenvolvimento:
make -C tools/hinfra install
# curl -fsSL https://raw.githubusercontent.com/gabehamasaki/hinfra/v1.0.0/scripts/install-hinfra.sh | bash

hinfra init --machine
# ou ~/.config/hinfra/config.yaml:
#   infraRepo: /caminho/para/hinfra
#   workloadsRepo: /caminho/para/hinfra-workloads

export KUBECONFIG=<infraRepo>/.secrets/vps-1.kubeconfig   # exige tailnet
tailscale up
hinfra doctor
hinfra mcp install
```

Detalhes: [`docs/12-mcp.md`](docs/12-mcp.md).

## Estrutura do repositório

| Caminho | Conteúdo |
| --- | --- |
| `ansible/` | Inventário, vault, playbook `site.yml`, roles (k3s, ArgoCD, …) |
| `platform/` | Values Helm / manifests da plataforma aplicados pelo Ansible |
| `clusters/production/` | `root-app.yaml` (adotantes) + `apps/example-app.yaml` |
| `ansible/roles/argocd/files/root-workloads.yaml` | Application raiz para repo workloads (VPS do operador) |
| `examples/argocd/` | Templates Postgres / Valkey / RustFS |
| `apps/example/` | App Kustomize de referência |
| `data-services/example/` | Postgres CNPG de referência |
| `tools/hinfra/` | CLI, TUI, servidor MCP |
| `scripts/` | `install-hinfra.sh`, `argocd-register-workloads-repo.sh`, … |

## Segredos e arquivos locais

| Artefato | No Git? |
| --- | --- |
| `ansible/group_vars/all/vault.yml` | Só criptografado com ansible-vault (ou só local — ver skill/docs) |
| `ansible/inventory/hosts.ini`, `local.yml` | Não — use `.example` |
| `.secrets/` (kubeconfig, SSH, chave Sealed Secrets) | **Nunca** |
| `hinfra-workloads/` (clone local) | **Nunca** (gitignore) |
| Sealed Secrets dos **projetos** | Repo workloads |

[`docs/05-segredos.md`](docs/05-segredos.md) · pós-migração pública: [`docs/ROTATE-ON-PUBLIC.md`](docs/ROTATE-ON-PUBLIC.md)

## CI dos projetos

Workflows em [`docs/deploy-workflow-template.yml`](docs/deploy-workflow-template.yml) (e variante monorepo) fazem commit de tags no **hinfra-workloads** usando o secret `HINFRA_WORKLOADS_TOKEN` no repositório do app.

[`docs/09-cicd.md`](docs/09-cicd.md)

## Licença e segurança

[LICENSE](LICENSE) · [SECURITY.md](SECURITY.md)
