# 09 — CI/CD

## O fluxo

```
merge na main (e/ou CI de testes)
      │
      │  (deploy manual: criar e pushar tag Git)
      ▼
push de tag (v* | dev/* | hg/*)
      │
      ├─ valida commit na branch do ambiente (default main)
      ├─ 1. build da imagem Docker
      ├─ 2. push para ghcr.io/<github-org>/<projeto>:<versão>
      ├─ 3. checkout do repo infra (PAT)
      ├─ 4. kustomize edit set image → newTag em apps/<projeto>/…
      └─ 5. commit e push no repo infra
                     │
                     ▼
      ArgoCD detecta o commit (polling ~3 min)
                     │
                     ▼
              pod novo sobe
```

**Versão** = nome da tag Git com `/` trocado por `-` (ex.: `dev/0.1.0` → `dev-0.1.0` no GHCR e no `kustomization.yaml`).

| Prefixo da tag | Ambiente | Caminho no infra (padrão) |
| --- | --- | --- |
| `v1.0.0` | produção | `apps/<projeto>` |
| `dev/0.1.0` | dev | `apps/<projeto>/overlays/dev` |
| `hg/0.1.0` | homolog | `apps/<projeto>/overlays/homolog` |

`hinfra.yml` pode definir `environments.<nome>.branch` (opcional; omitir = `main`) para validar que o commit tagueado pertence à branch daquele ambiente. O gatilho continua sendo **só push de tag**.

## Por que o CI escreve no Git em vez de no cluster

A alternativa comum seria dar ao GitHub Actions um kubeconfig e deixá-lo rodar `kubectl apply`. Isso significaria uma credencial de cluster armazenada no GitHub, e exposição da API do Kubernetes à internet para que o runner a alcance — as duas coisas que essa infra evita deliberadamente.

No modelo adotado o CI só sabe fazer duas coisas: publicar uma imagem e commitar uma linha num arquivo YAML. **Nenhuma credencial de cluster existe no GitHub.** O pior caso de um token do CI vazado é um commit indevido no repositório de infra — visível no histórico e revertível.

Ganhos secundários: o estado desejado do cluster tem histórico em Git, e rollback é `git revert` ou nova tag.

## O workflow

Templates em [`deploy-workflow-template.yml`](deploy-workflow-template.yml) (single) e [`deploy-workflow-monorepo-template.yml`](deploy-workflow-monorepo-template.yml). Use `hinfra init` / `scaffold_workflow` no repo do projeto para gerar `.github/workflows/deploy.yml` e `hinfra.yml` com `environments` e mapa de branches.

Gatilho:

```yaml
on:
  push:
    tags:
      - "v*"
      - "dev/**"
      - "hg/**"
```

Release manual (produção):

```bash
git checkout main && git pull
git tag v1.0.0
git push origin v1.0.0
```

Detalhes:

- **`GITHUB_TOKEN`** para o GHCR é nativo do Actions — permissão `packages: write` no job.
- **`HINFRA_WORKLOADS_TOKEN`** — PAT fine-grained no repo `hinfra-workloads`, `Contents: Read and write`.
- **`git diff --cached --quiet && exit 0`** evita commit vazio quando a versão não mudou.
- A imagem recebe **uma tag** por deploy (a versão semver), não `:latest` no manifesto.

### `hinfra.yml` — ambientes

```yaml
app: my-portfolio
appPath: apps/my-portfolio
namespace: my-portfolio
host: my-app.<apps_domain>
image: <github-org>/my-portfolio

environments:
  production:
    enabled: true
    tagPrefix: v
    branch: main
    appPath: apps/my-portfolio
    namespace: my-portfolio
  dev:
    enabled: false
    tagPrefix: dev/
    branch: develop
    appPath: apps/my-portfolio/overlays/dev
    namespace: my-portfolio-dev
    host: my-app.dev.<apps_domain>
  homolog:
    enabled: false
    tagPrefix: hg/
    appPath: apps/my-portfolio/overlays/homolog
    namespace: my-portfolio-hg
    host: my-app.hg.<apps_domain>
```

Com `enabled: false` em dev/homolog, tags `dev/*` e `hg/*` falham no CI até existir overlay + Application no infra (`scaffold_app` com `environments` habilitados).

### Monorepo (api + frontend)

