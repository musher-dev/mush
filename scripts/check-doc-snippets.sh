#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_BIN="$(mktemp)"
trap 'rm -f "$TMP_BIN"' EXIT

pushd "$ROOT_DIR" >/dev/null

go build -o "$TMP_BIN" ./cmd/mush

"$TMP_BIN" --help >/dev/null
"$TMP_BIN" version --json >/dev/null
"$TMP_BIN" paths --json >/dev/null

popd >/dev/null

echo "docs snippet commands validated"
