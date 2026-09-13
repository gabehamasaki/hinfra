# hinfra

Kit público de provisionamento (Ansible) + GitOps (ArgoCD) para k3s. Workloads de produção ficam no repo privado `hinfra-workloads`.

Documentação completa em [`docs/`](docs/) — **é a fonte de verdade**. Este arquivo e as skills apontam para lá em vez de repetir conteúdo, para não existirem duas versões que divergem.

## A regra que organiza tudo

Quatro camadas, separadas por **frequência de mudança** — é isso que define quem gerencia o quê:

| Camada | Quem gerencia | Onde |
| --- | --- | --- |
| Provisionamento (hardening, Tailscale, k3s) | Ansible, roda 1× por servidor | `ansible/` |
| Plataforma (cert-manager, Sealed Secrets, ArgoCD, KEDA, operadores) | Ansible + Helm | `ansible/roles/`, `platform/` |
| Serviços de dados (Postgres, Valkey, RustFS) | ArgoCD | `data-services/` |
| Projetos (imagem construída pelo CI) | ArgoCD | `apps/` |

O ArgoCD **não** gerencia as duas primeiras camadas: ele depende do cert-manager (certificado próprio) e do repositório Git para funcionar. Componente do qual o ArgoCD depende não pode depender do ArgoCD para existir.

Ao adicionar algo novo, a primeira pergunta é sempre: **isso muda com que frequência?** A resposta escolhe a camada.

## Invariantes

Violar qualquer um destes causa dano difícil ou impossível de reverter:

- **Nunca commitar `vault.yml`.** Mantenha só local (criptografado com ansible-vault). Use `vault.yml.example` como modelo.
- **Nunca expor console administrativo publicamente.** ArgoCD e RustFS são alcançáveis só pela tailnet, via middleware do Traefik. O DNS deles aponta para o IP Tailscale (`<TAILNET_IP>`), que não é roteável fora dela.
- **Sempre fixar `targetRevision`** em Application com fonte Helm. `"*"` trava o Application em `Unknown` sem mensagem de erro.
- **`.secrets/` nunca vai para o Git.** Contém chave SSH, kubeconfig e a master key do Sealed Secrets.
- **`git pull --rebase` antes de push.** O CI dos projetos commita no repo `hinfra-workloads` (bump de tag de imagem).

## Acesso

```bash
ssh vps                                              # usuário deploy; root desabilitado
export KUBECONFIG=.secrets/vps-1.kubeconfig          # exige estar na tailnet
```

Ansible precisa de pty — sem isso aborta com `requires blocking IO`:

```bash
cd ansible && script -qec "ansible-playbook -i inventory/hosts.ini site.yml \
  --private-key ../.secrets/vps-1_deploy_ed25519" /dev/null
```

## Restrição de recursos

2 vCPU / 8 GB, node único. Memória tem folga; **CPU é o limite real** — já há ~41% do orçamento de requests comprometido. Toda adição ao cluster precisa de `requests`/`limits` explícitos, e qualquer coisa que peça mais que ~200m de CPU merece justificativa (foi por isso que Kafka ficou de fora e Prometheus segue adiado).

## Verificar antes de afirmar

Esta infra tem várias camadas onde configuração errada **falha em silêncio** em vez de dar erro: valor de Helm que não existe é ignorado, task do Ansible pulada por `creates:` parece `ok`, anotação de Ingress sem efeito. Depois de mudar algo, confirme que produziu efeito — `kubectl get`, `curl`, o pod rodando — em vez de concluir pelo comando ter retornado sucesso.
