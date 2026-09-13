# observability (removido)

O stack de monitoring passou a ser gerido pelo **ArgoCD** (`platform-root` → `monitoring`).

Secrets (`grafana-admin`, `alertmanager-discord`) ficam no role **`observability_secrets`**.
