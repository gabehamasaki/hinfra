# 04 — Ansible

O Ansible é o que torna o repositório replicável: rodar os playbooks contra uma VPS nova — de qualquer provedor — produz o mesmo resultado, sem passo manual e sem conhecimento tribal.

## Estrutura

```
ansible/
  ansible.cfg              inventário padrão, become, vault password file
  requirements.yml         collections necessárias
  bootstrap.yml            roda 1× por servidor, como root — cria o deploy e fecha o root
  site.yml                 provisionamento principal — idempotente, roda quantas vezes quiser
  inventory/
    hosts.ini              [k3s_server] / [k3s_agents] / [k3s_cluster:children]
  group_vars/all/
    vars.yml               variáveis não-sensíveis
    vault.yml              segredos, criptografado com ansible-vault (commitado)
    vault.yml.example       modelo sem valores reais
  roles/
    common                  apt, ufw, fail2ban, libs Python do k8s
    tailscale               instala e entra na tailnet
    k3s_server              instala o k3s server, baixa o kubeconfig
    k3s_agent               instala o k3s agent (workers)
    k3s_backup              backup diário do datastore SQLite
    sealed_secrets          controller do Sealed Secrets
    cert_manager            cert-manager + ClusterIssuer
    argocd                  ArgoCD, middleware, credencial de repo, root-app
    keda                    KEDA (autoscaling)
    observability_secrets   Secrets do monitoring (Grafana/Discord); stack via ArgoCD
    cnpg_operator           operador do CloudNativePG
    data_services_secrets   credenciais de Postgres/Valkey/RustFS
```

## Preparar a máquina local

```bash
./scripts/setup-local-tools.sh    # ansible, gh, tailscale, kubeseal
gh auth login
sudo tailscale up
```

O script também instala as collections de `requirements.yml`:

```yaml
collections:
  - name: kubernetes.core     # módulos helm / k8s / k8s_info
  - name: community.general   # ufw
  - name: community.crypto    # geração do par de chaves SSH
  - name: ansible.posix       # authorized_key, sysctl
```

## Os playbooks

### `bootstrap.yml` — uma vez por servidor

Autenticado como `root`, é a única vez que root é usado.

```bash
ansible-playbook -i inventory/hosts.ini bootstrap.yml \
  --limit <host> -e ansible_user=root --private-key ~/.ssh/id_ed25519
```

Três plays, nessa ordem: cria o `deploy` → valida login e sudo numa conexão nova → só então fecha o root. O detalhe do gate anti-lockout está em [02 - Servidor e acesso](02-servidor-e-acesso.md).

> **Atenção ao `-e ansible_user=root`.** O inventário define `ansible_user=deploy` para o grupo `k3s_cluster`, e variáveis de inventário têm precedência **maior** que a flag `-u` da linha de comando. Só `-e` (extra vars) sobrescreve. Usar `-u root` aqui falha silenciosamente tentando conectar como `deploy`.

### `site.yml` — sempre que precisar

```bash
ansible-playbook -i inventory/hosts.ini site.yml \
  --private-key ~/www/hinfra/.secrets/vps-1_deploy_ed25519

ansible-playbook -i inventory/hosts.ini site.yml --limit worker-1   # um host só
```

Quatro plays, por grupo:

1. `k3s_cluster` → `common`, `tailscale`
2. `k3s_server` → `k3s_server`, `k3s_backup`
3. `k3s_agents` → `k3s_agent`
4. `k3s_server` → Helm (pre-task) → `sealed_secrets`, `cert_manager`, `argocd`, `keda`, `observability_secrets`, `cnpg_operator`, `data_services_secrets`

O play 4 roda com `KUBECONFIG=/etc/rancher/k3s/k3s.yaml` no environment, para que os módulos `kubernetes.core.*` encontrem o cluster.

É totalmente idempotente — rodar de novo sem mudanças resulta em `changed=0`.

## Detalhes que não são óbvios

### `template` e `copy` leem local; `kubernetes.core.k8s` lê remoto

