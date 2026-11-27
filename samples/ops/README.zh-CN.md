# ops 示例

本场景是运维命令，而不是凭据动作：健康检查、vault 备份恢复、主密码轮换。它们面向人类操作，脚本不传凭据 label。

See [English](README.md).

## 前置条件

- 一般基线是 daemon 正在运行且会话已解锁（`agent-pwd status`、`agent-pwd unlock`）。`status`、`doctor`、`audit verify`、`cred restore` 是公开操作，不要求会话解锁；`agent-pwd rotate-password` 则要求解锁。
- `restore_backup.sh` 需要数据目录中有名为 `vault.dat.bak.N` 的备份文件（macOS 上例如 `~/Library/Application Support/agent-pwd/`），并且恢复前要由人类明确核实。
- `rotate.sh` 需要交互式输入当前主密码和新主密码；daemon 会校验旧密码，错误尝试会触发退避。
- 所有 `agent-pwd` 命令默认输出 JSON。退出码：`0` 成功、`10` 待确认、`20` 未解锁/daemon 未运行、`30` 策略拒绝、`40` 凭据不存在；Shell 层的文件检查也可能退出 `1`。

## 脚本

| 文件 | 用法 | 预期行为 |
|---|---|---|
| `health.sh` | `health.sh` | 依次执行 `agent-pwd status`、`agent-pwd doctor`、`agent-pwd audit verify`；`set -e` 会在第一条失败命令处停止。 |
| `restore_backup.sh` | `restore_backup.sh <vault.dat.bak.N>` | 检查文件是否存在（否则打印 `backup not found: ...` 并退出 `1`），然后执行 `agent-pwd cred restore --from "$backup"` 和 `agent-pwd audit verify`。 |
| `rotate.sh` | `rotate.sh` | 执行 `agent-pwd lock`、`agent-pwd rotate-password`（交互式输入旧、新主密码）和 `agent-pwd audit verify`。按当前脚本，`lock` 会清除会话密钥，因此 `rotate-password` 退出 `20`，`set -e` 会跳过 `audit verify`。 |

## 示例

```bash
./samples/ops/health.sh
./samples/ops/restore_backup.sh '<vault.dat.bak.N>'
./samples/ops/rotate.sh
```

## 安全与注意事项

- 脚本中没有明文密钥，也不会打印密钥；`status`、`doctor`、`audit verify` 从不输出 secret 材料。切勿把主密码或 secret 粘贴进这些脚本。
- `lock` 会使 fill handle 和待确认动作失效：`fill` handle 是一次性的，默认 60 秒过期；确认只能由人类完成。退出 `10` 时应停止并请人类批准，不要自动批准。
- 这些脚本不传 `--cred`；在动作命令中，`--cred` 接受 label 或 ID，优先匹配 label（label 胜出）。
- 要真正轮换密码，请在 unlocked 会话中直接运行 `agent-pwd rotate-password`；`rotate.sh` 演示了该顺序，但会在自己的 `lock` 步骤后停在 `20`。
- `restore_backup.sh` 会覆盖 vault；请人工核实备份，并确保之后 `audit verify` 通过。
