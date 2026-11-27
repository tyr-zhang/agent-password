# fill 示例

本场景演示 `web` 凭据的一次性 fill handle：`password`（或其他受支持的字段），以及由 `totp_seed` 生成的 TOTP。`fill` 只返回不透明 handle；`fill apply` 通过 `stdin` 和 `AGENT_PWD_FILL` 把值注入受信任的子进程。

See [English](README.md).

## 前置条件

- `agent-pwd status` 必须显示 daemon 正在运行且已解锁（必要时执行 `agent-pwd unlock`）；`agent-pwd cred list` 可查看要用的 label。
- 需要一个含 `password` 的 `web` 凭据；`run_totp.sh` 还需要 `totp_seed`，以便 daemon 生成当前验证码。
- `show_length.py` 需要 `python3`，它会同时读取两条注入通道。
- 各凭据类型支持的 `--field`：`web` → `password`（默认）、`totp`、`username`；`apikey` → `token`；`server` → `password`；`envvar` → `value`。本目录的包装脚本面向 `web` 凭据。
- 所有 `agent-pwd` 动作默认输出 JSON。退出码：`0` 成功、`10` 待确认、`20` 未解锁/daemon 未运行、`30` 策略拒绝、`40` 凭据不存在。

## 脚本

| 文件 | 用法 | 预期行为 |
|---|---|---|
| `run_password.sh` | `run_password.sh <web-credential-label> [field]` | 执行 `agent-pwd fill --cred "$label" --field "$field"`（`field` 默认 `password`），用 `json_field` 取 `handle`，然后执行 `agent-pwd fill apply --handle "$handle" -- python3 "$HERE/show_length.py"`。 |
| `run_totp.sh` | `run_totp.sh <web-credential-label>` | 执行 `agent-pwd fill --cred "$label" --field totp`，并把 handle 应用到 `show_length.py`；凭据需要 `totp_seed`。 |
| `show_length.py` | `python3 show_length.py`（无参数；通过 `agent-pwd fill apply --handle <h> -- ...` 启动） | 读取 `AGENT_PWD_FILL` 和一行 stdin，只打印 `env length:`、`stdin length:` 和 `same value:`，从不打印值本身。 |

## 示例

```bash
./samples/fill/run_password.sh '<label>'
./samples/fill/run_password.sh '<label>' '<field>'
./samples/fill/run_totp.sh '<label>'
```

## 安全与注意事项

- 脚本中没有明文密钥，也不会打印密钥；`fill apply` 会脱敏子进程的 stdout/stderr，`show_length.py` 只报告长度。
- `fill` handle 是一次性的，默认 60 秒过期（`AGENT_PWD_FILL_TTL`）；即使子进程失败也会被消费，`fill apply` 必须在 `--` 后提供命令。确认只能由人类完成：动作退出 `10` 时应停止并请人类批准。
- `--cred` 接受凭据 label 或 ID；优先匹配 label，因此 label 胜出。
- 其他示例中的用户名是普通参数；如果用户名也敏感，请同样存入凭据并使用 `fill --field username`。
- 包装脚本会 source `../common.sh`；未传 label 时，该脚本也接受 `AGENT_PWD_CRED` 作为 label 回退值。
