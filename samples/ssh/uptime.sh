#!/usr/bin/env bash
# SSH: basic health check. Usage: uptime.sh <server-credential-label>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
agent-pwd ssh exec --cred "$label" -- 'uptime'
