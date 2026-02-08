#!/usr/bin/env bash
set -euo pipefail

echo "==> Installing Go tools..."

go install mvdan.cc/gofumpt@v0.7.0
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.8.0

echo "==> Downloading Go modules..."
go mod download

echo "==> Post-create setup complete."
