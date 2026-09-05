# 11 — Armadilhas

Problemas reais enfrentados durante a construção desta infra, com sintoma, causa e correção. Nenhum é hipotético.

---

## Incidente: rate limit do Let's Encrypt

**Sintoma.** O Application `rustfs` ficava alternando entre `Synced` e `OutOfSync` a cada poucos segundos, com dezenas de eventos `Partial sync operation`. O certificado nunca ficava `READY`. No fim:

```
429 urn:ietf:params:acme:error:rateLimited: too many certificates (5)
already issued for this exact set of identifiers in the last 168h0m0s
```

**Causa.** Três coisas se combinando:

1. O chart do RustFS, com `ingress.tls.enabled: true` e `ingress.tls.certManager.enabled: false` (o padrão), renderiza um Secret TLS **próprio**, com os valores literais de placeholder `tls.crt` / `tls.key`:

```yaml
{{- if and .Values.ingress.tls.enabled
        (not .Values.ingress.tls.certManager.enabled)
        (not .Values.ingress.tls.existingSecret.enabled) -}}
data:
  tls.crt: {{ .Values.ingress.tls.crt | b64enc | quote }}   # "tls.crt", literalmente
```

2. Esse Secret tem o **mesmo nome** que o cert-manager usa (`rustfs-tls`), e por ser parte do output do chart, passa a ser um recurso gerenciado pelo ArgoCD.

3. O loop: cert-manager emite o certificado → ArgoCD sincroniza e sobrescreve o Secret com o placeholder → cert-manager detecta `no PEM data was found` e reemite → repete. Cada volta consumia uma emissão da cota semanal.

**Correção.**

```yaml
ingress:
  tls:
    enabled: true
    certManager:
      enabled: true      # impede o chart de criar o Secret
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-cloudflare   # continua necessária
```

**A pegadinha dentro da pegadinha.** `certManager.enabled: true` **não** adiciona a annotation do cluster-issuer — o helper de annotations do chart só lê de `nginxAnnotations`, `traefikAnnotations`, `customAnnotations` e `annotations`. A flag só desliga o template do Secret. As duas coisas são necessárias.

**Lições.**

- Charts que oferecem "gerenciar TLS por conta própria" precisam ser explicitamente desligados quando o cert-manager é o dono. Ter a annotation não basta.
- Um Secret aparecendo repetidamente como `OutOfSync` num Application é sinal de disputa de ownership. Investigue antes que vire rate limit.
- Para testar emissão, use o servidor de staging do Let's Encrypt.

---

## Traefik mascarando o IP de origem

**Sintoma.** O middleware `IPAllowList` retornava `403` para **todo mundo**, inclusive de dentro da tailnet. Parecia funcionar (bloqueava o público) mas estava quebrado (bloqueava todos).

**Causa.** O Service do Traefik nasce com `externalTrafficPolicy: Cluster`. O kube-proxy aplica SNAT, e o Traefik enxerga um IP da rede de pods (`10.42.0.0/16`) como origem — que nunca casa com `100.64.0.0/10`.

**Correção.**

```bash
kubectl patch svc traefik -n kube-system -p '{"spec":{"externalTrafficPolicy":"Local"}}'
```

Aplicado de forma permanente pelo role `argocd`.

**Diagnóstico enganoso.** Antes de achar a causa real, o log do Traefik mostrava `middleware does not exist` e `secret does not exist` — dois erros **transitórios** de quando os recursos ainda estavam sendo criados. Perseguir a mensagem de erro mais recente teria levado ao lugar errado; o que resolveu foi comparar o comportamento esperado (200 de dentro, 403 de fora) com o observado (403 nos dois).

---

## Repositórios Helm que mudaram de lugar

**Sintoma.** `404` e `Moved Permanently` em URLs que a documentação ainda cita.

