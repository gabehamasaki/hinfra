---
name: novo-projeto
description: Coloca um projeto novo no ar, ponta a ponta — backend no cluster (manifestos, Application do ArgoCD, Dockerfile, workflow, secret do CI, rota no gateway api.<infra_domain>) e frontend na Vercel (vercel.json, workflow por tag, DNS). Use sempre que o usuário quiser fazer deploy de um projeto, subir um app ou site novo, conectar um repositório ao cluster, ou configurar CI/CD para algo. Use mesmo quando o pedido cobrir só uma parte ("cria o workflow", "faz o deploy do X", "bota esse repo no ar"), porque o fluxo atravessa dois repositórios e faltar um passo quebra o pipeline de um jeito difícil de diagnosticar.
---

# Novo projeto no cluster

Fluxo completo em [`docs/09-cicd.md`](../../../docs/09-cicd.md).

## Como o deploy funciona aqui

```
push de tag Git (v* produção; dev/* e hg/* quando overlays existirem)
   → Actions valida commit na branch do ambiente (default main)
   → Actions builda a imagem e publica no ghcr.io com a versão da tag
   → Actions commita newTag em apps/<projeto>/kustomization.yaml deste repo
   → ArgoCD detecta o commit (polling ~3 min) e aplica
```

O CI **escreve no Git, nunca no cluster**. Não existe credencial de cluster no GitHub, e a API do Kubernetes não é alcançável pela internet. O pior caso de um token do CI vazado é um commit revertível.

Não há webhook: o ArgoCD só existe dentro da tailnet e o GitHub não a alcança. Até ~3 min de espera é esperado. Para ver na hora:

```bash
hinfra argocd refresh --root
# equivalente manual:
kubectl annotate application root-app -n argocd argocd.argoproj.io/refresh=hard --overwrite
```

## Onde cada parte roda

