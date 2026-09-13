# 12 — `hinfra` (CLI, TUI e MCP)

Ferramenta Go local que roda na sua máquina: **TUI** para visão do cluster, **CLI** para scripts, e **MCP** para coding agents (Cursor, Claude, Codex, OpenCode).

**Não é uma camada da infra.** Não roda na VPS, não consome CPU do cluster, não entra no ArgoCD. Código em [`tools/hinfra/`](../tools/hinfra/).

## Instalação

```bash
# setup da máquina (interativo)
hinfra init --machine

# ou manual:
mkdir -p ~/.config/hinfra
cat > ~/.config/hinfra/config.yaml <<'EOF'
infraRepo: /home/hamasaki/www/infra
workloadsRepo: /home/hamasaki/www/infra/hinfra-workloads
EOF

make -C tools/hinfra install
hinfra doctor
hinfra mcp install
# ou: hinfra mcp install --cursor --codex --all
```

O endereço para checar a tailnet vem do `server:` do kubeconfig (não precisa de IP no repo). Opcional: `tailnetAPI: 100.x.x.x:6443` ou `HINFRA_TAILNET_API`.

Config legada `~/.config/infra-mcp/config.yaml` ainda funciona com aviso — migre para `~/.config/hinfra/`.

O kubeconfig é derivado de `<infraRepo>/.secrets/vps-1.kubeconfig`. Operações no cluster exigem tailnet (`tailscale up`).

## Uso

| Comando | O que faz |
| --- | --- |
| `hinfra` | Abre a TUI (se terminal interativo) |
| `hinfra tui` | TUI explícita |
| `hinfra doctor` | Valida config + kubeconfig + tailnet |
| `hinfra smoke` | Smoke test das tools de leitura no cwd |
| `hinfra init` | Onboarding de projeto (hinfra.yml, workflow, manifestos) |
| `hinfra mcp` | Servidor MCP stdio |
| `hinfra mcp install` | Registra MCP nos coding agents |

### CLI (exemplos)

```bash
hinfra deploy status [--app]
hinfra health [--app]
hinfra logs [--app] [--pod] [--tail 100] [--previous]
hinfra docs search targetRevision
hinfra restart [--app]
hinfra argocd refresh [--app] [--root]
hinfra seal secret -f secret.yaml [-o apps/<app>/sealed-secret.yaml] [--execute]
hinfra scaffold app --name my-app --host my-app.hamasakis.dev [--monorepo] [--write]
hinfra scaffold workflow [--app] [--monorepo] [--write]
```

Flag global `--json` em todos os comandos.

## MCP nos agents

`hinfra mcp install` suporta:

| Agent | Config |
| --- | --- |
| Cursor | `~/.cursor/mcp.json` |
| Claude Code | `claude mcp add --scope user infra -- <bin> mcp` |
| Codex | `~/.codex/config.toml` |
| OpenCode | `~/.config/opencode/opencode.json` |

Entrada comum: `command: hinfra`, `args: ["mcp"]`.

## Descoberta de contexto

Com o cwd num repo de projeto (ou `projectDir` no MCP quando o agent está no repo `infra`):

1. `hinfra.yml` na raiz (opcional)
2. `remote origin` → `owner/repo`
3. Validação com `apps/<app>/kustomization.yaml` no repo infra

Parâmetro MCP **`projectDir`**: caminho absoluto do repo do projeto em `deploy_status`, `app_health`, `scaffold_workflow`, etc.

### `hinfra.yml`

```yaml
app: my-portfolio
image: gabehamasaki/my-portfolio
appPath: apps/my-portfolio
namespace: my-portfolio
host: hamasakis.dev
exposure: public
```

Monorepo (api + web): `hinfra init` pergunta layout e gera `images` + `buildContexts`. Manifestos no infra: `scaffold_app` com `monorepo: true` (Ingress só web; API interna; migration PreSync).

## Ferramentas MCP (10)

| Ferramenta | Tipo |
| --- | --- |
| `deploy_status` | leitura (`env`: `production`, `dev`, `homolog`; retorna `version`) |
| `app_health` | leitura |
| `app_logs` | leitura |
| `infra_docs` | leitura |
| `app_restart` | mutação |
| `argocd_refresh` | mutação (`root: true` → root-app) |
| `rollback` | mutação (`confirm: true`) |
| `scaffold_app` | scaffold (`monorepo: true`) |
| `scaffold_workflow` | scaffold (`monorepo: true`) |
| `seal_secret` | scaffold / kubeseal (`execute: true`) |

