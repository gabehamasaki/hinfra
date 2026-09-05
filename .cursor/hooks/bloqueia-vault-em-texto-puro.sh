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

if [[ "$(git -C "$REPO" show ":$VAULT" 2>/dev/null | head -1)" == '$ANSIBLE_VAULT'* ]]; then
  echo '{"permission":"allow"}'
  exit 0
fi

MESSAGE=$(cat <<EOF
BLOQUEADO: $VAULT está staged em TEXTO PURO.

Commitar isso expõe os segredos no histórico do Git de forma permanente -
removê-los depois não os torna irrecuperáveis.

Para corrigir:
  script -qec "ansible-vault encrypt $VAULT --vault-password-file ~/.infra-vault-pass" /dev/null
  git add -f $VAULT

Confirme antes de tentar de novo:
  git show ":$VAULT" | head -1    # tem que começar com \$ANSIBLE_VAULT
EOF
)

printf '%s\n' "$MESSAGE" >&2

python3 -c 'import json,sys; m=sys.stdin.read(); print(json.dumps({"permission":"deny","user_message":m,"agent_message":m}))' <<<"$MESSAGE" 2>/dev/null || {
  echo '{"permission":"deny","user_message":"vault.yml staged em texto puro","agent_message":"vault.yml staged em texto puro"}'
}
exit 2
