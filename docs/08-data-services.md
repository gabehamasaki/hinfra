# 08 — Serviços de dados

Três serviços compartilhados por todos os projetos, mais o motor de autoscaling. Todos são `Application` do ArgoCD; os operadores por trás deles são instalados pelo Ansible.

| Serviço | Endpoint interno | Versão | Estado |
| --- | --- | --- | --- |
| Postgres | `postgres-rw.postgres.svc.cluster.local:5432` | CNPG operator `1.30.0` | 1 instância |
| Valkey | `valkey.valkey.svc.cluster.local:6379` | chart `0.12.0` | standalone |
| RustFS (S3) | `rustfs-svc.rustfs.svc.cluster.local:9000` | chart `1.0.0-rc.5` | standalone |
| KEDA | — | `2.20.2` | operador |

## Compartilhado, não por projeto

Cada projeto novo ganha seu **banco** dentro do Postgres, seu **prefixo de chaves** no Valkey e seu **bucket** no RustFS — não uma instância própria de cada coisa.

Uma instância por projeto multiplicaria RAM e CPU a cada projeto adicionado, o que não cabe em 2 vCPU / 8 GB. O custo dessa escolha é menos isolamento: um projeto que derrube o Postgres derruba todos. Para o porte atual (projetos pessoais), o trade-off compensa.

## Preparado para réplicas em múltiplos servidores

Todos os três foram escolhidos com um critério explícito: sair de instância única para replicado deve ser **mudar um campo**, não remontar a arquitetura. Hoje roda tudo com uma instância porque só existe um node — colocar réplicas no mesmo node não daria disponibilidade nenhuma, só consumiria recursos.

| Serviço | Para escalar | Onde |
| --- | --- | --- |
| Postgres | `instances: 1` → `3` | `hinfra-workloads/data-services/postgres/cluster.yaml` |
| Valkey | `replica.enabled: true` + `replica.replicas` | `hinfra-workloads` → `clusters/production/apps/valkey-app.yaml` |
| RustFS | `mode.distributed.enabled: true` (mín. 2 nodes) | `hinfra-workloads` → `clusters/production/apps/rustfs-app.yaml` |

---

## Postgres — CloudNativePG

O operador (`cnpg-system`) é instalado pelo Ansible; o cluster em si é um CRD versionado no Git.

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: postgres
  namespace: postgres
spec:
  instances: 1
  superuserSecret:
    name: postgres-superuser
  storage:
    size: 4Gi
    storageClass: local-path
  resources:
    requests: { cpu: 100m, memory: 128Mi }
    limits:   { memory: 256Mi }
  bootstrap:
    initdb:
      database: app
      owner: app
```

### Por que um operador e não um chart

Um chart genérico de Postgres entrega um StatefulSet — replicação, failover e promoção de primário ficam por sua conta. O CloudNativePG trata replicação por streaming como campo de primeira classe: mudar `instances` de 1 para 3 provisiona réplicas, configura a replicação e cuida de failover automático, sem Patroni e sem intervenção.

Como a exigência era "compartilhado, mas preparado para réplicas em múltiplos servidores", essa propriedade é exatamente o que se está comprando.

### Os três Services

O CNPG cria três, e a distinção importa:

| Service | Aponta para | Use para |
| --- | --- | --- |
| `postgres-rw` | Primário | Leitura e escrita — o padrão |
| `postgres-ro` | Só réplicas | Leituras que toleram lag (hoje sem réplica: não resolve) |
| `postgres-r` | Qualquer instância | Leitura em qualquer uma |

Aplicações devem usar `postgres-rw`. Os outros dois só ganham utilidade quando `instances > 1`.

### Criar banco e usuário para um projeto

```bash
export KUBECONFIG=.secrets/vps-1.kubeconfig
kubectl exec -it -n postgres postgres-1 -- psql -U postgres
```

```sql
CREATE ROLE meuprojeto WITH LOGIN PASSWORD 'senha-forte-aqui';
CREATE DATABASE meuprojeto OWNER meuprojeto;
REVOKE ALL ON DATABASE meuprojeto FROM PUBLIC;
```

Depois, o segredo de conexão para a aplicação:

```bash
kubectl create secret generic meuprojeto-db -n meuprojeto \
  --from-literal=DATABASE_URL='postgresql://meuprojeto:senha@postgres-rw.postgres.svc.cluster.local:5432/meuprojeto' \
  --dry-run=client -o yaml | kubeseal --format yaml > apps/meuprojeto/sealed-secret.yaml
```

A senha do superusuário está no vault, em `vault_postgres_superuser_password`.

### Backup

O CNPG suporta backup contínuo via Barman para destino S3 — e o RustFS já roda neste cluster. É a integração natural a fazer, e ainda não está configurada. Hoje o Postgres depende do backup do datastore do k3s, que **não** cobre os dados dentro do Postgres.

---

## Valkey

Fork do Redis mantido pela Linux Foundation, com a mesma API e os mesmos clientes.

### Por que não Redis

Os charts do Bitnami — a forma padrão de instalar Redis em Kubernetes por anos — estão sendo movidos para um repositório legado e pago pela Broadcom a partir de agosto/2026. Adotá-los agora seria adotar algo já em fim de vida. O Valkey é o sucessor open-source natural, e nenhum código de cliente muda.

```yaml
auth:
  enabled: true
  usersExistingSecret: valkey-auth
  aclUsers:
    default:
      permissions: "~* &* +@all"
