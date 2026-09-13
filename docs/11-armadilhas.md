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

## O SNAT do Tailscale quebra o IPAllowList (e um `apt upgrade` liga isso sozinho)

**Sintoma.** Idêntico ao caso acima e por isso especialmente confuso: `403` em **todos** os consoles, inclusive de dentro da tailnet, com o `externalTrafficPolicy: Local` corretamente aplicado. O access log do Traefik mostra `ClientHost: 10.42.0.1` — o endereço da bridge `cni0`, ou seja, o próprio node mascarando.

**Causa.** O `tailscaled` marca o pacote que entra pela tailnet e a regra `ts-postrouting` o mascara na saída para a rede de pods:

```
-A POSTROUTING -j ts-postrouting
-A ts-postrouting -m mark --mark 0x40000/0xff0000 -j MASQUERADE
```

Como `ts-postrouting` é a **primeira** cadeia do `POSTROUTING`, ela age antes de qualquer regra do kube-proxy ou do flannel. O `--snat-subnet-routes` do Tailscale é `true` por padrão e existe para subnet routes — que esta VPS **não anuncia** (`AdvertiseRoutes: null`). O SNAT não serve a nenhum propósito aqui e só destrói o IP de origem.

**O gatilho.** Um `apt upgrade` do role `common` atualizou o Tailscale (1.102.3 → 1.102.4) e reiniciou o `tailscaled`. Nada no Kubernetes mudou — o que muda de comportamento é a pilha de netfilter do Tailscale. É o tipo de regressão que aparece horas depois, sem relação óbvia com o último `kubectl apply`.

**Correção.**

```bash
sudo tailscale set --snat-subnet-routes=false
```

Aplicado de forma permanente pelo role `tailscale`, em **duas** tasks: a flag no `tailscale up` (para node novo) e um `tailscale set` idempotente (porque o `tailscale up` é pulado em node que já está na tailnet).

**Como diagnosticar isso em 3 comandos**, sem reler cadeias inteiras de iptables:

```bash
# 1. o pacote chega com o IP certo e sai reescrito? (mesma porta de origem = mesma conexão)
sudo tcpdump -ni tailscale0 'tcp port 443 and tcp[tcpflags] & tcp-syn != 0'
sudo tcpdump -ni cni0 'dst port 8443 and tcp[tcpflags] & tcp-syn != 0'

# 2. qual regra de NAT incrementa? compare o contador antes e depois de gerar tráfego
sudo iptables -t nat -L ts-postrouting -v -n

# 3. o SNAT está ligado?
sudo tailscale debug prefs | grep -i NoSNAT     # NoSNAT: false = SNAT ligado
```

**Lição operacional.** A allowlist de IP é a segunda camada e falha **fechada** (403 para todos), não aberta — nenhum acesso indevido foi liberado. A primeira camada, DNS apontando para um IP CGNAT não roteável, é a que sustenta a proteção enquanto isso. Não vale relaxar a allowlist (adicionando `10.42.0.0/16`, por exemplo) para contornar o sintoma: qualquer requisição que alcance o Traefik pelo IP público com o `Host` forçado apareceria com esse mesmo IP e passaria.

---

## SSL "Flexible" da Cloudflare + redirect na origem = loop infinito

**Sintoma.** `ERR_TOO_MANY_REDIRECTS` no navegador, e a origem parecendo saudável em todos os testes locais.

**Causa.** Com o modo SSL da zona em `Flexible`, a Cloudflare termina o TLS na borda e fala **HTTP** com a origem. Se a origem redireciona HTTP→HTTPS, ela devolve um 301 para HTTPS, a Cloudflare volta a buscar em HTTP, e o ciclo não termina.

**Correção.** `Full (strict)`, que é o correto aqui de qualquer forma: a origem tem certificado Let's Encrypt válido, emitido por DNS-01. E não configurar redirect na origem — quem faz isso é o `Always Use HTTPS` da Cloudflare, antes de gastar recurso da VPS.

Vale notar que `full` e `strict` são valores diferentes na API (`/zones/{id}/settings/ssl`): `full` não valida o certificado da origem.

---

## Rate limit sem `sourceCriterion` conta todo mundo no mesmo bucket

**Sintoma.** Um visitante ruidoso gera `429` para todos os outros.

**Causa.** Por padrão o `rateLimit` do Traefik conta por IP de origem da conexão TCP. Com a Cloudflare na frente, esse IP é **sempre** um IP dela — então todos os visitantes dividem o mesmo contador.

**Correção.** `sourceCriterion.requestHeaderName: Cf-Connecting-Ip`, que identifica o cliente real. O efeito colateral disso é o motivo de o middleware não ser global: requisição **sem** esse header (todo o tráfego da tailnet) também cai num bucket único compartilhado. Por isso ele é aplicado só por annotation, em Ingress público.

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

