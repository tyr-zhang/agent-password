# agent-pwd Operation Manual

[简体中文](manual.zh-CN.md)

This is the complete operator manual for `agent-pwd`, a personal credential proxy for AI agents. Secrets are stored encrypted in one local file; agents request **actions**, never plaintext. The CLI is the only interface: there is no HTTP API, no MCP server, and no GUI. All commands, flags, JSON keys, enums, and exit codes below are verbatim machine tokens.

## 1. Overview and concepts

`agent-pwd` is built around a split between a short-lived CLI client and a resident daemon that is the only process that ever holds decrypted keys.

| Component | What it is | Where it lives |
|---|---|---|
| CLI client | Every `agent-pwd ...` invocation; parses flags, talks to the daemon, prints redacted JSON | The binary `bin/agent-pwd` (or its installed copy) |
| Daemon | Resident process holding session keys, evaluating policy, executing actions, writing audit records | Started explicitly with `agent-pwd daemon` or installed as a per-user service |
| Vault | `vault.dat`, one AES-256-GCM encrypted body with an Argon2id-wrapped DEK in its header | Platform data directory, mode `0600` |
| Session | In-memory DEK/KEK plus idle/hard timeouts; unlocked only after `agent-pwd unlock` | Daemon memory only; never written to disk |
| Policy | Ordered `allow` / `confirm` / `deny` rules stored inside the encrypted vault | Evaluated by the daemon on every action |
| Audit | Append-only `audit.log` with a SHA-256 hash chain | Same data directory, mode `0600` |
| Label | The **primary key** of a credential. Unique across the vault; `--cred` accepts a label or a legacy ID | Stored in the vault metadata |

Normal flow:

```bash
agent-pwd init                       # create the vault and master password once
agent-pwd service install            # optional: autostart the daemon
agent-pwd daemon --foreground        # or start it manually in a terminal
agent-pwd unlock                     # unlock the session in the running daemon
agent-pwd status                     # confirm daemon, session, and KDF state
agent-pwd cred add web github        # store a credential by label
agent-pwd ssh exec --cred prod-db -- 'uptime'
agent-pwd audit verify               # check the hash chain
```

Key concepts:

- **CLI only.** An agent cannot open a socket directly; it runs commands. Action commands print JSON by default (`--human` for compact text). The daemon IPC envelope is never exposed to callers.
- **Labels are primary.** `cred add <kind> <label>` with an existing label overwrites that credential: the ID and `created_at` are preserved, `updated_at` and all secret/policy-relevant fields are replaced, and older duplicate labels are removed. `--cred`, `cred show`, and `cred rm` accept either the label or the ID.
- **Actions, not values.** There is no `cred show --secret`, no HTTP API, and no command that returns a stored value. The daemon injects secrets into SSH connections, SQL clients, HTTP requests, child processes, and one-shot fill handles.
- **Policy first.** Every action is classified by the first matching policy rule; unknown combinations fall through to `confirm`. A credential marked `risk: high` downgrades an `allow` to `confirm`; an explicit `deny` always wins.
- **Everything is audited.** Started, approved, denied, timeout, completed, and denied-by-policy outcomes are appended to a hash-chained log that `audit verify` can check for edits, gaps, and reordering.

Data locations:

