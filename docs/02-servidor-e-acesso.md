# 02 — Servidor e acesso

## A máquina

| | |
| --- | --- |
| Provedor | Hostinger, plano KVM 2 |
| ID da VM (API Hostinger) | `<hostinger-vm-id>` |
| Hostname | `<node-hostname>` |
| IP público | `<VPS_PUBLIC_IP>` |
| IP na tailnet | `<TAILNET_IP>` |
| SO | Ubuntu 24.04.4 LTS (kernel 6.8.0) |
| CPU / RAM / Disco | 2 vCPU · 8 GB · 96 GB |

O hostname real (`<node-hostname>`) é o nome com que o node se registra no Kubernetes. O inventário do Ansible chama a mesma máquina de `vps-1` — são coisas diferentes, e confundir as duas já custou um playbook quebrado (ver [11 - Armadilhas](11-armadilhas.md)).

## Usuário `deploy` — por que root saiu de cena

A VPS chegou como a maioria chega: login SSH direto como `root`, com `PermitRootLogin yes` e autenticação por senha não desabilitada explicitamente. Nesse estado, qualquer uso de `ssh vps` é literalmente uma sessão root, e uma chave vazada — ou uma máquina de controle comprometida rodando Ansible — entrega a máquina inteira de imediato, sem nenhuma camada de auditoria no meio.

A correção custa pouco: não muda nada nos manifestos do Kubernetes, no cert-manager, no ArgoCD ou nos workflows de CI. Só afeta a camada SSH/Ansible.

O que existe hoje:

- Usuário `deploy`, no grupo `sudo`, com **par de chaves SSH próprio** — não reaproveita a chave que autenticava como root, então aquela chave pode ser aposentada.
- `PermitRootLogin no` e `PasswordAuthentication no`. Login só por chave, e nunca como root.
- `sudo` sem senha (`NOPASSWD`) para o `deploy`.

### Sobre o `NOPASSWD`

Foi uma decisão consciente com trade-off real. `sudo` com senha trava automação não-interativa: todo `ansible-playbook` exigiria digitar a senha, e o próprio bootstrap falhou nesse ponto na primeira execução. Com `NOPASSWD`, quem tem a chave do `deploy` consegue virar root — o mesmo raio de alcance de um login root direto.

A segurança real não vem daí, vem de três outras coisas:

1. **Root é inalcançável pela rede.** Toda ação privilegiada passa pelo `deploy` + `sudo`, o que fica registrado e é revogável removendo uma chave.
2. **Autenticação por senha está desligada.** Elimina a superfície de força bruta, que é o vetor real contra uma VPS exposta.
3. **`fail2ban` + `ufw limit` na porta 22.** Tentativas repetidas são bloqueadas.

Se um dia isso precisar ser mais rígido, o caminho é restringir o sudoers a comandos específicos em vez de voltar a exigir senha.

### O daemon do k3s continua rodando como root

Isso é inerente a qualquer runtime de containers — precisa de capabilities de kernel, iptables, montagem de volumes. Não é uma exceção à regra: a regra é que *nenhum humano e nenhuma automação faz login interativo como root*. Um daemon systemd rodando com privilégio é uma coisa diferente de uma sessão SSH root aberta para a internet.

## Bootstrap: o gate anti-lockout

Fechar o acesso root é irreversível pela própria conexão que você está usando. Se a chave do `deploy` não funcionar e o root já estiver bloqueado, o SSH acabou.

O `ansible/bootstrap.yml` é dividido em três plays justamente por causa disso:

1. **Cria** o usuário `deploy`, instala a chave, configura o sudoers. Ainda não mexe no `sshd`.
2. **Valida** — abre uma conexão SSH nova, autenticada como `deploy`, e roda `sudo -n true`. Se falhar, o playbook para aqui com uma mensagem explícita.
3. **Fecha** o root — só executa se o play 2 passou.

Antes de rodar isso pela primeira vez, duas redes de segurança adicionais:

- **Console VNC/recovery da Hostinger** confirmado funcionando. É o acesso fora-de-banda se o SSH travar.
- **Snapshot da VPS** tirado pela API da Hostinger.

