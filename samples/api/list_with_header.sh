#!/usr/bin/env bash
# API: extra header and pagination path. Usage: list_with_header.sh <label> <path>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
path="${2:-/v1/items?page=1&limit=20}"
agent-pwd api call --cred "$label" --method GET --path "$path" \
  --header 'Accept: application/json' \
  --header 'X-Client: agent-pwd-sample'
