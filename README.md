# hinfra

Kit open-source de provisionamento (Ansible) + GitOps (ArgoCD) para uma VPS com k3s, Tailscale e Traefik.

Documentação em [`docs/`](docs/). Resumo visual: [`docs/overview.html`](docs/overview.html).

## Quick start

```bash
# CLI (release v1.0.0)
curl -fsSL https://raw.githubusercontent.com/gabehamasaki/hinfra/v1.0.0/scripts/install-hinfra.sh | bash

git clone https://github.com/gabehamasaki/hinfra.git
cd hinfra
cp ansible/inventory/hosts.ini.example ansible/inventory/hosts.ini
cp ansible/group_vars/all/local.yml.example ansible/group_vars/all/local.yml
cp ansible/group_vars/all/vault.yml.example ansible/group_vars/all/vault.yml
# preencha vault.yml e: ansible-vault encrypt ansible/group_vars/all/vault.yml

hinfra init --machine   # aponta hinfa + opcional hinfra-workloads privado
./scripts/setup-local-tools.sh
```

**Produção:** mantenha apps e data-services reais num repositório **privado** [`hinfra-workloads`](https://github.com/gabehamasaki/hinfra-workloads) (modelo). Este repo público traz só `apps/example/` e `data-services/example/`.

## Estrutura

| Caminho | Conteúdo |
| --- | --- |
| `ansible/` | Provisionamento (1× por VPS) |
| `platform/` | cert-manager, ArgoCD values (via Ansible) |
| `clusters/production/` | `root-app` com Applications de **exemplo** |
| `apps/example/` | App de referência |
| `data-services/example/` | Postgres CNPG de referência |
| `tools/hinfra/` | CLI, TUI, MCP |

## hinfra CLI

```bash
make -C tools/hinfra install   # desenvolvimento
hinfra doctor
hinfra mcp install
hinfra version
```

Ver [`docs/12-mcp.md`](docs/12-mcp.md).

## Segredos

- `vault.yml` — **não** vai para o Git; só local (`vault.yml.example` como modelo).
- `hosts.ini`, `local.yml` — cópias locais a partir dos `.example`.
- Sealed Secrets de **projetos** — no repo privado workloads.

[`docs/05-segredos.md`](docs/05-segredos.md)

## CI dos projetos

Workflows em [`docs/deploy-workflow-template.yml`](docs/deploy-workflow-template.yml) fazem push de tags no repo **`gabehamasaki/hinfra-workloads`** com secret `HINFRA_WORKLOADS_TOKEN`.
