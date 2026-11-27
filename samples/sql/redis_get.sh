#!/usr/bin/env bash
# Redis: read-only command allowlist. Usage: redis_get.sh <label> <key>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
key="${2:?usage: redis_get.sh <label> <key>}"
agent-pwd sql query --cred "$label" -- "GET ${key}"
