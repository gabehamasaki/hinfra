---
name: provisionar-servidor
description: Provisiona e altera servidores desta infra via Ansible — bootstrap, hardening de SSH, Tailscale, k3s, backup. Use sempre que a tarefa envolver rodar ansible-playbook, adicionar um worker ao cluster, criar ou alterar um role, provisionar uma VPS nova, ou mexer em firewall, usuário deploy ou acesso SSH — inclusive quando o pedido for apenas "roda o playbook", "aplica no servidor" ou "adiciona mais um servidor". O Ansible aqui tem armadilhas de precedência de variáveis e de resolução de caminhos que falham em silêncio ou com mensagens enganosas.
---

# Provisionar servidor

O Ansible é o que torna este repositório replicável: rodar os playbooks contra uma VPS nova — de qualquer provedor — produz o mesmo resultado, sem passo manual.

Detalhes completos em [`docs/04-ansible.md`](../../../docs/04-ansible.md) e [`docs/03-kubernetes.md`](../../../docs/03-kubernetes.md). Este documento traz o procedimento e o que costuma dar errado.

## Os dois playbooks

**`bootstrap.yml`** roda uma única vez por servidor, autenticado como root — é a única vez que root é usado. Cria o usuário `deploy`, valida que ele funciona numa conexão nova, e só então desabilita o login root.

**`site.yml`** roda sempre que precisar. É idempotente: sem mudanças, resulta em `changed=0`.

```bash
cd ansible

# servidor novo, uma vez só
script -qec "ansible-playbook -i inventory/hosts.ini bootstrap.yml \
  --limit <host> -e ansible_user=root --private-key ~/.ssh/id_ed25519" /dev/null

# provisionamento normal
script -qec "ansible-playbook -i inventory/hosts.ini site.yml \
  --private-key ../.secrets/vps-1_deploy_ed25519" /dev/null
```

## Armadilhas

Cada uma destas já custou uma execução quebrada nesta infra. Elas compartilham um padrão: **falham em silêncio ou apontam para o lugar errado**.

### Precedência de variáveis mata o `-u`

O inventário define `ansible_user=deploy` para o grupo `k3s_cluster`, e variáveis de inventário têm precedência **maior** que a flag `-u` da linha de comando. Usar `-u root` no bootstrap conecta como `deploy` e falha com `Permission denied (publickey)`.

Use `-e ansible_user=root` — extra vars ficam no topo da precedência.

### Ansible exige pty neste ambiente

Sem ele: `ERROR: Ansible requires blocking IO on stdin/stdout/stderr`. Envolva sempre em `script -qec "..." /dev/null`. Vale para `ansible`, `ansible-playbook` e `ansible-vault`.

### `template`/`copy` leem local; `kubernetes.core.k8s` lê remoto

`ansible.builtin.template` e `ansible.builtin.copy` resolvem o `src:` na **máquina de controle**. Já o `kubernetes.core.k8s` executa como script *dentro* do host remoto, então o `src:` dele precisa existir **lá**. Apontar um caminho do repositório nele falha com `No such file or directory` para um arquivo que obviamente existe.

O padrão correto é em dois passos — copiar, depois aplicar:

```yaml
- ansible.builtin.copy:          # lê local, escreve remoto
    src: "{{ playbook_dir }}/../clusters/production/root-app.yaml"
    dest: /tmp/root-app.yaml

- kubernetes.core.k8s:           # lê no host remoto
    state: present
    src: /tmp/root-app.yaml
```

### Módulos `kubernetes.core` precisam de libs no host remoto

Eles executam no alvo, não na máquina de controle. O role `common` instala `python3-kubernetes`, `python3-yaml` e `python3-jsonpatch` via apt — o Ubuntu 24.04 aplica PEP 668, então `pip` no sistema não é opção.

### `creates:` é checagem fraca de idempotência

Presença de arquivo não prova que a configuração aconteceu. O `tailscale up` era pulado porque o arquivo de estado existia, embora o node nunca tivesse logado — e a falha só aparecia numa task posterior. Cheque o estado real:

```yaml
- ansible.builtin.command: tailscale status --json
  register: ts
  changed_when: false
  failed_when: false

- ansible.builtin.command: tailscale up --reset ...
  when: ts.rc != 0 or (ts.stdout | from_json).BackendState != "Running"
```

### Nome do node ≠ nome do inventário

O k3s registra o node com o hostname do SO (`srv1957194`); o inventário chama a mesma máquina de `vps-1`. Tasks que consultam o node usam `{{ ansible_hostname }}`.

### `tailscale up` exige `--reset` em automação

Se qualquer preferência não-default divergir do estado salvo, ele recusa a execução pedindo que todas as flags sejam mencionadas. `--reset` evita isso. E a tag (`tag:k8s`) precisa existir em `tagOwners` na ACL do admin console **antes** de qualquer authkey usá-la.

## Adicionar um worker

O grupo `[k3s_agents]` já existe no inventário, vazio.

1. Adicionar o host em `ansible/inventory/hosts.ini` sob `[k3s_agents]`
2. Rodar `bootstrap.yml --limit <host> -e ansible_user=root`
3. Atualizar `~/.ssh/config` para o usuário `deploy` e a chave nova
4. Rodar `site.yml`

Não use `--limit` no passo 4 de forma que exclua o servidor: o role `k3s_agent` lê o IP Tailscale e o node-token do host `[k3s_server]` **na mesma execução**. Rodar o `site.yml` inteiro é sempre seguro.

Depois: `kubectl get nodes` deve mostrar ambos como `Ready`. Cargas com estado precisam de `nodeSelector` fixando no server, porque `local-path` prende o volume ao node onde foi criado.

## Depois de rodar

O `site.yml` baixa o kubeconfig para `.secrets/`, reescreve o endereço para o IP Tailscale e renomeia cluster/contexto/usuário de `default` para o nome do host. Confirme que o cluster respondeu de fato:

```bash
export KUBECONFIG=../.secrets/vps-1.kubeconfig && kubectl get nodes
```
