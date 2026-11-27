#!/usr/bin/env bash
# Health check. Usage: health.sh
set -euo pipefail
agent-pwd status
agent-pwd doctor
agent-pwd audit verify
