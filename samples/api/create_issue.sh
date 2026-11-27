#!/usr/bin/env bash
# API: POST with a JSON body (requires confirmation by default).
# Usage: create_issue.sh <apikey-label> <owner> <repo> <title>
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/../common.sh"
label=$(cred_label "${1:-}")
owner="${2:?missing owner}"
repo="${3:?missing repo}"
title="${4:?missing title}"
body=$(python3 -c 'import json,sys; print(json.dumps({"title":sys.argv[1]}))' "$title")
path="/repos/${owner}/${repo}/issues"

set +e
output=$(agent-pwd api call --cred "$label" --method POST --path "$path" --body "$body")
code=$?
set -e
handle_pending "$output" "$code"
