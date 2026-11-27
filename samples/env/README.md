# env samples

This scenario injects an `envvar` credential's value into a child process as exactly one environment variable. The children verify that the variable exists without ever printing its value.

See [简体中文](README.zh-CN.md).

## Prerequisites

- `agent-pwd status` must show the daemon running and unlocked (`agent-pwd unlock` if needed); `agent-pwd cred list` shows the label to use.
- An `envvar` credential with `key` and `value`. The default variable name used by the wrappers is `DEPLOY_TOKEN`, overridable with the `[env-name]` argument.
- `bash` and `python3` are available; the nested example uses `bash -c` to start a grandchild process.
- All `agent-pwd` actions emit JSON by default. Exit codes: `0` success, `10` pending confirmation, `20` locked/daemon absent, `30` denied, `40` credential not found. The child's own exit code appears as `exit_code` in the JSON result.

## Scripts

| File | Usage | Expected behavior |
|---|---|---|
| `deploy.sh` | `deploy.sh <label|id> [env-name]` | Runs `agent-pwd env run --cred "$label" -- "$HERE/check_presence.sh" "$name"`; `name` defaults to `DEPLOY_TOKEN`, and the child reports presence and length only. |
| `nested_process.sh` | `nested_process.sh <label|id> [env-name]` | Runs `agent-pwd env run --cred "$label" -- bash -c '"$0" "$1"' "$HERE/check_presence.sh" "$name"`; the grandchild inherits the injected variable and reports presence and length. |
| `check_presence.sh` | `check_presence.sh [name]` (no usage comment; from `name="${1:-DEPLOY_TOKEN}"`) | Child process used by the two wrappers: prints `<name> is present (length N)` on success, or `<name> is missing` to stderr with exit `1`; never prints the value. |

## Examples

```bash
./samples/env/deploy.sh '<label>'
./samples/env/deploy.sh '<label>' '<env-name>'
./samples/env/nested_process.sh '<label>'
./samples/env/nested_process.sh '<label>' '<env-name>'
```

## Safety and notes

- No script contains a plaintext secret and no script prints one; a child that echoes the value gets `[REDACTED]` from the daemon's redactor. The child process itself is trusted with the value.
- `fill` handles in other scenarios are one-shot and expire after 60 seconds; confirmation is human-only — if an action exits `10`, stop and ask a human.
- `--cred` accepts a credential label or ID; labels are matched first, so the label wins.
- `env run` sets exactly one environment variable; `check_presence.sh` reports only its name and length.
- The scripts source `../common.sh`, which also accepts `AGENT_PWD_CRED` as a label fallback when no label is passed.
