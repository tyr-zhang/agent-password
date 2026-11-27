#!/usr/bin/env bash
# Jenkins login demo. Usage: run_jenkins.sh <credential-label> <username> [url]
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/../common.sh"

label=$(cred_label "${1:-}")
username="${2:?usage: run_jenkins.sh <credential-label> <username> [url]}"
url="${3:-https://jenkins.example.com/}"

response=$(agent-pwd fill --cred "$label" --field password)
handle=$(printf '%s\n' "$response" | json_field handle)
agent-pwd fill apply --handle "$handle" -- python3 "$HERE/jenkins.py" "$url" "$username"
