#!/usr/bin/env bash
set -euo pipefail

ensure_writable_dir() {
  local dir="$1"

  if mkdir -p "$dir" 2>/dev/null && test -w "$dir"; then
    return 0
  fi

  echo "WARN: '$dir' is not writable; attempting to repair ownership/permissions..."
  ls -ld "$dir" 2>/dev/null || true

  # Named volumes can retain old root ownership across rebuilds; fix only the relevant subtree.
  if [[ "$dir" == "$HOME/.claude"* ]]; then
    sudo mkdir -p "$HOME/.claude" "$dir" || true
    sudo chown -R "$USER:$USER" "$HOME/.claude" || true
    sudo chmod -R u+rwX "$HOME/.claude" || true
  elif [[ "$dir" == "$HOME/.codex"* ]]; then
    sudo mkdir -p "$HOME/.codex" "$dir" || true
    sudo chown -R "$USER:$USER" "$HOME/.codex" || true
    sudo chmod -R u+rwX "$HOME/.codex" || true
  else
    sudo mkdir -p "$dir" || true
    sudo chown -R "$USER:$USER" "$dir" || true
    sudo chmod -R u+rwX "$dir" || true
  fi

  mkdir -p "$dir" 2>/dev/null || true
  if ! test -w "$dir"; then
    echo "WARN: '$dir' is still not writable. Continuing, but some tools may fail." >&2
    ls -ld "$dir" 2>/dev/null || true
    return 1
  fi

  return 0
}

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

echo "==> Installing shellcheck..."
if ! command -v shellcheck >/dev/null 2>&1; then
  sudo apt-get update
  sudo apt-get install -y --no-install-recommends shellcheck
fi

ensure_writable_dir "$HOME/.claude" || true
ensure_writable_dir "$HOME/.claude/downloads" || true
ensure_writable_dir "$HOME/.codex" || true
ensure_writable_dir "$HOME/.config/gh" || true

echo "==> Installing Claude (native)..."
export PATH="$HOME/.local/bin:$PATH"
if ! command -v claude >/dev/null 2>&1 || [[ "$(command -v claude)" != "$HOME/.local/bin/claude" ]]; then
  set +e
  curl -fsSL https://claude.ai/install.sh | bash
  claude_install_rc=$?
  set -e
  hash -r

  if (( claude_install_rc != 0 )); then
    echo "WARN: Claude native install failed (exit $claude_install_rc). Continuing." >&2
  fi
fi
if ! command -v claude >/dev/null 2>&1; then
  echo "WARN: claude CLI not found after installation attempt. Continuing." >&2
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

go install mvdan.cc/gofumpt@v0.9.2
go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0

echo "==> Installing GoReleaser..."
if ! command -v goreleaser >/dev/null 2>&1; then
  go install github.com/goreleaser/goreleaser/v2@v2.13.3 || {
    echo "WARN: GoReleaser installation failed. Continuing." >&2
  }
fi

echo "==> Downloading Go modules..."
go mod download

echo "==> Post-create setup complete."