**Causa.** O k3s registra o node pelo hostname do SO (`<node-hostname>`). O inventário do Ansible o chama de `vps-1`.

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

## Onboarding monorepo / CI / Argo (hinfra)

Problemas recorrentes ao subir projetos api+web — detalhes e ordem de deploy em [09 - CI/CD](09-cicd.md).

| Sintoma | Causa | Correção |
| --- | --- | --- |
| CI `lstat docker: no such file` | `file` do build-push relativo ao `context` | `file` na raiz do repo (`backend/docker/Dockerfile`) |
| `kubeseal`: `sealed-secrets-controller` not found | controller neste cluster chama-se `sealed-secrets` em `kube-system` | flags `--controller-name` / `--controller-namespace` ou `hinfra seal secret` |
| `-app.yaml` no Git, app não no Argo | `root-app` não sincronizou o diretório | `hinfra argocd refresh --root` |
| Sync `Running` / pods com `:latest` | operação antiga + manifesto base | terminar op; sync na revision do commit do CI |
| `ImagePullBackOff` 403 | GHCR nasce privado | Package settings → public ou `imagePullSecret` |
| API `Progressing` / probes 500 | migration depois dos probes | Job `PreSync`; probes leves até DB migrado |
| MCP `cwd não está em um repositório git` | agent aberto no repo `infra` | `projectDir` absoluto do projeto nas tools MCP |

---

## Prune apagou um release que o ArgoCD não instalou

**Sintoma.** O Application `monitoring` ficou `Synced`/`Healthy` — e o `kube-prometheus-stack` sumiu do cluster: Grafana, operator, kube-state-metrics, node-exporter e Alertmanager deletados. Restaram só o Prometheus e os manifests do próprio Application.

**Causa.** O Application chegou a ter o chart como source. Nesse período o ArgoCD carimbou `argocd.argoproj.io/tracking-id` nos recursos — inclusive nos que o **Helm** tinha criado. Ao reduzir o desired state para só os manifests extras, esses recursos viraram "tracked mas ausentes do Git", e `syncPolicy.automated.prune: true` fez exatamente o que promete.

**Correção.** `prune: false` no Application antes de encolher o desired state. Depois `helm upgrade --install` recria tudo; o release e os Secrets sobrevivem porque não são tocados pelo prune. Recursos recriados pelo Helm nascem sem o `tracking-id`, o que encerra o risco.

**Detalhe que atrasa a correção.** O `platform-root` desfaz qualquer `kubectl patch` no Application em ~3 min pelo `selfHeal` — a correção precisa ir para o Git. E o repo-server pode continuar resolvendo `main` para um commit antigo mesmo com refresh `hard`: é cache de refs no Redis. `kubectl rollout restart deployment argocd-redis -n argocd` resolve.

---

## Chart Helm grande não renderiza no repo-server

**Sintoma.** Application em `Unknown` com `failed to generate manifest ... context deadline exceeded` ou `not a valid chart repository or cannot be reached`.

**Causa.** O `helm pull` baixa o `index.yaml` inteiro do repositório antes do chart. O do `prometheus-community` tem 6,3 MB e esta VPS recebe do GitHub Pages a ~126 KB/s — só o índice leva ~50 s, e o helm embutido no repo-server desiste em 120 s. Não é DNS, não é IPv6 (o pod só tem `::1`) e não é CPU: o mesmo `helm pull` contra `ghcr.io` leva 3 s.

**Correção.** Usar origem que dispense o `index.yaml` — chart espelhado como OCI, ou versionado no próprio repo. Aumentar `ARGOCD_EXEC_TIMEOUT` não resolve: o timeout de 120 s é do cliente HTTP dentro do helm.

---

## Padrões que se repetem

Olhando o conjunto, quatro categorias explicam quase tudo:

1. **Configuração ignorada em silêncio.** Helm com chave inexistente, pnpm com `overrides` no lugar errado, Ansible pulando task por `creates:`. Nada falha — só não acontece. Sempre confirme que a configuração *fez efeito*, não que foi *escrita*.

2. **O erro visível não é a causa.** Traefik reclamando de middleware inexistente enquanto o problema era SNAT; certificado falhando por rate limit cuja causa era um chart sobrescrevendo Secret. Compare comportamento esperado × observado antes de perseguir a última mensagem de log.

3. **Ecossistema em movimento.** Bitnami saindo do ar, repositórios mudando de org, pnpm mudando de formato de config. Ferramenta que funcionou ano passado pode não funcionar hoje — verifique URLs e formatos antes de fixá-los.

4. **Local ≠ remoto.** Módulos do Ansible que resolvem caminho no host errado, libs Python que precisam estar do outro lado, arquivos que existem no repositório mas não no contexto do Docker build. Sempre pergunte: *onde este código está rodando de fato?*
