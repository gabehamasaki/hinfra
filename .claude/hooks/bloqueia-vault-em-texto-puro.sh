#!/usr/bin/env bash
# Impede commit do ansible-vault em texto puro.
#
# Este é o único erro desta infra que não tem desfazer: segredo que entra no
# histórico do Git continua recuperável mesmo depois de removido, e rotacionar
# não apaga o valor antigo. O fluxo correto (encrypt -> git add -f) é fácil de
# executar pela metade, então vale um guardrail.
set -uo pipefail

VAULT="ansible/group_vars/all/vault.yml"
REPO="$(pwd)"

CMD="$(cat | python3 -c '
import json, sys
d = json.load(sys.stdin)
print(d.get("command") or d.get("tool_input", {}).get("command", ""))
' 2>/dev/null || true)"

# Só interessa commit - deixa passar todo o resto sem custo.
[[ "$CMD" == *"git commit"* ]] || { echo '{"permission":"allow"}'; exit 0; }

# Não está staged: nada a proteger.
git -C "$REPO" diff --cached --name-only 2>/dev/null | grep -qx "$VAULT" || {
  echo '{"permission":"allow"}'
  exit 0
}

MESSAGE=$(cat <<EOF
BLOQUEADO: $VAULT não pode ser commitado (repo público).

Mantenha vault.yml só na máquina local, fora do Git. Use vault.yml.example como modelo.

Para corrigir:
  git reset HEAD -- $VAULT
EOF
)

printf '%s\n' "$MESSAGE" >&2

python3 -c 'import json,sys; m=sys.stdin.read(); print(json.dumps({"permission":"deny","user_message":m,"agent_message":m}))' <<<"$MESSAGE" 2>/dev/null || {
  echo '{"permission":"deny","user_message":"vault.yml staged em texto puro","agent_message":"vault.yml staged em texto puro"}'
}
exit 2
