# web-login 示例

本场景用 `web` 凭据登录网站：Shell 包装脚本为 `password` 字段申请一次性 `fill` handle，由受信任的 Playwright 子进程填写表单，agent 始终看不到明文。

See [English](README.md).

## 前置条件

- `agent-pwd status` 必须显示 daemon 正在运行且已解锁（必要时执行 `agent-pwd unlock`）；`agent-pwd cred list` 可查看要用的 label。
- 需要 `python3`、Playwright 的 Python 包（`pip install playwright`），以及它可用的 Chromium 浏览器；脚本使用 `p.chromium.launch(headless=False)`，因此需要图形显示环境。
- 需要一个包含 `password` 的 `web` 凭据；用户名由普通参数传入。TOTP 请使用单独的 `samples/fill/run_totp.sh` 示例，并按需扩展浏览器脚本。
- 所有 `agent-pwd` 动作默认输出 JSON。退出码：`0` 成功、`10` 待确认、`20` 未解锁/daemon 未运行、`30` 策略拒绝、`40` 凭据不存在。

## 脚本

| 文件 | 用法 | 预期行为 |
|---|---|---|
| `run_jenkins.sh` | `run_jenkins.sh <label|id> <username> [url]` | 执行 `agent-pwd fill --cred "$label" --field password`，把 handle 应用到 `jenkins.py`，只打印浏览器页面标题和 URL。`url` 默认 `https://jenkins.example.com/`。 |
| `run_generic.sh` | `run_generic.sh <label> <username> <url> <user_sel> <pass_sel> [submit_sel]` | 申请 `password` fill handle，用调用方提供的选择器应用到 `generic_form.py`；`submit_sel` 默认 `button[type=submit]`。 |
| `jenkins.py` | `python3 jenkins.py <url> <username> [user_selector] [pass_selector] [submit_selector]`（通过 `agent-pwd fill apply --handle <h> -- ...` 调用） | 打开 Chromium，通过 `AGENT_PWD_FILL` 填写表单并点击提交，等待 `networkidle`，打印 `title:` 和 `url:`。选择器默认 `input[name="j_username"]`、`input[name="j_password"]`、`text=Sign in`；参数不足或 `AGENT_PWD_FILL` 为空时退出 `2`。 |
| `generic_form.py` | `python3 generic_form.py <url> <username> <user_selector> <pass_selector> [submit_selector]`（通过 `agent-pwd fill apply --handle <h> -- ...` 调用） | 使用调用方提供的选择器执行同样的浏览器流程；提交选择器默认 `button[type="submit"]`；参数不足或 `AGENT_PWD_FILL` 为空时退出 `2`。 |

## 示例

```bash
./samples/web-login/run_jenkins.sh '<label>' '<username>' '<url>'
AGENT_PWD_CRED='<label>' ./samples/web-login/run_jenkins.sh '<label>' '<username>'
./samples/web-login/run_generic.sh '<label>' '<username>' '<url>' '<user-selector>' '<pass-selector>'
./samples/web-login/run_generic.sh '<label>' '<username>' '<url>' '<user-selector>' '<pass-selector>' '<submit-selector>'
```

## 安全与注意事项

- 脚本中没有明文密钥，也不会打印密钥；`fill apply` 会脱敏子进程的 stdout/stderr。`agent-pwd` 不负责打开浏览器或定位元素，这些操作由受信任的 Playwright 子进程完成。
- `fill` handle 是一次性的，默认 60 秒过期（即使子进程失败也会被消费）；请使用同一客户端立即 `apply`。动作退出 `10` 时必须停下来交由人类批准——确认只能由人类完成。
- `--cred` 接受凭据 label 或 ID；优先匹配 label，因此 label 胜出。
- `generic_form.py` 的模块文档字符串漏写了 `<username>`，但实际参数处理需要它；请按上面的示例传入用户名。
- 包装脚本会 source `../common.sh`；未传 label 时，该脚本也接受 `AGENT_PWD_CRED` 作为 label 回退值。
