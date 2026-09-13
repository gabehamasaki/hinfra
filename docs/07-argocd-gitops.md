# 07 — ArgoCD e GitOps

| | |
| --- | --- |
| Versão | `v3.5.2` (chart `argo-cd-10.8.0`) |
| Namespace | `argocd` |
| Console | `https://argocd.<infra_domain>` — só tailnet |
| Usuário | `admin` (senha em `.secrets/argocd-admin-password.txt`) |
| Repositório observado | `github.com/<github-org>/hinfra` (e forks de workloads, se usar) |
| Sincronização | Polling, ~3 min |

## O que o ArgoCD gerencia — e o que não

Dois App of Apps costumam coexistir na VPS de produção:

| Raiz | Repositório | Conteúdo |
| --- | --- | --- |
| `workloads-root` | `hinfra-workloads` | Projetos e serviços de dados |
| `platform-root` | `hinfra` (este repo) | Plataforma visível no GitOps — hoje **observabilidade** (`monitoring`, `monitoring-extras`) |

Applications de plataforma usam o AppProject **`platform`** e o label `hinfra.layer=platform`.

Ele **não** gerencia cert-manager, Sealed Secrets, KEDA, o operador do CloudNativePG, nem ele mesmo. Esses são instalados pelo Ansible. O motivo é dependência circular — o ArgoCD precisa do cert-manager para o próprio certificado e do repositório Git para existir. Observabilidade **pode** ser GitOps porque o ArgoCD não depende dela para subir. O raciocínio completo está em [01 - Arquitetura](01-arquitetura.md).

## App of Apps

Um `Application` raiz aponta para um diretório e sincroniza recursivamente tudo que encontra lá. Como cada arquivo daquele diretório é ele próprio um `Application`, adicionar um projeto ao cluster é adicionar um arquivo ao Git.

```yaml
# clusters/production/root-app.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/<github-org>/hinfra.git
    targetRevision: main
    path: clusters/production/apps
    directory:
      recurse: true
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

O `root-app` é aplicado pelo Ansible (role `argocd`) — é o único ponto de contato entre as duas camadas.

Applications atuais:

| Application | Fonte | Destino |
| --- | --- | --- |
| `my-portfolio` | Git, `apps/my-portfolio` (Kustomize) | ns `my-portfolio` |
| `postgres` | Git, `data-services/postgres` | ns `postgres` |
| `valkey` | Chart Helm `valkey` 0.12.0 | ns `valkey` |
| `rustfs` | Chart Helm `rustfs` 1.0.0-rc.5 | ns `rustfs` |

## Duas formas de escrever um Application

**Manifestos no próprio repositório** — quando os arquivos são nossos:

```yaml
source:
  repoURL: https://github.com/<github-org>/hinfra.git
  targetRevision: main
  path: data-services/postgres
```

Se houver um `kustomization.yaml` no diretório, o ArgoCD detecta e usa Kustomize automaticamente.

**Chart Helm de terceiros** — quando o software vem pronto:

```yaml
source:
  repoURL: https://valkey.io/valkey-helm/
  chart: valkey
  targetRevision: "0.12.0"
  helm:
    values: |
      auth:
        enabled: true
```

A declaração de *qual chart, qual versão, quais values* continua versionada no Git, mesmo com o chart vindo de fora.

> **Sempre fixe a versão.** `targetRevision: "*"` deixou o Application do Valkey travado em `Unknown` sem erro visível. Além disso, sem versão fixa um chart pode mudar sob seus pés entre syncs.

## Acesso ao repositório privado

O repositório é privado, então o ArgoCD precisa de credencial. É um Secret com um label específico, criado pelo role `argocd` a partir do vault:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: infra-repo-creds
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
stringData:
  type: git
  url: "https://github.com/<github-org>/hinfra.git"
  username: <github-org>
  password: "{{ vault_workloads_repo_token }}"
```