| Platform | Directory |
|---|---|
| macOS | `~/Library/Application Support/agent-pwd/` |
| Windows | `%APPDATA%\agent-pwd\` |
| Linux | `$XDG_DATA_HOME/agent-pwd/` (default `~/.local/share/agent-pwd/`) |

`AGENT_PWD_HOME` overrides the directory. Files are created `0600` and directories `0700` (Windows uses a per-user ACL). Inside the directory you will find `vault.dat`, `audit.log`, `known_hosts` (created when SSH host-key verification needs it), `vault.dat.bak.1` through `vault.dat.bak.5`, and `daemon.log` when the service is installed.

## 2. Install, upgrade, and service management

Requirements: Go 1.25+ and `make`. The Makefile exports `CGO_ENABLED=0`; `make cross` builds darwin/arm64, darwin/amd64, linux/amd64, and windows/amd64 binaries.

```bash
git clone https://github.com/tyr-zhang/agent-password.git
cd agent-password
make build                           # produces bin/agent-pwd
cp bin/agent-pwd ~/.local/bin/agent-pwd
agent-pwd version                    # {"version": ..., "commit": ..., "date": ...}
```

On Windows, copy `bin/agent-pwd.exe` to a directory on your `PATH`. On Linux, `~/.local/bin` must be on `PATH` if you want to call `agent-pwd` without a full path.

Daemon startup forms:

```bash
agent-pwd daemon --foreground        # stays attached; Ctrl-C stops it
agent-pwd daemon                     # detaches into the background
agent-pwd daemon &                   # equivalent background usage from a shell
```

Action commands **never** start a daemon implicitly. If none is running they exit with code `20` and the error tells you to start one. Use a service for unattended availability:

```bash
agent-pwd service install            # LaunchAgent / systemd user unit / scheduled task
agent-pwd service status             # report autostart status
agent-pwd service uninstall          # remove the per-user autostart entry
```

The service runs `agent-pwd daemon --foreground` using the absolute path of the binary that executed `service install`. It installs a macOS LaunchAgent `com.tyrzhang.agent-pwd`, a systemd user unit `agent-pwd`, or a Windows scheduled task `agent-pwd`. If you later move or replace the binary, reinstall the service so the stored path stays correct.

Upgrade procedure:

```bash
git -C agent-password pull
cd agent-password && make build
cp bin/agent-pwd ~/.local/bin/agent-pwd
agent-pwd version
agent-pwd doctor
```

The vault format is versioned; copying a new binary over the old one does not change `vault.dat`. Restart the daemon after upgrading so it runs the new code. A daemon restart locks the session (keys live only in memory), so run `agent-pwd unlock` again.

Useful build checks:

```bash
make test                            # unit, real-daemon integration, security regressions
make race
make vet
make cross
make docs-check                      # verify both generated contracts are in sync
```

## 3. First run and sessions

Initialize the vault once (interactive input is preferred so the master password never enters shell history):

```bash
agent-pwd init
# prompts: master password, then confirm master password
```

`init` accepts `--password` / `--new-password` for automation; both mean the master password. It creates the data directory, writes the encrypted `vault.dat`, and installs the default policy rules. The master password is never stored: losing it means losing the vault.

Start the daemon, unlock, then check state:

```bash
agent-pwd daemon --foreground        # in one terminal, or use service install
agent-pwd unlock                     # prompts for the master password
agent-pwd status
agent-pwd lock
```

- `unlock` accepts `--password` (interactive prompt is preferred) and returns the number of credentials loaded.
- `status` reports `unlocked`, `version`, `data_dir`, `vault`, `failures`, `session_id`, effective timeouts in `*_seconds`, `pending`, `service`, `audit_ok`, and `expires_at`. It never prints secrets.
- `lock` zeroizes session keys, resets the redactor, and cancels fill handles and pending confirmations. Unlock again with `agent-pwd unlock`.

Session defaults, all configurable at **daemon startup** with Go duration strings such as `12h` or `30m`:

| Setting | Default | Environment variable |
|---|---|---|
| Idle timeout | 8h | `AGENT_PWD_IDLE_TIMEOUT` |
| Hard cap since unlock | 24h | `AGENT_PWD_HARD_TIMEOUT` |
| Pending confirmation TTL | 5m | `AGENT_PWD_CONFIRM_TTL` |
| Fill handle TTL | 60s | `AGENT_PWD_FILL_TTL` |

```bash
AGENT_PWD_IDLE_TIMEOUT=2h AGENT_PWD_HARD_TIMEOUT=12h \
AGENT_PWD_CONFIRM_TTL=10m AGENT_PWD_FILL_TTL=30s \
  agent-pwd daemon --foreground
