---
name: agent-pwd
description: Use the agent-pwd CLI to run actions with stored credentials (SSH, SQL, HTTP APIs, environment injection, web form filling) without ever reading plaintext secrets. Trigger when a task needs a password, API token, SSH key, or env secret that is stored in agent-pwd, or when the user asks to log in to a site, query a database, or call an authenticated API through the proxy.
---

# agent-pwd skill

[简体中文](SKILL.zh-CN.md)

`agent-pwd` is a credential proxy. It lets you **use** credentials but never **read** them. The only interface is the CLI; there is no HTTP API and no MCP server.

Before acting, read the machine-readable contract with `agent-pwd docs --format json` if you need exact schemas. The full operator manual is in [`docs/manual.md`](../docs/manual.md).

## Hard rules

1. Never attempt to print, dump, guess, or exfiltrate a secret. There is no command that returns a stored value.
2. `cred show` returns metadata only. Do not read `vault.dat`, backups, `audit.log`, daemon memory, or the IPC socket looking for keys.
3. Do not start the daemon yourself unless the user explicitly asks. A missing daemon means exit code `20`; tell the user to run `agent-pwd daemon` or `agent-pwd service install`.
4. If an action exits `10`, **stop**. Do not retry, do not auto-approve. Hand the `confirmation_id` and `poll_token` to a human.
5. Use credential **labels** in `--cred`; they are the primary key and are matched before IDs. Legacy `cred_...` IDs still work. Adding a credential with an existing label overwrites it.

## Standard workflow

```bash
agent-pwd status                  # is the daemon running and unlocked?
agent-pwd cred list               # metadata only — find the label
agent-pwd <action> --cred <label|id> ...
```

All action commands emit JSON by default. A successful result is printed directly; failures print `{"ok":false,"error":{"code":N,"message":"..."}}`.

| Exit | Meaning | What to do |
|---|---|---|
| 0 | success | parse the result |
| 10 | pending confirmation | stop, surface `confirmation_id` + `poll_token` to a human |
| 20 | locked / daemon absent | ask the human to start the daemon and `agent-pwd unlock` |
| 30 | policy denied | report the denial; do not try to bypass it |
| 40 | credential not found | check `cred list` for the correct label |
| 1 | other error | report `error.message` |

## Action commands

```bash
# SSH: run a remote command
agent-pwd ssh exec --cred prod-db -- 'uptime'

# SQL: reads are auto-allowed; writes require confirmation
agent-pwd sql query --cred prod-mysql -- 'SELECT id, email FROM users LIMIT 10'
agent-pwd sql query --cred prod-mysql -- 'SELECT * FROM users WHERE id = ?' --arg 42

# HTTP API: token is injected by the daemon, never by the caller
agent-pwd api call --cred github --method GET --path /user
agent-pwd api call --cred github --method POST --path /repos/x/y/issues --body '{"title":"bug"}'

# Environment variable injected into a child process
agent-pwd env run --cred deploy-token -- ./deploy.sh

# Redis: read-only command allowlist
agent-pwd sql query --cred cache -- 'GET session:42'
```

`--insecure-host-key` disables SSH host-key verification; use it only for tests and tell the user when you do.

## Web login with `fill`

Never ask for a password. Request a one-shot handle and pass it to a trusted automation script:

```bash
agent-pwd fill --cred github --field password
# -> {"handle":"...","field":"password","expires_at":"...","note":"..."}

agent-pwd fill apply --handle <handle> -- python3 login.py alice@example.com
```

`fill apply` injects the value into the child process through `AGENT_PWD_FILL` and stdin. The script owns the browser interaction; `agent-pwd` only supplies the secret and redacts all child output. Handles expire in 60 seconds and are single-use. For 2FA, request `--field totp` and apply it the same way.

## Confirmation workflow

When an action exits `10`, print the returned JSON to the human and stop. The human runs:

```bash
agent-pwd confirm approve <confirmation_id>   # Touch ID / Windows Hello, else master password
agent-pwd confirm result <confirmation_id> --token <poll_token>
```

You may call `confirm result` once the human says approval is done. Add `--wait` to the original action if the user wants the command to block until then. `confirm deny <id>` rejects. Unattended tasks stay blocked — this is intentional.

## Language and output

- `agent-pwd --lang en|zh-CN docs` selects documentation language; `AGENT_PWD_LANG` is the fallback.
- JSON keys, enums, error codes, exit codes, command names, and flags are never translated.
- Use `--human` only when a human explicitly asks for readable text.

## Related documents

- AI contract: [`docs/contract.md`](../docs/contract.md)
- Operator manual: [`docs/manual.md`](../docs/manual.md)
- i18n guide: [`docs/i18n.md`](../docs/i18n.md)
- Security policy: [`SECURITY.md`](../SECURITY.md)