O token é um PAT fine-grained restrito ao repositório `infra`, com permissão `Contents: Read and write` — o mesmo usado pelo CI para fazer o bump de imagem.

## Sem webhook: por que o sync é por polling

O caminho normal seria um webhook do GitHub apontando para `https://argocd.<infra_domain>/api/webhook`, dando sync quase instantâneo. Isso exigiria que o ArgoCD fosse alcançável pelo GitHub — ou seja, exposto na internet.

A console do ArgoCD dá acesso equivalente a execução de código arbitrário no cluster. Expô-la publicamente contradiria toda a premissa de manter a API do Kubernetes fora da internet. A escolha foi manter o ArgoCD privado e aceitar até ~3 minutos de latência no deploy.

Alternativas, se a espera incomodar:

- Reduzir `timeout.reconciliation` no ConfigMap `argocd-cm` (custa CPU, não expõe nada).
- Forçar um refresh manual quando houver pressa:

```bash
kubectl annotate application root-app -n argocd argocd.argoproj.io/refresh=hard --overwrite
```

> Ao mudar os *values* de um Application com fonte Helm, o refresh precisa ser no **`root-app`**, não no Application filho. Os values vêm do arquivo em `clusters/production/apps/`, que é o `root-app` quem sincroniza. Atualizar só o filho não puxa a mudança.

## Ajustes de recursos

Numa máquina de 2 vCPU, componentes não usados foram desligados:

```yaml
dex:
  enabled: false            # SSO externo — não usado
notifications:
  enabled: true             # Degraded / sync failed → Discord (vault_alertmanager_discord_webhook_url)
applicationSet:
  replicas: 0               # ApplicationSet — não usado
```

> `applicationSet` **não tem** toggle `enabled` neste chart. Setar `applicationSet.enabled: false` é silenciosamente ignorado — o valor não existe no `values.yaml` e o Helm não reclama de chaves desconhecidas. A única forma de desligar é `replicas: 0`.

Requests e limits foram definidos em todos os componentes (`server`, `repoServer`, `controller`, `redis`) para que a plataforma não dispute CPU com as cargas dos projetos.

O `server` roda com `server.insecure: true`: o TLS é terminado no Ingress pelo Traefik, com certificado do cert-manager. Sem isso haveria TLS duplicado, o que quebra o roteamento.

## Senha do admin

O chart cria um Secret `argocd-initial-admin-secret` na primeira instalação. Ele foi lido, a senha foi trocada por uma aleatória e o secret inicial foi **apagado** — é a recomendação do próprio ArgoCD.

A task do Ansible que mostra a senha inicial tem guarda para não quebrar em execuções seguintes:

```yaml
when: argocd_admin_secret.resources | length > 0
```

Trocar a senha de novo:

```bash
NEWPASS=$(openssl rand -base64 18)
HASH=$(python3 -c "import bcrypt,sys; print(bcrypt.hashpw(sys.argv[1].encode(), bcrypt.gensalt(rounds=10)).decode())" "$NEWPASS")
kubectl -n argocd patch secret argocd-secret \
  -p '{"stringData": {"admin.password": "'"$HASH"'", "admin.passwordMtime": "'"$(date -u +%FT%TZ)"'"}}'
echo "$NEWPASS"
```

## Diagnóstico

```bash
kubectl get applications -n argocd                        # visão geral
kubectl describe application <nome> -n argocd             # eventos e condições
kubectl logs -n argocd argocd-application-controller-0 --tail=100
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-repo-server --tail=100
```

Erros de renderização de chart Helm aparecem no **repo-server**, não no application-controller. Foi assim que o problema de `aclUsers` do Valkey apareceu — o Application ficava `Unknown` sem mensagem, e o erro real (`auth.enabled is true but no authentication method is configured`) só estava no log do repo-server.

Um Application preso em `Unknown` normalmente significa falha ao gerar os manifestos. Comece pelo repo-server.