agent-pwd status                     # shows the effective values in seconds
```

If `AGENT_PWD_HARD_TIMEOUT` is shorter than `AGENT_PWD_IDLE_TIMEOUT`, the daemon raises it to the idle value. Invalid or non-positive durations are ignored. Changing these variables requires restarting the daemon; `AGENT_PWD_LANG` is separate and can be set per CLI invocation.

Wrong-password backoff:

- A failed `unlock` (or a wrong old password during `rotate-password`) counts as a failure and locks further attempts for 1s, then 2s, 4s, 8s, 16s, and 32s, capped at 60s.
- The CLI prints `retry after Ns` on stderr with `--human`; the JSON error contains `code` and `message`. `status` shows the running `failures` count.
- After 5 failed attempts the daemon stops itself. Restart it deliberately, then try again; do not automate password guessing.

## 4. Credential management

`agent-pwd cred add <web|apikey|server|envvar> <label>` is interactive by default: fields are read from the terminal/stdin and hidden fields are not echoed, keeping them out of shell history.

```bash
agent-pwd cred add web github
agent-pwd cred add apikey openai
agent-pwd cred add server prod-mysql
agent-pwd cred add envvar deploy-token
```

Interactive field order by kind:

| Kind | Prompted fields | Hidden fields |
|---|---|---|
| `web` | `username`, `password`, `totp_seed (optional)`, `url (optional)` | `password` |
| `apikey` | `base_url`, `header_name (default Authorization)`, `token` | `token` |
| `server` | `protocol (ssh/mysql/postgres/redis)`, `host`, `port (optional)`, `server_user`, `password (or leave empty for key)`, `private_key (optional)`, `key_path (optional)`, `passphrase (optional)`, `database (optional)` | `password`, `private_key`, `passphrase` |
| `envvar` | `key`, `value` | `value` |

For automation use `--secret` with a JSON object. All fields are optional at the JSON level but each kind has required fields enforced by the daemon:

| JSON field | Type | Kind | Required | Meaning |
|---|---|---|---|---|
| `username` | string | `web` | yes | Login name |
| `password` | string | `web`, `server` | `web` yes | Web password; for `server`, one of password/private key/key path is required |
| `totp_seed` | string | `web` | no | Base32 TOTP seed used by `fill --field totp` |
| `url` | string | `web` | no | Optional site URL |
| `base_url` | string | `apikey` | yes | Scheme + host prefix for `--path`, e.g. `https://api.example.com` |
| `header_name` | string | `apikey` | no | Auth header name; defaults to `Authorization` |
| `token` | string | `apikey` | yes | Value injected into the auth header by the daemon |
| `protocol` | string | `server` | yes | `ssh`, `mysql`, `postgres`/`postgresql`, or `redis` |
| `host` | string | `server` | yes | Hostname or IP |
| `port` | number | `server` | no | TCP port; defaults are 22 (ssh), 3306 (mysql), 5432 (postgres), 6379 (redis) |
| `server_user` | string | `server` | yes | SSH/database user |
| `private_key` | string | `server` | no | PEM private key contents |
| `key_path` | string | `server` | no | Path to a private key file, read by the daemon |
| `passphrase` | string | `server` | no | Passphrase for an encrypted private key |
| `database` | string | `server` | no | Database name for SQL protocols |
| `key` | string | `envvar` | yes | Environment variable name |
| `value` | string | `envvar` | yes | Environment variable value |

Other `cred add` flags: `--risk normal|high` (defaults to `normal`; `high` forces confirmation for every action), `--tags a,b,c` (comma-separated), and `--port N`. `--port` is only a compatibility alias: it overrides `secret.port`, whether the port came from `--secret` or the interactive prompt. Always put the canonical `port` field inside `--secret` for automation.

Concrete example for every kind:

```bash
# web: interactive (hidden password prompt)
agent-pwd cred add web github --tags work,2fa --risk high

# web: automation
agent-pwd cred add web github \
  --secret '{"username":"alice@example.com","password":"<password>","totp_seed":"JBSWY3DPEHPK3PXP","url":"https://github.com"}'

# apikey: interactive
agent-pwd cred add apikey openai

# apikey: automation (header_name defaults to Authorization)
agent-pwd cred add apikey openai \
  --secret '{"base_url":"https://api.openai.com","header_name":"Authorization","token":"<token>"}'

# server: interactive (port is prompted like other fields)
agent-pwd cred add server prod-mysql

# server: automation — include port in the JSON secret
agent-pwd cred add server prod-mysql \
  --secret '{"protocol":"mysql","host":"127.0.0.1","port":3306,"server_user":"app","password":"<password>","database":"appdb"}'

# server: SSH key authentication
agent-pwd cred add server bastion \
  --secret '{"protocol":"ssh","host":"bastion.example.com","port":22,"server_user":"ops","key_path":"/Users/me/.ssh/id_ed25519","passphrase":"<passphrase>"}'

# envvar: interactive
agent-pwd cred add envvar deploy-token

# envvar: automation
agent-pwd cred add envvar deploy-token \
  --secret '{"key":"DEPLOY_TOKEN","value":"<value>"}'
```

Overwrite-by-label semantics:

- The label is the primary key. `cred add web github` followed by `cred add web github` replaces the first entry in place.
- `id` and `created_at` are preserved; `kind`, `risk_level`, `tags`, `updated_at`, and the secret fields are replaced.
- If an older vault contains duplicate labels, adding that label keeps one entry and removes the duplicates, so policy references and audit IDs remain stable.
- Use `--cred github` (label) or the `cred_...` ID anywhere a credential is referenced. Labels are what humans and agents should use.
- Resolution is deterministic: all labels are checked first, then IDs. If one credential's label equals another credential's ID, the label match wins.

Inspect and remove:

```bash
agent-pwd cred list                  # metadata only: id, kind, label, risk_level, tags, timestamps
agent-pwd cred show github           # by label; only metadata, no secret output path exists
agent-pwd cred show cred_ab12cd34ef567890
agent-pwd cred rm github             # by label
agent-pwd cred rm cred_ab12cd34ef567890
```

Backups and restore:

- Every vault write rotates up to five encrypted backups: `vault.dat.bak.1` (newest), then `.bak.2` … `.bak.5`. The preceding `.bak.N` is renamed to `.bak.N+1` before the new snapshot is written.
- Backups are encrypted with the same vault keys; keeping the master password safe protects them too.
- Restore is an explicit human action. Verify the candidate first, then run the command; restoring overwrites `vault.dat` and locks the session, so unlock again afterward.

