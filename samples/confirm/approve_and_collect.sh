#!/usr/bin/env bash
# Approve and collect a pending result. Usage: approve_and_collect.sh <id> <token>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
id="${1:?usage: approve_and_collect.sh <confirmation_id> <poll_token>}"
token="${2:?missing poll token}"

agent-pwd confirm approve "$id"
agent-pwd confirm result "$id" --token "$token" --wait
