# 06 — TLS e Ingress

## Traefik

Vem embutido no k3s, instalado via `HelmChart` do próprio k3s no namespace `kube-system`. Versão atual: `v3.7.1` (chart `traefik-40.1.4`). É o único ingress controller do cluster — não faz sentido adicionar nginx-ingress ao lado num node só.

Ele atende **todos** os hosts na mesma porta 443, tanto na interface pública quanto na `tailscale0`. A separação entre "público" e "só tailnet" não é feita por porta ou por listener, e sim por um middleware de allowlist de IP. Isso torna a configuração desse middleware parte crítica da segurança, não um detalhe.

### Como customizar o chart embutido

O k3s instala o Traefik via `HelmChart` próprio, então não há release Helm para o Ansible gerenciar. A forma suportada de mudar values é um **`HelmChartConfig`** com o mesmo nome e namespace, que o k3s faz merge com os values dele. Fica em `platform/traefik/helmchartconfig.yaml.j2`, aplicado pelo role `traefik`:

| Value | Para quê |
| --- | --- |
| `service.spec.externalTrafficPolicy: Local` | Sem isso o `IPAllowList` responde 403 para todos (ver [11 - Armadilhas](11-armadilhas.md)) |
| `ports.web/websecure.forwardedHeaders.trustedIPs` | Ranges da Cloudflare — só deles o Traefik aceita `X-Forwarded-For` |
| `logs.access.enabled: true` + `format: json` | Access log, com `Cf-Connecting-Ip` e `Cf-Ray` preservados |
| `ports.websecure.http.middlewares` | Middlewares aplicados a **toda** requisição na 443 |

Duas armadilhas de nomenclatura aqui, ambas do tipo que falha em silêncio:

- O caminho é **`ports.websecure.http.middlewares`**, não `ports.websecure.middlewares`. A segunda forma não existe no chart, e o Helm aceita chave desconhecida sem reclamar.
- Não existe `ports.web.redirectTo`. Redirect por entrypoint é `ports.web.http.redirections.entryPoint`. Aqui não usamos nenhum dos dois: o redirect HTTP→HTTPS é feito pela Cloudflare (`Always Use HTTPS`), antes de gastar recurso da origem.

O role move o `externalTrafficPolicy` do patch imperativo que existia no role `argocd` para os values. Manter os dois seria pedir para o Helm reverter o patch numa reinstalação.

Depois de mudar values, **confirme que pegou** — o helm-controller do k3s roda um Job de upgrade e só depois recria o pod, então checar "Deployment disponível" dá falso positivo enquanto o Traefik antigo ainda serve:

```bash
kubectl -n kube-system get deploy traefik \
  -o jsonpath='{.spec.template.spec.containers[0].args}' | tr ',' '\n' | grep -i forwarded
```

### Middlewares compartilhados

Dois, em `kube-system` (`platform/traefik/middlewares.yaml.j2`):

- **`security-headers`** — HSTS, `nosniff`, `frameDeny`, `Referrer-Policy`. Aplicado **globalmente** no entrypoint `websecure`, porque é inofensivo para qualquer origem.
- **`public-rate-limit`** — aplicado **só por annotation, em Ingress público**. Nunca global, e isso não é preferência: o middleware identifica o cliente pelo header `CF-Connecting-IP`, e requisição sem esse header (todo o tráfego da tailnet) cai num único bucket compartilhado. Global, os consoles começariam a tomar `429` uns por causa dos outros.

```yaml
traefik.ingress.kubernetes.io/router.middlewares: "kube-system-public-rate-limit@kubernetescrd"
```

O rate limit da Cloudflare (100 req/10s por IP) é mais estrito e age antes, na borda. O do Traefik é a rede de segurança para o caso de algo chegar à origem por outro caminho.

### `externalTrafficPolicy: Local` — sem isso a allowlist não funciona

Por padrão o Service do Traefik nasce com `externalTrafficPolicy: Cluster`, e o kube-proxy aplica SNAT no tráfego que entra pelo LoadBalancer. O Traefik enxerga o IP interno do cluster como origem, **não o IP real do cliente**.

O efeito: o middleware `IPAllowList` compara um IP da faixa `10.42.0.0/16` (rede de pods) contra `100.64.0.0/10` (tailnet), não bate nunca, e responde `403` **para todo mundo** — inclusive para quem está legitimamente na tailnet. O sintoma é confuso porque a regra parece estar funcionando (bloqueia o público) enquanto na verdade está quebrada (bloqueia todos).

O role `argocd` corrige isso:

```yaml
- name: Traefik - externalTrafficPolicy Local
  kubernetes.core.k8s:
    state: patched
    kind: Service
    name: traefik
    namespace: kube-system
    definition:
      spec:
        externalTrafficPolicy: Local
```

Com `Local`, o kube-proxy não mascara a origem e o IP real chega ao Traefik. Num cluster de node único não há custo — o trade-off normal dessa flag (perder balanceamento entre nodes) só aparece com múltiplos nodes.

## O middleware tailnet-only

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: argocd-tailnet-only
  namespace: argocd
spec:
  ipAllowList:
    sourceRange:
      - 100.64.0.0/10     # faixa CGNAT usada pelo Tailscale