Só a **API** vai para o cluster; o frontend vai para a Vercel ([seção abaixo](#frontends-na-vercel)). A mesma tag `v*` dispara os dois workflows do repo do projeto: `deploy.yml` (imagem da API + bump no `kustomization.yaml`) e `vercel-deploy.yml` (frontend). Referência viva: o `schedule-visits`.

O `kustomization.yaml` no repo infra precisa listar **todas** as imagens que o CI atualiza — hoje, só a da API.

> **Débito conhecido:** o `scaffold_app --monorepo`, o `scaffold_workflow`, o [`deploy-workflow-monorepo-template.yml`](deploy-workflow-monorepo-template.yml) e o `deploy_status` ainda assumem duas imagens (api + web) com Ingress no web. Até serem atualizados, remova do resultado o `web-deployment`, o `web-service`, o Ingress do web e a imagem `-web`, e crie o Ingress do gateway descrito em [06 - TLS e Ingress](06-tls-e-ingress.md#gateway-de-apis-apiinfra_domain).

### Primeiro deploy (monorepo com DB)

Ordem que evita sync preso e probes falhando antes da migration:

1. Manifestos + Application no repo infra; **`hinfra argocd refresh --root`**.
2. `hinfra seal secret` + role/DB no Postgres — [08 - data services](08-data-services.md).
3. `gh secret set HINFRA_WORKLOADS_TOKEN`; pacotes GHCR públicos ou `imagePullSecret`.
4. Tag `v*` na `main` do projeto → CI bumpa versão no `kustomization.yaml` do infra.
5. Aguardar sync do Argo.
6. Job de migration → Deployment da API → Ingress do gateway (`api.<infra_domain>/<projeto>`).
7. `curl https://api.<infra_domain>/<projeto>/<rota>` e, com o frontend publicado, a mesma rota via `https://<projeto>.<apps_domain>/api/...`.

## Frontends na Vercel

Frontend não roda no cluster. Ele é publicado pela Vercel, e **só por tag `v*` na `main`** — o mesmo gatilho dos backends, para que "versão" signifique a mesma coisa nos dois lados.

**No repo do projeto:**

- `vercel.json` na pasta do frontend, com o deploy automático por push desligado:

  ```json
  {
    "git": { "deploymentEnabled": false },
    "rewrites": [
      { "source": "/api/:path*", "destination": "https://api.<infra_domain>/<projeto>/:path*" },
      { "source": "/(.*)", "destination": "/index.html" }
    ]
  }
  ```

  O primeiro rewrite mantém o navegador na mesma origem (sem CORS, e o front continua chamando `/api/...`). O segundo é o fallback de SPA — só é necessário com roteamento no cliente (`BrowserRouter`); site estático sem router dispensa os dois.
- `.github/workflows/vercel-deploy.yml`: em push de tag `v*`, falha se o commit da tag não for ancestral de `origin/main`, depois `vercel pull --environment=production` → `vercel build --prod` → `vercel deploy --prebuilt --prod`. O `--prebuilt` publica exatamente o código da tag; um Deploy Hook buildaria a ponta da `main`.
- Secrets: `VERCEL_TOKEN` (criado no painel, em Account Settings → Tokens — a CLI recusa criar token com a sessão de login dela), `VERCEL_ORG_ID` e `VERCEL_PROJECT_ID`.

**Na Vercel:** projeto ligado ao repo, com *Root Directory* apontando para a pasta do frontend em monorepo. Confira o **Framework Preset**: projeto criado pela API com root directory pode nascer com `framework: null`, e sem preset a Vercel não sabe que a saída do Vite é `dist/`.

**DNS:** o host do frontend em `<apps_domain>` aponta para os IPs que a Vercel recomenda, em **DNS only** — ver [02 - Servidor e acesso](02-servidor-e-acesso.md#dns).

**Cutover de um frontend que já estava no cluster:** Vercel validada em `*.vercel.app` → DNS → só então remover do cluster. O alias `*.vercel.app` com hash pede login (Deployment Protection); teste pelo alias de produção do projeto.

### TUI e versão em produção

`hinfra tui` lista cada Application de projeto com coluna **VERSÃO** (`newTag` de `apps/<app>/kustomization.yaml`). `hinfra deploy status` verifica os quatro elos usando essa versão; use `--env dev` ou `--env homolog` quando overlays existirem.

## Onboarding de um projeto novo

### 1. Manifestos no repo infra

`hinfra scaffold app` ou copiar de `apps/schedule-visits` (API atrás do gateway). Frontend não entra aqui — ver [Frontends na Vercel](#frontends-na-vercel).

`kustomization.yaml` é o arquivo que o CI edita:

```yaml
images:
  - name: ghcr.io/<github-org>/meuprojeto
    newTag: latest
```

### 2. Registrar no ArgoCD

`clusters/production/apps/meuprojeto-app.yaml` — ver template em versões anteriores deste doc ou `scaffold_app`.

### 3. DNS, Dockerfile, workflow, secret

Como antes; workflow **por tag**, não por push na `main`.

### 4. Push de tag

O primeiro `git push origin v0.1.0` (ou `v1.0.0`) dispara o pipeline.

## Imagens privadas no GHCR

Inalterado — pacote público ou `imagePullSecret` no namespace.

## Diagnóstico

```bash
gh run list --repo <github-org>/meuprojeto --limit 5
hinfra deploy status --app meuprojeto
export KUBECONFIG=.secrets/vps-1.kubeconfig
kubectl get application meuprojeto -n argocd
```

`ErrImagePull` / `ImagePullBackOff`: build ainda não terminou, pacote privado sem pull secret, ou `newTag` sem imagem correspondente no registry.