```bash
agent-pwd doctor                     # lists backup count and newest backup timestamp
agent-pwd cred restore --from /path/to/data-dir/vault.dat.bak.2
agent-pwd unlock
```

## 5. Policy engine

Policy rules are stored inside the encrypted vault and evaluated top to bottom; the first rule matching the credential pattern, action, and condition wins. If nothing matches, the decision is `confirm` (never silently `allow` and never a hard `deny` that would make a new credential unusable). Patterns are Go `path.Match` globs matched against `<kind>:<id>` — for example `server:cred_ab12...`, not the label.

Default rules installed by `init`, in priority order:

| # | Rule ID | `cred_pattern` | `action` | `condition` | Decision |
|---|---|---|---|---|---|
| 1 | `default-ssh` | `server:*` | `ssh` | — | `allow` |
| 2 | `default-sql-read` | `server:*` | `sql` | `sql_readonly` | `allow` |
| 3 | `default-api-read` | `apikey:*` | `api` | `http_readonly` | `allow` |
| 4 | `default-fill` | `*` | `fill` | — | `allow` |
| 5 | `default-env` | `*` | `env` | — | `allow` |
| 6 | `default-sql-write` | `*` | `sql` | — | `confirm` |
| 7 | `default-api-write` | `*` | `api` | — | `confirm` |
| 8 | `default-fallback` | `*` | `*` | — | `confirm` |

Conditions:

- `sql_readonly`: the query starts (after comment stripping and trimming) with `SELECT`, `SHOW`, `DESCRIBE`, `DESC`, or `EXPLAIN`, and contains no second statement. A single trailing `;` is tolerated; `WITH` is treated as a write and needs confirmation.
- `http_readonly`: the HTTP method is `GET` or `HEAD`. Every other method falls through to the write rule and needs confirmation.
- No condition means the rule applies to all matching actions.

New rules are inserted at the **front**, so the newest rule has the highest priority:

```bash
agent-pwd policy add --cred-pattern 'server:*' --action ssh --decision deny
agent-pwd policy add --cred-pattern 'apikey:*' --action api --condition http_readonly --decision allow
agent-pwd policy add --cred-pattern 'server:cred_ab12cd34ef567890' --action sql --condition sql_readonly --decision allow --id allow-one-sql
agent-pwd policy add --cred-pattern 'apikey:*' --action '*' --decision confirm
agent-pwd policy list
agent-pwd policy rm allow-one-sql
```

Notes:

- `--decision` must be `allow`, `confirm`, or `deny`; `--condition` must be empty, `sql_readonly`, or `http_readonly`.
- `--id` is optional; the daemon generates `rule_...` if omitted and rejects a duplicate ID. `policy rm <id>` uses that rule ID, not a credential ID.
- Priority is list order: `policy list` shows rules in the exact order evaluated. A broad `deny` placed above a narrow `allow` stops evaluation immediately.
- A credential with `--risk high` downgrades a matched `allow` to `confirm`. A matched `deny` wins outright; it is never downgraded or overridden by a later rule.
- The `action` value is one of `ssh`, `sql`, `api`, `fill`, `env`, or `*`. Denied actions are still recorded in the audit log with outcome `denied`.

## 6. Actions

All action commands take `--cred <label|id>`, print JSON by default, and return redacted results. Use `--` to separate CLI options from the remote command or SQL text. Actions have a 30-second execution timeout; output is capped at 1 MiB (`truncated: true` when cut), and SQL results are capped at 1000 rows.

### SSH execution

`agent-pwd ssh exec --cred <label|id> -- <command...>` requires a `server` credential with `protocol=ssh`, a password and/or key, and by default a `known_hosts` file in the data directory. The default port is 22. Password, private key, and passphrase never appear in the result.

```bash
agent-pwd ssh exec --cred bastion -- 'uptime'
agent-pwd ssh exec --cred prod-db -- 'systemctl --user status app'
agent-pwd ssh exec --cred bastion --insecure-host-key -- 'uname -a'   # tests only
```

Host keys: strict `known_hosts` verification is the default. If the file does not exist, the action fails with a message giving its path; add the host key to `<data-dir>/known_hosts` first. `--insecure-host-key` disables verification entirely and permits man-in-the-middle attacks — use it only for tests and disclose when you do. The result contains `output`, `exit_code`, and `truncated`.

### SQL: MySQL, PostgreSQL, Redis

`agent-pwd sql query --cred <label|id> -- "<SQL>" [--arg v]...` works with `server` credentials using `mysql`, `postgres`, or `redis`. `--arg` binds values as query parameters (repeat the flag for multiple values) instead of interpolating them into SQL text.

