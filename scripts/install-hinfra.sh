#!/usr/bin/env bash
# Install hinfra CLI from GitHub Releases.
set -euo pipefail

HINFRA_VERSION="${HINFRA_VERSION:-v1.0.0}"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"
REPO="gabehamasaki/hinfra"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *)
    echo "OS não suportado: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *)
    echo "arquitetura não suportada: $(uname -m)" >&2
    exit 1
    ;;
esac

asset="hinfra-${os}-${arch}"
url="https://github.com/${REPO}/releases/download/${HINFRA_VERSION}/${asset}"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "Baixando ${url} ..."
curl -fsSL "$url" -o "${tmpdir}/hinfra"
chmod +x "${tmpdir}/hinfra"
mkdir -p "$INSTALL_DIR"
mv "${tmpdir}/hinfra" "${INSTALL_DIR}/hinfra"

if ! command -v hinfra >/dev/null 2>&1; then
  echo "Instalado em ${INSTALL_DIR}/hinfra — adicione ao PATH se necessário." >&2
else
  hinfra version
fi