```bash
# snapshot antes de qualquer mudança arriscada (sobrescreve o snapshot anterior)
# feito via MCP/API da Hostinger com virtualMachineId=1957194
```

## Firewall

Duas camadas, com propósitos distintos.

### `ufw`, no host (gerido pelo Ansible)

```
22/tcp    LIMIT   Anywhere              # rate-limit contra força bruta
80/tcp    ALLOW   173.245.48.0/20       # ... uma regra por range da Cloudflare
443/tcp   ALLOW   173.245.48.0/20       #     (15 ranges IPv4 + 7 IPv6 = 44 regras)
Anywhere on tailscale0  ALLOW           # tráfego de cluster e API
```

Política padrão: `deny (incoming)`, `allow (outgoing)`, `deny (routed)`.

A porta `6443` (API do Kubernetes) **não aparece na lista** — e é esse o ponto. Ela só é alcançável através da interface `tailscale0`, coberta pela regra de interface. Da internet pública, a porta simplesmente não responde.

**80/443 só aceitam tráfego da Cloudflare.** É isso que impede alguém de contornar o proxy batendo direto no IP de origem: sem essa restrição, o WAF, o rate limit e o anti-DDoS da Cloudflare seriam decoração, porque a origem continuaria alcançável por quem descobrisse o IP (e um IP público é descoberto por varredura, não por DNS). A lista de ranges é versionada em `group_vars/all/vars.yml` como `cloudflare_ip_ranges_v4`/`_v6`, e o role `common` a confere contra `https://api.cloudflare.com/client/v4/ips` **e falha o playbook se divergir** — uma mudança da Cloudflare tem que aparecer como erro, não como site fora do ar para parte dos visitantes.

A consequência operacional: qualquer host público novo **precisa** estar proxied na Cloudflare. Um registro em "DNS only" apontando para o IP público fica inalcançável.

### `fail2ban`

O pacote é instalado desde o início, mas até haver `/etc/fail2ban/jail.local` ele não banіa nada. A jail `sshd` usa `backend = systemd` porque o Ubuntu manda o log do sshd para o journal, não para o `/var/log/auth.log` que o jail padrão do pacote espera — sem isso a jail sobe, aparece no `fail2ban-client status` e nunca bane ninguém. O `bantime` cresce a cada reincidência (fator 2, teto de uma semana), e `ignoreip` inclui `100.64.0.0/10`: banir a própria tailnet custaria o acesso administrativo ao cluster inteiro.

```bash
sudo fail2ban-client status sshd    # confirma que a jail está ativa e lendo do journal
```

Essa configuração é portável: `ufw` funciona em qualquer VPS de qualquer provedor, o que atende ao requisito de replicabilidade.

### Firewall da Hostinger (opcional, não gerido pelo Ansible)

A API da Hostinger permite regras a nível de hipervisor, que filtram antes do pacote chegar na VM. É uma camada extra legítima, mas **deliberadamente fora do Ansible**: seria específica da Hostinger e quebraria a portabilidade do repositório. Se for usada, é configuração manual documentada à parte.

## Tailscale — a rede entre servidores

O problema: workers futuros precisam alcançar a porta `6443` do control plane. Expor essa porta na internet, mesmo restrita por IP, é uma superfície que não vale a pena — e a Hostinger não garante rede privada entre VPS arbitrárias.

A solução é uma mesh WireGuard: cada servidor entra na mesma tailnet e ganha um IP na faixa `100.64.0.0/10`. O k3s escuta a API e roda o tráfego de cluster **apenas nessa interface**.

Configuração aplicada pelo role `tailscale`:

```bash
tailscale up --reset \
  --authkey=<do vault> \
  --hostname=<nome no inventário> \
  --advertise-tags=tag:k8s \
  --ssh=false
```

Detalhes que importam:

