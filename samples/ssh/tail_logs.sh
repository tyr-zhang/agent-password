#!/usr/bin/env bash
# SSH: tail recent logs. Usage: tail_logs.sh <server-credential-label> [path] [lines]
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
path="${2:-/var/log/system.log}"
lines="${3:-50}"
agent-pwd ssh exec --cred "$label" -- "tail -n ${lines} ${path}"
