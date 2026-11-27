#!/usr/bin/env bash
# MySQL: read-only table sizes. Usage: mysql_report.sh <server-credential-label>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
agent-pwd sql query --cred "$label" -- \
  'SELECT table_name, table_rows FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_rows DESC LIMIT 20'
