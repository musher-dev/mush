#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 ]]; then
  echo "usage: .githooks/run-pre-push.sh <remote-name> <remote-url> <stdin-file>" >&2
  exit 1
fi

remote_name="$1"
remote_url="$2"
stdin_file="$3"

if [[ ! -f "$stdin_file" ]]; then
  echo "pre-push stdin file not found: $stdin_file" >&2
  exit 1
fi

zero_sha='0000000000000000000000000000000000000000'
changed_files=()
fallback_full=0

while read -r local_ref local_sha _remote_ref remote_sha; do
  [[ -z "${local_ref:-}" ]] && continue

  # Skip deleted refs.
  if [[ "$local_sha" == "$zero_sha" ]]; then
    continue
  fi

  # New branch or unknown remote SHA: use conservative fallback.
  if [[ "$remote_sha" == "$zero_sha" ]]; then
    fallback_full=1
    continue
  fi

  while IFS= read -r changed; do
    [[ -n "$changed" ]] && changed_files+=("$changed")
  done < <(git diff --name-only "${remote_sha}..${local_sha}")
done <"$stdin_file"

if [[ $fallback_full -eq 1 ]]; then
  task check:lint
  task check:test
  exit 0
fi

if [[ ${#changed_files[@]} -eq 0 ]]; then
  exit 0
fi

go_files=()
shell_files=()
workflow_files=()
has_mod_changes=0
run_install_tests=0

for file in "${changed_files[@]}"; do
  case "$file" in
    *.go)
      go_files+=("$file")
      ;;
    go.mod | go.sum)
      has_mod_changes=1
      ;;
    *.sh | .githooks/*)
      shell_files+=("$file")
      [[ "$file" == "install.sh" ]] && run_install_tests=1
      ;;
    .github/workflows/*.yml | .github/workflows/*.yaml)
      workflow_files+=("$file")
      ;;
    test/install/*)
      run_install_tests=1
      ;;
  esac
done

if [[ ${#go_files[@]} -gt 0 ]]; then
  task check:fmt:files -- "${go_files[@]}"
  task check:lint:files -- "${go_files[@]}"
fi

if [[ $has_mod_changes -eq 1 ]]; then
  task check:mod
  task check:vuln
fi

if [[ ${#shell_files[@]} -gt 0 ]]; then
  task check:shell:files -- "${shell_files[@]}"
fi

if [[ ${#workflow_files[@]} -gt 0 ]]; then
  task check:workflow:files -- "${workflow_files[@]}"
fi

if [[ $run_install_tests -eq 1 ]]; then
  task check:install
fi

if [[ ${#go_files[@]} -gt 0 ]] || [[ $has_mod_changes -eq 1 ]]; then
  if [[ $has_mod_changes -eq 1 ]]; then
    task check:test:packages -- ./...
  else
    declare -A package_set=()
    for file in "${go_files[@]}"; do
      dir="$(dirname "$file")"
      if [[ "$dir" == "." ]]; then
        package_set["./"]=1
      else
        package_set["./$dir"]=1
      fi
    done

    packages=()
    for pkg in "${!package_set[@]}"; do
      packages+=("$pkg")
    done

    if [[ ${#packages[@]} -gt 0 ]]; then
      task check:test:packages -- "${packages[@]}"
    fi
  fi
fi

# Keep context for future debugging without lint warnings.
if [[ -n "$remote_name" ]] && [[ -n "$remote_url" ]]; then
  :
fi
