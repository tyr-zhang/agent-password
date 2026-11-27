#!/usr/bin/env bash
# Restore a backup after explicit human verification.
# Usage: restore_backup.sh <vault.dat.bak.N>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
backup="${1:?usage: restore_backup.sh <vault.dat.bak.N>}"
if [[ ! -f "$backup" ]]; then
  echo "backup not found: $backup" >&2
  exit 1
fi
agent-pwd cred restore --from "$backup"
agent-pwd audit verify