- **`--reset`** é obrigatório na prática. Sem ele, o `tailscale up` recusa a execução se qualquer flag não-default divergir do estado salvo, com um erro que só aparece na segunda execução.
- **`--advertise-tags=tag:k8s`** exige que a tag exista em `tagOwners` na política de ACL do admin console do Tailscale **antes** de qualquer authkey tentar usá-la. Sem isso o join falha com `requested tags are invalid or not permitted`.
- **`--ssh=false`** — o Tailscale SSH ficaria sobreposto ao acesso SSH normal com o usuário `deploy`. Um caminho de acesso só.
- A authkey deve ser **reutilizável e sem expiração**. Chaves com expiração padrão (90 dias) derrubam nodes do cluster quando vencem.
- **IP forwarding** (`net.ipv4.ip_forward` e `net.ipv6.conf.all.forwarding`) é habilitado pelo role — o flannel precisa disso para rotear tráfego de pods sobre a tailnet.

Estado atual da tailnet:

| Node | IP | O que é |
| --- | --- | --- |
| `vps-1` | `<TAILNET_IP>` | A VPS |
| `<tailscale-device>` | `<DEV_TAILNET_IP>` | Máquina local de desenvolvimento |

## DNS

Os dois domínios são registrados na Hostinger, mas o DNS autoritativo é o **Cloudflare** (nameservers `eva`/`woz.ns.cloudflare.com`).

| Zona | Zone ID | Uso |
| --- | --- | --- |
| `<infra_domain>` | `<CF_ZONE_INFRA>` | Serviços de plataforma |
| `<apps_domain>` | `<CF_ZONE_APPS>` | Projetos |

### Registros que importam

| Registro | Aponta para | Proxy | Alcance |
| --- | --- | --- | --- |
| `<apps_domain>` (apex) | `<VPS_PUBLIC_IP>` (público) | **Sim** | Internet, via Cloudflare |
| `schedule-visits.<apps_domain>` | `<VPS_PUBLIC_IP>` (público) | **Sim** | Internet, via Cloudflare |
| `study.<apps_domain>` | `<TAILNET_IP>` (tailnet) | Não | Só tailnet |
| `argocd.<infra_domain>` | `<TAILNET_IP>` (tailnet) | Não | Só tailnet |
| `grafana.<infra_domain>` | `<TAILNET_IP>` (tailnet) | Não | Só tailnet |
| `s3.<infra_domain>` | `<TAILNET_IP>` (tailnet) | Não | Só tailnet |

Os hosts públicos são **proxied** (nuvem laranja): o IP da VPS deixa de aparecer no DNS e o tráfego passa pelo WAF, pelo rate limit e pela mitigação de DDoS da Cloudflare. Num node de 2 vCPU isso não é conforto, é a única defesa que funciona — rejeitar um flood localmente ainda gastaria CPU da própria VPS.

Os hosts da tailnet **não podem** ser proxied: a Cloudflare não aceita um IP `100.64.0.0/10` como origem. Continuam em "DNS only", protegidos por não serem roteáveis.

Configuração da zona dos apps, junto com o proxy:

| Ajuste | Valor | Por quê |
| --- | --- | --- |
| SSL/TLS | **Full (strict)** | Com `Flexible` a Cloudflare fala HTTP com a origem; qualquer redirect HTTP→HTTPS na origem viraria loop infinito |
| Always Use HTTPS | On | Redirect 301 feito na borda |
| WAF Managed Free Ruleset | Implantado | Existir na zona não basta: sem uma regra `execute` na fase `http_request_firewall_managed`, o ruleset não é executado |
| Rate limiting | 100 req / 10s por IP | O plano Free só permite período de 10s (a API recusa 60s com `not entitled to use the period 60`) |

Os registros de e-mail de `<apps_domain>` (MX, SPF, DKIM, DMARC, apontando para o Hostinger Mail) **não foram tocados** e não devem ser.

### O padrão "registro público apontando para IP privado"

Parece contraditório, mas é deliberado e é o que evita ter que editar `/etc/hosts` em cada dispositivo.

Um IP na faixa `100.64.0.0/10` é CGNAT — **não é roteável pela internet**. Publicá-lo no DNS não expõe nada: quem não está na tailnet resolve o nome, tenta conectar e não chega a lugar nenhum. Quem está na tailnet resolve o mesmo nome e chega direto.

