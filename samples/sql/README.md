# SQL samples

This scenario queries databases with a `server` credential whose `protocol` is `mysql`, `postgres`, or `redis`. Values are bound with repeatable `--arg` flags instead of being interpolated into SQL text.

See [简体中文](README.zh-CN.md).

## Prerequisites

- `agent-pwd status` must show the daemon running and unlocked (`agent-pwd unlock` if needed); `agent-pwd cred list` shows the label to use.
- A `server` credential matching the protocol: `mysql`, `postgres` (PostgreSQL uses TLS with `sslmode=require`), or `redis`, with host, port, server user, and password/key as required. Default ports are `3306`, `5432`, and `6379` respectively.
- Network access and an existing `users`/`orders` schema for the queries shown; Redis accepts only its read-only command allowlist (`GET`, `MGET`, `HGET`, `HGETALL`, `HMGET`, `LRANGE`, `LLEN`, `SMEMBERS`, `SISMEMBER`, `ZRANGE`, `ZSCORE`, `TTL`, `PTTL`, `EXISTS`, `TYPE`, `INFO`, `DBSIZE`).
- `SELECT`, `SHOW`, `DESCRIBE`, `DESC`, and `EXPLAIN` are auto-allowed; writes require human confirmation and return exit `10`.
- All `agent-pwd` actions emit JSON by default. Exit codes: `0` success, `10` pending confirmation, `20` locked/daemon absent, `30` denied, `40` credential not found.

## Scripts

| File | Usage | Expected behavior |
|---|---|---|
| `mysql_find_user.sh` | `mysql_find_user.sh <label> <user_id>` | Runs `SELECT id, email, created_at FROM users WHERE id = ?` with `--arg "$user_id"`; a read-only query. |
| `mysql_report.sh` | `mysql_report.sh <label|id>` | Runs a read-only `information_schema.tables` query ordered by `table_rows DESC LIMIT 20`, returning table names and row estimates. |
| `mysql_update.sh` | `mysql_update.sh <label> <user_id> [active]` | Runs `UPDATE users SET active = ? WHERE id = ?` with `--arg "$active" --arg "$user_id"` (`active` defaults to `1`); expects exit `10` and prints the pending JSON plus the `agent-pwd confirm approve` / `agent-pwd confirm result` commands through `handle_pending`. |
| `postgres_explain.sh` | `postgres_explain.sh <label> <user_id>` | Runs `EXPLAIN SELECT * FROM orders WHERE user_id = $1` with `--arg "$user_id"`; auto-allowed as a read. |
| `redis_get.sh` | `redis_get.sh <label> <key>` | Runs `GET ${key}` through `agent-pwd sql query`; a read-only Redis command. |

## Examples

```bash
./samples/sql/mysql_find_user.sh '<label>' '<user-id>'
./samples/sql/mysql_report.sh '<label>'
./samples/sql/mysql_update.sh '<label>' '<user-id>'
./samples/sql/mysql_update.sh '<label>' '<user-id>' '<active>'
./samples/sql/postgres_explain.sh '<label>' '<user-id>'
./samples/sql/redis_get.sh '<label>' '<key>'
```

## Safety and notes

- No script contains a plaintext secret and no script prints one; SQL cells and JSON output are redacted by the daemon. Do not paste secret values into `--arg`.
- If `mysql_update.sh` exits `10`, stop: confirmation is human-only (Touch ID / Windows Hello or master-password fallback) and automatic approval is not supported. `fill` handles in other scenarios are one-shot and expire after 60 seconds.
- `--cred` accepts a credential label or ID; labels are matched first, so the label wins.
- `--port` on `agent-pwd cred add server` is only a compatibility alias: it overrides `secret.port`. Put the canonical `port` field inside `--secret` for automation.
- The scripts source `../common.sh`, which also accepts `AGENT_PWD_CRED` as a label fallback when no label is passed.
