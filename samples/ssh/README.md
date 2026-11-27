# SSH samples

This scenario runs remote commands through a `server` credential with `protocol=ssh`. The daemon injects the stored password or key; the scripts only pass the label and the command text.

See [简体中文](README.zh-CN.md).

## Prerequisites

- `agent-pwd status` must show the daemon running and unlocked (`agent-pwd unlock` if needed); `agent-pwd cred list` shows the label to use.
- A `server` credential with `protocol=ssh` plus a password and/or key. Strict `known_hosts` verification is on by default: the data directory must contain `known_hosts` with the remote host key. `--insecure-host-key` disables verification and is for tests only; these scripts do not use it.
- The remote host must provide the invoked commands (`uptime`, `tail`, `docker`) with suitable permissions.
- All `agent-pwd` actions emit JSON by default. Exit codes: `0` success, `10` pending confirmation, `20` locked/daemon absent, `30` denied, `40` credential not found.

## Scripts

| File | Usage | Expected behavior |
|---|---|---|
| `uptime.sh` | `uptime.sh <label|id>` | Runs `agent-pwd ssh exec --cred "$label" -- 'uptime'` and prints the redacted JSON result (including `output`, `exit_code`, and `truncated`). |
| `tail_logs.sh` | `tail_logs.sh <label|id> [path] [lines]` | Runs `tail -n ${lines} ${path}` over SSH; `path` defaults to `/var/log/system.log` and `lines` to `50`. |
| `docker_ps.sh` | `docker_ps.sh <label|id>` | Runs `docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"` over SSH and prints the redacted result. |

## Examples

```bash
./samples/ssh/uptime.sh '<label>'
./samples/ssh/tail_logs.sh '<label>' '<remote-log-path>'
./samples/ssh/tail_logs.sh '<label>' '<remote-log-path>' '<line-count>'
./samples/ssh/docker_ps.sh '<label>'
```

## Safety and notes

- No script contains a plaintext secret and no script prints one; password, private key, and passphrase never appear in SSH output, and the daemon redacts secrets in results.
- `fill` handles (used in other scenarios) are one-shot and expire after 60 seconds; confirmation is human-only — if any action exits `10`, stop and ask a human instead of auto-approving.
- `--cred` accepts a credential label or ID; labels are matched first, so the label wins.
- `--port` on `agent-pwd cred add server` is only a compatibility alias: it overrides `secret.port`. Put the canonical `port` field inside `--secret` for automation; the default SSH port is `22`.
- The scripts source `../common.sh`, which also accepts `AGENT_PWD_CRED` as a label fallback when no label is passed.
