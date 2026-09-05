# 10 — Runbooks

Procedimentos operacionais. Todos assumem que você está na tailnet e com o kubeconfig exportado:

```bash
tailscale up
export KUBECONFIG=/home/hamasaki/www/infra/.secrets/vps-1.kubeconfig
```

## Checagem geral de saúde

```bash
kubectl get nodes
kubectl get applications -n argocd
kubectl get pods -A | grep -v Running | grep -v Completed
kubectl top nodes
kubectl get certificate -A
ssh vps 'sudo ufw status verbose && tailscale status'
```

Referência do estado saudável: ~9% de CPU, ~2,9 GB de RAM, todos os Applications `Synced`/`Healthy`.

---

## Provisionar / reconfigurar um servidor

```bash
cd ansible
ansible-playbook -i inventory/hosts.ini site.yml \
  --private-key /home/hamasaki/www/infra/.secrets/vps-1_deploy_ed25519
```

Idempotente — pode rodar sempre. Se der `ERROR: Ansible requires blocking IO`, envolva num pty:

```bash
script -qec "ansible-playbook -i inventory/hosts.ini site.yml --private-key ..." /dev/null
```

## Adicionar um worker

Passo a passo em [03 - Kubernetes](03-kubernetes.md#adicionar-um-worker).

## Adicionar um projeto

Passo a passo em [09 - CI/CD](09-cicd.md#onboarding-de-um-projeto-novo).

---

## Forçar sincronização do ArgoCD

```bash
kubectl annotate application root-app -n argocd argocd.argoproj.io/refresh=hard --overwrite
```

Ao mudar values de um Application com fonte Helm, refresque o **`root-app`** — é ele que aplica o arquivo alterado. Refrescar só o filho não puxa a mudança.

## Investigar um Application travado

```bash
kubectl get application <nome> -n argocd -o jsonpath='{.status.sync.status} / {.status.health.status}{"\n"}'
kubectl describe application <nome> -n argocd | tail -30

# recursos que não estão Synced
kubectl get application <nome> -n argocd -o json | python3 -c "
import json,sys
d=json.load(sys.stdin)
for r in d.get('status',{}).get('resources',[]):
    if r.get('status') != 'Synced': print(r)
"

# erro de renderização de chart aparece aqui, não no controller
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-repo-server --tail=100 | grep -i error
```

`Unknown` quase sempre é falha ao gerar manifestos. Comece pelo repo-server.

---

## Criar banco para um projeto

```bash
kubectl exec -it -n postgres postgres-1 -- psql -U postgres
```

```sql
CREATE ROLE meuprojeto WITH LOGIN PASSWORD 'senha-forte';
CREATE DATABASE meuprojeto OWNER meuprojeto;
REVOKE ALL ON DATABASE meuprojeto FROM PUBLIC;
```

Connection string: `postgresql://meuprojeto:senha@postgres-rw.postgres.svc.cluster.local:5432/meuprojeto`

## Acessar o Postgres da máquina local

```bash
kubectl port-forward -n postgres svc/postgres-rw 5432:5432
# outro terminal:
psql "postgresql://postgres:<vault_postgres_superuser_password>@localhost:5432/app"
```

## Acessar o Valkey

```bash
kubectl exec -it -n valkey deploy/valkey -- valkey-cli
AUTH default <vault_valkey_password>
```

## Criar um bucket no RustFS

Console web em `https://s3.hamasakis.cloud` (só tailnet), ou via API S3 de dentro do cluster, com qualquer cliente compatível apontando para `rustfs-svc.rustfs.svc.cluster.local:9000` e as credenciais de `vault_rustfs_access_key` / `vault_rustfs_secret_key`.

---

## Backup do datastore do k3s

```bash
ssh vps 'sudo /usr/local/bin/k3s-backup.sh'                     # sob demanda
ssh vps 'ls -la /var/backups/k3s/'                              # local (7 dias)
ssh vps 'sudo systemctl list-timers k3s-backup.timer'           # agendamento
ssh vps 'sudo systemctl status k3s-backup'                      # falhas do último run

# cópia offsite no R2 (30 dias)
ssh vps 'sudo rclone --config /etc/k3s-backup/rclone.conf ls r2:infra-backups/k3s/srv1957194/'
```

Baixar um backup do R2 para a máquina local:

```bash
ssh vps 'sudo rclone --config /etc/k3s-backup/rclone.conf copy \
  r2:infra-backups/k3s/srv1957194/state-<timestamp>.db.gz /tmp/'
scp vps:/tmp/state-<timestamp>.db.gz .
```

## Restaurar o datastore do k3s

> Destrutivo. Reverte o cluster inteiro ao momento do backup.

```bash
ssh vps
sudo systemctl stop k3s
sudo cp /var/lib/rancher/k3s/server/db/state.db /var/lib/rancher/k3s/server/db/state.db.bak
sudo gunzip -c /var/backups/k3s/state-<timestamp>.db.gz | sudo tee /var/lib/rancher/k3s/server/db/state.db > /dev/null
sudo systemctl start k3s
```

Confira `kubectl get nodes` e `kubectl get applications -n argocd` depois.

## Snapshot da VPS

Feito via API da Hostinger, `virtualMachineId=1957194`. **Um snapshot novo sobrescreve o anterior.** Tire um antes de qualquer mudança arriscada no host — SSH, firewall, upgrade de kernel.

---

## Rotacionar um segredo

```bash
ansible-vault edit ansible/group_vars/all/vault.yml
cd ansible && ansible-playbook -i inventory/hosts.ini site.yml --private-key ...
kubectl rollout restart deployment/<nome> -n <namespace>
```

## Trocar a senha do ArgoCD

```bash
NEWPASS=$(openssl rand -base64 18)
HASH=$(python3 -c "import bcrypt,sys; print(bcrypt.hashpw(sys.argv[1].encode(), bcrypt.gensalt(rounds=10)).decode())" "$NEWPASS")
kubectl -n argocd patch secret argocd-secret \
  -p '{"stringData": {"admin.password": "'"$HASH"'", "admin.passwordMtime": "'"$(date -u +%FT%TZ)"'"}}'
echo "Nova senha: $NEWPASS"
```

## Renovar a authkey do Tailscale

Authkeys expiram. Se um node cair da tailnet, gere uma nova em `login.tailscale.com/admin/settings/keys` (reutilizável, sem expiração, com a tag `k8s`), atualize `vault_tailscale_authkey` e rode o `site.yml`.

---

## Certificado não emite

```bash
kubectl get certificate -A
kubectl describe certificate <nome> -n <ns> | grep -A5 Message
kubectl get order,challenge -A
```

Causas comuns:

| Sintoma | Causa provável |
| --- | --- |
| `429 rateLimited` | 5 certificados/semana para o mesmo host. Só esperar — o cert-manager tenta sozinho. |
| `no PEM data was found` | Algo mais está sobrescrevendo o Secret TLS (chart criando o próprio). |
| Desafio DNS travado | Token do Cloudflare inválido ou sem escopo na zona. |

Validar o token:

```bash
curl -s -X GET "https://api.cloudflare.com/client/v4/accounts/347d85d3ee7db762e6af1cdfb874f8e8/tokens/verify" \
  -H "Authorization: Bearer <token>"
```

## Serviço inacessível pelo navegador

Ordem de verificação:

1. **DNS** — `nslookup <host> 1.1.1.1`. Se o `1.1.1.1` responde e sua máquina não, é cache local.
2. **Tailnet** — hosts `*.hamasakis.cloud` exigem `tailscale status` conectado.
3. **Certificado** — `kubectl get certificate -A`.
4. **Ingress** — `kubectl get ingress -A`.
5. **Pod** — `kubectl get pods -n <ns>`.

`403` de dentro da tailnet costuma significar que o `externalTrafficPolicy` do Traefik voltou a `Cluster`:

```bash
kubectl get svc traefik -n kube-system -o jsonpath='{.spec.externalTrafficPolicy}{"\n"}'   # deve ser Local
```

## Perdi acesso SSH

1. Console VNC/recovery no painel da Hostinger.
2. Restaurar o snapshot mais recente.
3. Em último caso, reinstalar e rodar `bootstrap.yml` + `site.yml` — é o que torna o repositório replicável.

---

## Desligar um serviço temporariamente

```bash
kubectl scale deployment/rustfs -n rustfs --replicas=0
kubectl scale deployment/rustfs -n rustfs --replicas=1
```

Não funciona para recursos geridos por operador (o `Cluster` do Postgres) — o operador reconcilia de volta. Ver [08 - Serviços de dados](08-data-services.md).

## Escalar para réplicas quando houver workers

| Serviço | Mudança |
| --- | --- |
| Postgres | `instances: 1` → `3` em `data-services/postgres/cluster.yaml` |
| Valkey | `replica.enabled: true` em `clusters/production/apps/valkey-app.yaml` |
| RustFS | `mode.distributed.enabled: true` em `clusters/production/apps/rustfs-app.yaml` |

Commit e push — o ArgoCD aplica.
