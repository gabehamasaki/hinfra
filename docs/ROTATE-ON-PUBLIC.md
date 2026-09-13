# Rotação de credenciais ao abrir o kit

Execute após tornar o `hinfra` público e migrar para `hinfra-workloads`:

1. `ansible-vault edit ansible/group_vars/all/vault.yml` — gere novos valores para:
   - `vault_cloudflare_api_token`
   - `vault_tailscale_authkey`
   - `vault_workloads_repo_token` (ArgoCD read no repo privado)
   - senhas Postgres / Valkey / RustFS e chaves R2, se aplicável
2. PAT de CI nos projetos (`HINFRA_WORKLOADS_TOKEN`) — fine-grained, write só em `hinfra-workloads`.
3. `cd ansible && ansible-playbook -i inventory/hosts.ini site.yml` (com chave deploy).
4. `kubectl -n argocd delete secret infra-repo-creds --ignore-not-found`
5. Re-sellar SealedSecrets se senhas de DB mudaram.
