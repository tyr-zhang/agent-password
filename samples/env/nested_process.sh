#!/usr/bin/env bash
# Env injection: grandchild inherits the injected variable.
# Usage: nested_process.sh <envvar-label> [env-name]
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/../common.sh"
label=$(cred_label "${1:-}")
name="${2:-DEPLOY_TOKEN}"
agent-pwd env run --cred "$label" -- bash -c '"$0" "$1"' "$HERE/check_presence.sh" "$name"
