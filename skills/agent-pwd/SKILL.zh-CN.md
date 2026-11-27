---
name: agent-pwd
description: 通过 agent-pwd CLI 使用已存储的凭据执行动作（SSH、SQL、HTTP API、环境变量注入、网页填写），而绝不读取明文密钥。当任务需要 agent-pwd 中的密码、API token、SSH 密钥或环境变量，或需要通过代理登录网站、查询数据库、调用带认证的 API 时使用。
---

# agent-pwd 技能

[English](SKILL.md)

`agent-pwd` 是凭据代理：可以**使用**凭据，但不能**读取**凭据。唯一入口是 CLI，没有 HTTP API，也没有 MCP Server。

需要精确 schema 时，先用 `agent-pwd docs --format json` 读取机器可读契约；完整操作手册见 [`docs/manual.zh-CN.md`](../docs/manual.zh-CN.md)。

## 硬性规则

1. 绝不要尝试打印、转存、猜测或外传密钥。不存在任何返回已存储值的命令。
2. `cred show` 只返回元数据。不要读取 `vault.dat`、备份、`audit.log`、daemon 内存或 IPC socket 来寻找密钥。
3. 除非用户明确要求，不要自行启动 daemon。缺少 daemon 会返回退出码 `20`；请告知用户运行 `agent-pwd daemon` 或 `agent-pwd service install`。
4. 动作以 `10` 退出时**立即停止**，不要重试，不要自动批准；把 `confirmation_id` 和 `poll_token` 交给人类。
5. `--cred` 使用凭据 **label**（主标识，优先于 ID 匹配），也兼容旧版 `cred_...` ID。再次添加同名 label 会覆盖原凭据。

## 标准流程

```bash
agent-pwd status                  # daemon 是否运行并已解锁？
agent-pwd cred list               # 只看元数据，找到 label
agent-pwd <动作> --cred <label> ...
```

动作命令默认输出 JSON。成功时直接打印结果对象；失败时打印 `{"ok":false,"error":{"code":N,"message":"..."}}`。

| 退出码 | 含义 | 处理方式 |
|---|---|---|
| 0 | 成功 | 解析结果 |
| 10 | 待人工确认 | 停止，把 `confirmation_id` + `poll_token` 交给人类 |
| 20 | 未解锁 / daemon 未运行 | 请人类启动 daemon 并执行 `agent-pwd unlock` |
| 30 | 策略拒绝 | 如实报告，不要绕过 |
| 40 | 凭据不存在 | 用 `cred list` 核对 label |
| 1 | 其他错误 | 报告 `error.message` |

## 动作命令

```bash
# SSH：执行远程命令
agent-pwd ssh exec --cred prod-db -- 'uptime'

# SQL：读操作自动放行，写操作需要确认
agent-pwd sql query --cred prod-mysql -- 'SELECT id, email FROM users LIMIT 10'
agent-pwd sql query --cred prod-mysql -- 'SELECT * FROM users WHERE id = ?' --arg 42

# HTTP API：token 由 daemon 注入，调用方不接触
agent-pwd api call --cred github --method GET --path /user
agent-pwd api call --cred github --method POST --path /repos/x/y/issues --body '{"title":"bug"}'

# 环境变量注入子进程
agent-pwd env run --cred deploy-token -- ./deploy.sh

# Redis：只读命令白名单
agent-pwd sql query --cred cache -- 'GET session:42'
```

`--insecure-host-key` 会关闭 SSH 主机密钥校验，仅用于测试；使用时须告知用户。

## 用 `fill` 登录网站

不要索要密码。申请一次性 handle，再交给受信任的自动化脚本：

```bash
agent-pwd fill --cred github --field password
# -> {"handle":"...","field":"password","expires_at":"...","note":"..."}

agent-pwd fill apply --handle <handle> -- python3 login.py alice@example.com
```

`fill apply` 通过 `AGENT_PWD_FILL` 和 stdin 将值注入子进程；脚本负责浏览器交互，`agent-pwd` 只提供密钥并对子进程输出脱敏。handle 60 秒过期且一次性。2FA 使用 `--field totp`，流程相同。

## 确认流程

动作以 `10` 退出时，把返回的 JSON 展示给人类并停止。由人类执行：

```bash
agent-pwd confirm approve <confirmation_id>   # Touch ID / Windows Hello，否则主密码
agent-pwd confirm result <confirmation_id> --token <poll_token>
```

人类确认完成后可调用一次 `confirm result`。若希望原命令阻塞到确认结束，可在原动作上加 `--wait`。`confirm deny <id>` 表示拒绝。无人值守任务会一直阻塞，这是有意设计。

## 语言与输出

- `agent-pwd --lang en|zh-CN docs` 选择文档语言；`AGENT_PWD_LANG` 为兜底。
- JSON key、枚举、错误码、退出码、命令名和 flag 永不翻译。
- 只有人类明确要求时才使用 `--human`。

## 相关文档

- AI 契约：[`docs/contract.zh-CN.md`](../docs/contract.zh-CN.md)
- 操作手册：[`docs/manual.zh-CN.md`](../docs/manual.zh-CN.md)
- 多语言指南：[`docs/i18n.zh-CN.md`](../docs/i18n.zh-CN.md)
- 安全政策：[`SECURITY.zh-CN.md`](../SECURITY.zh-CN.md)
