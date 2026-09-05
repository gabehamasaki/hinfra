#!/usr/bin/env bash
# Impede commit do ansible-vault em texto puro.
#
# Este é o único erro desta infra que não tem desfazer: segredo que entra no
# histórico do Git continua recuperável mesmo depois de removido, e rotacionar
# não apaga o valor antigo. O fluxo correto (encrypt -> git add -f) é fácil de
# executar pela metade, então vale um guardrail.
#
# Sai com 2 para bloquear e devolver a mensagem ao Claude.
set -uo pipefail

VAULT="ansible/group_vars/all/vault.yml"
REPO="${CLAUDE_PROJECT_DIR:-$PWD}"

CMD="$(cat | python3 -c \
  'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("command",""))' \
  2>/dev/null || true)"

# Só interessa commit - deixa passar todo o resto sem custo.
[[ "$CMD" == *"git commit"* ]] || exit 0

# Não está staged: nada a proteger.
git -C "$REPO" diff --cached --name-only 2>/dev/null | grep -qx "$VAULT" || exit 0

if [[ "$(git -C "$REPO" show ":$VAULT" 2>/dev/null | head -1)" == '$ANSIBLE_VAULT'* ]]; then
  exit 0
fi

cat >&2 <<EOF
BLOQUEADO: $VAULT está staged em TEXTO PURO.

Commitar isso expõe os segredos no histórico do Git de forma permanente -
removê-los depois não os torna irrecuperáveis.

Para corrigir:
  script -qec "ansible-vault encrypt $VAULT --vault-password-file ~/.infra-vault-pass" /dev/null
  git add -f $VAULT

Confirme antes de tentar de novo:
  git show ":$VAULT" | head -1    # tem que começar com \$ANSIBLE_VAULT
EOF
exit 2