| O quê | Antes | Agora |
| --- | --- | --- |
| Chart do Sealed Secrets | `bitnami-labs.github.io/sealed-secrets` | `bitnami.github.io/sealed-secrets` |
| API do GitHub (kubeseal) | Redirect 301 não seguido | `curl -sL` (seguir redirect) |

**Contexto maior.** A Broadcom está movendo os containers e charts do Bitnami para um repositório legado e pago (agosto/2026). Isso afeta qualquer coisa que dependa de `bitnami/*`. Foi por isso que o Redis virou **Valkey** aqui.

**Lição.** Antes de fixar a URL de um chart, confirme que ela responde:

```bash
curl -s -o /dev/null -w "%{http_code}\n" https://<repo>/index.yaml
```

---

## Valkey: `aclUsers` obrigatório mesmo com Secret externo

**Sintoma.** Application em `Unknown`, sem resources, sem erro visível no `describe`.

**Causa.** Só aparecia no log do repo-server:

```
auth.enabled is true but no authentication method is configured.
Please provide auth.aclUsers or auth.aclConfig
```

O `usersExistingSecret` fornece apenas a **senha**. As **permissões** têm que vir do values.

**Correção.**

```yaml
auth:
  enabled: true
  usersExistingSecret: valkey-auth
  aclUsers:
    default:
      permissions: "~* &* +@all"
```

**Lição.** Erro de renderização de chart não aparece no application-controller. Vá direto ao repo-server:

```bash
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-repo-server --tail=100 | grep -i error
```

---

## `targetRevision: "*"` em fonte Helm

**Sintoma.** Application preso em `Unknown` com `Revision: *`, sem resolver para versão nenhuma.

**Correção.** Fixe a versão:

```bash
curl -s https://<repo>/index.yaml | python3 -c "
import sys,yaml; d=yaml.safe_load(sys.stdin)
e=d['entries']['<chart>'][0]; print(e['version'], e['appVersion'])"
```

Fixar versão também evita que um chart mude sozinho entre syncs.

---

## `applicationSet.enabled` não existe

**Sintoma.** `applicationSet.enabled: false` nos values, e o `argocd-applicationset-controller` continuava rodando.

**Causa.** O chart do ArgoCD não tem esse toggle. Helm ignora silenciosamente chaves desconhecidas.

**Correção.** `applicationSet.replicas: 0`.

**Lição.** Values que não existem falham em silêncio. Confirme antes de assumir:

```bash
helm show values <repo>/<chart> | grep -A3 "^<chave>:"
```

(`dex.enabled` e `notifications.enabled` **existem** neste mesmo chart — a inconsistência é do chart, não sua.)

---

## Nome do node ≠ nome do inventário

**Sintoma.** `Error from server (NotFound): nodes "vps-1" not found`, com o node perfeitamente `Ready`.

**Causa.** O k3s registra o node pelo hostname do SO (`srv1957194`). O inventário do Ansible o chama de `vps-1`.

**Correção.** `{{ ansible_hostname }}`, não `{{ inventory_hostname }}`.

---

## Precedência de variáveis do Ansible

**Sintoma.** `ansible-playbook -u root` conectando como `deploy` e falhando com `Permission denied (publickey)`.

**Causa.** `ansible_user=deploy` no inventário tem precedência **maior** que a flag `-u` da linha de comando.

**Correção.** Use extra vars, que ficam no topo da precedência:

```bash
ansible-playbook ... -e ansible_user=root
```

---

## `creates:` como checagem de idempotência

**Sintoma.** `tailscale ip -4` falhando com `no current Tailscale IPs; state: NeedsLogin`, logo depois de uma task de login marcada como `ok`.

**Causa.** A task usava `creates: /var/lib/tailscale/tailscaled.state`. O arquivo existia (criado na instalação) mas o node nunca havia logado. O Ansible pulou a task.

**Correção.** Checar o estado real, não a presença de arquivo:

