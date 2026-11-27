# env 示例

本场景把 `envvar` 凭据的值作为恰好一个环境变量注入子进程。子进程只验证变量存在，绝不打印变量值。

See [English](README.md).

## 前置条件

- `agent-pwd status` 必须显示 daemon 正在运行且已解锁（必要时执行 `agent-pwd unlock`）；`agent-pwd cred list` 可查看要用的 label。
- 需要一个含 `key` 和 `value` 的 `envvar` 凭据。包装脚本默认使用变量名 `DEPLOY_TOKEN`，可通过 `[env-name]` 参数覆盖。
- 需要 `bash` 和 `python3`；嵌套示例使用 `bash -c` 启动孙进程。
- 所有 `agent-pwd` 动作默认输出 JSON。退出码：`0` 成功、`10` 待确认、`20` 未解锁/daemon 未运行、`30` 策略拒绝、`40` 凭据不存在。子进程自身的退出码显示在 JSON 结果的 `exit_code` 中。

## 脚本

| 文件 | 用法 | 预期行为 |
|---|---|---|
| `deploy.sh` | `deploy.sh <label|id> [env-name]` | 执行 `agent-pwd env run --cred "$label" -- "$HERE/check_presence.sh" "$name"`；`name` 默认 `DEPLOY_TOKEN`，子进程只报告存在性和长度。 |
| `nested_process.sh` | `nested_process.sh <label|id> [env-name]` | 执行 `agent-pwd env run --cred "$label" -- bash -c '"$0" "$1"' "$HERE/check_presence.sh" "$name"`；孙进程继承注入的变量并报告存在性和长度。 |
| `check_presence.sh` | `check_presence.sh [name]`（无用法注释；来自 `name="${1:-DEPLOY_TOKEN}"`） | 供两个包装脚本使用的子进程：成功时打印 `<name> is present (length N)`，缺失时向 stderr 打印 `<name> is missing` 并退出 `1`；从不打印值。 |

## 示例

```bash
./samples/env/deploy.sh '<label>'
./samples/env/deploy.sh '<label>' '<env-name>'
./samples/env/nested_process.sh '<label>'
./samples/env/nested_process.sh '<label>' '<env-name>'
```

## 安全与注意事项

- 脚本中没有明文密钥，也不会打印密钥；子进程若回显值，daemon 的脱敏器会替换为 `[REDACTED]`。子进程本身被视为可信，能够接触该值。
- 其他场景中的 `fill` handle 是一次性的，默认 60 秒过期；确认只能由人类完成——动作退出 `10` 时应停止并请人类批准。
- `--cred` 接受凭据 label 或 ID；优先匹配 label，因此 label 胜出。
- `env run` 只设置一个环境变量；`check_presence.sh` 只报告变量名和长度。
- 脚本会 source `../common.sh`；未传 label 时，该脚本也接受 `AGENT_PWD_CRED` 作为 label 回退值。
