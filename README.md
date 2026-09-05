# infra

Repositório único de provisionamento (Ansible) + GitOps (ArgoCD) da infraestrutura em `hamasakis.cloud` / `hamasakis.dev`.

> **Documentação completa em [`docs/`](docs/)** — arquitetura, runbooks, e as armadilhas já enfrentadas.
> Para um resumo visual, abra [`docs/overview.html`](docs/overview.html) no navegador.

## Visão geral

- **Provisionamento** (`ansible/`): prepara qualquer VPS nova do zero — usuário não-root, Tailscale, k3s, e os componentes de plataforma (Sealed Secrets, cert-manager, ArgoCD). Roda uma vez por servidor.
- **GitOps** (`clusters/`, `apps/`): o que o ArgoCD observa continuamente. Só workloads de projeto vivem aqui — os componentes de plataforma são geridos pelo Ansible, não pelo ArgoCD (evita ArgoCD gerenciar as próprias dependências).
- **Plataforma** (`platform/`): manifests dos componentes de plataforma (ClusterIssuer, values do Helm), aplicados pelos roles do Ansible — não pelo ArgoCD.

```
ansible/         provisionamento (rode 1x por VPS nova)
clusters/
  production/
    root-app.yaml    Application raiz (App of Apps) -> aponta pra apps/
    apps/             uma Application por projeto
platform/
  cert-manager/       ClusterIssuer (aplicado via Ansible)
  sealed-secrets/     notas de backup da master key
  argocd/             values do Helm
apps/
  <projeto>/          manifests do projeto (deployment/service/ingress/kustomization)
```

## Preparar sua máquina local

```bash
./scripts/setup-local-tools.sh   # instala ansible, gh, tailscale, kubeseal
gh auth login
sudo tailscale up
```

## Acesso

- SSH: `ssh vps` (usuário `deploy`, root desabilitado). Ver `ansible/bootstrap.yml`.
- kubectl: sua máquina precisa estar na tailnet (`tailscale up`). Kubeconfig local separado, `server:` apontando pro IP tailscale do node `k3s_server`.
- ArgoCD: só acessível via Tailscale, em `https://argocd.hamasakis.cloud` (sem exposição pública).

### DNS do ArgoCD (passo manual, por cluster)
Depois que o node `k3s_server` tiver um IP Tailscale, crie um registro `A` público em `argocd.<infra_domain>` apontando pra esse IP (ex: `100.86.241.1`), **não proxiado**. Parece contraditório ("público" apontando pra algo privado), mas funciona: o IP do Tailscale (faixa `100.64.0.0/10`) só é roteável por quem está na tailnet — fora dela a conexão simplesmente não chega, e o middleware do Traefik (`platform/argocd/tailnet-only-middleware.yaml.j2`) bloqueia mesmo assim. Isso evita precisar editar `/etc/hosts` em cada dispositivo. Sem esse registro, o cert-manager consegue emitir o certificado normalmente (o desafio é DNS-01, não depende de reachability), mas ninguém consegue resolver o nome.

## Provisionar uma VPS nova (worker ou cluster novo)

```bash
# 1. Uma vez, autenticado como root (cria o usuário `deploy` e fecha SSH root)
ansible-playbook -i ansible/inventory/hosts.ini ansible/bootstrap.yml --limit <host> -u root -k

# 2. Provisiona Tailscale + k3s (agent se o host estiver em [k3s_agents], server se em [k3s_server])
ansible-playbook -i ansible/inventory/hosts.ini ansible/site.yml --limit <host>
```

Pra adicionar um worker: acrescente o host em `ansible/inventory/hosts.ini` sob `[k3s_agents]` e rode os dois comandos acima com `--limit` nesse host.

## Registrar um projeto novo

1. Criar `apps/<projeto>/` com `deployment.yaml`, `service.yaml`, `ingress.yaml`, `kustomization.yaml` (copiar de `apps/my-portfolio/` como referência).
2. Criar `clusters/production/apps/<projeto>-app.yaml` (Application do ArgoCD apontando pra `apps/<projeto>`).
3. No repo do projeto, copiar `docs/deploy-workflow-template.yml` pra `.github/workflows/deploy.yml`, ajustando `IMAGE_NAME`/`APP_PATH`.
4. Configurar no repo do projeto o secret `INFRA_REPO_TOKEN` (PAT fine-grained, restrito a este repo `infra`, permissão Contents: Read/Write).
5. Se o pacote do GHCR for privado, criar um `imagePullSecret` no namespace do projeto (documentar no `deployment.yaml`).
6. Commitar e dar push — o ArgoCD sincroniza por polling (~3min) automaticamente.

## Segredos

- Segredos de cluster (tokens de API, etc): **Sealed Secrets** — `kubeseal` local, nunca comitar segredo em texto puro.
- Segredos de provisionamento (Tailscale authkey, token do Cloudflare, PAT do repo infra): **ansible-vault** — ver `ansible/group_vars/all/vault.yml.example`. A senha do vault fica em `~/.infra-vault-pass` (fora do git) — guarde uma cópia num gerenciador de senhas, sem ela o `vault.yml` commitado não decifra em outra máquina.
- **Backup da master key do Sealed Secrets é obrigatório** logo após a instalação — sem ela, um cluster novo/recriado não decifra os SealedSecrets existentes. Ver `platform/sealed-secrets/README.md`.
