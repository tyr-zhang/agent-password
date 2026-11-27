# agent-pwd

[![CI](https://github.com/tyr-zhang/agent-password/actions/workflows/ci.yml/badge.svg)](https://github.com/tyr-zhang/agent-password/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8.svg)](https://go.dev/)

[English](README.md) | 简体中文

个人自用的 AI 凭据代理：凭据加密存放在单一文件中，AI 只能请求**动作**，拿不到明文。CLI 是唯一入口——没有 HTTP API、没有 MCP Server、没有 GUI。

## 为什么做这个

传统密码管理器以“人方便读出密码”为第一目标；本项目恰好相反：AI 可以*使用*凭据完成任务，但无法读取凭据本身。

- 四类凭据：`web`、`apikey`、`server`、`envvar`。
- 策略引擎：`allow` / `confirm` / `deny`；未知动作默认 `confirm`。
- 哈希链审计日志，动作可追溯、凭据可即时吊销。
- 单一 Go 二进制，`CGO_ENABLED=0`，支持 macOS / Windows / Linux。

## 安装

```bash
git clone https://github.com/tyr-zhang/agent-password.git
cd agent-password
make build                         # 生成 bin/agent-pwd
cp bin/agent-pwd ~/.local/bin/agent-pwd
```

## 快速开始

```bash
agent-pwd init                     # 交互式设置主密码
agent-pwd service install          # 可选：LaunchAgent / systemd / 计划任务
agent-pwd daemon &                 # 或：agent-pwd daemon --foreground
agent-pwd unlock
agent-pwd status
```

凭据逐字段录入，敏感字段隐藏输入，不会进入 shell 历史：

```bash
agent-pwd cred add web github
agent-pwd cred add apikey openai
agent-pwd cred add server prod-mysql          # 端口与其他字段一样交互输入
agent-pwd cred add envvar deploy-token
agent-pwd cred list
agent-pwd cred show <label|id>     # 仅元数据，不存在输出 secret 的路径
```

凭据 **label 是主标识**：再次添加同名 label 会覆盖该凭据，并保留其 ID 和 `created_at`。所有引用凭据的地方（`--cred`、`cred show`、`cred rm`）都可以使用 label 或旧版 `cred_...` ID；如果某个 label 恰好等于另一个凭据的 ID，**label 优先**。

自动化场景下，把所有服务器字段（包括 `port`）放进 `--secret`；`--port` 仅作为兼容别名保留：

```bash
agent-pwd cred add server prod-mysql \
  --secret '{"protocol":"mysql","host":"127.0.0.1","port":3306,"server_user":"app","password":"<密码>","database":"appdb"}'
```

动作命令：

```bash
agent-pwd ssh exec --cred <id> -- 'uptime'
agent-pwd sql query --cred <id> -- 'SELECT count(*) FROM users'
agent-pwd api call --cred <id> --method GET --path /v1/me
agent-pwd env run --cred <id> -- ./deploy.sh
agent-pwd fill --cred <id> --field password
agent-pwd fill apply --handle <handle> -- ./browser-automation
```

高风险动作以退出码 `10` 返回 `confirmation_id`，动作挂起直到人工批准：

```bash
agent-pwd confirm approve <id>     # Touch ID / Windows Hello，失败降级主密码
agent-pwd confirm result <id> --token <poll_token>
```

退出码：`0` 成功，`10` 待确认，`20` 未解锁或 daemon 未运行，`30` 策略拒绝，`40` 凭据不存在，`1` 其他错误。

## AI 接入

```bash
agent-pwd docs > ~/.claude/agent-pwd-contract.md
```

把生成的契约嵌入 `CLAUDE.md` 或技能说明。契约包含能力边界、JSON schema、退出码和确认流程，并明确声明 AI 不能读取密码。

仓库自带可直接使用的人工技能：[`skills/agent-pwd/SKILL.zh-CN.md`](skills/agent-pwd/SKILL.zh-CN.md)；完整操作手册见 [`docs/manual.zh-CN.md`](docs/manual.zh-CN.md)。每种场景的可运行示例在 [`samples/`](samples/README.zh-CN.md)：网页登录、SSH、SQL、API、环境变量注入、fill、确认流程和运维。

## 多语言

CLI 与生成文档支持英文和简体中文：

```bash
agent-pwd --lang zh-CN docs
AGENT_PWD_LANG=zh-CN agent-pwd help
```

优先级：`--lang` > `AGENT_PWD_LANG` > `en`。机器可读值（JSON key、枚举、退出码、错误码）永远不翻译。详见 [docs/i18n.zh-CN.md](docs/i18n.zh-CN.md) 以及生成的[中文](docs/contract.zh-CN.md)/[英文](docs/contract.md)契约。

## 数据位置

| 平台 | 目录 |
|---|---|
| macOS | `~/Library/Application Support/agent-pwd/` |
| Windows | `%APPDATA%\agent-pwd\` |
| Linux | `$XDG_DATA_HOME/agent-pwd/`（默认 `~/.local/share/agent-pwd/`） |

`AGENT_PWD_HOME` 可覆盖目录。Vault、备份和审计日志都在此目录中；文件权限 0600、目录 0700（Windows 使用当前用户 ACL）。

会话默认：空闲 8 小时、单次解锁硬上限 24 小时、确认 5 分钟、fill 句柄 60 秒。daemon 启动时可用 `AGENT_PWD_IDLE_TIMEOUT`、`AGENT_PWD_HARD_TIMEOUT`、`AGENT_PWD_CONFIRM_TTL`、`AGENT_PWD_FILL_TTL` 覆盖（Go duration 格式，如 `12h`）。`agent-pwd status` 显示实际生效值。

## 安全模型

防住：上下文注入读取明文、静默执行高风险动作、vault 被拷贝后直接读取、审计被事后篡改。**明确不防**：以同一操作系统用户身份运行的恶意软件——它可以读取进程内存、替换二进制或访问本机 socket。操作系统账户才是真正的安全下限。二次确认是防误导，不是防入侵。

漏洞报告方式见 [SECURITY.zh-CN.md](SECURITY.zh-CN.md)，完整设计见[设计文档](docs/design/2026-11-26-personal-ai-credential-proxy-design.md)。

## 参与贡献

见 [CONTRIBUTING.zh-CN.md](CONTRIBUTING.zh-CN.md)、[CODE_OF_CONDUCT.zh-CN.md](CODE_OF_CONDUCT.zh-CN.md) 和 [CHANGELOG.md](CHANGELOG.md)。

```bash
make test       # 单元 + 真实 daemon 集成 + 安全回归
make race
make vet
make cross      # darwin/windows/linux，CGO_ENABLED=0
make docs-check # 校验两种语言生成文档
```
