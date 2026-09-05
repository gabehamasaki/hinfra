# 12 — MCP local (`infra-mcp`)

Servidor MCP em Go que roda na sua máquina (stdio) e dá ao agente — de dentro de qualquer repo de projeto — leitura do cluster, ações limitadas no próprio app, scaffold de CI/CD e acesso aos docs da infra.

**Não é uma camada da infra.** Não roda na VPS, não consome CPU do cluster, não entra no ArgoCD. É ferramenta de desenvolvimento local em [`tools/mcp/`](../tools/mcp/).

## Instalação

```bash
# config obrigatória (uma vez por máquina)
mkdir -p ~/.config/infra-mcp
cat > ~/.config/infra-mcp/config.yaml <<EOF
infraRepo: /home/hamasaki/www/infra
EOF

# build e instalação
make -C tools/mcp install

# validar config, kubeconfig e tailnet
infra-mcp --selftest

# Claude Code (escopo user — disponível em todo repo)
claude mcp add --scope user infra -- ~/.local/bin/infra-mcp

# Cursor (escopo global — disponível em todo repo)
# ~/.cursor/mcp.json:
#   { "mcpServers": { "infra": { "command": "/home/hamasaki/.local/bin/infra-mcp" } } }
# Depois: Settings → MCP → confirmar "infra" conectado (toggle para ligar/desligar quando precisar)
```

O kubeconfig é derivado automaticamente de `<infraRepo>/.secrets/vps-1.kubeconfig`. Toda ferramenta que toca o cluster exige estar na tailnet; fora dela, o erro é explícito em ~2s: `não estou na tailnet — rode tailscale up`.

## Descoberta de contexto

Com o cwd num repo de projeto, o servidor descobre qual app corresponde:

1. `hinfra.yml` na raiz do repo (opcional) — binding explícito
2. `remote origin` → `owner/repo`
3. Validação cruzada com `apps/<app>/kustomization.yaml` no repo infra

Se as pistas discordarem, a ferramenta **não adivinha** — pede confirmação ou parâmetro `app` explícito.

### `hinfra.yml` (opcional)

Formato simples (um serviço):

```yaml
app: my-portfolio
image: gabehamasaki/my-portfolio
appPath: apps/my-portfolio
namespace: my-portfolio
host: hamasakis.dev
exposure: public   # ou tailnet
```

Formato monorepo (várias imagens, ex. api + web):

```yaml
app: my-app
host: my-app.hamasakis.dev
exposure: public
appPath: apps/my-app
namespace: my-app
images:
  api: gabehamasaki/my-app-api
  web: gabehamasaki/my-app-web
routing:
  apiPath: /api
  webPath: /
```

Útil para forks, monorepos ou nomes de imagem que não seguem o basename do repo. O campo `image` (singular) continua válido para projetos de imagem única. O `scaffold_workflow` com `write: true` pode gerar este arquivo junto com o workflow.

## Ferramentas (9)

### Leitura

| Ferramenta | O que faz |
| --- | --- |
| `deploy_status` | Encadeia 4 elos: SHA em `origin/main`, bump no infra, Application Synced/Healthy, imagem no pod |
| `app_health` | Pods, restarts, imagem em execução e events do namespace |
| `app_logs` | Logs com `previous: true` para CrashLoopBackOff |
| `infra_docs` | `search` ou `read` em `docs/` do repo infra |

### Mutação

| Ferramenta | O que faz |
| --- | --- |
| `app_restart` | Rollout restart — só namespaces com diretório em `apps/` |
| `argocd_refresh` | Refresh hard no Application correto (`root-app` para Helm) |
| `rollback` | Commit pra frente com `kustomize edit set image` — exige `confirm: true` |

### Scaffold

| Ferramenta | O que faz |
| --- | --- |
| `scaffold_app` | Gera manifestos no repo infra (`write: false` por padrão) |
| `scaffold_workflow` | Gera workflow + `hinfra.yml` no repo do projeto (template single ou monorepo conforme `hinfra.yml`) |

`scaffold_app` e `scaffold_workflow` nunca sobrescrevem arquivos existentes — mostram diff.

## Smoke test manual

```bash
tailscale status
infra-mcp --selftest

# no repo de um projeto:
# deploy_status, app_health, app_logs
# scaffold_workflow com write:false
```

## Segurança (honesta)

O kubeconfig em `.secrets/` é cluster-admin. A restrição de `app_restart` a namespaces de `apps/` é **fronteira de API**, não autorização — protege contra engano do agente, não contra atacante. Quem roda o binário pode usar `kubectl` com o mesmo arquivo.

O MCP **não manipula segredos** — nem PAT, nem Secret do Kubernetes. Para `INFRA_REPO_TOKEN`, imprime o comando `gh secret set` e o resto é manual.

## O que o MCP não faz

- `kubectl apply`, `delete`, `exec`
- Leitura de Secrets do cluster
- DNS no Cloudflare
- Rodar no cluster
