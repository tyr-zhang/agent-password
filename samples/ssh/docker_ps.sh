#!/usr/bin/env bash
# SSH: inspect Docker containers. Usage: docker_ps.sh <label|id>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
agent-pwd ssh exec --cred "$label" -- 'docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"'
