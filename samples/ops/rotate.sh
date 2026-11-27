#!/usr/bin/env bash
# Rotate the master password interactively. Usage: rotate.sh
set -euo pipefail
agent-pwd lock
agent-pwd rotate-password
agent-pwd audit verify
