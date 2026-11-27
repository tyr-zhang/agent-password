# ssh 示例

本场景通过 `protocol=ssh` 的 `server` 凭据执行远程命令。daemon 注入存储的密码或私钥，脚本只传递 label 和命令文本。

See [English](README.md).

## 前置条件

- `agent-pwd status` 必须显示 daemon 正在运行且已解锁（必要时执行 `agent-pwd unlock`）；`agent-pwd cred list` 可查看要用的 label。
- 需要一个 `protocol=ssh` 的 `server` 凭据，以及密码和/或私钥。默认启用严格的 `known_hosts` 校验：数据目录中必须有包含远端主机密钥的 `known_hosts`。`--insecure-host-key` 会关闭校验，仅限测试；这些脚本没有使用该参数。
- 远端主机必须提供所调用的命令（`uptime`、`tail`、`docker`）并具有相应权限。
- 所有 `agent-pwd` 动作默认输出 JSON。退出码：`0` 成功、`10` 待确认、`20` 未解锁/daemon 未运行、`30` 策略拒绝、`40` 凭据不存在。

## 脚本

| 文件 | 用法 | 预期行为 |
|---|---|---|
| `uptime.sh` | `uptime.sh <server-credential-label>` | 执行 `agent-pwd ssh exec --cred "$label" -- 'uptime'`，打印脱敏后的 JSON 结果（含 `output`、`exit_code`、`truncated`）。 |
| `tail_logs.sh` | `tail_logs.sh <server-credential-label> [path] [lines]` | 通过 SSH 执行 `tail -n ${lines} ${path}`；`path` 默认 `/var/log/system.log`，`lines` 默认 `50`。 |
| `docker_ps.sh` | `docker_ps.sh <server-credential-label>` | 通过 SSH 执行 `docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"`，打印脱敏结果。 |

## 示例

```bash
./samples/ssh/uptime.sh '<label>'
./samples/ssh/tail_logs.sh '<label>' '<remote-log-path>'
./samples/ssh/tail_logs.sh '<label>' '<remote-log-path>' '<line-count>'
./samples/ssh/docker_ps.sh '<label>'
```

## 安全与注意事项

- 脚本中没有明文密钥，也不会打印密钥；密码、私钥和口令短语都不会出现在 SSH 输出中，daemon 会脱敏结果里的 secret。
- `fill` handle（其他场景使用）是一次性的，默认 60 秒过期；确认只能由人类完成——任何动作退出 `10` 时都应停止并请人类批准，不要自动批准。
- `--cred` 接受凭据 label 或 ID；优先匹配 label，因此 label 胜出。
- `agent-pwd cred add server` 的 `--port` 只是兼容性别名：它会覆盖 `secret.port`。自动化时请把规范的 `port` 字段写入 `--secret`；SSH 默认端口为 `22`。
- 脚本会 source `../common.sh`；未传 label 时，该脚本也接受 `AGENT_PWD_CRED` 作为 label 回退值。