```yaml
- name: Checar status atual do Tailscale
  ansible.builtin.command: tailscale status --json
  register: tailscale_status_raw
  changed_when: false
  failed_when: false

- name: Entrar na tailnet (só se ainda não estiver logado)
  ansible.builtin.command: tailscale up --reset ...
  when: >
    tailscale_status_raw.rc != 0
    or (tailscale_status_raw.stdout | from_json).BackendState != "Running"
```

---

## `tailscale up` exige todas as flags não-default

**Sintoma.**

```
Error: changing settings via 'tailscale up' requires mentioning all
non-default flags. To proceed, either re-run your command with --reset
```

**Causa.** Uma execução anterior deixou preferências salvas. Chamadas seguintes com flags diferentes são recusadas.

**Correção.** `--reset` sempre, em automação.

---

## Tag do Tailscale precisa existir antes

**Sintoma.** `requested tags [tag:k8s] are invalid or not permitted`.

**Causa.** A tag precisa estar declarada em `tagOwners` na política de ACL do admin console **antes** de qualquer authkey usá-la. Não dá para criar tag pela authkey.

**Correção.** Declarar a tag no admin console e gerar a authkey já com ela.

---

## Módulos `kubernetes.core` precisam de libs no host remoto

**Sintoma.** `Failed to import the required Python library (kubernetes)`.

**Causa.** Os módulos executam no host alvo, não na máquina de controle. E o Ubuntu 24.04 aplica PEP 668, o que impede instalar via `pip` no sistema.

**Correção.** Via apt, no role `common`:

```
python3-kubernetes  python3-yaml  python3-jsonpatch
```

---

## `k8s.src` lê do host remoto, `template`/`copy` leem do local

**Sintoma.** `Failed to load resource definition: No such file or directory: '/home/.../root-app.yaml'` — o arquivo existe, mas na máquina de controle.

**Correção.** Copiar primeiro, aplicar depois:

```yaml
- ansible.builtin.copy:      # lê local, escreve remoto
    src: "{{ playbook_dir }}/../clusters/production/root-app.yaml"
    dest: /tmp/root-app.yaml

- kubernetes.core.k8s:       # lê no host remoto
    state: present
    src: /tmp/root-app.yaml
```

---

## Refresh no Application errado

**Sintoma.** Mudança nos values de um Application Helm não surtia efeito, mesmo com refresh forçado e commit no repositório.

**Causa.** Para fontes Helm, o `revision` do Application é a **versão do chart**, não o commit Git. Os values vêm do arquivo em `clusters/production/apps/`, que é sincronizado pelo **`root-app`**.

**Correção.** Refrescar o `root-app`.

---

## Pipeline: Node 20 × pnpm 11

**Sintoma.** `Error [ERR_UNKNOWN_BUILTIN_MODULE]: No such built-in module: node:sqlite`.

**Causa.** `corepack enable` sem versão fixa baixa o pnpm mais recente (11.x), que exige Node ≥ 22.13. A imagem base era `node:20-alpine`.

**Correção.** `node:22-alpine`. Alternativa: fixar a versão do pnpm via `packageManager` no `package.json`.

---

## Pipeline: `pnpm.overrides` ignorado

**Sintoma.**

```
ERR_PNPM_LOCKFILE_CONFIG_MISMATCH: the current "overrides" configuration
doesn't match the value found in the lockfile
```

**Causa.** O pnpm 10+ não lê mais `pnpm.overrides` do `package.json` — a configuração migrou para `pnpm-workspace.yaml`. O aviso aparece, mas como aviso, e o build só quebra no `--frozen-lockfile`.

**Correção.** Criar `pnpm-workspace.yaml` e remover a chave do `package.json`:

```yaml
packages:
  - .
overrides:
  vite: 6.3.5
allowBuilds:
  '@tailwindcss/oxide': true
  esbuild: true
```

`allowBuilds` é a outra novidade: o pnpm passou a exigir aprovação explícita de scripts de postinstall nativos. Em ambiente não-interativo ele escreve placeholders no arquivo em vez de perguntar.

