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
      ├─ 2. push para ghcr.io/gabehamasaki/<projeto>:<versão>
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
- **`INFRA_REPO_TOKEN`** — PAT fine-grained no repo `infra`, `Contents: Read and write`.
- **`git diff --cached --quiet && exit 0`** evita commit vazio quando a versão não mudou.
- A imagem recebe **uma tag** por deploy (a versão semver), não `:latest` no manifesto.

### `hinfra.yml` — ambientes

```yaml
app: my-portfolio
appPath: apps/my-portfolio
namespace: my-portfolio
host: my-portfolio.hamasakis.dev
image: gabehamasaki/my-portfolio

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
    host: my-portfolio.dev.hamasakis.dev
  homolog:
    enabled: false
    tagPrefix: hg/
    appPath: apps/my-portfolio/overlays/homolog
    namespace: my-portfolio-hg
    host: my-portfolio.hg.hamasakis.dev
```

Com `enabled: false` em dev/homolog, tags `dev/*` e `hg/*` falham no CI até existir overlay + Application no infra (`scaffold_app` com `environments` habilitados).

### Monorepo (api + web)

Mesma convenção de tags; duas imagens com o **mesmo** `newTag`. Ver [`deploy-workflow-monorepo-template.yml`](deploy-workflow-monorepo-template.yml) e `buildContexts` em `hinfra.yml` ([seção anterior neste doc](09-cicd.md)).

O `kustomization.yaml` no repo infra precisa listar **todas** as imagens que o CI atualiza.

O `scaffold_workflow` do MCP escolhe o template certo quando `hinfra.yml` tem bloco `images`, ou com `monorepo: true`. O `deploy_status` exige que cada imagem esteja rodando com a versão do manifesto.

### Primeiro deploy (monorepo com DB)

Ordem que evita sync preso e probes falhando antes da migration:

1. Manifestos + Application no repo infra; **`hinfra argocd refresh --root`**.
2. `hinfra seal secret` + role/DB no Postgres — [08 - data services](08-data-services.md).
3. `gh secret set INFRA_REPO_TOKEN`; pacotes GHCR públicos ou `imagePullSecret`.
4. Tag `v*` na `main` do projeto → CI bumpa versão no `kustomization.yaml` do infra.
5. Aguardar sync do Argo.
6. Job de migration (`PreSync`) → Deployments api/web → Ingress no **web**.
7. `hinfra deploy status` e `curl` no host.

### TUI e versão em produção

`hinfra tui` lista cada Application de projeto com coluna **VERSÃO** (`newTag` de `apps/<app>/kustomization.yaml`). `hinfra deploy status` verifica os quatro elos usando essa versão; use `--env dev` ou `--env homolog` quando overlays existirem.

## Onboarding de um projeto novo

### 1. Manifestos no repo infra

`hinfra scaffold app` ou copiar de `apps/my-portfolio`.

`kustomization.yaml` é o arquivo que o CI edita:

```yaml
images:
  - name: ghcr.io/gabehamasaki/meuprojeto
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
gh run list --repo gabehamasaki/meuprojeto --limit 5
hinfra deploy status --app meuprojeto
export KUBECONFIG=.secrets/vps-1.kubeconfig
kubectl get application meuprojeto -n argocd
```

`ErrImagePull` / `ImagePullBackOff`: build ainda não terminou, pacote privado sem pull secret, ou `newTag` sem imagem correspondente no registry.
