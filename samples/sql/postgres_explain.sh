#!/usr/bin/env bash
# PostgreSQL: EXPLAIN is read-only and auto-allowed. Usage: postgres_explain.sh <label> <user_id>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
user_id="${2:?usage: postgres_explain.sh <label> <user_id>}"
agent-pwd sql query --cred "$label" -- 'EXPLAIN SELECT * FROM orders WHERE user_id = $1' --arg "$user_id"
