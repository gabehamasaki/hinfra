# 03 — Kubernetes (k3s)

## O que está rodando

| | |
| --- | --- |
| Distribuição | k3s, canal `stable` |
| Versão atual | `v1.36.4+k3s1` |
| Runtime | containerd 2.3.4 |
| Node | `srv1957194` — control-plane, único |
| Datastore | SQLite embutido (padrão do k3s single-server) |
| CNI | flannel, sobre a interface `tailscale0` |
| Ingress | Traefik `v3.7.1` (chart `40.1.4`), embutido no k3s |
| Storage | `local-path` (provisioner embutido do k3s) |

## Por que k3s

Um control plane completo via kubeadm (etcd + apiserver + scheduler + controller-manager, cada um em seu pod, mais o CNI e o ingress instalados à parte) consumiria uma fatia grande de uma máquina de 2 vCPU / 8 GB antes de qualquer carga real subir. O k3s entrega o mesmo Kubernetes num binário só, já com Traefik, local-path provisioner e containerd embutidos.

O datastore SQLite é a escolha padrão para servidor único e não impede workers — só HA de control plane. Adicionar um segundo *server* (não agent) exigiria trocar para etcd embutido, o que é uma decisão para quando houver essa necessidade.

## Como o k3s foi instalado

```bash
curl -sfL https://get.k3s.io | \
  INSTALL_K3S_CHANNEL=stable \
  sh -s - server \
  --node-ip 100.86.241.1 \
  --advertise-address 100.86.241.1 \
  --flannel-iface tailscale0 \
  --tls-san 100.86.241.1 \
  --write-kubeconfig-mode 644
```

Cada flag tem um motivo:

| Flag | Por quê |
| --- | --- |
| `--node-ip` / `--advertise-address` | O node se identifica no cluster pelo IP da tailnet. Sem isso ele anunciaria o IP público, e workers futuros tentariam falar com ele por um caminho que não queremos usar. |
| `--flannel-iface tailscale0` | O tráfego entre pods de nodes diferentes passa pela mesh WireGuard, não pela internet. |
| `--tls-san` | Inclui o IP da tailnet no certificado da API — sem isso o `kubectl` recusaria a conexão por hostname mismatch. |
| `--write-kubeconfig-mode 644` | Permite que o Ansible, conectado como `deploy` (não-root), leia o kubeconfig para os roles de plataforma. |

Depois da instalação o role ajusta o MTU da interface `flannel.1` para `1280`, compatível com o overhead do WireGuard. Sem esse ajuste, pacotes grandes fragmentam de forma silenciosa e o sintoma é intermitente e difícil de diagnosticar — conexões que funcionam para payloads pequenos e travam para grandes.

## Storage

O `local-path` grava em disco local do node. Isso tem duas consequências que importam quando workers entrarem:

1. **Um PVC fica preso ao node onde foi criado.** Um pod que use aquele volume só pode ser agendado naquele node.
2. **`local-path` não suporta expansão de volume.** Aumentar um PVC exige recriar o volume e migrar os dados.

### Política adotada

- Aplicações **sem estado** podem rodar em qualquer node, incluindo workers futuros.
- Aplicações **com estado** ficam fixadas no node `server` via `nodeSelector` até existir storage distribuído.
- Como disco é o recurso abundante aqui (92 GB livres) e não é redimensionável, os PVCs já nascem com folga.

PVCs atuais:

| Namespace | PVC | Tamanho |
| --- | --- | --- |
| `postgres` | `postgres-1` | 4 Gi |
| `rustfs` | `rustfs-data` | 5 Gi |
| `rustfs` | `rustfs-logs` | 256 Mi |
| `valkey` | `valkey` | 512 Mi |

Storage distribuído (Longhorn, ou o modo `distributed` do próprio RustFS) fica fora de escopo enquanto houver um node só — Longhorn em particular é pesado demais para 2 vCPU.

## Backup do datastore

O estado inteiro do cluster — todos os manifestos, secrets, CRDs — vive num arquivo SQLite em `/var/lib/rancher/k3s/server/db/state.db`. Sem backup, perder esse arquivo é perder o cluster.

O role `k3s_backup` instala:

- **`/usr/local/bin/k3s-backup.sh`** — usa `sqlite3 .backup`, que faz cópia consistente de um banco **em uso**, sem parar o k3s. Copiar o arquivo com `cp` num banco ativo produz backup corrompido.
- **`k3s-backup.timer`** (systemd) — roda diariamente.
- **Retenção de 7 dias**, com os arquivos comprimidos em `/var/backups/k3s/`.

```bash
ssh vps 'sudo /usr/local/bin/k3s-backup.sh'                    # rodar sob demanda
ssh vps 'ls -la /var/backups/k3s/'                             # listar
ssh vps 'sudo systemctl list-timers k3s-backup.timer'          # conferir o agendamento
```

### Limitação atual

Os backups ficam **na própria VPS**. Isso protege contra corrupção do banco ou remoção acidental, mas não contra perda da máquina inteira. Levar as cópias para fora é o próximo passo natural — e o RustFS, já rodando no cluster, é o destino óbvio assim que estiver em uso.

## Adicionar um worker

O grupo `[k3s_agents]` já existe no inventário, vazio. O processo é o mesmo do primeiro servidor:

```bash
# 1. adicionar o host em ansible/inventory/hosts.ini sob [k3s_agents]
# 2. bootstrap (cria o deploy, fecha o root) — roda como root, uma única vez
ansible-playbook -i ansible/inventory/hosts.ini ansible/bootstrap.yml \
  --limit worker-1 -e ansible_user=root --private-key ~/.ssh/id_ed25519

# 3. provisiona (Tailscale + k3s agent) — já como deploy
ansible-playbook -i ansible/inventory/hosts.ini ansible/site.yml --limit worker-1
```

O role `k3s_agent` monta o join sozinho, lendo do host `[k3s_server]` (na mesma execução) o IP da tailnet e o node-token:

```bash
K3S_URL=https://<ip-tailnet-do-server>:6443 \
K3S_TOKEN=<node-token> \
sh -s - agent --node-ip <ip-tailnet-do-worker> --flannel-iface tailscale0
```

Por isso o `--limit` do passo 3 não pode excluir o server do inventário: o play do server precisa rodar antes (mesmo que sem mudanças) para publicar esses valores. Rodar o `site.yml` sem `--limit` é sempre seguro — é idempotente.

### O que revisar depois de adicionar um worker

- `kubectl get nodes` deve mostrar os dois como `Ready`.
- Cargas com estado precisam de `nodeSelector` fixando no server (ver Storage acima).
- Para o Postgres, escalar réplicas é mudar `instances: 1` para `3` em `data-services/postgres/cluster.yaml`.
- Para o Valkey, é `replica.enabled: true` no Application correspondente.
- Para o RustFS, o modo `distributed` precisa de pelo menos 2 nodes e faz erasure coding entre eles.
