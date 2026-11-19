# Template do vault.yml: só referências ao 1Password, nenhum valor. Seguro no Git.
# A fonte de verdade é o item "hinfra ansible vault" do cofre Infra no 1Password.
#
# Gerar o vault.yml (criptografado, nunca em texto puro no disco, nunca commitado):
#   op inject < group_vars/all/vault.yml.tpl \
#     | script -qec "ansible-vault encrypt - --output group_vars/all/vault.yml" /dev/null
#
# Bloco literal (|-) em vez de aspas: um valor com ' ou " quebraria o YAML.
#
# Fora deste template de propósito (opcionais ou sem valor guardado):
#   vault_tailscale_authkey                  gerar no admin do Tailscale antes de provisionar servidor novo
#   vault_alertmanager_critical_webhook_url  opcional
#   vault_k3s_backup_healthcheck_url         opcional
#   vault_hinfra_repo_token                  opcional; o role cai no vault_workloads_repo_token

vault_cloudflare_api_token: |-
  {{ op://Infra/hinfra ansible vault/vault_cloudflare_api_token }}
vault_workloads_repo_token: |-
  {{ op://Infra/hinfra ansible vault/vault_workloads_repo_token }}
vault_alertmanager_discord_webhook_url: |-
  {{ op://Infra/hinfra ansible vault/vault_alertmanager_discord_webhook_url }}
vault_grafana_admin_password: |-
  {{ op://Infra/hinfra ansible vault/vault_grafana_admin_password }}
vault_postgres_superuser_password: |-
  {{ op://Infra/hinfra ansible vault/vault_postgres_superuser_password }}
vault_valkey_password: |-
  {{ op://Infra/hinfra ansible vault/vault_valkey_password }}
vault_rustfs_access_key: |-
  {{ op://Infra/hinfra ansible vault/vault_rustfs_access_key }}
vault_rustfs_secret_key: |-
  {{ op://Infra/hinfra ansible vault/vault_rustfs_secret_key }}
vault_r2_access_key_id: |-
  {{ op://Infra/hinfra ansible vault/vault_r2_access_key_id }}
vault_r2_secret_access_key: |-
  {{ op://Infra/hinfra ansible vault/vault_r2_secret_access_key }}
