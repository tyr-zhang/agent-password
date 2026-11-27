#!/usr/bin/env bash
# MySQL: bound-parameter read. Usage: mysql_find_user.sh <label> <user_id>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
user_id="${2:?usage: mysql_find_user.sh <label> <user_id>}"
agent-pwd sql query --cred "$label" -- 'SELECT id, email, created_at FROM users WHERE id = ?' --arg "$user_id"