```bash
# MySQL: default read allowed by default-sql-read
agent-pwd sql query --cred prod-mysql -- 'SELECT count(*) FROM users'

# PostgreSQL: bound parameters, no string interpolation
agent-pwd sql query --cred analytics -- 'SELECT id, email FROM users WHERE id = $1' --arg 42

# MySQL placeholder style with two bound arguments
agent-pwd sql query --cred prod-mysql -- 'SELECT id FROM users WHERE team = ? AND active = ?' --arg ops --arg 1

# Writes match default-sql-write and return exit code 10 for confirmation
agent-pwd sql query --cred prod-mysql -- 'UPDATE users SET active = 0 WHERE id = 42'

# Redis uses the same command with a read-only allowlist
agent-pwd sql query --cred cache -- 'GET session:42'
agent-pwd sql query --cred cache -- 'HGETALL user:42'
```

Rules for the read path:

- `SELECT`, `SHOW`, `DESCRIBE`, `DESC`, and `EXPLAIN` are auto-allowed for `server:*`; other leading verbs, including `WITH`, require confirmation.
- Multi-statement input is rejected as read-only: a single trailing semicolon is fine, a second `;` is not. Comments are stripped before classification, so comment tricks cannot fake a read. MySQL connections also set `multiStatements=false`.
- Redis supports only the read-only commands `GET`, `MGET`, `HGET`, `HGETALL`, `HMGET`, `LRANGE`, `LLEN`, `SMEMBERS`, `SISMEMBER`, `ZRANGE`, `ZSCORE`, `TTL`, `PTTL`, `EXISTS`, `TYPE`, `INFO`, and `DBSIZE`. A stored password is sent as `AUTH` automatically; the default port is 6379. Writes are rejected by the executor even if policy rules are loosened.
- PostgreSQL uses TLS with `sslmode=require`; its default port is 5432.

### HTTP API calls

`agent-pwd api call --cred <label|id> --method GET --path /v1/x` requires an `apikey` credential. The daemon injects `token` into the configured `header_name` (default `Authorization`); the caller never handles the token. The default method is `GET`.

```bash
agent-pwd api call --cred github --method GET --path /user
agent-pwd api call --cred github --method HEAD --path /v1/status
agent-pwd api call --cred openai --method POST --path /v1/chat/completions \
  --header 'Content-Type: application/json' \
  --body '{"model":"example","messages":[]}'
agent-pwd api call --cred github --method GET --path /orgs/x/repos --allow-redirect
```

- `--path` must be absolute, starting with `/`, and must not contain `..` or `//`. It is appended to `base_url`; `--method` accepts `GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE`, and `OPTIONS`.
- `GET` and `HEAD` are auto-allowed via `http_readonly`; `POST` and other methods match `default-api-write` and return exit code `10` until a human approves.
- `--header k:v` is repeatable. You cannot override the configured auth header, `Authorization`, `Cookie`, `Host`, or `Proxy-Authorization`.
- Redirects are off by default (the original response is returned). `--allow-redirect` permits redirects only within the same origin host; cross-origin redirects are blocked.
- The result contains `status`, a small allowlist of response headers (`Content-Type`, `Content-Length`, `Date`, `Server`, `Cache-Control`, `ETag`), `body`, and `truncated`. Bodies are redacted before output.

### Environment variable injection

`agent-pwd env run --cred <label|id> -- <command...>` sets exactly one environment variable (`key=value`) in a child process and returns redacted combined output with `exit_code`.

```bash
agent-pwd env run --cred deploy-token -- ./deploy.sh
agent-pwd env run --cred deploy-token -- python3 -c 'import os; print("token set:", bool(os.getenv("DEPLOY_TOKEN")))'
```

There is no command to print the stored value. If a child echoes it, the daemon's redactor replaces it with `[REDACTED]`. The child process itself is trusted with the value: anything it deliberately exfiltrates is outside the proxy's guarantees.

### Web password and TOTP filling with `fill`

`fill` never returns a secret. It issues an opaque, one-shot, expiring handle; `fill apply` consumes it and injects the value into a trusted child process through both `stdin` and `AGENT_PWD_FILL`.

```bash
agent-pwd fill --cred github --field password
# -> {"handle":"<handle>","field":"password","expires_at":"...","note":"..."}

agent-pwd fill apply --handle <handle> -- ./browser-login.sh alice@example.com

# 2FA: generate the current 6-digit TOTP from the web credential's totp_seed
agent-pwd fill --cred github --field totp
agent-pwd fill apply --handle <handle> -- ./enter-otp.sh
```

Field mapping:

| Credential kind | Supported `--field` values |
|---|---|
| `web` | `password` (default), `totp`, `username` |
| `apikey` | `token` |
| `server` | `password` |
| `envvar` | `value` |

