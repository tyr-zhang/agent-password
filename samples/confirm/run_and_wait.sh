#!/usr/bin/env bash
# Trigger a high-risk action and print the human-approval instructions.
# Usage: run_and_wait.sh <high-risk-credential-label>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")

set +e
output=$(agent-pwd env run --cred "$label" -- true)
code=$?
set -e
handle_pending "$output" "$code"
