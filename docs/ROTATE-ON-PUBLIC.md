# Rotação de credenciais ao abrir o kit

Execute após tornar o `hinfra` público e migrar para `hinfra-workloads`:

**Sintoma:** ArgoCD só mostra `workloads-root` (Unknown) ou `example`/`root-app`; erro *authentication required: Repository not found* no `hinfra-workloads`. Falta o secret `workloads-repo-creds` (não é o mesmo que `infra-repo-creds` do hinfa público).

```bash
export KUBECONFIG=.secrets/vps-1.kubeconfig
HINFRA_WORKLOADS_TOKEN='<PAT read no hinfra-workloads>' ./scripts/argocd-register-workloads-repo.sh
```

Alternativa: `ansible-playbook … site.yml` com `vault_workloads_repo_token` no vault. Depois do sync, voltam os Applications filhos (`my-portfolio`, `postgres`, …).


1. `ansible-vault edit ansible/group_vars/all/vault.yml` — gere novos valores para:
   - `vault_cloudflare_api_token`
   - `vault_tailscale_authkey`
   - `vault_workloads_repo_token` (ArgoCD read no repo privado)
   - senhas Postgres / Valkey / RustFS e chaves R2, se aplicável
2. PAT de CI nos projetos (`HINFRA_WORKLOADS_TOKEN`) — fine-grained, write só em `hinfra-workloads`.
3. `cd ansible && ansible-playbook -i inventory/hosts.ini site.yml` (com chave deploy).
4. `kubectl -n argocd delete secret infra-repo-creds --ignore-not-found`
5. Re-sellar SealedSecrets se senhas de DB mudaram.