dataStorage:
  enabled: true
  requestedSize: 512Mi
  className: local-path
resources:
  requests: { cpu: 25m, memory: 32Mi }
  limits:   { memory: 128Mi }
replica:
  enabled: false
```

> `aclUsers.default` é obrigatório mesmo usando `usersExistingSecret`. O Secret fornece só a **senha**; as **permissões** vêm do values. Sem isso o chart falha na renderização com `auth.enabled is true but no authentication method is configured` — e o Application fica em `Unknown` sem mostrar o motivo.

Conectar:

```
valkey://default:<vault_valkey_password>@valkey.valkey.svc.cluster.local:6379
```

Como a instância é compartilhada, **prefixe as chaves por projeto** (`meuprojeto:sessao:123`) para evitar colisão. Um `FLUSHALL` de um projeto apaga os dados de todos.

---

## RustFS — armazenamento S3

Object storage compatível com S3, escrito em Rust. Mais leve que o MinIO, o que importa nesta máquina.

```yaml
mode:
  standalone:
    enabled: true
  distributed:
    enabled: false
secret:
  existingSecret: rustfs-credentials
storageclass:
  name: local-path
  dataStorageSize: 5Gi
  logStorageSize: 256Mi
ingress:
  enabled: true
  className: traefik
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-cloudflare
    traefik.ingress.kubernetes.io/router.middlewares: "argocd-argocd-tailnet-only@kubernetescrd"
  hosts:
    - host: s3.hamasakis.cloud
      paths: [{ path: /, pathType: Prefix }]
  tls:
    enabled: true
    certManager:
      enabled: true
```

### Duas portas, dois públicos

| Porta | O quê | Como acessar |
| --- | --- | --- |
| `9000` | API S3 | `rustfs-svc.rustfs.svc.cluster.local:9000` — de dentro do cluster |
| `9001` | Console web | `https://s3.hamasakis.cloud` — só tailnet |

O Ingress aponta **apenas para o console** (porta 9001) — é assim que o chart o define. A API S3 não é exposta via Ingress, e não precisa: as aplicações que a consomem rodam no mesmo cluster e falam direto com o Service. Isso significa que o RustFS está plenamente funcional para os projetos mesmo que o Ingress ou o certificado estejam com problema.

### `tls.certManager.enabled: true` não é opcional

Com `tls.enabled: true` e `tls.certManager.enabled: false` (o padrão), o chart renderiza **seu próprio Secret TLS** com conteúdo placeholder inválido, no mesmo nome que o cert-manager usa. O resultado foi um loop de reemissão que estourou o rate limit do Let's Encrypt. O relato está em [11 - Armadilhas](11-armadilhas.md).

A flag `certManager.enabled` **não** adiciona a annotation do cluster-issuer — ela apenas impede o chart de criar o Secret. As duas coisas são necessárias: a flag e a annotation.

Credenciais no vault: `vault_rustfs_access_key` e `vault_rustfs_secret_key`.

---

## KEDA — desligar o que não está em uso

Instalado como plataforma, disponível para qualquer workload.

### O que dá e o que não dá para fazer

**Não existe** hoje um mecanismo maduro para "acordar" no primeiro pacote TCP um Postgres, Valkey ou Kafka escalado a zero. O KEDA tem o `http-add-on`, que intercepta requisições HTTP, segura a conexão, escala o pod e então encaminha — mas isso só funciona para HTTP. Para protocolos binários seria preciso um proxy TCP que aceitasse a conexão, disparasse o scale-up e esperasse o backend subir; não há solução consolidada para isso.

O que funciona bem:

1. **Escala por horário** (`cron` scaler) — desliga fora do horário de uso. Confiável e simples.
2. **Liga/desliga manual** — `kubectl scale`, para períodos sem uso previsto.

Exemplo em `data-services/example/rustfs/idle-schedule.yaml.example` (kit público) ou `hinfra-workloads/data-services/rustfs/` (produção):

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: rustfs-idle-schedule
  namespace: rustfs
spec:
  scaleTargetRef:
    name: rustfs
  minReplicaCount: 0
  maxReplicaCount: 1
  triggers:
    - type: cron
      metadata:
        timezone: America/Sao_Paulo
        start: "0 8 * * *"
        end: "0 22 * * *"
        desiredReplicas: "1"
```

Está como `.example` de propósito — não ativo por padrão.

### Vale a pena hoje?

Provavelmente não, e é honesto dizer. Com o cluster inteiro consumindo ~2,9 GB dos 8 GB e ~9% de CPU, os três serviços de dados somam algo em torno de 400–500 MB. Desligá-los à noite economiza pouco e adiciona uma classe de falha ("por que a aplicação não conecta?" — porque são 23h).

A motivação original era Kafka, que ficou fora. O KEDA está instalado para quando aparecer uma carga que realmente justifique — e aí o mecanismo já existe, documentado e pronto.

### Escalar manualmente

```bash
kubectl scale deployment/rustfs -n rustfs --replicas=0    # desligar
kubectl scale deployment/rustfs -n rustfs --replicas=1    # ligar
```

Cuidado com recursos geridos por operador (o `Cluster` do Postgres, por exemplo): o operador reconcilia e desfaz alterações manuais de réplicas. Para o Postgres, o campo certo é `instances` no CRD — e reduzir para 0 não é operação suportada.
