# 05 — Segredos

Três mecanismos, cada um com um papel distinto. A regra que vale para todos: **nenhum segredo em texto puro no Git, nunca.**

| Mecanismo | Para quê | Onde vive |
| --- | --- | --- |
| **ansible-vault** | Segredos de provisionamento — o Ansible precisa deles antes do cluster existir | `ansible/group_vars/all/vault.yml`, criptografado, commitado |
| **Secrets criados pelo Ansible** | Credenciais que só o cluster consome | Direto no cluster, nunca passam pelo Git |
| **Sealed Secrets** | Segredos de projeto que precisam viver no Git, para o ArgoCD aplicar | Criptografados no repositório |

## ansible-vault

Contém tudo que o Ansible precisa antes de haver cluster:

```yaml
vault_cloudflare_api_token          # cert-manager, DNS-01
vault_tailscale_authkey             # join na tailnet
vault_infra_repo_token              # ArgoCD clonar o repo privado
vault_postgres_superuser_password
vault_valkey_password
vault_rustfs_access_key
vault_rustfs_secret_key
vault_r2_access_key_id              # backup offsite no R2
vault_r2_secret_access_key
```

O arquivo criptografado **é commitado** — esse é o objetivo do ansible-vault. Um clone novo do repositório já vem com todos os segredos, e só precisa da senha para abrir.

```bash
ansible-vault edit ansible/group_vars/all/vault.yml       # editar
ansible-vault decrypt ansible/group_vars/all/vault.yml    # abrir temporariamente
ansible-vault encrypt ansible/group_vars/all/vault.yml    # fechar de novo
git add -f ansible/group_vars/all/vault.yml               # o .gitignore cobre a versão em texto puro
```

O `git add -f` é necessário porque o `.gitignore` bloqueia `vault.yml` justamente para impedir commit acidental da versão descriptografada. Depois de criptografar, o `-f` força o add consciente.

### A senha do vault

Fica em `~/.infra-vault-pass`, **fora do repositório**, referenciada pelo `ansible.cfg`.

> Essa senha é a chave de tudo. Sem ela, o `vault.yml` commitado é inútil em qualquer outra máquina — e com ela, todos os tokens acima ficam acessíveis. Guarde uma cópia num gerenciador de senhas.

## Secrets criados diretamente pelo Ansible

Para credenciais que **só o cluster consome**, passar por Git — mesmo criptografado — é indireção sem ganho. O Ansible lê do vault e cria o Secret direto:

```yaml
- name: Criar Secret com o token da API do Cloudflare
  kubernetes.core.k8s:
    state: present
    definition:
      apiVersion: v1
      kind: Secret
      metadata:
        name: cloudflare-api-token
        namespace: cert-manager
      stringData:
        api-token: "{{ vault_cloudflare_api_token }}"
```

Os manifestos versionados no Git referenciam apenas o **nome** do Secret, nunca o valor. Um `Cluster` do Postgres aponta para `postgres-superuser`; o valor chegou lá por outro caminho.

Secrets criados assim hoje:

| Secret | Namespace | Criado por |
| --- | --- | --- |
| `cloudflare-api-token` | `cert-manager` | role `cert_manager` |
| `infra-repo-creds` | `argocd` | role `argocd` |
| `postgres-superuser` | `postgres` | role `data_services_secrets` |
| `valkey-auth` | `valkey` | role `data_services_secrets` |
| `rustfs-credentials` | `rustfs` | role `data_services_secrets` |

## Sealed Secrets

Para o caso que os dois anteriores não cobrem: um segredo que precisa estar **no Git** porque o ArgoCD é quem vai aplicá-lo — tipicamente segredos de projeto (`apps/<projeto>/sealed-secret.yaml`).

O `kubeseal` criptografa localmente com a chave pública do cluster, e só o controller lá dentro consegue abrir. O arquivo resultante é seguro em repositório público.

```bash
kubeseal --format yaml < meu-secret.yaml > apps/<projeto>/sealed-secret.yaml
```

| | |
| --- | --- |
| Controller | `sealed-secrets` no namespace `kube-system` |
| Chart | `sealed-secrets-2.19.3` (app `0.39.1`) |
| Repo Helm | `https://bitnami.github.io/sealed-secrets` |

### Backup da master key — obrigatório

Os `SealedSecret` commitados só podem ser decifrados pela chave privada **daquele cluster específico**. Um cluster novo gera uma chave nova e não consegue abrir nada do que já está no repositório.