## TUI

`hinfra` sem argumentos abre um dashboard de consumo do node, com gráficos de área para histórico e gauges para ocupação atual.

### Scenes

| Tecla | Scene | Conteúdo |
| --- | --- | --- |
| `1` | dashboard | CPU, memória, disco e rede do node; top pods; resumo dos Applications |
| `2` | nodes | detalhe por node: capacity, uptime, kubelet, histórico de CPU e memória |
| `3` | storage | uso do disco decomposto (imagens / resto / livre) e PVCs por tamanho |
| `4` | apps | Applications do ArgoCD (coluna versão = `newTag` em produção), filtráveis por camada e por nome |

Dentro de um app: `enter` detalhe, `l` logs, `d` pipeline de deploy, `a` refresh hard, `g` pula para o app do diretório atual. Na scene de nodes, `n` passa para o próximo node.

Globais: `r` recarrega, `w` alterna o watch, `?` ajuda, `q` sai. Mutações destrutivas (restart, rollback) ficam só na CLI.

O **watch já começa ligado** e recarrega a cada 5s. Um dashboard de consumo com números congelados engana mais do que informa, e é o watch que alimenta o histórico dos gráficos de área — sem ele as séries nunca saem de "acumulando histórico".

### Uso × requests

Cada gauge de CPU e memória mostra **duas** séries: `█` é o uso instantâneo do metrics-server e `▒` são os requests reservados. Nesta VPS o orçamento de requests satura muito antes do uso real (hoje ~42% de CPU reservada contra ~9% em uso), então decidir capacidade pelo uso instantâneo leva a pods `Pending` sem aviso. É por isso que as duas séries dividem a mesma barra.

### Layout adaptativo

Acima de 100 colunas os painéis vão numa grade 2×2 com gráficos; abaixo disso vira um painel compacto só de gauges, porque quatro painéis nessa largura truncariam os números.

Na vertical, **nenhum painel é escondido por falta de espaço**. A altura do terminal só decide o tamanho dos gráficos (entre 3 e 8 linhas); o que ainda assim não couber fica acessível por rolagem — `j`/`k` linha a linha, `pgup`/`pgdn` ou `ctrl+u`/`ctrl+d` meia tela, `home`/`end` para os extremos. O indicador `↕` no canto direito do rodapé aparece só quando há conteúdo fora da tela e mostra a posição.

Isso substituiu uma versão que omitia painéis inteiros quando não cabiam: o resultado era informação sumindo em silêncio, sem nada na tela indicando que existia. Rolar é pior que ver tudo de uma vez, mas é muito melhor que não saber o que está faltando.

Barra de status e rodapé também se ajustam à largura, descartando os itens menos importantes (versão do kubelet e uptime na barra; dicas da scene no rodapé). Se qualquer uma dessas linhas quebrasse, roubaria uma linha do corpo e empurraria o topo para fora da tela.

### Fontes das métricas

Três APIs, porque nenhuma entrega tudo:

| Dado | Origem |
| --- | --- |
| CPU e memória em uso | `metrics.k8s.io/v1beta1` (metrics-server) |
| capacity, allocatable, conditions | API core (`/api/v1/nodes`) |
| uso real de disco, rede, uptime | summary API do kubelet (`/api/v1/nodes/<node>/proxy/stats/summary`) |
| requests reservados | soma dos requests dos pods agendados e não terminados |

Falha parcial degrada o painel em vez de derrubá-lo: sem a summary API o disco fica sem uso real, mas CPU, memória e requests continuam corretos.

O histórico dos gráficos é um ring buffer em memória, preenchido a cada coleta — abrir a TUI mostra `acumulando histórico...` até haver amostras suficientes, e nada é persistido entre execuções.

Os mesmos dados saem na CLI com `hinfra metrics` (e `--json` para script).

## Migração de `infra-mcp`

```bash
make -C tools/hinfra install
mv ~/.config/infra-mcp ~/.config/hinfra   # opcional
hinfra mcp install --all
hinfra doctor
rm ~/.local/bin/infra-mcp                 # binário antigo
```

## Segurança

O kubeconfig em `.secrets/` é cluster-admin. O MCP **não manipula segredos** — para `HINFRA_WORKLOADS_TOKEN`, imprime `gh secret set`.

## O que não faz

- `kubectl apply`, `delete`, `exec`
- Leitura de Secrets do cluster
- DNS no Cloudflare
- Rodar no cluster
