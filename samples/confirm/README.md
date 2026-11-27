# confirm samples

This scenario shows how to handle a pending action: an action exits `10` with a `confirmation_id` and `poll_token`, a human approves or denies it, and the final result is collected. Approval cannot be automated.

See [简体中文](README.zh-CN.md).

## Prerequisites

- `agent-pwd status` must show the daemon running and unlocked (`agent-pwd unlock` if needed); `agent-pwd cred list` shows the label to use.
- For `run_and_wait.sh`, a credential whose policy requires confirmation (for example `agent-pwd cred add ... --risk high`, or a matching `confirm` policy rule). Without it, `env run ... -- true` completes with exit `0` and there is nothing pending.
- A human at the same daemon with Touch ID / Windows Hello available, or the master password for the fallback (`--no-biometric` forces that path).
- Pending actions live only in daemon memory, expire after 5 minutes by default, and are cancelled by `lock` or a daemon restart.
- All `agent-pwd` actions emit JSON by default. Exit codes: `0` success, `10` pending confirmation, `20` locked/daemon absent, `30` denied, `40` credential not found.

## Scripts

| File | Usage | Expected behavior |
|---|---|---|
| `run_and_wait.sh` | `run_and_wait.sh <label|id>` | Runs `agent-pwd env run --cred "$label" -- true`; through `handle_pending`, an exit `10` prints the pending JSON and the commands `agent-pwd confirm approve <id>` and `agent-pwd confirm result <id> --token <token>` to stderr, then returns `10`. Other codes are printed and passed through. The script itself does not wait. |
| `approve_and_collect.sh` | `approve_and_collect.sh <id> <token>` (arguments: `<confirmation_id> <poll_token>`) | Human-only: runs `agent-pwd confirm approve "$id"` then `agent-pwd confirm result "$id" --token "$token" --wait`, which polls until the decision is made (client deadline about 6 minutes). |
| `deny.sh` | `deny.sh <confirmation_id>` | Human-only: runs `agent-pwd confirm deny "$id"` to reject the pending action. |

## Examples

```bash
./samples/confirm/run_and_wait.sh '<label>'
# After exit 10, copy confirmation_id and poll_token from the JSON:
./samples/confirm/approve_and_collect.sh '<confirmation_id>' '<poll_token>'
# Or reject it:
./samples/confirm/deny.sh '<confirmation_id>'
```

## Safety and notes

- No script contains a plaintext secret and no script prints one. `fill` handles in other scenarios are one-shot and expire after 60 seconds.
- Confirmation is human-only: on exit `10`, stop and surface `confirmation_id` and `poll_token` to a human; do not retry or auto-approve. `confirm result` is one-shot for the final result, so collect it only once.
- `--cred` accepts a credential label or ID; labels are matched first, so the label wins.
- `approve_and_collect.sh` and `deny.sh` invoke human-only commands; they are not steps an agent should run on its own.
- The scripts source `../common.sh`, which also accepts `AGENT_PWD_CRED` as a label fallback when no label is passed.
