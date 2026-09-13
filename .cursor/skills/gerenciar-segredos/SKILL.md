---
name: gerenciar-segredos
description: Cria, rotaciona e decide onde guardar segredos desta infra — tokens de API, senhas, chaves de acesso, credenciais de recuperação. Use sempre que a tarefa envolver adicionar uma credencial nova, rotacionar um token, editar o ansible-vault, usar kubeseal, configurar acesso a um serviço externo, ou responder onde guardar uma chave. Existem três mecanismos distintos com propósitos diferentes, e escolher o errado — ou commitar em texto puro — tem consequência irreversível, porque segredo que entra no histórico do Git continua recuperável mesmo depois de removido.
---

# Gerenciar segredos

Detalhes e inventário completo em [`docs/05-segredos.md`](../../../docs/05-segredos.md).

## Qual mecanismo usar

A pergunta que decide é **quem precisa ler o segredo e quando**.

| Quem lê | Mecanismo | Onde vive |
| --- | --- | --- |
| O Ansible, antes de o cluster existir | **ansible-vault** | `ansible/group_vars/all/vault.yml`, criptografado, commitado |
| Só o cluster | **Secret criado pelo Ansible** | Direto no cluster; nunca passa pelo Git |
| O ArgoCD, a partir do Git | **Sealed Secrets** | Criptografado no repositório |
| Um humano, numa crise | **1Password** | Fora daqui — ver abaixo |

Note o segundo caso: se só o cluster consome, passar por Git — mesmo criptografado — é indireção sem ganho. O Ansible lê do vault e cria o Secret; o manifesto versionado referencia apenas o **nome**.

O Sealed Secrets existe para o caso que os outros não cobrem: um segredo que precisa estar **no Git** porque o ArgoCD é quem vai aplicá-lo, tipicamente de projeto.

## ansible-vault

O arquivo criptografado **é commitado** — esse é o objetivo. Um clone novo já vem com todos os segredos, e só precisa da senha.

```bash
ansible-vault edit ansible/group_vars/all/vault.yml
```

Se precisar abrir temporariamente (para acrescentar várias entradas de uma vez, por exemplo), o ciclo tem três passos e **nenhum pode ser pulado**:

```bash
script -qec "ansible-vault decrypt ansible/group_vars/all/vault.yml --vault-password-file ~/.infra-vault-pass" /dev/null
# ... editar ...
script -qec "ansible-vault encrypt ansible/group_vars/all/vault.yml --vault-password-file ~/.infra-vault-pass" /dev/null
git add -f ansible/group_vars/all/vault.yml
```

O `git add -f` é necessário porque o `.gitignore` bloqueia esse caminho de propósito, para impedir commit acidental da versão em texto puro. O `-f` é o gesto consciente de que agora está criptografado.

**Confirme antes de commitar** — este é o erro caro e irreversível:

```bash
head -1 ansible/group_vars/all/vault.yml     # tem que ser $ANSIBLE_VAULT;1.1;AES256
git show :ansible/group_vars/all/vault.yml | head -1   # o que está de fato staged
```

Também acrescente a variável em `vault.yml.example`, com valor fictício, para que o modelo continue completo.

A senha do vault fica em `~/.infra-vault-pass`, referenciada pelo `ansible.cfg`, fora do repositório.

## Secret criado pelo Ansible

Padrão usado por Cloudflare, Postgres, Valkey, RustFS e R2:

```yaml
- name: Secret do serviço
  kubernetes.core.k8s:
    state: present
    definition:
      apiVersion: v1
      kind: Secret
      metadata:
        name: <nome>
        namespace: <ns>
      stringData:
        chave: "{{ vault_<variavel> }}"
  no_log: true
```

O `no_log: true` evita que o valor apareça na saída do playbook.

## Sealed Secrets

```bash
export KUBECONFIG=.secrets/vps-1.kubeconfig
kubeseal \
  --controller-name=sealed-secrets \
  --controller-namespace=kube-system \
  --namespace <projeto> \
  --format yaml < meu-secret.yaml > apps/<projeto>/sealed-secret.yaml
```

Ou `hinfra seal secret -f meu-secret.yaml --execute` (mesmos defaults).

O arquivo gerado é seguro em repositório — só o controller daquele cluster decifra.

É exatamente por isso que a **master key precisa de backup**: um cluster novo gera chave nova e não abre nada do que já está commitado.

## Credenciais de recuperação vão para o 1Password

Três arquivos têm natureza diferente de todo o resto — são o que você precisa quando *tudo* deu errado:

1. `~/.infra-vault-pass` — sem ela, nenhum segredo do repositório abre
2. `.secrets/backup-sealed-secrets-key.yaml` — sem ela, um cluster novo não decifra os SealedSecrets
3. `.secrets/argocd-admin-password.txt`

A propriedade que define onde eles moram: precisam estar acessíveis quando **tanto a máquina local quanto a VPS sumiram**. Isso exclui qualquer lugar dentro de um dos dois.

Se surgir a pergunta "posso guardar no RustFS / na VPS / num repo privado?", a resposta é não, por motivos distintos:

- **RustFS ou VPS** — backup guardado dentro daquilo que ele restaura não é backup. A master key existe para reconstruir um cluster perdido; dentro do cluster, ela some justamente quando serviria. E a senha do vault na VPS expande o raio de um comprometimento: de "ganhou a VPS" para "ganhou Cloudflare, Tailscale e GitHub junto".
- **Repositório privado no GitHub** — privado não é criptografado, é texto puro com controle de acesso; o histórico do Git é permanente; e o `vault_infra_repo_token` (um PAT do GitHub) mora *dentro* do vault, então guardar a senha que o abre no GitHub faz uma única conta comprometida cascatear para tudo.

Dados de **backup** (dumps do k3s) são outra classe e vão para o R2 — automatizados, grandes, escritos por máquina. A distinção está em `docs/05-segredos.md`.

## Rotacionar

```bash
ansible-vault edit ansible/group_vars/all/vault.yml
cd ansible && script -qec "ansible-playbook -i inventory/hosts.ini site.yml --private-key ../.secrets/vps-1_deploy_ed25519" /dev/null
kubectl rollout restart deployment/<nome> -n <ns>
```

O restart não é opcional: pods não recarregam Secret montado como variável de ambiente sem reiniciar.

## Escopo mínimo

Toda credencial que vive na VPS deve enxergar exatamente uma coisa — assim um vazamento não vira acesso geral. Os exemplos em uso: o token do Cloudflare só edita DNS das duas zonas; o PAT do GitHub é fine-grained restrito ao repo `infra`; o token do R2 só lê e escreve objetos no bucket `infra-backups`.

Ao criar credencial nova, procure ativamente a opção mais restrita que ainda funciona.
