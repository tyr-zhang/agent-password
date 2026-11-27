#!/usr/bin/env bash
# Generic form demo. Usage: run_generic.sh <label> <username> <url> <user_sel> <pass_sel> [submit_sel]
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/../common.sh"

label=$(cred_label "${1:-}")
username="${2:?usage: run_generic.sh <label> <username> <url> <user_sel> <pass_sel> [submit_sel]}"
url="${3:?missing url}"
user_sel="${4:?missing user selector}"
pass_sel="${5:?missing password selector}"
submit_sel="${6:-button[type=submit]}"

response=$(agent-pwd fill --cred "$label" --field password)
handle=$(printf '%s\n' "$response" | json_field handle)
agent-pwd fill apply --handle "$handle" -- \
  python3 "$HERE/generic_form.py" "$url" "$username" "$user_sel" "$pass_sel" "$submit_sel"
