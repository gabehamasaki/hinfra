---
name: servico-no-cluster
description: Adiciona ou altera serviços no cluster k3s — Applications do ArgoCD, charts Helm, Ingress, e a decisão de expor público ou só na tailnet. Use sempre que a tarefa mencionar subir um serviço no cluster, criar um Application, mudar values de um chart, expor algo em um domínio, ou instalar uma ferramenta (dashboard, banco, monitoramento, painel) no Kubernetes. Cobre qual camada deve gerenciar o quê e armadilhas de configuração que falham em silêncio — uma delas já queimou o rate limit do Let's Encrypt por uma semana.
---

# Serviço no cluster

Detalhes completos em [`docs/07-argocd-gitops.md`](../../../docs/07-argocd-gitops.md), [`docs/06-tls-e-ingress.md`](../../../docs/06-tls-e-ingress.md) e [`docs/08-data-services.md`](../../../docs/08-data-services.md).

## Primeiro: qual camada gerencia isso?

A pergunta que decide é **com que frequência muda**, e se o ArgoCD depende disso para funcionar.

| Vai para o Ansible (`ansible/roles/`) | Vai para o ArgoCD (`clusters/production/apps/`) |
| --- | --- |
| Operadores e controladores | Instâncias e workloads |
| Coisas das quais o ArgoCD depende (cert-manager, Sealed Secrets) | Coisas que dependem do ArgoCD |
| Muda raramente | Muda com frequência |

Exemplo real: o **operador** do CloudNativePG é instalado pelo Ansible; o **Cluster** de Postgres em si é um Application do ArgoCD. O Ansible instala o motor, o ArgoCD configura o que roda.

Nunca deixe os dois gerenciando o mesmo recurso — ownership duplo gera drift, e drift sobre recurso stateful gera incidente.

## Onde os arquivos moram

- `data-services/<nome>/` — serviços de dados compartilhados
- `apps/<projeto>/` — projetos cujo CI constrói a imagem
- `platform/<nome>/` — manifestos aplicados pelo Ansible

O Application (o "ponteiro") vai sempre em `clusters/production/apps/`, que é o diretório que o `root-app` observa recursivamente.

## Duas formas de Application

**Manifestos deste repositório**, quando os arquivos são nossos:

```yaml
source:
  repoURL: https://github.com/gabehamasaki/infra.git
  targetRevision: main
  path: data-services/postgres
```

Um `kustomization.yaml` no diretório é detectado automaticamente.

**Chart Helm de terceiros**, quando o software vem pronto:

```yaml
source:
  repoURL: https://valkey.io/valkey-helm/
  chart: valkey
  targetRevision: "0.12.0"        # nunca "*"
  helm:
    values: |
      auth:
        enabled: true
```

Descubra a versão antes de fixar:

```bash
curl -s https://<repo>/index.yaml | python3 -c "
import sys,yaml; d=yaml.safe_load(sys.stdin)
e=d['entries']['<chart>'][0]; print(e['version'], e['appVersion'])"
```

## Armadilhas

### Valor de Helm que não existe é ignorado em silêncio

`applicationSet.enabled: false` foi aceito sem reclamação e não teve efeito nenhum — esse toggle não existe no chart do ArgoCD (lá é `replicas: 0`). O Helm não valida chaves desconhecidas.

Confirme que a chave existe antes de confiar nela:

```bash
ssh vps "helm repo add <nome> <url> && helm show values <nome>/<chart>" | grep -A3 "^<chave>:"
```

E depois de aplicar, verifique que produziu efeito (`kubectl get pods`), em vez de concluir pelo sync ter dado `Synced`.

### Chart que gerencia o próprio TLS briga com o cert-manager

O chart do RustFS, com `tls.enabled: true` e `tls.certManager.enabled: false` (o padrão), renderiza um Secret TLS **próprio, com conteúdo placeholder inválido**, no mesmo nome que o cert-manager usa. O ciclo: cert-manager emite → ArgoCD sobrescreve com o placeholder → cert-manager detecta corrupção e reemite → repete, até estourar o limite de 5 certificados por semana do Let's Encrypt.

Ao expor um chart de terceiros com TLS, procure ativamente a opção que **desliga a gestão de TLS do próprio chart** e deixa o cert-manager como dono. E note que essas flags costumam apenas suprimir o Secret — a annotation `cert-manager.io/cluster-issuer` continua sendo necessária. São duas coisas.

Sinal de alerta: um Secret aparecendo repetidamente como `OutOfSync` significa disputa de ownership. Investigue antes que vire rate limit.

### `targetRevision: "*"` trava sem erro

O Application fica em `Unknown` sem mensagem. Fixe a versão sempre.

### Mudança em values pede refresh no `root-app`

Para fonte Helm, o `revision` do Application é a versão do chart, não o commit. Os values vêm do arquivo em `clusters/production/apps/`, que quem sincroniza é o `root-app`:

```bash
kubectl annotate application root-app -n argocd argocd.argoproj.io/refresh=hard --overwrite
```

Refrescar só o Application filho não puxa a mudança.

## Exposição: público ou tailnet

A decisão padrão para qualquer coisa administrativa é **tailnet**. Console de admin exposto publicamente é superfície de ataque desproporcional ao ganho, e o Tailscale roda no celular, então não se perde acesso móvel.

```yaml
annotations:
  cert-manager.io/cluster-issuer: letsencrypt-cloudflare
  traefik.ingress.kubernetes.io/router.middlewares: "argocd-argocd-tailnet-only@kubernetescrd"
```

O middleware vive no namespace `argocd` mas é referenciado por namespace no nome, então **é reutilizável de qualquer namespace** — não duplique o recurso.

O registro DNS acompanha a decisão:

| Alcance | Aponta para |
| --- | --- |
| Público | `187.127.62.20` |
| Tailnet | `100.86.241.1` |

Publicar um IP de tailnet no DNS público é seguro e intencional: a faixa `100.64.0.0/10` é CGNAT e não é roteável pela internet. Isso evita ter que editar `/etc/hosts` em cada dispositivo.

## Segredos do serviço

Credenciais que só o cluster consome são criadas direto pelo Ansible a partir do vault (role `data_services_secrets`); o manifesto no Git referencia apenas o **nome** do Secret. Ver a skill `gerenciar-segredos`.

## Verificação

```bash
kubectl annotate application root-app -n argocd argocd.argoproj.io/refresh=hard --overwrite
kubectl get applications -n argocd
kubectl get pods -n <namespace>
kubectl get certificate -A                    # se expôs com TLS
curl -s -o /dev/null -w "%{http_code}\n" https://<host>/
```

Um Application `Synced` cujo pod não está `Running` significa que o manifesto foi aplicado mas o workload falhou — vá para a skill `diagnosticar-cluster`.
