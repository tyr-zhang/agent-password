# confirm 示例

本场景演示待确认动作的处理：动作以退出码 `10` 返回 `confirmation_id` 和 `poll_token`，由人类批准或拒绝，再取回最终结果。批准过程无法自动化。

See [English](README.md).

## 前置条件

- `agent-pwd status` 必须显示 daemon 正在运行且已解锁（必要时执行 `agent-pwd unlock`）；`agent-pwd cred list` 可查看要用的 label。
- `run_and_wait.sh` 需要一个策略要求确认的凭据（例如创建时使用 `agent-pwd cred add ... --risk high`，或命中 `confirm` 策略规则）。否则 `env run ... -- true` 会以退出 `0` 完成，没有待确认动作。
- 需要同一 daemon 上的人类用户，具备 Touch ID / Windows Hello，或可提供主密码作为回退（`--no-biometric` 强制走回退路径）。
- 待确认动作只存在于 daemon 内存中，默认 5 分钟过期，`lock` 或 daemon 重启会取消它们。
- 所有 `agent-pwd` 动作默认输出 JSON。退出码：`0` 成功、`10` 待确认、`20` 未解锁/daemon 未运行、`30` 策略拒绝、`40` 凭据不存在。

## 脚本

| 文件 | 用法 | 预期行为 |
|---|---|---|
| `run_and_wait.sh` | `run_and_wait.sh <high-risk-credential-label>` | 执行 `agent-pwd env run --cred "$label" -- true`；退出 `10` 时通过 `handle_pending` 向 stderr 打印待确认 JSON 和命令 `agent-pwd confirm approve <id>`、`agent-pwd confirm result <id> --token <token>`，然后返回 `10`。其他退出码会被原样打印和传递。脚本本身不会等待。 |
| `approve_and_collect.sh` | `approve_and_collect.sh <id> <token>`（参数为 `<confirmation_id> <poll_token>`） | 仅供人类使用：执行 `agent-pwd confirm approve "$id"`，然后执行 `agent-pwd confirm result "$id" --token "$token" --wait`，轮询直到做出决定（客户端期限约 6 分钟）。 |
| `deny.sh` | `deny.sh <confirmation_id>` | 仅供人类使用：执行 `agent-pwd confirm deny "$id"` 拒绝待确认动作。 |

## 示例

```bash
./samples/confirm/run_and_wait.sh '<label>'
# 退出 10 后，从 JSON 中复制 confirmation_id 和 poll_token：
./samples/confirm/approve_and_collect.sh '<confirmation_id>' '<poll_token>'
# 或拒绝该动作：
./samples/confirm/deny.sh '<confirmation_id>'
```

## 安全与注意事项

- 脚本中没有明文密钥，也不会打印密钥。其他场景中的 `fill` handle 是一次性的，默认 60 秒过期。
- 确认只能由人类完成：退出 `10` 时必须停止，把 `confirmation_id` 和 `poll_token` 交给人类；不要重试或自动批准。`confirm result` 的最终结果只能取回一次。
- `--cred` 接受凭据 label 或 ID；优先匹配 label，因此 label 胜出。
- `approve_and_collect.sh` 和 `deny.sh` 调用的是仅供人类的命令，agent 不应自行执行这些步骤。
- 脚本会 source `../common.sh`；未传 label 时，该脚本也接受 `AGENT_PWD_CRED` 作为 label 回退值。