Handles expire after 60 seconds by default (`AGENT_PWD_FILL_TTL` at daemon startup), are bound to the session and client that requested them, and are consumed even if the child later fails. `fill apply` requires a command after `--`. Browser navigation and DOM interaction belong to the child automation script, not to `agent-pwd`.

### Why no command prints plaintext

- `cred list` and `cred show` return `PublicCredential` objects with no `Secret` field at all; JSON output uses only declared fields.
- The daemon registers every stored secret with a redactor that replaces plain, URL-escaped, hex, base64, and quoted forms with `[REDACTED]` in SSH output, SQL cells, HTTP headers/bodies, and child-process output. Patterns such as `Bearer ...` and `password=...` are redacted too.
- Fill values and one-time tokens stay in daemon memory; only opaque handles and pointers to results cross the CLI boundary.
- Size caps and row limits reduce accidental leakage through very large responses. They are defense in depth, not a substitute for reviewing what child processes do.

## 7. Confirmation workflow

High-risk actions do not fail silently. The CLI prints the pending JSON object and exits with code `10`:

```json
{
  "status": "pending",
  "confirmation_id": "c_0123456789abcdef",
  "poll_token": "<poll_token>",
  "expires_at": "2026-11-27T12:34:56Z",
  "reason": "rule:default-api-write",
  "summary": "apikey/github api POST /v1/...",
  "next": "tell the human the confirmation_id and poll token; then run agent-pwd confirm result <id> --token <poll_token>"
}
```

An agent must stop here: do not retry, suppress, or auto-approve. Surface `confirmation_id` and `poll_token` to a human.

```bash
# Human, in another terminal: Touch ID / Windows Hello, then master-password fallback
agent-pwd confirm approve c_0123456789abcdef

# Force the master-password path (useful on Linux, which has no system biometric API)
agent-pwd confirm approve c_0123456789abcdef --no-biometric
# --password is accepted; interactive prompt is preferred

# Reject the action
agent-pwd confirm deny c_0123456789abcdef

# Collect the final result exactly once
agent-pwd confirm result c_0123456789abcdef --token <poll_token>

# Poll until a decision is made
agent-pwd confirm result c_0123456789abcdef --token <poll_token> --wait
```

Details:

- `confirm approve` uses platform-native biometrics when available (Touch ID on macOS, Windows Hello on Windows, none on Linux), then falls back to a master-password proof. The password is derived into an HMAC proof by the client; it is never sent to the daemon. `--no-biometric` skips the native attempt.
- `confirm result` is one-shot for the **final** outcome: the poll token is cleared after the first terminal result. Repeated polling while the state is `pending` is safe; the final result is not.
- `--wait` polls every 500 ms until a decision is made (client deadline about 6 minutes). The AI contract also documents `--wait` on the original action to make the action block until a human decides; otherwise use `confirm result` as shown above.
- Pending actions live only in daemon memory and expire after 5 minutes by default (`AGENT_PWD_CONFIRM_TTL`). On expiry the audit records outcome `timeout` with `TIMEOUT`. `lock` cancels pending actions; a daemon restart cancels all of them, and later `confirm approve` calls return `NOT_FOUND`.
- Audit records mark `started`, then `approved` / `denied` / `timeout`, and `completed` after execution. An approved action may still fail during execution; that error is returned by `confirm result` with its own exit code.
- Unattended jobs therefore stop at high-risk actions by design. Automatic approval is not supported.

## 8. Audit and operations

### Audit log

```bash
agent-pwd audit tail --n 50         # latest records, default 50, no secret material
agent-pwd audit verify              # full hash-chain check
```

Each `audit.log` line is JSON with `seq`, `ts`, `credential_id`, `action`, `decision`, `confirm_source`, `outcome`, `result_code`, `detail_hash`, `policy_rule`, `client`, `prev_hash`, and `hash`. The hash is SHA-256 over the previous hash plus the canonical record. `audit verify` detects malformed lines, sequence gaps, `prev_hash` mismatches, hash mismatches, and non-monotonic timestamps; it reports `ok`, `records`, `first_seq`, `last_seq`, and a `problem` string when broken. The daemon verifies the chain at startup and refuses `unlock` with `CORRUPT` if it is broken. Writes are append-only, locked, and fsynced; a failed audit write prevents the action or locks the session if it fails after execution.

### Health check and password rotation

```bash
agent-pwd doctor                    # permissions, backups, audit chain, daemon, platform hardening
agent-pwd rotate-password           # prompts for current and new master password
agent-pwd rotate-password --new-salt
```

