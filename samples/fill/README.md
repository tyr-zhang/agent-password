# fill samples

This scenario demonstrates one-shot fill handles for a `web` credential: `password` (or another supported field) and TOTP generated from `totp_seed`. `fill` returns only an opaque handle; `fill apply` injects the value into a trusted child through `stdin` and `AGENT_PWD_FILL`.

See [简体中文](README.zh-CN.md).

## Prerequisites

- `agent-pwd status` must show the daemon running and unlocked (`agent-pwd unlock` if needed); `agent-pwd cred list` shows the label to use.
- A `web` credential with a `password`; `run_totp.sh` additionally needs a `totp_seed` so the daemon can generate the current code.
- `python3` for `show_length.py`, which consumes both injection channels.
- Supported `--field` values by credential kind: `web` → `password` (default), `totp`, `username`; `apikey` → `token`; `server` → `password`; `envvar` → `value`. These wrappers are written for `web` credentials.
- All `agent-pwd` actions emit JSON by default. Exit codes: `0` success, `10` pending confirmation, `20` locked/daemon absent, `30` denied, `40` credential not found.

## Scripts

| File | Usage | Expected behavior |
|---|---|---|
| `run_password.sh` | `run_password.sh <label|id> [field]` | Runs `agent-pwd fill --cred "$label" --field "$field"` (`field` defaults to `password`), extracts `handle` with `json_field`, then runs `agent-pwd fill apply --handle "$handle" -- python3 "$HERE/show_length.py"`. |
| `run_totp.sh` | `run_totp.sh <label|id>` | Runs `agent-pwd fill --cred "$label" --field totp` and applies the handle to `show_length.py`; requires `totp_seed` on the credential. |
| `show_length.py` | `python3 show_length.py` (no arguments; launched through `agent-pwd fill apply --handle <h> -- ...`) | Reads `AGENT_PWD_FILL` and one stdin line, then prints `env length:`, `stdin length:`, and `same value:` — never the value itself. |

## Examples

```bash
./samples/fill/run_password.sh '<label>'
./samples/fill/run_password.sh '<label>' '<field>'
./samples/fill/run_totp.sh '<label>'
```

## Safety and notes

- No script contains a plaintext secret and no script prints one; `fill apply` redacts child stdout/stderr, and `show_length.py` reports lengths only.
- `fill` handles are one-shot and expire after 60 seconds by default (`AGENT_PWD_FILL_TTL`); they are consumed even if the child fails, and `fill apply` requires a command after `--`. Confirmation is human-only: if an action exits `10`, stop and ask a human.
- `--cred` accepts a credential label or ID; labels are matched first, so the label wins.
- The username in other samples is a plain argument; if a username is sensitive, store it as a credential and use `fill --field username` too.
- The wrappers source `../common.sh`, which also accepts `AGENT_PWD_CRED` as a label fallback when no label is passed.
