#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: .githooks/run-commit-msg.sh <commit-msg-file>" >&2
  exit 1
fi

msg_file="$1"
if [[ ! -f "$msg_file" ]]; then
  echo "commit message file not found: $msg_file" >&2
  exit 1
fi

# First non-empty, non-comment line is the subject.
subject="$(grep -v '^[[:space:]]*#' "$msg_file" | sed -e '/^[[:space:]]*$/d' | head -n 1 || true)"

if [[ -z "$subject" ]]; then
  echo "commit message subject is empty" >&2
  exit 1
fi

# Allow merge/revert-generated messages.
if [[ "$subject" =~ ^Merge[[:space:]] ]] || [[ "$subject" =~ ^Revert[[:space:]] ]]; then
  exit 0
fi

conventional_re='^(feat|fix|docs|chore|ci|refactor|test|revert)(\([a-z0-9._/-]+\))?(!)?: .+'
if [[ "$subject" =~ $conventional_re ]]; then
  exit 0
fi

cat >&2 <<'EOF'
Invalid commit message.
Expected Conventional Commits format:
  <type>(optional-scope): <description>

Examples:
  feat(link): add retry logic for transient failures
  fix(auth): handle expired token refresh
  chore(ci): optimize pre-push checks
EOF
exit 1
