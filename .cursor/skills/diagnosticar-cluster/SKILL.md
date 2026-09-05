---
name: diagnosticar-cluster
description: Diagnostica problemas nesta infra — pod que não sobe, site fora do ar, certificado que não emite, Application travado em Unknown ou OutOfSync, 403 inesperado, ImagePullBackOff, DNS que não resolve, acesso SSH ou kubectl que parou de funcionar. Use sempre que algo estiver quebrado, lento, instável ou "estranho" no cluster ou na VPS, inclusive quando o relato for vago ("o site caiu", "não tô conseguindo acessar", "parou de funcionar"). Nesta infra o erro visível frequentemente não é a causa raiz, e seguir a escada de diagnóstico evita horas perseguindo a mensagem errada.
---

# Diagnosticar

Casos completos, com sintoma, causa e correção, em [`docs/11-armadilhas.md`](../../../docs/11-armadilhas.md). Comandos de operação em [`docs/10-runbooks.md`](../../../docs/10-runbooks.md).

## A regra central

**O erro visível frequentemente não é a causa.** Dois casos reais desta infra:

- O Traefik logava `middleware does not exist` e `secret does not exist`. Ambos eram transitórios, de quando os recursos ainda estavam sendo criados. A causa real do `403` era SNAT mascarando o IP de origem — nada a ver com as mensagens.
- Um certificado falhava por rate limit do Let's Encrypt. A causa real era um chart sobrescrevendo o Secret TLS, muitas vezes, até estourar a cota.

Antes de perseguir a última mensagem de log, compare **comportamento esperado × observado**. No caso do `403`, o que resolveu foi notar que dava 403 *nos dois caminhos* — de dentro e de fora da tailnet — quando o esperado era 200 de dentro. Isso apontou para "o filtro não enxerga o IP certo", não para "o middleware sumiu".

## Estado geral primeiro

```bash
export KUBECONFIG=.secrets/vps-1.kubeconfig
kubectl get nodes
kubectl get applications -n argocd
kubectl get pods -A | grep -v Running | grep -v Completed
kubectl top nodes
kubectl get certificate -A
ssh vps 'sudo ufw status verbose && tailscale status'
```

Referência de saudável: ~10% de CPU, ~3 GB de RAM, todos os Applications `Synced`/`Healthy`.

## Escadas por sintoma

### Application em `Unknown`

Quase sempre é falha ao **gerar** os manifestos, não ao aplicá-los. O erro não aparece no application-controller:

```bash
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-repo-server --tail=100 | grep -i error
```

Foi assim que apareceu `auth.enabled is true but no authentication method is configured` do Valkey — invisível no `describe`.

Causas frequentes: `targetRevision: "*"`, values incompletos para o que o chart exige, credencial do repositório faltando.

### Application oscilando entre `Synced` e `OutOfSync`

Disputa de ownership: algo fora do ArgoCD está reescrevendo um recurso que ele gerencia. Veja qual:

```bash
kubectl get application <nome> -n argocd -o json | python3 -c "
import json,sys
d=json.load(sys.stdin)
for r in d.get('status',{}).get('resources',[]):
    if r.get('status') != 'Synced': print(r)"
```

Se for um Secret TLS, o culpado provável é o chart criando o próprio certificado. Ver a skill `servico-no-cluster`.

### `403` vindo da tailnet

O middleware `IPAllowList` está comparando o IP errado. Confirme:

```bash
kubectl get svc traefik -n kube-system -o jsonpath='{.spec.externalTrafficPolicy}{"\n"}'   # tem que ser Local
```

Com `Cluster`, o kube-proxy aplica SNAT e o Traefik vê um IP da rede de pods — que nunca casa com `100.64.0.0/10`, então bloqueia **todos**, inclusive quem está legitimamente na tailnet.

Teste os dois caminhos, porque a diferença entre eles é diagnóstica:

```bash
curl -s -o /dev/null -w "%{http_code}\n" --resolve <host>:443:100.86.241.1 https://<host>/    # espera 200
curl -s -o /dev/null -w "%{http_code}\n" -k --resolve <host>:443:187.127.62.20 https://<host>/ # espera 403
```

### Certificado não emite

```bash
kubectl describe certificate <nome> -n <ns> | grep -A5 Message
kubectl get order,challenge -A
```

| Mensagem | Causa |
| --- | --- |
| `429 rateLimited` | 5 certificados/semana para o mesmo host. Só esperar — o cert-manager tenta sozinho com backoff. |
| `no PEM data was found` | Algo está sobrescrevendo o Secret TLS. Procure o chart criando o próprio. |
| Desafio DNS parado | Token do Cloudflare inválido ou sem escopo na zona. |

Validar o token (endpoint de **conta**, não `/user/tokens/verify`, que recusa tokens `cfat_` válidos):

```bash
curl -s "https://api.cloudflare.com/client/v4/accounts/347d85d3ee7db762e6af1cdfb874f8e8/tokens/verify" \
  -H "Authorization: Bearer <token>"
```

### Serviço inacessível pelo navegador

Verifique nesta ordem, do mais externo para o mais interno:

1. **DNS** — `nslookup <host> 1.1.1.1`. Se o `1.1.1.1` responde e sua máquina não, é cache local (inclusive cache **negativo**, que persiste depois de o registro passar a existir). `ipconfig /flushdns` no Windows.
2. **Tailnet** — hosts `*.hamasakis.cloud` exigem Tailscale conectado. No Windows é um dispositivo diferente do WSL: os dois precisam estar conectados para seus respectivos usos.
3. **Certificado** — `kubectl get certificate -A`
4. **Ingress** — `kubectl get ingress -A`
5. **Pod** — `kubectl get pods -n <ns>`

### `ImagePullBackOff`

Uma destas três: o build ainda não terminou, o pacote do GHCR é privado sem `imagePullSecret`, ou a tag no `kustomization.yaml` não corresponde a imagem publicada.

```bash
kubectl describe pod -n <ns> -l app=<nome> | tail -20
```

### Perdi acesso SSH

1. Console VNC/recovery no painel da Hostinger
2. Restaurar o snapshot (API da Hostinger, `virtualMachineId=1957194`)
3. Em último caso, reinstalar e rodar `bootstrap.yml` + `site.yml` — é exatamente para isso que o repositório é replicável

## Padrões que explicam quase tudo

Quando o sintoma não encaixar em nenhuma escada acima, essas quatro categorias cobrem quase todos os casos já vistos:

1. **Configuração ignorada em silêncio.** Valor de Helm inexistente, task pulada por `creates:`, config em arquivo que mudou de lugar. Nada falha — só não acontece. Verifique que a mudança *fez efeito*, não que foi *escrita*.
2. **O erro visível não é a causa.** Compare esperado × observado antes de perseguir a última mensagem.
3. **Ecossistema em movimento.** Repositórios Helm mudam de organização, charts mudam de formato de config. Confirme que a URL responde antes de assumir que está errado o seu uso.
4. **Local ≠ remoto.** Módulo do Ansible resolvendo caminho no host errado, lib Python faltando do outro lado, arquivo que existe no repositório mas não no contexto do Docker build. Pergunte sempre: *onde este código está rodando de fato?*