`doctor` reports `data_dir`, `dir_permissions`, `vault_exists`, `audit_ok`, `audit_records`, `audit_problem`, `backups`, `newest_backup`, `daemon`, `service`, `platform`, `security_note`, and `unlocked`. Rotation requires an unlocked session and verifies the old password (wrong attempts share the backoff described in section 3). Without `--new-salt`, only the DEK is rewrapped under a new KEK; with `--new-salt` a fresh DEK is derived and all credentials are re-encrypted.

### Service, backups, and i18n

```bash
agent-pwd service install
agent-pwd service status
agent-pwd service uninstall

agent-pwd doctor                    # confirms backup rotation status
agent-pwd cred restore --from /path/to/data-dir/vault.dat.bak.1

agent-pwd --lang zh-CN status
agent-pwd status --lang zh-CN
AGENT_PWD_LANG=zh-CN agent-pwd docs
agent-pwd docs --format json
```

Language precedence is `--lang` > `AGENT_PWD_LANG` > `en`. `--lang zh-CN` and `--lang=zh-CN` are equivalent; aliases `zh`, `zh-cn`, `zh_cn`, `zh-hans`, and `zh_hans` map to `zh-CN`, and unsupported values fall back to `en`. The OS locale is never consulted. Only human-facing prose is translated: commands, flags, JSON keys, enums, decisions, exit/error codes, IDs, tokens, and file paths stay verbatim. `make docs` regenerates the two contract files; `make docs-check` verifies they are in sync. `--json` is the default output mode and `--human` switches to compact text.

## 9. Troubleshooting

| Symptom | Likely cause | What to do |
|---|---|---|
| Exit `20`, error `NO_DAEMON`, message "daemon is not running" | No daemon listening on the Unix socket / named pipe | Run `agent-pwd service install` or `agent-pwd daemon`; check `agent-pwd service status`. The CLI never starts it for you. |
| Exit `20`, "vault is locked; run agent-pwd unlock" | Session idle (8h) or hard (24h) timeout elapsed, `lock` was called, or a daemon restart cleared memory | Run `agent-pwd unlock`; review `AGENT_PWD_IDLE_TIMEOUT` / `AGENT_PWD_HARD_TIMEOUT` at startup. |
| "too many failed attempts; retry in Ns" | Wrong master password triggered exponential backoff | Wait the indicated seconds; `status` shows `failures`. Five failures stop the daemon — restart it deliberately. |
| "wrong master password" during `rotate-password` | Supplied current password is wrong | Use the current password; the failure also counts toward backoff. |
| Exit `30` / `DENIED`, "policy denied this action (rule ...)" | A `deny` rule matched, or the action was blocked by policy | Report the denial; inspect `agent-pwd policy list`. Do not try to bypass it. |
| Exit `40` / `NOT_FOUND`, "credential not found" | Wrong `--cred` value, or the credential was removed/overwritten | Check `agent-pwd cred list`; use the current label or the `cred_...` ID. |
| `CORRUPT` on unlock, `audit_ok: false` | Broken audit hash chain | Run `agent-pwd audit verify` and inspect `doctor`; restore a reviewed backup if needed. Do not edit `audit.log`. |
| "unknown, used or expired fill handle" | Handle already consumed, expired (60s default), or session/client changed | Issue a new `fill` handle and apply it with the same client within the TTL; check `AGENT_PWD_FILL_TTL`. |
| `confirm result` says "confirmation not found" | Pending action expired (5m), was locked/cancelled, or the daemon restarted | Re-run the action after a human decision path is available; pending state is memory-only. |
| "invalid poll token" | Wrong token, or final result already collected once | Use the `poll_token` from the original exit-10 response; the final result is one-shot. |
| Confirmation stays `pending` | No human has approved; unattended approval is intentionally unsupported | Ask a human to run `confirm approve <id>`; use `--wait` to block. |
| Two entries for one label, or unexpected overwrite | Legacy vault with duplicate labels; `cred add` now treats label as primary key | Re-run `cred add <kind> <label>` with the desired data to deduplicate, then verify with `cred list`. |
| SSH fails with "no known_hosts at ..." | Strict host-key verification found no trust store | Add the host key to `<data-dir>/known_hosts`; avoid `--insecure-host-key` outside tests. |
| SSH/SQL/API returns exit `1` with an executor error | Wrong protocol for the action, unsupported method, invalid path, or a remote error | Match credential kind to action (`server`+`ssh`, `server`+`mysql|postgres|redis`, `apikey`+`api`, `envvar`+`env`); inspect `error.message`. |
| Linux: confirmation has no biometric prompt | No system-level biometric API | Use `confirm approve --no-biometric` with the master password. |
| macOS: service keeps restarting or loses session | Service launched the detaching daemon form | Reinstall the service; it must run `daemon --foreground` (LaunchAgent handles this automatically). |
| Windows: cannot reach daemon | Named pipe ACL allows only the current user and SYSTEM | Run the CLI as the same user that started the daemon; check `service status`. |

