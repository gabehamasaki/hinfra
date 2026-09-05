#!/usr/bin/env bash
# Leva o kubeconfig do cluster para o lado Windows, onde ferramentas gráficas
# como o Lens rodam (elas não enxergam o filesystem do WSL).
#
# Uso:  ./scripts/sync-kubeconfig-windows.sh [caminho/do/kubeconfig]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
SRC="${1:-${REPO_ROOT}/.secrets/vps-1.kubeconfig}"

if [ ! -f "$SRC" ]; then
  echo "kubeconfig não encontrado: $SRC" >&2
  echo "rode o site.yml primeiro - ele baixa o kubeconfig para .secrets/" >&2
  exit 1
fi

if ! grep -q "^/" /proc/version 2>/dev/null && ! grep -qi microsoft /proc/version; then
  echo "este script só faz sentido dentro do WSL" >&2
  exit 1
fi

WINUSER="$(cmd.exe /c 'echo %USERNAME%' 2>/dev/null | tr -d '\r\n')"
WINKUBE="/mnt/c/Users/${WINUSER}/.kube"
DEST="${WINKUBE}/config"

mkdir -p "$WINKUBE"

if [ -f "$DEST" ]; then
  # Mescla preservando outros clusters já configurados no Windows.
  # SRC vem primeiro de propósito: no merge do kubectl o primeiro arquivo a
  # definir uma chave vence, então o repositório sobrescreve uma entrada antiga
  # de mesmo nome - que é o que se quer depois de recriar um cluster.
  KUBECONFIG="${SRC}:${DEST}" kubectl config view --flatten > "${DEST}.tmp"
  mv "${DEST}.tmp" "$DEST"
  echo "mesclado em C:\\Users\\${WINUSER}\\.kube\\config"
else
  cp "$SRC" "$DEST"
  echo "copiado para C:\\Users\\${WINUSER}\\.kube\\config"
fi

echo
echo "contextos disponíveis no Windows:"
KUBECONFIG="$DEST" kubectl config get-contexts

# O kubeconfig aponta para um IP da tailnet: sem Tailscale ativo no Windows
# (o do WSL não vale), qualquer ferramenta gráfica de lá não alcança o cluster.
TS="/mnt/c/Program Files/Tailscale/tailscale.exe"
if [ -x "$TS" ]; then
  if "$TS" status >/dev/null 2>&1; then
    echo
    echo "Tailscale do Windows: conectado"
  else
    echo
    echo "AVISO: o Tailscale do Windows não está conectado - o Lens não vai"
    echo "alcançar o cluster até você conectá-lo." >&2
  fi
else
  echo
  echo "AVISO: Tailscale não encontrado no Windows. O kubeconfig aponta para um"
  echo "IP da tailnet, então é necessário instalá-lo e conectar." >&2
fi