Isso quebra o requisito de replicabilidade se a chave não for guardada.

```bash
export KUBECONFIG=.secrets/vps-1.kubeconfig
kubectl get secret -n kube-system \
  -l sealedsecrets.bitnami.com/sealed-secrets-key=active \
  -o yaml > backup-sealed-secrets-key.yaml
```

Restaurar num cluster novo:

```bash
kubectl apply -f backup-sealed-secrets-key.yaml
kubectl delete pod -n kube-system -l app.kubernetes.io/name=sealed-secrets
```

O backup atual está em `.secrets/backup-sealed-secrets-key.yaml` — em texto puro, no disco local. Está fora do Git, mas **deveria estar num gerenciador de senhas**, não só ali.

## Credenciais de acesso humano

| O quê | Onde está | Observação |
| --- | --- | --- |
| Chave SSH do `deploy` | `.secrets/vps-1_deploy_ed25519` | Gerada pelo `bootstrap.yml`, fora do Git |
| Kubeconfig | `.secrets/vps-1.kubeconfig` | Baixado a cada `site.yml`, fora do Git |
| Senha admin do ArgoCD | `.secrets/argocd-admin-password.txt` | Gerada aleatoriamente; o secret inicial do ArgoCD foi apagado |
| Senha do ansible-vault | `~/.infra-vault-pass` | Fora do repositório |

Tudo que está em `.secrets/` está coberto pelo `.gitignore`.

## As três credenciais de recuperação

Três arquivos vivem hoje apenas no disco da máquina local, em texto puro. Nenhum está no Git, mas **perder a máquina significa perder o acesso**:

1. `~/.infra-vault-pass` — sem ela, nenhum segredo do repositório abre
2. `.secrets/backup-sealed-secrets-key.yaml` — sem ela, um cluster novo não decifra os SealedSecrets
3. `.secrets/argocd-admin-password.txt` — recuperável recriando a senha, mas incômodo

O destino é o **1Password**. A propriedade que define isso é única: precisam estar acessíveis quando *tanto o notebook quanto a VPS* sumiram — o que exclui qualquer lugar que esteja dentro de um dos dois.

### Por que não em lugares que parecem razoáveis

**No RustFS / na VPS.** Backup guardado dentro daquilo que ele restaura não é backup. A master key do Sealed Secrets existe para reconstruir um cluster perdido; dentro do cluster, ela some exatamente quando serve para algo. A senha do vault destranca o `vault.yml` para *reprovisionar* a VPS — na VPS, some junto com ela.

Além disso, a senha do vault na VPS expande o raio de alcance de um comprometimento: hoje quem invade a VPS ganha a VPS; com ela lá, ganha também o token do Cloudflare (DNS das duas zonas), a authkey do Tailscale (a rede privada) e o PAT do GitHub.

**Num repositório privado do GitHub.** Passa no teste de "acessível quando tudo sumiu", mas falha em outros três pontos:

- Privado não é criptografado — é texto puro com controle de acesso. Um PAT vazado com escopo `repo`, um clique errado em visibilidade ou a conta comprometida expõem tudo diretamente.
- Histórico do Git é permanente: commitou e removeu depois, continua recuperável. Rotacionar não apaga o valor antigo.
- Dentro do próprio `vault.yml` mora o `vault_infra_repo_token`, um PAT do GitHub. Guardar a senha que abre esse vault no GitHub faz uma única conta comprometida cascatear para tudo. Com o 1Password, são dois comprometimentos independentes.

### Duas classes de coisa, dois destinos

A distinção que explica por que backups vão para o R2 e credenciais vão para o 1Password:

| | Credenciais de recuperação | Dados de backup |
| --- | --- | --- |
| Exemplos | Senha do vault, master key | Dumps do k3s |
| Tamanho | Alguns bytes | Megabytes por dia |
| Frequência | Muda raramente | Muda todo dia |
| Quem busca | Humano, numa crise | Máquina, automaticamente |
| Destino | **1Password** | **Cloudflare R2** |

## Rotacionar um segredo

```bash
ansible-vault edit ansible/group_vars/all/vault.yml         # trocar o valor
ansible-playbook -i inventory/hosts.ini site.yml            # reaplica os Secrets
kubectl rollout restart deployment/<nome> -n <namespace>    # pods relêem o Secret
```

Pods não recarregam Secrets montados como variável de ambiente sem reiniciar — o restart é parte do processo, não opcional.
