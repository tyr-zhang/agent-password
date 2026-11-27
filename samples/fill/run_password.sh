#!/usr/bin/env bash
# Fill: issue a one-shot password handle and apply it to a child.
# Usage: run_password.sh <label|id> [field]
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/../common.sh"
label=$(cred_label "${1:-}")
field="${2:-password}"

response=$(agent-pwd fill --cred "$label" --field "$field")
handle=$(printf '%s\n' "$response" | json_field handle)
agent-pwd fill apply --handle "$handle" -- python3 "$HERE/show_length.py"
