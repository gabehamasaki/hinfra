# 09 — CI/CD

## O fluxo

```
push na main do repo do projeto
      │
      ├─ 1. build da imagem Docker
      ├─ 2. push para ghcr.io/gabehamasaki/<projeto>:<sha>
      ├─ 3. checkout do repo infra (PAT)
      ├─ 4. kustomize edit set image  →  nova tag em apps/<projeto>/kustomization.yaml
      └─ 5. commit e push no repo infra
                     │
                     ▼
      ArgoCD detecta o commit (polling ~3 min)
                     │
                     ▼
              pod novo sobe
```

## Por que o CI escreve no Git em vez de no cluster

A alternativa comum seria dar ao GitHub Actions um kubeconfig e deixá-lo rodar `kubectl apply`. Isso significaria uma credencial de cluster armazenada no GitHub, e exposição da API do Kubernetes à internet para que o runner a alcance — as duas coisas que essa infra evita deliberadamente.

No modelo adotado o CI só sabe fazer duas coisas: publicar uma imagem e commitar uma linha num arquivo YAML. **Nenhuma credencial de cluster existe no GitHub.** O pior caso de um token do CI vazado é um commit indevido no repositório de infra — visível no histórico e revertível.

Ganhos secundários: o estado desejado do cluster tem histórico em Git, e rollback é `git revert`.

## O workflow

Template em [`deploy-workflow-template.yml`](deploy-workflow-template.yml), copiado para `.github/workflows/deploy.yml` em cada projeto.

```yaml
name: deploy
on:
  push:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: gabehamasaki/CHANGE-ME
  APP_PATH: apps/CHANGE-ME

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4

      - name: Login no GHCR
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build e push da imagem
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: |
            ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ github.sha }}
            ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:latest

      - name: Checkout do repo infra
        uses: actions/checkout@v4
        with:
          repository: gabehamasaki/infra
          token: ${{ secrets.INFRA_REPO_TOKEN }}
          path: infra

      - name: Instalar kustomize
        uses: imranismail/setup-kustomize@v2

      - name: Atualizar a tag da imagem no repo infra
        working-directory: infra/${{ env.APP_PATH }}
        run: |
          kustomize edit set image ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ github.sha }}

      - name: Commit e push no repo infra
        working-directory: infra
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add ${{ env.APP_PATH }}/kustomization.yaml
          git diff --cached --quiet && exit 0
          git commit -m "deploy: ${{ env.IMAGE_NAME }}@${{ github.sha }}"
          git push
```

Detalhes:

- **`GITHUB_TOKEN`** para o GHCR é nativo do Actions — não precisa criar nada. A permissão `packages: write` no job é o que o habilita.
- **`INFRA_REPO_TOKEN`** é um PAT fine-grained, restrito ao repositório `infra`, com `Contents: Read and write`. Um PAT clássico com escopo `repo` daria acesso a *todos* os repositórios da conta — escopo desnecessário para bumpar uma tag.
- **`git diff --cached --quiet && exit 0`** evita commit vazio quando o SHA não mudou.
- A imagem recebe **duas tags**: o SHA (imutável, é o que o manifesto referencia) e `latest` (conveniência para `docker pull` manual). O deploy sempre usa o SHA — `latest` num manifesto de Kubernetes torna impossível saber o que está rodando.

### Monorepo (api + web)

Para projetos com várias imagens no mesmo repo, use o template [`deploy-workflow-monorepo-template.yml`](deploy-workflow-monorepo-template.yml). Convenção: Dockerfiles em `./api` e `./web`, imagens `gabehamasaki/<app>-api` e `gabehamasaki/<app>-web`.

Binding explícito no repo do projeto via `hinfra.yml`:

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

O `kustomization.yaml` no repo infra precisa listar **todas** as imagens que o CI atualiza:

```yaml
images:
  - name: ghcr.io/gabehamasaki/my-app-api
    newTag: latest
  - name: ghcr.io/gabehamasaki/my-app-web
    newTag: latest
```

O `scaffold_workflow` do MCP escolhe o template certo automaticamente quando `hinfra.yml` tem bloco `images`. O `deploy_status` exige que cada imagem esteja rodando com o SHA do deploy.

## Onboarding de um projeto novo

### 1. Manifestos no repo infra

