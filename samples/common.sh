#!/usr/bin/env bash
# Shared helpers for samples. Source this file; no secrets are stored here.
set -euo pipefail

cred_label() {
  local label="${1:-${AGENT_PWD_CRED:-}}"
  if [[ -z "$label" ]]; then
    echo "usage: $0 <label|id> [args...]" >&2
    echo "or set AGENT_PWD_CRED=<label>" >&2
    return 2
  fi
  printf '%s\n' "$label"
}

json_field() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

handle_pending() {
  local output="$1"
  local code="$2"
  if [[ "$code" -eq 10 ]]; then
    printf '%s\n' "$output"
    local id token
    id=$(printf '%s\n' "$output" | json_field confirmation_id)
    token=$(printf '%s\n' "$output" | json_field poll_token)
    printf '\nAction is pending. Ask a human to run:\n' >&2
    printf '  agent-pwd confirm approve %s\n' "$id" >&2
    printf '  agent-pwd confirm result %s --token %s\n' "$id" "$token" >&2
    return 10
  fi
  printf '%s\n' "$output"
  return "$code"
}
