#!/usr/bin/env bash
# MySQL: write example. Expects exit 10 and a human confirmation.
# Usage: mysql_update.sh <label> <user_id> [active]
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
user_id="${2:?usage: mysql_update.sh <label> <user_id> [active]}"
active="${3:-1}"

set +e
output=$(agent-pwd sql query --cred "$label" -- 'UPDATE users SET active = ? WHERE id = ?' --arg "$active" --arg "$user_id")
code=$?
set -e
handle_pending "$output" "$code"
