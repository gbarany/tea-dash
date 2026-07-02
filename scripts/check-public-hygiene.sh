#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"

status=0

check_regex() {
  local name="$1"
  local pattern="$2"
  local matches
  matches="$(git grep -nE "$pattern" -- . ':(exclude).git' ':(exclude)scripts/check-public-hygiene.sh' || true)"
  if [[ -n "$matches" ]]; then
    printf 'public hygiene check failed: %s\n%s\n' "$name" "$matches" >&2
    status=1
  fi
}

check_regex "local home directory paths" '(^|[^[:alnum:]_])/(Users|home)/[^[:space:]`"'"'"']+'
check_regex "macOS per-user temporary paths" '(^|[^[:alnum:]_])/var/folders/[^[:space:]`"'"'"']+'
check_regex "secret-manager item routes" 'op://'

exit "$status"
