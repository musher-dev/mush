#!/usr/bin/env bash
set -euo pipefail

mapfile -d '' staged_files < <(git diff --cached --name-only --diff-filter=ACMR -z)

if [[ ${#staged_files[@]} -eq 0 ]]; then
  exit 0
fi

go_files=()
shell_files=()
workflow_files=()
has_mod_changes=0

for file in "${staged_files[@]}"; do
  case "$file" in
    *.go)
      go_files+=("$file")
      ;;
    go.mod | go.sum)
      has_mod_changes=1
      ;;
    *.sh | .githooks/*)
      shell_files+=("$file")
      ;;
    .github/workflows/*.yml | .github/workflows/*.yaml)
      workflow_files+=("$file")
      ;;
  esac
done

if [[ ${#go_files[@]} -gt 0 ]]; then
  task check:fmt:files -- "${go_files[@]}"

  # golangci-lint run requires all named files to be in one directory.
  # Convert file paths to unique ./pkg/... patterns so multi-package
  # commits are linted correctly.
  declare -A go_dirs
  for f in "${go_files[@]}"; do
    go_dirs["$(dirname "$f")"]=1
  done
  lint_targets=()
  for d in "${!go_dirs[@]}"; do
    lint_targets+=("./${d}/...")
  done
  task check:lint:files -- "${lint_targets[@]}"
fi

if [[ $has_mod_changes -eq 1 ]]; then
  task check:mod
fi

if [[ ${#shell_files[@]} -gt 0 ]]; then
  task check:shell:files -- "${shell_files[@]}"
fi

if [[ ${#workflow_files[@]} -gt 0 ]]; then
  task check:workflow:files -- "${workflow_files[@]}"
fi