Os módulos `ansible.builtin.template` e `ansible.builtin.copy` resolvem o `src:` **na máquina de controle** e escrevem o `dest:` no host remoto. Já o `kubernetes.core.k8s` executa como um script *dentro* do host remoto, então o `src:` dele precisa ser um caminho que existe **lá**.

Por isso o padrão em todos os roles de plataforma é em dois passos:

```yaml
- name: Renderizar o ClusterIssuer
  ansible.builtin.template:              # lê local, escreve remoto
    src: "{{ playbook_dir }}/../platform/cert-manager/cluster-issuer.yaml.j2"
    dest: /tmp/cluster-issuer.yaml

- name: Aplicar o ClusterIssuer
  kubernetes.core.k8s:                   # lê o arquivo no host remoto
    state: present
    src: /tmp/cluster-issuer.yaml
```

Aplicar direto o caminho local no módulo `k8s` falha com `No such file or directory`.

### Bibliotecas Python no host remoto

Os módulos `kubernetes.core.*` rodam no host alvo e precisam da lib `kubernetes` **lá**, não na máquina de controle. O role `common` instala via apt:

```
python3-kubernetes  python3-yaml  python3-jsonpatch
```

Ubuntu 24.04 aplica PEP 668 (ambiente gerenciado externamente), então instalar via `pip` no sistema falharia — apt é o caminho certo aqui.

### Helm é instalado como pre-task

O play de plataforma instala o Helm antes dos roles rodarem, porque os módulos `kubernetes.core.helm*` só encapsulam o binário — não o embarcam:

```yaml
pre_tasks:
  - name: Instalar Helm
    ansible.builtin.shell: |
      if ! command -v helm >/dev/null 2>&1; then
        curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
      fi
    args:
      creates: /usr/local/bin/helm
```

### `creates:` é uma checagem fraca de idempotência

O role do Tailscale usava `creates: /var/lib/tailscale/tailscaled.state` para pular o `tailscale up`. O arquivo existia mas o node **nunca tinha logado** — o playbook pulava a task e falhava depois, ao tentar ler o IP. A correção foi checar o estado real:

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

A lição vale para qualquer role: presença de arquivo é proxy ruim para "já está configurado".

### Nome do node ≠ nome no inventário

O k3s registra o node com o **hostname real do SO** (`<node-hostname>`), não com o alias do inventário (`vps-1`). Tasks que consultam o node precisam de `{{ ansible_hostname }}`, não `{{ inventory_hostname }}`.

### Ansible e I/O não-bloqueante

Em alguns ambientes (o WSL usado aqui, entre eles) o Ansible aborta com:

```
ERROR: Ansible requires blocking IO on stdin/stdout/stderr.
```

O contorno é rodar dentro de um pty:

```bash
script -qec "ansible-playbook -i inventory/hosts.ini site.yml" /dev/null
```

## Vault

`ansible.cfg` aponta para o arquivo de senha:

```ini
vault_password_file = ~/.infra-vault-pass
```

Esse arquivo **não está no repositório** e precisa existir na máquina que roda os playbooks. Sem ele, o `vault.yml` commitado não decifra. Detalhes em [05 - Segredos](05-segredos.md).

## Provisionar um servidor novo do zero

Vale tanto para um worker deste cluster quanto para um cluster inteiramente novo em outro provedor:

```bash
# 1. adicionar o host ao inventário, no grupo certo
# 2. bootstrap
ansible-playbook -i inventory/hosts.ini bootstrap.yml \
  --limit <host> -e ansible_user=root --private-key ~/.ssh/id_ed25519
# 3. atualizar ~/.ssh/config para o usuário deploy e a chave nova
# 4. provisionar
ansible-playbook -i inventory/hosts.ini site.yml
```

Para um **cluster separado** (staging, por exemplo), duplique o inventário (`hosts-staging.ini`) com seus próprios grupos e crie um `clusters/staging/` no repositório. Os roles são os mesmos.
