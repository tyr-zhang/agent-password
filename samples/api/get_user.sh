#!/usr/bin/env bash
# API: authenticated GET. Usage: get_user.sh <apikey-label>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
agent-pwd api call --cred "$label" --method GET --path /user