Platform notes: macOS uses a LaunchAgent and Touch ID helper; Linux uses a systemd user unit (or manual start) and master-password confirmation, with `PR_SET_DUMPABLE=0` blocking core dumps but not same-user `ptrace`; Windows uses a scheduled task, named-pipe ACL, and Windows Hello when available. Data directories and paths are listed in section 1; `AGENT_PWD_HOME` overrides them.

## 10. Security boundaries

Protected by design:

- Plaintext secrets never enter CLI output, `cred show`, `vault.dat`, `audit.log`, or the AI context; all outbound text is redacted.
- High-risk actions cannot complete silently; denial, approval, and timeout are audited.
- A copied vault cannot be read without the master password; Argon2id protects it offline.
- Audit rewriting, deletion, or reordering is detectable through the hash chain.
- Session keys are zeroized on lock, idle timeout, hard timeout, daemon stop, and failed audit writes.

Explicitly out of scope:

- Malware running as the **same OS user**, which can read process memory, attach with `ptrace`, replace the binary, or use the local socket/pipe and data directory.
- Direct access to the daemon socket/named pipe or data directory by another process of the same user; those processes are inside the trust boundary.
- Account compromise, physical access, coercion, or a malicious child process that a human deliberately authorized to receive injected secrets.
- Misuse of a `fill` handle by a child process already allowed to run. `fill` is a capability: the guarantee is that `agent-pwd` itself never prints the value and that the action is auditable and revocable (`lock`, `cred rm`).

Second confirmation is anti-injection, not anti-intrusion: it prevents a misled agent from finishing a high-risk action quietly; it does not stop a hostile same-user process from forging confirmation. The operating-system account is the real security floor. Report suspected vulnerabilities privately as described in [SECURITY.md](../SECURITY.md).

## 11. Quick command reference and links

| Task | Command |
|---|---|
| Build / upgrade | `make build`; copy `bin/agent-pwd` to your `PATH` |
| Check build | `agent-pwd version` |
| Install / remove autostart | `agent-pwd service install` / `agent-pwd service uninstall` / `agent-pwd service status` |
| Start daemon | `agent-pwd daemon --foreground` or `agent-pwd daemon` |
| Initialize / unlock / lock / inspect | `agent-pwd init` / `agent-pwd unlock` / `agent-pwd lock` / `agent-pwd status` |
| Add credentials | `agent-pwd cred add <web|apikey|server|envvar> <label> [--secret JSON] [--risk normal|high] [--tags a,b] [--port N]` |
| List / show / remove / restore | `agent-pwd cred list` / `agent-pwd cred show <label|id>` / `agent-pwd cred rm <label|id>` / `agent-pwd cred restore --from <backup>` |
| Policy | `agent-pwd policy add --cred-pattern '<glob>' --action '<glob>' --decision allow|confirm|deny [--condition ...] [--id ...]`; `agent-pwd policy list`; `agent-pwd policy rm <id>` |
| SSH | `agent-pwd ssh exec --cred <label|id> -- '<command>' [--insecure-host-key]` |
| SQL / Redis | `agent-pwd sql query --cred <label|id> -- '<SQL or command>' [--arg v]...` |
| HTTP API | `agent-pwd api call --cred <label|id> --method GET --path /v1/x [--header k:v] [--body ...] [--allow-redirect]` |
| Environment injection | `agent-pwd env run --cred <label|id> -- <command...>` |
| Fill / apply | `agent-pwd fill --cred <label|id> --field password|totp|username|token`; `agent-pwd fill apply --handle <h> -- <command...>` |
| Confirm | `agent-pwd confirm approve <id> [--no-biometric]` / `agent-pwd confirm deny <id>` / `agent-pwd confirm result <id> --token <poll_token> [--wait]` |
| Audit / health | `agent-pwd audit tail [--n 50]` / `agent-pwd audit verify` / `agent-pwd doctor` |
| Change master password | `agent-pwd rotate-password [--new-salt]` |
| Machine contract / help / language | `agent-pwd docs [--format md|json]`; `agent-pwd help`; `agent-pwd --lang en|zh-CN <command>` |

Related documents:

- [AI integration contract](contract.md) — generated by `agent-pwd docs`; the exact command registry and JSON contract.
- [Localization guide](i18n.md) — supported languages and untranslatable machine tokens.
- [agent-pwd skill](../skills/agent-pwd/SKILL.md) — agent-facing hard rules and usage patterns.
- [Runnable samples](../samples/README.md) — multiple examples for web login, SSH, SQL, API, env, fill, confirmation and ops.
- [README](../README.md) — install quick start and project overview.
- [Security policy](../SECURITY.md) — trust boundary and private vulnerability reporting.
