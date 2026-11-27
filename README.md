# agent-pwd

[![CI](https://github.com/tyr-zhang/agent-password/actions/workflows/ci.yml/badge.svg)](https://github.com/tyr-zhang/agent-password/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8.svg)](https://go.dev/)

English | [简体中文](README.zh-CN.md)

A personal credential proxy for AI agents. Secrets are stored encrypted in a single file; agents receive **actions**, never plaintext. The CLI is the only interface — no HTTP API, no MCP server, no GUI.

## Why

Traditional password managers optimize for humans reading secrets. This project optimizes for the opposite: an AI can *use* a credential to complete a task but cannot read the credential itself.

- Four credential kinds: `web`, `apikey`, `server`, `envvar`.
- Policy engine: `allow` / `confirm` / `deny`; unknown actions default to `confirm`.
- Audit log with SHA-256 hash chaining; every action is traceable and revocable.
- Single Go binary, `CGO_ENABLED=0`, macOS / Windows / Linux.

## Install

```bash
git clone https://github.com/tyr-zhang/agent-password.git
cd agent-password
make build                         # bin/agent-pwd
cp bin/agent-pwd ~/.local/bin/agent-pwd
```

## Quick start

```bash
agent-pwd init                     # interactive master password
agent-pwd service install          # optional: LaunchAgent / systemd / scheduled task
agent-pwd daemon &                 # or: agent-pwd daemon --foreground
agent-pwd unlock
agent-pwd status
```

Credentials are entered field by field; sensitive fields are hidden and never enter shell history:

```bash
agent-pwd cred add web github
agent-pwd cred add apikey openai
agent-pwd cred add server prod-mysql          # port is prompted like other fields
agent-pwd cred add envvar deploy-token
agent-pwd cred list
agent-pwd cred show <label|id>     # metadata only; no secret output path exists
```

A credential **label is the primary key**. Adding the same label overwrites that credential while preserving its ID and `created_at`. Everywhere a credential is referenced (`--cred`, `cred show`, `cred rm`), use the label or the legacy `cred_...` ID; if a label happens to equal another credential's ID, the **label wins**.

For automation, put every server field — including `port` — in `--secret`; `--port` is kept only as a compatibility alias:

```bash
agent-pwd cred add server prod-mysql \
  --secret '{"protocol":"mysql","host":"127.0.0.1","port":3306,"server_user":"app","password":"<password>","database":"appdb"}'
```

Actions:

```bash
agent-pwd ssh exec --cred <label|id> -- 'uptime'
agent-pwd sql query --cred <label|id> -- 'SELECT count(*) FROM users'
agent-pwd api call --cred <label|id> --method GET --path /v1/me
agent-pwd env run --cred <label|id> -- ./deploy.sh
agent-pwd fill --cred <label|id> --field password
agent-pwd fill apply --handle <handle> -- ./browser-automation
```

High-risk actions exit with code `10` and return a `confirmation_id`; the action stays pending until a human approves it:

```bash
agent-pwd confirm approve <id>     # Touch ID / Windows Hello, master-password fallback
agent-pwd confirm result <id> --token <poll_token>
```

Exit codes: `0` success, `10` pending confirmation, `20` locked or daemon not running, `30` policy denied, `40` credential not found, `1` other error.

## AI integration

```bash
agent-pwd docs > ~/.claude/agent-pwd-contract.md
```

Embed the generated contract in `CLAUDE.md` or an agent skill description. It documents capabilities, limits, JSON schemas, exit codes, and the confirmation workflow — including the explicit rule that the AI cannot read secrets.

A ready-to-use skill is provided at [`skills/agent-pwd/SKILL.md`](skills/agent-pwd/SKILL.md). The complete operator manual is [`docs/manual.md`](docs/manual.md). Runnable demos for every scenario live in [`samples/`](samples/README.md) — web login, SSH, SQL, API, env injection, fill, confirmation and ops.

## Languages

The CLI and generated documents support English and Simplified Chinese:

```bash
agent-pwd --lang zh-CN docs
AGENT_PWD_LANG=zh-CN agent-pwd help
```

Precedence: `--lang` > `AGENT_PWD_LANG` > `en`. Machine-readable values (JSON keys, enums, exit codes, error codes) are never translated. See [docs/i18n.md](docs/i18n.md) and the generated [English](docs/contract.md) / [Chinese](docs/contract.zh-CN.md) contracts.

## Data locations

| Platform | Directory |
|---|---|
| macOS | `~/Library/Application Support/agent-pwd/` |
| Windows | `%APPDATA%\agent-pwd\` |
| Linux | `$XDG_DATA_HOME/agent-pwd/` (default `~/.local/share/agent-pwd/`) |

`AGENT_PWD_HOME` overrides the directory. Vault, backups and audit log live there; files are `0600` and directories `0700` (Windows uses a per-user ACL).

Session defaults: 8 hours idle, 24 hours hard cap, 5-minute confirmation timeout, 60-second fill handle. Override at daemon startup with `AGENT_PWD_IDLE_TIMEOUT`, `AGENT_PWD_HARD_TIMEOUT`, `AGENT_PWD_CONFIRM_TTL`, `AGENT_PWD_FILL_TTL` (Go durations such as `12h`). `agent-pwd status` shows the effective values.

## Security model

Protects against context injection reading plaintext, silent high-risk actions, copied vault files, and retrospective audit tampering. **Out of scope:** malware running as the same OS user, which can read process memory, replace the binary, or access the local socket. The operating-system account is the real security boundary. Confirmation prevents *misleading* an agent, not *intrusion*.

See [SECURITY.md](SECURITY.md) for reporting instructions and the full [design document](docs/design/2026-11-26-personal-ai-credential-proxy-design.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) and [CHANGELOG.md](CHANGELOG.md).

```bash
make test       # unit + real-daemon integration + security regressions
make race
make vet
make cross      # darwin/windows/linux, CGO_ENABLED=0
make docs-check # verify both generated contracts
```
