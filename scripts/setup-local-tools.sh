#!/usr/bin/env bash
# Instala as ferramentas necessárias pra operar este repo numa máquina local:
# Ansible (+ collections), GitHub CLI, cliente Tailscale, kubeseal.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

echo "== Ansible + collections =="
sudo apt update
sudo apt install -y ansible sshpass
ansible-galaxy collection install -r "${REPO_ROOT}/ansible/requirements.yml"

echo "== GitHub CLI =="
if ! command -v gh >/dev/null; then
  (type -p wget >/dev/null || (sudo apt update && sudo apt-get install wget -y)) \
  && sudo mkdir -p -m 755 /etc/apt/keyrings \
  && wget -nv -O/tmp/githubcli-archive-keyring.gpg https://cli.github.com/packages/githubcli-archive-keyring.gpg \
  && sudo cp /tmp/githubcli-archive-keyring.gpg /etc/apt/keyrings/githubcli-archive-keyring.gpg \
  && sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
  && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
  && sudo apt update \
  && sudo apt install gh -y
fi

echo "== Tailscale client =="
curl -fsSL https://tailscale.com/install.sh | sh

echo "== kubeseal CLI =="
KUBESEAL_VERSION=$(curl -s https://api.github.com/repos/bitnami-labs/sealed-secrets/releases/latest | grep -oP '"tag_name": "v\K[^"]+')
curl -OL "https://github.com/bitnami-labs/sealed-secrets/releases/download/v${KUBESEAL_VERSION}/kubeseal-${KUBESEAL_VERSION}-linux-amd64.tar.gz"
tar -xvzf "kubeseal-${KUBESEAL_VERSION}-linux-amd64.tar.gz" kubeseal
sudo install -m 755 kubeseal /usr/local/bin/kubeseal
rm -f kubeseal "kubeseal-${KUBESEAL_VERSION}-linux-amd64.tar.gz"

echo "== Versions =="
ansible --version | head -1
gh --version | head -1
tailscale version
kubeseal --version
echo "Tudo instalado. Rode 'gh auth login' pra autenticar o GitHub CLI."
