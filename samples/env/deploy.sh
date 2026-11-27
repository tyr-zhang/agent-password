#!/usr/bin/env bash
# Env injection: run a child with the secret present. Usage: deploy.sh <label|id> [env-name]
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/../common.sh"
label=$(cred_label "${1:-}")
name="${2:-DEPLOY_TOKEN}"
agent-pwd env run --cred "$label" -- "$HERE/check_presence.sh" "$name"
