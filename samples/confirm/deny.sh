#!/usr/bin/env bash
# Deny a pending action. Usage: deny.sh <confirmation_id>
set -euo pipefail
id="${1:?usage: deny.sh <confirmation_id>}"
agent-pwd confirm deny "$id"
