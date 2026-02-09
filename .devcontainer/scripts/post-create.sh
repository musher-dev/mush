#!/usr/bin/env bash
set -euo pipefail

echo "==> Ensuring prerequisites..."
if ! command -v curl >/dev/null 2>&1; then
  sudo apt-get update
  sudo apt-get install -y --no-install-recommends curl ca-certificates
else
  # ca-certificates is sometimes missing even when curl is present.
  if ! dpkg -s ca-certificates >/dev/null 2>&1; then
    sudo apt-get update
    sudo apt-get install -y --no-install-recommends ca-certificates
  fi
fi

echo "==> Installing Claude (native)..."
export PATH="$HOME/.local/bin:$PATH"
if ! command -v claude >/dev/null 2>&1 || [[ "$(command -v claude)" != "$HOME/.local/bin/claude" ]]; then
  curl -fsSL https://claude.ai/install.sh | bash
  hash -r
fi
if ! command -v claude >/dev/null 2>&1; then
  echo "ERROR: claude CLI not found after installation." >&2
  exit 1
fi

echo "==> Installing Codex CLI..."
if ! command -v codex >/dev/null 2>&1; then
  npm install -g @openai/codex@latest
fi
if ! command -v codex >/dev/null 2>&1; then
  echo "ERROR: codex CLI not found after installation." >&2
  exit 1
fi

echo "==> Installing Go tools..."

go install mvdan.cc/gofumpt@v0.7.0
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.8.0

echo "==> Downloading Go modules..."
go mod download

echo "==> Post-create setup complete."
