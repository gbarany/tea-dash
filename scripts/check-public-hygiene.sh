#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"

files="$(git ls-files)"
status=0

check_regex() {
  local name="$1"
  local pattern="$2"
  local matches
  matches="$(grep -nE "$pattern" $files 2>/dev/null || true)"
  if [[ -n "$matches" ]]; then
    printf 'public hygiene check failed: %s\n%s\n' "$name" "$matches" >&2
    status=1
  fi
}

check_regex "local home directory paths" '(^|[^[:alnum:]_])/(Users|home)/[^[:space:]`"'"'"']+'
check_regex "macOS per-user temporary paths" '(^|[^[:alnum:]_])/var/folders/[^[:space:]`"'"'"']+'

op_matches="$(grep -nE 'op://[^<]' $files 2>/dev/null |
  grep -vE 'op://Private/tea-dash/credential|op://vault/item/credential|op://\.\.\.' || true)"
if [[ -n "$op_matches" ]]; then
  printf 'public hygiene check failed: non-placeholder 1Password references\n%s\n' "$op_matches" >&2
  status=1
fi

exit "$status"