```

Aplicado em um Ingress pela annotation:

```yaml
traefik.ingress.kubernetes.io/router.middlewares: "argocd-argocd-tailnet-only@kubernetescrd"
```

O formato é `<namespace>-<nome>@kubernetescrd`. Como o namespace faz parte da referência, o middleware é **reutilizável entre namespaces** — o Ingress do RustFS, no namespace `rustfs`, aponta para esse mesmo objeto que vive em `argocd`. Não precisa duplicar o recurso por namespace.

Quem usa hoje:

| Ingress | Host | Regra |
| --- | --- | --- |
| `argocd-server` | `argocd.<infra_domain>` | Tailnet-only |
| `rustfs` | `s3.<infra_domain>` | Tailnet-only |
| `my-portfolio` | `<apps_domain>` | Público, sem middleware |

### Duas camadas, não uma

O middleware não é a única proteção. Os hosts tailnet-only resolvem para `<TAILNET_IP>`, um IP CGNAT que não é roteável pela internet — quem está fora da tailnet nem chega a fazer a requisição. O middleware é a segunda camada, para o caso de alguém alcançar o Traefik pelo IP público forçando o header `Host`.

Verificação:

```bash
# de dentro da tailnet — espera 200
curl -s -o /dev/null -w "%{http_code}\n" \
  --resolve argocd.<infra_domain>:443:<TAILNET_IP> https://argocd.<infra_domain>/

# forçando pelo IP público — espera 403
curl -s -o /dev/null -w "%{http_code}\n" -k \
  --resolve argocd.<infra_domain>:443:<VPS_PUBLIC_IP> https://argocd.<infra_domain>/
```

## cert-manager

| | |
| --- | --- |
| Versão | `v1.21.1` |
| Namespace | `cert-manager` |
| Repo Helm | `https://charts.jetstack.io` |
| ClusterIssuer | `letsencrypt-cloudflare` |

### Por que DNS-01 e não HTTP-01

O desafio HTTP-01 exige que o Let's Encrypt alcance a porta 80 do host. Isso funcionaria para `<apps_domain>`, mas **não** para `argocd.<infra_domain>` — que, por definição, não é alcançável de fora.

O DNS-01 prova a posse do domínio criando um registro TXT via API do Cloudflare. Não depende de reachability nenhuma, o que permite emitir certificado Let's Encrypt válido para serviços que só existem dentro da tailnet. Como bônus, suportaria wildcard se um dia for necessário.

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-cloudflare
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: you@example.com
    privateKeySecretRef:
      name: letsencrypt-cloudflare-account-key
    solvers:
      - dns01:
          cloudflare:
            apiTokenSecretRef:
              name: cloudflare-api-token
              key: api-token
        selector:
          dnsZones:
            - "<infra_domain>"
            - "<apps_domain>"
```

O token do Cloudflare é um **Account API Token** (prefixo `cfat_`), com permissão `Zone:DNS:Edit` escopada nas duas zonas.

> Tokens desse tipo se verificam em `/accounts/{account_id}/tokens/verify`, **não** no endpoint clássico `/user/tokens/verify` — que responde `Invalid API Token` para um token perfeitamente válido, o que confunde bastante o diagnóstico.

### Como um certificado é emitido

O cert-manager opera por *ingress-shim*: ao ver a annotation `cert-manager.io/cluster-issuer` num Ingress com `tls:`, ele cria sozinho o `Certificate`, o `CertificateRequest`, o `Order` e o `Challenge`, e no fim grava o Secret nomeado no `tls.secretName`.

```bash
kubectl get certificate -A                       # visão geral
kubectl describe certificate <nome> -n <ns>      # motivo quando READY=False
kubectl get order,challenge -A                   # detalhe do desafio ACME
```

Certificados ativos:

| Namespace | Certificate | Host |
| --- | --- | --- |
| `argocd` | `argocd-server-tls` | `argocd.<infra_domain>` |
| `my-portfolio` | `my-portfolio-tls` | `<apps_domain>` |
| `rustfs` | `rustfs-tls` | `s3.<infra_domain>` |

## Rate limits do Let's Encrypt

O limite que importa aqui: **5 certificados por semana para o mesmo conjunto exato de hostnames**. Ele conta emissões bem-sucedidas, em janela móvel de 168 horas.

Esse limite já foi atingido nesta infra, no host `s3.<infra_domain>`, por um loop de reemissão — o chart do RustFS criava um Secret TLS com conteúdo inválido, o cert-manager detectava a corrupção e reemitia, o ArgoCD sincronizava o Secret inválido de volta, e o ciclo se repetia até estourar o limite. O relato completo está em [11 - Armadilhas](11-armadilhas.md).

Duas lições operacionais:

1. **Qualquer chart que ofereça "gerenciar o próprio TLS" precisa ser explicitamente desligado** quando o cert-manager é quem deve mandar. Ter só a annotation não basta se o chart também renderiza um Secret com o mesmo nome.
2. **Para testar mudanças de emissão, use o servidor de staging do Let's Encrypt** (`https://acme-staging-v02.api.letsencrypt.org/directory`), que tem limites muito mais folgados. Certificados de staging não são confiáveis pelo navegador, mas provam que o fluxo funciona.

Quando o limite é atingido, não há o que fazer além de esperar a janela passar — o cert-manager continua tentando sozinho, com backoff, e emite assim que liberar.
