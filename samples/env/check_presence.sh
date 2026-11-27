#!/usr/bin/env bash
# Child process: verify an injected env var exists without printing its value.
set -euo pipefail
name="${1:-DEPLOY_TOKEN}"
value="${!name:-}"
if [[ -z "$value" ]]; then
  echo "$name is missing" >&2
  exit 1
fi
echo "$name is present (length ${#value})"