```bash
mkdir -p apps/meuprojeto
# copiar de apps/my-portfolio como base:
#   deployment.yaml  service.yaml  ingress.yaml  kustomization.yaml
```

`kustomization.yaml` é o arquivo que o CI edita:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: meuprojeto
resources:
  - deployment.yaml
  - service.yaml
  - ingress.yaml
images:
  - name: ghcr.io/gabehamasaki/meuprojeto
    newTag: latest
```

`ingress.yaml`, para um projeto público:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: meuprojeto
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-cloudflare
spec:
  ingressClassName: traefik
  tls:
    - hosts: [meuprojeto.hamasakis.dev]
      secretName: meuprojeto-tls
  rules:
    - host: meuprojeto.hamasakis.dev
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: meuprojeto
                port: { number: 80 }
```

Para um serviço interno, some o `ingress.yaml`. Para algo administrativo, adicione a annotation do middleware tailnet-only (ver [06 - TLS e Ingress](06-tls-e-ingress.md)).

### 2. Registrar no ArgoCD

`clusters/production/apps/meuprojeto-app.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: meuprojeto
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/gabehamasaki/infra.git
    targetRevision: main
    path: apps/meuprojeto
  destination:
    server: https://kubernetes.default.svc
    namespace: meuprojeto
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

### 3. DNS

```
meuprojeto.hamasakis.dev  A  187.127.62.20   (público)
```

Se for administrativo, aponte para `100.86.241.1` (tailnet) em vez do IP público.

### 4. No repositório do projeto

- `Dockerfile` (ou `api/Dockerfile` + `web/Dockerfile` em monorepos)
- `.github/workflows/deploy.yml` — template single em [`deploy-workflow-template.yml`](deploy-workflow-template.yml) ou monorepo em [`deploy-workflow-monorepo-template.yml`](deploy-workflow-monorepo-template.yml)
- `hinfra.yml` (recomendado em monorepos e forks) — ver [12 - MCP](12-mcp.md)
- Secret `INFRA_REPO_TOKEN`:

```bash
gh secret set INFRA_REPO_TOKEN --repo gabehamasaki/meuprojeto --body "<PAT>"
```

### 5. Dependências (se houver)

Banco, cache ou bucket: ver [08 - Serviços de dados](08-data-services.md).

### 6. Push

O primeiro push dispara o build. O ArgoCD sincroniza em até ~3 min, ou force com refresh manual.

## Imagens privadas no GHCR

Pacotes do GHCR nascem privados. Duas saídas:

**Tornar o pacote público** (para projetos abertos) — Package settings no GitHub, `Change visibility`. Sem `imagePullSecret`.

**Manter privado** — criar um `imagePullSecret` no namespace:

```bash
kubectl create secret docker-registry ghcr-pull -n meuprojeto \
  --docker-server=ghcr.io \
  --docker-username=gabehamasaki \
  --docker-password='<PAT com read:packages>'
```

E referenciar no `deployment.yaml`:

```yaml
spec:
  template:
    spec:
      imagePullSecrets:
        - name: ghcr-pull
```

## O caso real: `my-portfolio`

Vite + React + TypeScript, build estático servido por nginx.

```dockerfile
FROM node:22-alpine AS build
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm build

FROM nginx:1.27-alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

Três problemas reais apareceram na primeira execução do pipeline, todos documentados em [11 - Armadilhas](11-armadilhas.md):

1. `node:20` + corepack baixando pnpm 11.x, que exige Node ≥ 22.13.
2. `pnpm.overrides` no `package.json` — ignorado pelo pnpm 10+, que espera `pnpm-workspace.yaml`.
3. `pnpm-workspace.yaml` não copiado para o estágio de build antes do `install`.

## Diagnóstico

```bash
gh run list --repo gabehamasaki/meuprojeto --limit 5
gh run watch <run-id> --repo gabehamasaki/meuprojeto --exit-status
gh run view <run-id> --repo gabehamasaki/meuprojeto --log

export KUBECONFIG=.secrets/vps-1.kubeconfig
kubectl get application meuprojeto -n argocd
kubectl get pods -n meuprojeto
kubectl describe pod -n meuprojeto -l app=meuprojeto
```

`ErrImagePull` / `ImagePullBackOff` geralmente é uma destas: o build ainda não terminou, o pacote é privado sem `imagePullSecret`, ou a tag no `kustomization.yaml` não corresponde a nenhuma imagem publicada.
