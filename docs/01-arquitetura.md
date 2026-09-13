# 01 — Arquitetura

## O problema que essa infra resolve

Uma VPS única precisa hospedar vários projetos independentes, cada um com seu repositório e seu ciclo de deploy, sem que subir um projeto novo vire trabalho manual. Junto disso, três restrições moldaram todas as decisões:

1. **Vai crescer para mais servidores.** Outras VPS entram como workers no futuro. A infra precisa já estar pronta para isso, não ser reformada depois.
2. **O repositório precisa ser replicável.** Nada pode estar preso a esta VPS específica — o mesmo processo tem que provisionar qualquer máquina nova, em qualquer provedor.
3. **Nada opera como root.** Acesso administrativo passa por um usuário dedicado e auditável.

## As quatro camadas

A infra é dividida em quatro camadas, e a linha que as separa é **com que frequência cada coisa muda**. Essa é a decisão estrutural mais importante do projeto, e todo o resto decorre dela.

```
┌─ 1. Provisionamento (Ansible) ──────── roda 1× por servidor ──────┐
│  hardening · usuário deploy · Tailscale · k3s · backup            │
└───────────────────────────────────────────────────────────────────┘
┌─ 2. Plataforma (Ansible + Helm) ────── muda raramente ────────────┐
│  Sealed Secrets · cert-manager · ArgoCD · KEDA · operador do PG   │
└───────────────────────────────────────────────────────────────────┘
┌─ 3. Serviços de dados (ArgoCD) ─────── muda de vez em quando ─────┐
│  Postgres · Valkey · RustFS                                       │
└───────────────────────────────────────────────────────────────────┘
┌─ 4. Projetos (ArgoCD) ──────────────── muda a cada push ──────────┐
│  my-portfolio · (próximos projetos)                               │
└───────────────────────────────────────────────────────────────────┘
```

### Por que Ansible nas camadas 1 e 2, e ArgoCD nas 3 e 4

A primeira versão do plano colocava *tudo* sob o ArgoCD, inclusive cert-manager e Sealed Secrets. Uma revisão do plano derrubou isso ao apontar uma dependência circular: o ArgoCD precisa do cert-manager (para o próprio certificado TLS) e do repositório Git (para saber o que aplicar). Se o ArgoCD gerencia o cert-manager, quem instala o cert-manager antes do ArgoCD existir? E se o `root-app` aponta para um repositório que ainda não foi criado, o ArgoCD nasce quebrado.

A separação resolve isso sem ambiguidade:

- **Ansible instala o motor.** Operadores e controladores — coisas que raramente mudam e que o ArgoCD precisa para funcionar.
- **ArgoCD configura o que roda.** Instâncias, workloads, versões de imagem — coisas que mudam com frequência e que ganham histórico, review e rollback ao viverem no Git.

Uma consequência prática: nenhum componente é gerenciado pelas duas camadas ao mesmo tempo. Ownership duplo gera drift, e drift em cima de recurso stateful gera incidente.

## Decisões e seus porquês

| Decisão | Por quê |
| --- | --- |
| **k3s** em vez de kubeadm | 2 vCPU / 8 GB não comporta o overhead de um control plane completo. O k3s já traz Traefik, local-path e containerd num binário só. |
| **Tailscale** como rede entre servidores | A API do Kubernetes (6443) nunca fica exposta à internet. Workers futuros entram por uma rede privada independente do provedor — não depende da rede interna da Hostinger, então funciona com VPS de qualquer lugar. |
| **DNS-01** (e não HTTP-01) no cert-manager | Não depende da porta 80 nem de o host ser alcançável de fora. É o que permite emitir certificado válido para serviços que só existem dentro da tailnet. |
| **App of Apps** no ArgoCD | Um único `Application` raiz observa um diretório; adicionar um projeto é adicionar um arquivo. Sem clicar em nada. |
| **CI escreve no Git, não no cluster** | O GitHub Actions nunca recebe credencial de cluster. Ele faz push de uma tag nova no repositório de infra, e o ArgoCD puxa. O raio de alcance de um token vazado do CI é um commit revertível. |
| **Instâncias compartilhadas** de Postgres/Valkey/RustFS | Uma instância por projeto multiplicaria o consumo de RAM por projeto. Compartilhado, cada projeto ganha seu banco/bucket/prefixo dentro da mesma instância. |
| **Valkey** em vez de Redis | Os charts do Bitnami estão sendo movidos para um repositório pago (agosto/2026). O Valkey é o fork open-source do Redis sob a Linux Foundation, com a mesma API. |
| **Sem Kafka** | Mesmo em modo KRaft, um broker JVM pediria ~1–1,5 GB de heap — perto de um quinto da RAM da máquina, competindo com todo o resto. Ficou fora até haver hardware que justifique. |
| **CloudNativePG** para o Postgres | Replicação é campo de primeira classe: sair de 1 instância para 3 é mudar um número no manifesto. Um chart genérico exigiria remontar a topologia. |

## Fluxo de uma requisição

Duas portas de entrada, com regras completamente diferentes:

**Internet pública** → `:80` / `:443` na interface pública → Traefik → apps de projeto (`*.hamasakis.dev`).
Serviços de plataforma nesse caminho recebem `403` do middleware do Traefik.

**Tailnet** → IP `<TAILNET_IP>` → Traefik (consoles) ou direto na porta `6443` (API do Kubernetes).
É por aqui que passam `kubectl`, o console do ArgoCD e o console do RustFS.

O `ufw` reforça a mesma divisão no nível do host: só `22`, `80` e `443` abertos na interface pública; tudo liberado em `tailscale0`.

## Fluxo de um deploy

```
push no repo do projeto
   ↓
GitHub Actions: build da imagem → push para ghcr.io
   ↓
GitHub Actions: kustomize edit set image → commit no repo infra
   ↓
ArgoCD detecta o commit (polling ~3 min)
   ↓
kubectl apply → pod novo sobe
```

Não há webhook. O ArgoCD só é alcançável pela tailnet, e o GitHub não consegue alcançar um endpoint dentro dela — a escolha foi manter o ArgoCD privado e aceitar o polling. Ver [07 - ArgoCD e GitOps](07-argocd-gitops.md).

## Onde cada coisa mora no repositório

```
ansible/          camadas 1 e 2 — provisionamento imperativo, roda 1× por servidor
platform/         manifestos e values dos componentes de plataforma (aplicados pelo Ansible)
clusters/
  production/
    root-app.yaml       o Application raiz (App of Apps)
    apps/               um Application por projeto e por serviço de dados
data-services/    manifestos das instâncias compartilhadas (camada 3, via ArgoCD)
apps/             manifestos dos projetos (camada 4, via ArgoCD)
scripts/          utilitários de máquina local
docs/             esta documentação
```

O nome `clusters/production/` já antecipa um segundo ambiente: um `clusters/staging/` reaproveitaria os mesmos roles do Ansible apontando para outro inventário.