- **Backend** (API, worker): cluster, exposto em `api.<infra_domain>/<projeto>` — ver [`docs/06-tls-e-ingress.md`](../../../docs/06-tls-e-ingress.md#gateway-de-apis-apiinfra_domain).
- **Frontend**: Vercel, deploy só por tag `v*` na `main`, host em `<apps_domain>` com DNS only — ver [`docs/09-cicd.md`](../../../docs/09-cicd.md#frontends-na-vercel). Nada de nginx servindo estático no cluster.

Projeto só de frontend não toca este repo. Projeto com API e frontend segue as duas seções.

## `hinfra` (preferido)

No repo do projeto, rode `hinfra init` (pergunta single vs monorepo api+web, layout `api/web` ou `backend/frontend`). O scaffold de monorepo ainda gera a parte web para o cluster — débito conhecido; remova-a e use a Vercel (ver abaixo).

Com MCP instalado (`hinfra mcp install`, ver [`docs/12-mcp.md`](../../../docs/12-mcp.md)), use as ferramentas em vez de copiar arquivos manualmente. Se o agent estiver aberto no repo **infra**, passe `projectDir` com o caminho absoluto do projeto.

| Passo | Ferramenta MCP |
| --- | --- |
| 1 + 2 — manifestos + Application | `scaffold_app` (`monorepo: true` para api+web; `write: false` preview) |
| Registrar Application no Argo | `argocd_refresh` com `root: true` após gravar manifestos |
| 4 — workflow + `hinfra.yml` | `scaffold_workflow` (`monorepo: true` se ainda não houver `images:` no hinfra.yml) |
| SealedSecret de projeto | `seal_secret` ou `hinfra seal secret` |
| 5 — secret do CI | **não** — o MCP imprime `gh secret set`; configure manualmente |
| Verificação | `deploy_status` (4 elos; step 3 pode ser transitório pós-sync) |

## Checklist

O fluxo atravessa dois repositórios. Faltar um passo tipicamente se manifesta como `ImagePullBackOff` ou como um deploy que simplesmente nunca acontece.

### No repositório `infra`

**1. Manifestos** em `apps/<projeto>/` — Deployment e Service da API, migration (se houver DB), sealed secret e o **Ingress do gateway** (`api-ingress.yaml`: host `api.<infra_domain>`, path `/<projeto>`, middlewares de rate limit + strip/add prefix, `tls` **sem** `secretName`). Copie de `apps/schedule-visits` no `hinfra-workloads`. O `scaffold_app --monorepo` ainda gera `web-*` e Ingress no web: apague.

O `kustomization.yaml` é o arquivo que o CI edita, então o bloco `images` precisa existir:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: <projeto>
resources: [deployment.yaml, service.yaml, ingress.yaml]
images:
  - name: ghcr.io/gabehamasaki/<projeto>
    newTag: latest
```

Defina `requests`/`limits` no deployment — CPU é o recurso escasso nesta máquina.

**2. Application** em `clusters/production/apps/<projeto>-app.yaml` — incluído no `scaffold_app`, ou copie de `schedule-visits-app.yaml` com `syncPolicy.automated` (prune + selfHeal) e `CreateNamespace=true`.

### No repositório do projeto

**3. Dockerfile da API.** Confira que o estágio de build copia **todos** os arquivos de configuração da raiz antes do install — um `pnpm-workspace.yaml` ou similar esquecido quebra o build só no CI, nunca localmente.

**4. Workflow + `hinfra.yml`** — `scaffold_workflow` ou `hinfra init`. Monorepo: bloco `images` e opcional `buildContexts` (paths Docker na raiz do repo — ver [`docs/09-cicd.md`](../../../docs/09-cicd.md)).

Para monorepos (API + frontend), `hinfra.yml` típico — só a API tem imagem:

```yaml
app: <projeto>
host: <projeto>.hamasakis.dev
exposure: public
appPath: apps/<projeto>
namespace: <projeto>
images:
  api: gabehamasaki/<projeto>-api
buildContexts:
  api: { context: ./backend, file: backend/docker/Dockerfile }
routing:
  apiPath: /api
```

**Primeiro deploy monorepo com DB:** root-app refresh → Postgres role/DB ([`08-data-services.md`](../../../docs/08-data-services.md)) → sealed secret → tag `v*` na main → sync Argo → migration PreSync. Tabela de falhas em [`09-cicd.md`](../../../docs/09-cicd.md).

**5. Secret do CI:**

```bash
gh secret set HINFRA_WORKLOADS_TOKEN --repo gabehamasaki/<projeto> --body "<PAT>"
```

PAT fine-grained restrito ao repo `infra`, permissão `Contents: Read and write`. Um PAT clássico com escopo `repo` daria acesso a todos os repositórios da conta — escopo desnecessário para bumpar uma tag.

### Fora dos repositórios

**6. DNS** no Cloudflare. A API **não** precisa de registro novo: o `api.<infra_domain>` já existe (proxied). O frontend ganha `<projeto>.<apps_domain>` apontando para os IPs da Vercel, em DNS only. Algo administrativo aponta para `<TAILNET_IP>` (tailnet) e leva a annotation do middleware — ver a skill `servico-no-cluster`.

### Frontend na Vercel

**8.** `vercel.json` na pasta do frontend com `git.deploymentEnabled: false`, rewrite `/api/:path*` → `https://api.<infra_domain>/<projeto>/:path*` e fallback de SPA se houver router no cliente.

**9.** `.github/workflows/vercel-deploy.yml` por tag `v*` (guarda de commit na `main`; `vercel pull/build/deploy --prebuilt --prod`) e os secrets `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID`.

**10.** Projeto na Vercel com Root Directory do frontend e **Framework Preset conferido** (pode nascer `null`). Validar no alias de produção `*.vercel.app` antes do DNS.

**7. Imagem privada?** Pacotes do GHCR nascem privados. Ou torne o pacote público (Package settings no GitHub), ou crie um `imagePullSecret` no namespace e referencie no deployment.

## Verificação

Não conclua que funcionou porque o workflow ficou verde — o deploy tem um segundo salto depois disso. Use `deploy_status` no MCP ou manualmente:

```bash
gh run watch <run-id> --repo gabehamasaki/<projeto> --exit-status
git -C <repo-infra> pull --rebase          # o CI acabou de commitar aqui
kubectl get application <projeto> -n argocd
kubectl get pods -n <projeto>
curl -s -o /dev/null -w "%{http_code}\n" https://<host>/
```

`ImagePullBackOff` costuma ser uma destas três: o build ainda não terminou, o pacote é privado sem `imagePullSecret`, ou a tag no `kustomization.yaml` não corresponde a nenhuma imagem publicada.

## Problemas comuns de build

Estes já apareceram e são específicos de ambiente de CI, não do código:

- **Gerenciador de pacotes vs versão do Node.** `corepack enable` sem versão fixa baixa a mais recente, que pode exigir um Node mais novo que a imagem base. Fixe a versão ou suba a base.
- **Configuração que migrou de arquivo.** O pnpm 10+ ignora `pnpm.overrides` do `package.json` e espera `pnpm-workspace.yaml`; o aviso é só um warning, mas quebra com `--frozen-lockfile`.
- **Arquivo novo não copiado para o estágio de build.** Funciona local, quebra no CI. Ao adicionar config na raiz do projeto, confira o `COPY` do Dockerfile.