A proteção real vem de duas camadas independentes:

1. O IP não é alcançável fora da tailnet.
2. O middleware `IPAllowList` do Traefik responde `403` para qualquer origem fora de `100.64.0.0/10`.

O custo é que o hostname fica visível em consultas DNS públicas. Quem quiser esconder isso também precisa de Split DNS no Tailscale, o que exige rodar um resolver dentro da tailnet — complexidade que não se pagou aqui.

### Propagação de DNS

Ao criar um registro novo, o Cloudflare já responde na hora nos servidores autoritativos, mas o resolver da sua rede pode demorar — especialmente se ele tiver cacheado um "esse nome não existe" (cache negativo) de uma consulta anterior, que costuma ter TTL próprio e mais longo.

Para diagnosticar:

```bash
nslookup argocd.<infra_domain> 1.1.1.1     # consulta direto o resolver da Cloudflare
```

Se o `1.1.1.1` responde certo e a sua máquina não, é cache local ou do provedor. `ipconfig /flushdns` no Windows resolve o lado local; o resto é esperar. Editar o arquivo `hosts` funciona como contorno imediato, mas não deveria virar permanente.

## Acessando

### SSH

```bash
ssh vps          # ~/.ssh/config aponta para deploy@<VPS_PUBLIC_IP> com a chave dedicada
```

```
Host vps
   HostName <VPS_PUBLIC_IP>
   User deploy
   Port 22
   IdentityFile "~/www/hinfra/.secrets/vps-1_deploy_ed25519"
```

### kubectl

Exige estar na tailnet — o kubeconfig aponta para o IP `<TAILNET_IP>`, não para o público.

```bash
tailscale up                                                # se ainda não estiver conectado
export KUBECONFIG=~/www/hinfra/.secrets/vps-1.kubeconfig
kubectl get nodes
```

O kubeconfig é baixado automaticamente pelo role `k3s_server` a cada execução do `site.yml`, já com o endereço do servidor reescrito de `127.0.0.1` para o IP da tailnet. Ele fica em `.secrets/`, que está no `.gitignore`.

O role também renomeia cluster, contexto e usuário de `default` (o padrão do k3s) para o nome do host no inventário. Além de `default` ser inútil como nome em ferramentas gráficas, dois clusters chamados `default` colidiriam ao serem mesclados num kubeconfig só — o que quebraria assim que existisse um segundo cluster.

### Ferramentas gráficas no Windows (Lens)

Num ambiente WSL há duas fronteiras a atravessar, e as duas importam:

1. **O Lens roda no Windows** e não enxerga o filesystem do WSL — precisa do kubeconfig em `C:\Users\<usuário>\.kube\config`.
2. **O kubeconfig aponta para um IP da tailnet**, então o **Tailscale precisa estar conectado no Windows**. O Tailscale do WSL não serve: são dispositivos distintos na tailnet.

```bash
./scripts/sync-kubeconfig-windows.sh
```

O script descobre o usuário do Windows, copia o kubeconfig e avisa se o Tailscale de lá não estiver conectado. Se já existir um `config` no Windows, ele **mescla** em vez de sobrescrever, preservando outros clusters — com o arquivo do repositório tendo precedência, para que recriar um cluster atualize a entrada em vez de manter a antiga.

Verificar se o Windows alcança a API antes de abrir o Lens:

```bash
'/mnt/c/Windows/System32/curl.exe' -sk -o /dev/null -w "%{http_code}\n" https://<TAILNET_IP>:6443/version
```

`401` é a resposta certa — significa que a API respondeu e só faltou credencial. Timeout significa que o Tailscale do Windows está desconectado.

> **Métricas.** O cluster tem **Prometheus + Grafana** na camada de plataforma (`monitoring`), acessível na tailnet em `https://grafana.<infra_domain>` — ver [13 - Observabilidade](13-observabilidade.md). O Lens pode usar o mesmo Prometheus para gráficos; **não** instale outro stack pelo botão do Lens. Para números pontuais sem abrir o Grafana, `kubectl top` e `hinfra metrics` usam o metrics-server.