---

## Pipeline: arquivo faltando no estágio de build

**Sintoma.** Mesmo erro de lockfile mismatch, **depois** de criar o `pnpm-workspace.yaml`. Funcionava local, quebrava no CI.

**Causa.** O Dockerfile copiava só `package.json` e `pnpm-lock.yaml` antes do `pnpm install`. O arquivo novo não existia dentro do build.

**Correção.**

```dockerfile
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
```

**Lição.** Ao adicionar um arquivo de configuração na raiz de um projeto, verifique o Dockerfile — o estágio de build costuma copiar uma lista explícita.

---

## Ansible e I/O não-bloqueante

**Sintoma.**

```
ERROR: Ansible requires blocking IO on stdin/stdout/stderr.
Non-blocking file handles detected: <stdout>, <stderr>
```

**Correção.** Rodar dentro de um pty:

```bash
script -qec "ansible-playbook ..." /dev/null
```

Vale para `ansible`, `ansible-playbook` e `ansible-vault`.

---

## Account API Token do Cloudflare no endpoint errado

**Sintoma.** `{"success":false,"errors":[{"code":1000,"message":"Invalid API Token"}]}` para um token válido.

**Causa.** Tokens de conta (prefixo `cfat_`) se verificam em `/accounts/{account_id}/tokens/verify`. O endpoint clássico `/user/tokens/verify` é para tokens de usuário e recusa os de conta.

**Correção.**

```bash
curl -s -X GET "https://api.cloudflare.com/client/v4/accounts/<account_id>/tokens/verify" \
  -H "Authorization: Bearer cfat_..."
```

---

## Cache negativo de DNS

**Sintoma.** `ERR_NAME_NOT_RESOLVED` no navegador para um registro recém-criado, mesmo com `1.1.1.1` respondendo corretamente.

**Causa.** Um resolver no caminho (roteador, provedor, ou o próprio SO) cacheou um `NXDOMAIN` de antes do registro existir. Cache negativo tem TTL próprio, muitas vezes maior que o do registro.

**Correção.** Diagnosticar comparando resolvers; contornar via arquivo `hosts` se houver pressa; no mais, esperar.

```bash
nslookup <host> 1.1.1.1        # se responde aqui e não na sua máquina, é cache
ipconfig /flushdns             # Windows
```

---

## Sudo com senha trava automação

**Sintoma.** `sudo: a password is required` no gate de validação do bootstrap.

**Causa.** O sudoers do `deploy` foi criado exigindo senha; o gate roda `sudo -n true` (não-interativo).

**Correção.** `NOPASSWD` para o `deploy`. O raciocínio sobre o trade-off está em [02 - Servidor e acesso](02-servidor-e-acesso.md).

---

## Padrões que se repetem

Olhando o conjunto, quatro categorias explicam quase tudo:

1. **Configuração ignorada em silêncio.** Helm com chave inexistente, pnpm com `overrides` no lugar errado, Ansible pulando task por `creates:`. Nada falha — só não acontece. Sempre confirme que a configuração *fez efeito*, não que foi *escrita*.

2. **O erro visível não é a causa.** Traefik reclamando de middleware inexistente enquanto o problema era SNAT; certificado falhando por rate limit cuja causa era um chart sobrescrevendo Secret. Compare comportamento esperado × observado antes de perseguir a última mensagem de log.

3. **Ecossistema em movimento.** Bitnami saindo do ar, repositórios mudando de org, pnpm mudando de formato de config. Ferramenta que funcionou ano passado pode não funcionar hoje — verifique URLs e formatos antes de fixá-los.

4. **Local ≠ remoto.** Módulos do Ansible que resolvem caminho no host errado, libs Python que precisam estar do outro lado, arquivos que existem no repositório mas não no contexto do Docker build. Sempre pergunte: *onde este código está rodando de fato?*
