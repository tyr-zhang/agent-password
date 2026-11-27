# sql 示例

本场景用 `protocol` 为 `mysql`、`postgres` 或 `redis` 的 `server` 凭据查询数据库。值通过可重复的 `--arg` 参数绑定，而不是拼接进 SQL 文本。

See [English](README.md).

## 前置条件

- `agent-pwd status` 必须显示 daemon 正在运行且已解锁（必要时执行 `agent-pwd unlock`）；`agent-pwd cred list` 可查看要用的 label。
- 需要一个与协议匹配的 `server` 凭据：`mysql`、`postgres`（PostgreSQL 使用 TLS，`sslmode=require`）或 `redis`，并按需要提供 host、port、server user、密码/私钥。默认端口分别是 `3306`、`5432`、`6379`。
- 需要网络连通性，以及示例查询所依赖的 `users`/`orders` 表结构；Redis 仅支持只读命令白名单（`GET`、`MGET`、`HGET`、`HGETALL`、`HMGET`、`LRANGE`、`LLEN`、`SMEMBERS`、`SISMEMBER`、`ZRANGE`、`ZSCORE`、`TTL`、`PTTL`、`EXISTS`、`TYPE`、`INFO`、`DBSIZE`）。
- `SELECT`、`SHOW`、`DESCRIBE`、`DESC`、`EXPLAIN` 自动放行；写操作需要人类确认并返回退出码 `10`。
- 所有 `agent-pwd` 动作默认输出 JSON。退出码：`0` 成功、`10` 待确认、`20` 未解锁/daemon 未运行、`30` 策略拒绝、`40` 凭据不存在。

## 脚本

| 文件 | 用法 | 预期行为 |
|---|---|---|
| `mysql_find_user.sh` | `mysql_find_user.sh <label> <user_id>` | 执行 `SELECT id, email, created_at FROM users WHERE id = ?`，使用 `--arg "$user_id"`；只读查询。 |
| `mysql_report.sh` | `mysql_report.sh <label|id>` | 执行只读的 `information_schema.tables` 查询，按 `table_rows DESC LIMIT 20` 排序，返回表名和行数估算。 |
| `mysql_update.sh` | `mysql_update.sh <label> <user_id> [active]` | 执行 `UPDATE users SET active = ? WHERE id = ?`，使用 `--arg "$active" --arg "$user_id"`（`active` 默认 `1`）；预期退出 `10`，并通过 `handle_pending` 打印待确认 JSON 及 `agent-pwd confirm approve` / `agent-pwd confirm result` 命令。 |
| `postgres_explain.sh` | `postgres_explain.sh <label> <user_id>` | 执行 `EXPLAIN SELECT * FROM orders WHERE user_id = $1`，使用 `--arg "$user_id"`；作为只读操作自动放行。 |
| `redis_get.sh` | `redis_get.sh <label> <key>` | 通过 `agent-pwd sql query` 执行 `GET ${key}`；属于 Redis 只读命令。 |

## 示例

```bash
./samples/sql/mysql_find_user.sh '<label>' '<user-id>'
./samples/sql/mysql_report.sh '<label>'
./samples/sql/mysql_update.sh '<label>' '<user-id>'
./samples/sql/mysql_update.sh '<label>' '<user-id>' '<active>'
./samples/sql/postgres_explain.sh '<label>' '<user-id>'
./samples/sql/redis_get.sh '<label>' '<key>'
```

## 安全与注意事项

- 脚本中没有明文密钥，也不会打印密钥；daemon 会脱敏 SQL 单元格和 JSON 输出。不要把 secret 值写进 `--arg`。
- `mysql_update.sh` 退出 `10` 时必须停止：确认只能由人类完成（Touch ID / Windows Hello 或主密码回退），不支持自动批准。其他场景中的 `fill` handle 是一次性的，默认 60 秒过期。
- `--cred` 接受凭据 label 或 ID；优先匹配 label，因此 label 胜出。
- `agent-pwd cred add server` 的 `--port` 只是兼容性别名：它会覆盖 `secret.port`。自动化时请把规范的 `port` 字段写入 `--secret`。
- 脚本会 source `../common.sh`；未传 label 时，该脚本也接受 `AGENT_PWD_CRED` 作为 label 回退值。
