# agent-pwd 操作手册

[English](manual.md)

本手册是 `agent-pwd` 的完整操作文档。`agent-pwd` 是个人自用的 AI 凭据代理：凭据加密存放于单个本地文件，AI 只能请求**动作**，拿不到明文。CLI 是唯一入口——没有 HTTP API、没有 MCP Server、也没有 GUI。下文所有命令、标志、JSON key、枚举和退出码均为原样机器 token。

## 1. 概览与核心概念

`agent-pwd` 把短生命周期的 CLI 客户端与常驻 daemon 分开：只有 daemon 会持有解密后的密钥。

| 组件 | 说明 | 位置 |
|---|---|---|
| CLI 客户端 | 每一次 `agent-pwd ...` 调用；解析标志、与 daemon 通信、输出脱敏 JSON | 二进制 `bin/agent-pwd`（或其安装副本） |
| daemon | 常驻进程：持有会话密钥、执行策略判定、运行动作、写审计记录 | 通过 `agent-pwd daemon` 显式启动，或安装为当前用户服务 |
| Vault | `vault.dat`，单个 AES-256-GCM 加密体，头部含 Argon2id 包裹的 DEK | 平台数据目录，权限 `0600` |
| Session | 内存中的 DEK/KEK 及空闲/硬超时；仅在 `agent-pwd unlock` 后解锁 | 仅存在于 daemon 内存，永不落盘 |
| Policy | 存放在加密 vault 中的有序 `allow` / `confirm` / `deny` 规则 | 每次动作由 daemon 判定 |
| Audit | 追加写入、带 SHA-256 哈希链的 `audit.log` | 同一数据目录，权限 `0600` |
| Label | 凭据的**主键**，在 vault 内唯一；`--cred` 接受 label 或旧版 ID | 保存在 vault 元数据中 |

典型流程：

```bash
agent-pwd init                       # 一次性创建 vault 与主密码
agent-pwd service install            # 可选：配置 daemon 自启动
agent-pwd daemon --foreground        # 或在终端中手动启动
agent-pwd unlock                     # 解锁运行中 daemon 的会话
agent-pwd status                     # 确认 daemon、会话与 KDF 状态
agent-pwd cred add web github        # 按 label 存放凭据
agent-pwd ssh exec --cred prod-db -- 'uptime'
agent-pwd audit verify               # 校验哈希链
```

核心概念：

- **只有 CLI。** Agent 不能直接打开 socket，只能执行命令。动作命令默认输出 JSON（用 `--human` 切换为紧凑文本）；daemon IPC 信封不会暴露给调用方。
- **Label 是主键。** `cred add <kind> <label>` 遇到已存在的 label 会原地覆盖该凭据：ID 与 `created_at` 保留，`updated_at` 及所有 secret/策略相关字段被替换，旧 vault 中重复的 label 会被清理。`--cred`、`cred show` 与 `cred rm` 都接受 label 或 ID。
- **动作而非值。** 不存在 `cred show --secret`，没有 HTTP API，也没有任何命令返回已存储的值。daemon 会把密钥注入 SSH 连接、SQL 客户端、HTTP 请求、子进程和一次性 fill handle。
- **策略先行。** 每条动作由第一条匹配的策略规则判定；未知组合一律落到 `confirm`。标记为 `risk: high` 的凭据会把 `allow` 降级为 `confirm`；显式 `deny` 永远优先。
- **全程审计。** `started`、`approved`、`denied`、`timeout`、`completed` 以及策略拒绝结果都会追加到哈希链日志，`audit verify` 可检测篡改、缺口和顺序变化。

数据目录：

| 平台 | 目录 |
|---|---|
| macOS | `~/Library/Application Support/agent-pwd/` |
| Windows | `%APPDATA%\agent-pwd\` |
| Linux | `$XDG_DATA_HOME/agent-pwd/`（默认 `~/.local/share/agent-pwd/`） |

`AGENT_PWD_HOME` 可覆盖该目录。文件权限为 `0600`、目录为 `0700`（Windows 使用当前用户 ACL）。目录中会有 `vault.dat`、`audit.log`、`known_hosts`（SSH 主机密钥校验需要时创建）、`vault.dat.bak.1` 至 `vault.dat.bak.5`，以及安装服务后的 `daemon.log`。

## 2. 安装、升级与服务管理

要求：Go 1.25+ 与 `make`。Makefile 导出 `CGO_ENABLED=0`；`make cross` 会构建 darwin/arm64、darwin/amd64、linux/amd64 与 windows/amd64 二进制。

```bash
git clone https://github.com/tyr-zhang/agent-password.git
cd agent-password
make build                           # 生成 bin/agent-pwd
cp bin/agent-pwd ~/.local/bin/agent-pwd
agent-pwd version                    # {"version": ..., "commit": ..., "date": ...}
```

在 Windows 上把 `bin/agent-pwd.exe` 复制到 `PATH` 中的目录。Linux 上若希望不带完整路径调用 `agent-pwd`，`~/.local/bin` 必须在 `PATH` 中。

daemon 启动方式：

```bash
agent-pwd daemon --foreground        # 保持前台运行；Ctrl-C 停止
agent-pwd daemon                     # 脱离终端在后台运行
agent-pwd daemon &                   # 从 shell 后台运行的等价写法
```

动作命令**绝不会**隐式启动 daemon。如果没有 daemon 在监听，命令以退出码 `20` 退出，并提示启动方法。若需无人值守可用，请安装服务：

```bash
agent-pwd service install            # LaunchAgent / systemd user unit / 计划任务
agent-pwd service status             # 报告自启动状态
agent-pwd service uninstall          # 删除当前用户的自启动项
```

服务会以执行 `service install` 时二进制的绝对路径运行 `agent-pwd daemon --foreground`。它在 macOS 上安装 LaunchAgent `com.tyrzhang.agent-pwd`，在 Linux 上安装 systemd user unit `agent-pwd`，在 Windows 上创建计划任务 `agent-pwd`。若之后移动或替换了二进制文件，请重新安装服务以保持路径正确。

升级步骤：

```bash
git -C agent-password pull
cd agent-password && make build
cp bin/agent-pwd ~/.local/bin/agent-pwd
agent-pwd version
agent-pwd doctor
```

vault 格式带版本号；用新二进制覆盖旧文件不会修改 `vault.dat`。升级后重启 daemon 以加载新代码。daemon 重启会锁定会话（密钥只在内存中），因此需要重新执行 `agent-pwd unlock`。

常用构建检查：

```bash
make test                            # 单元测试、真实 daemon 集成测试、安全回归测试
make race
make vet
make cross
make docs-check                      # 校验两份生成的契约是否同步
```

## 3. 首次运行与会话

首次初始化 vault（推荐交互式输入，主密码不会进入 shell 历史）：

```bash
agent-pwd init
# 提示：master password，然后 confirm master password
```

`init` 接受 `--password` / `--new-password` 用于自动化，两者都表示主密码。它会创建数据目录、写入加密的 `vault.dat` 并安装默认策略规则。主密码永不存储：丢失主密码即丢失整个 vault。

启动 daemon、解锁、检查状态：

```bash
agent-pwd daemon --foreground        # 在某个终端中，或使用 service install
agent-pwd unlock                     # 提示输入主密码
agent-pwd status
agent-pwd lock
```

- `unlock` 接受 `--password`（推荐使用交互式提示），返回已加载的凭据数量。
- `status` 报告 `unlocked`、`version`、`data_dir`、`vault`、`failures`、`session_id`、以 `*_seconds` 表示的实际生效超时、`pending`、`service`、`audit_ok` 与 `expires_at`，绝不打印密钥。
- `lock` 清零会话密钥、重置脱敏器，并使 fill handle 与待确认操作失效。之后用 `agent-pwd unlock` 重新解锁。

会话默认值，均可在 **daemon 启动时**用 Go duration 字符串（如 `12h`、`30m`）覆盖：

| 设置 | 默认值 | 环境变量 |
|---|---|---|
| 空闲超时 | 8h | `AGENT_PWD_IDLE_TIMEOUT` |
| 自解锁起的硬上限 | 24h | `AGENT_PWD_HARD_TIMEOUT` |
| 待确认操作 TTL | 5m | `AGENT_PWD_CONFIRM_TTL` |
| fill handle TTL | 60s | `AGENT_PWD_FILL_TTL` |

```bash
AGENT_PWD_IDLE_TIMEOUT=2h AGENT_PWD_HARD_TIMEOUT=12h \
AGENT_PWD_CONFIRM_TTL=10m AGENT_PWD_FILL_TTL=30s \
  agent-pwd daemon --foreground
agent-pwd status                     # 以秒显示实际生效值
```

若 `AGENT_PWD_HARD_TIMEOUT` 小于 `AGENT_PWD_IDLE_TIMEOUT`，daemon 会将其提升到空闲超时值。无效或非正的 duration 会被忽略。修改这些变量需要重启 daemon；`AGENT_PWD_LANG` 与之独立，可在每次 CLI 调用时设置。

密码错误退避：

- `unlock` 失败（`rotate-password` 时旧密码错误同理）会累加失败次数，并在 1s、2s、4s、8s、16s、32s 内禁止再次尝试，最长 60s。
- CLI 使用 `--human` 时会在 stderr 打印 `retry after Ns`；JSON 错误包含 `code` 与 `message`。`status` 显示当前的 `failures` 计数。
- 连续 5 次失败后 daemon 自行停止。请有意重启后再试；不要自动化猜测密码。

## 4. 凭据管理

`agent-pwd cred add <web|apikey|server|envvar> <label>` 默认交互式录入：字段从终端/stdin 读取，隐藏字段不回显，从而不会进入 shell 历史。

```bash
agent-pwd cred add web github
agent-pwd cred add apikey openai
agent-pwd cred add server prod-mysql
agent-pwd cred add envvar deploy-token
```

各类凭据的交互字段顺序：

| Kind | 提示字段 | 隐藏字段 |
|---|---|---|
| `web` | `username`、`password`、`totp_seed (optional)`、`url (optional)` | `password` |
| `apikey` | `base_url`、`header_name (default Authorization)`、`token` | `token` |
| `server` | `protocol (ssh/mysql/postgres/redis)`、`host`、`port (optional)`、`server_user`、`password (or leave empty for key)`、`private_key (optional)`、`key_path (optional)`、`passphrase (optional)`、`database (optional)` | `password`、`private_key`、`passphrase` |
| `envvar` | `key`、`value` | `value` |

自动化场景使用 `--secret` 加 JSON 对象。JSON 层面所有字段均可省略，但 daemon 会校验每种凭据的必填项：

| JSON 字段 | 类型 | Kind | 必填 | 含义 |
|---|---|---|---|---|
| `username` | string | `web` | 是 | 登录用户名 |
| `password` | string | `web`、`server` | `web` 必填 | 网站密码；`server` 只需 password/private key/key path 之一 |
| `totp_seed` | string | `web` | 否 | Base32 TOTP seed，供 `fill --field totp` 使用 |
| `url` | string | `web` | 否 | 可选的网站 URL |
| `base_url` | string | `apikey` | 是 | 与 `--path` 拼接的 scheme + host 前缀，如 `https://api.example.com` |
| `header_name` | string | `apikey` | 否 | 认证 header 名称，默认 `Authorization` |
| `token` | string | `apikey` | 是 | 由 daemon 注入认证 header 的值 |
| `protocol` | string | `server` | 是 | `ssh`、`mysql`、`postgres`/`postgresql` 或 `redis` |
| `host` | string | `server` | 是 | 主机名或 IP |
| `port` | number | `server` | 否 | TCP 端口；默认 22 (ssh)、3306 (mysql)、5432 (postgres)、6379 (redis) |
| `server_user` | string | `server` | 是 | SSH/数据库用户名 |
| `private_key` | string | `server` | 否 | PEM 私钥内容 |
| `key_path` | string | `server` | 否 | 私钥文件路径，由 daemon 读取 |
| `passphrase` | string | `server` | 否 | 加密私钥的口令 |
| `database` | string | `server` | 否 | SQL 协议使用的数据库名 |
| `key` | string | `envvar` | 是 | 环境变量名 |
| `value` | string | `envvar` | 是 | 环境变量值 |

`cred add` 其他标志：`--risk normal|high`（默认 `normal`；`high` 会让该凭据的所有动作强制确认）、`--tags a,b,c`（逗号分隔）和 `--port N`。`--port` 仅为兼容别名：无论端口来自 `--secret` 还是交互提示，它都会覆盖 `secret.port`。自动化时请始终把规范的 `port` 字段放进 `--secret`。

每类凭据的具体示例：

```bash
# web：交互式（密码隐藏输入）
agent-pwd cred add web github --tags work,2fa --risk high

# web：自动化
agent-pwd cred add web github \
  --secret '{"username":"alice@example.com","password":"<密码>","totp_seed":"JBSWY3DPEHPK3PXP","url":"https://github.com"}'

# apikey：交互式
agent-pwd cred add apikey openai

# apikey：自动化（header_name 默认 Authorization）
agent-pwd cred add apikey openai \
  --secret '{"base_url":"https://api.openai.com","header_name":"Authorization","token":"<token>"}'

# server：交互式（端口与其他字段一样交互输入）
agent-pwd cred add server prod-mysql

# server：自动化——把 port 放进 secret JSON
agent-pwd cred add server prod-mysql \
  --secret '{"protocol":"mysql","host":"127.0.0.1","port":3306,"server_user":"app","password":"<密码>","database":"appdb"}'

# server：SSH 密钥认证
agent-pwd cred add server bastion \
  --secret '{"protocol":"ssh","host":"bastion.example.com","port":22,"server_user":"ops","key_path":"/Users/me/.ssh/id_ed25519","passphrase":"<口令>"}'

# envvar：交互式
agent-pwd cred add envvar deploy-token

# envvar：自动化
agent-pwd cred add envvar deploy-token \
  --secret '{"key":"DEPLOY_TOKEN","value":"<值>"}'
```

按 label 覆盖语义：

- label 是主键。先执行 `cred add web github`，再执行一次同名命令，会原地替换第一条记录。
- `id` 与 `created_at` 保留；`kind`、`risk_level`、`tags`、`updated_at` 及 secret 字段被替换。
- 如果旧 vault 中存在重复 label，再次添加该 label 会只保留一条并删除重复项，从而使策略引用与审计 ID 保持稳定。
- 任何引用凭据的地方都可用 `--cred github`（label）或 `cred_...` ID。人和 agent 都应使用 label。
- 解析顺序是确定的：先匹配所有 label，再匹配 ID。如果某个凭据的 label 恰好等于另一个凭据的 ID，label 优先。

查看与删除：

```bash
agent-pwd cred list                  # 仅元数据：id、kind、label、risk_level、tags、时间戳
agent-pwd cred show github           # 按 label；只有元数据，不存在输出 secret 的路径
agent-pwd cred show cred_ab12cd34ef567890
agent-pwd cred rm github             # 按 label
agent-pwd cred rm cred_ab12cd34ef567890
```

备份与恢复：

- 每次写 vault 都会轮转最多五个加密备份：`vault.dat.bak.1`（最新）、`.bak.2` … `.bak.5`。写入新快照前，会把原有 `.bak.N` 改名为 `.bak.N+1`。
- 备份使用同一套 vault 密钥加密；妥善保管主密码同样能保护备份。
- 恢复是明确的人工操作。请先核对候选备份，再执行命令；恢复会覆盖 `vault.dat` 并锁定会话，之后需要重新解锁。

```bash
agent-pwd doctor                     # 列出备份数量与最新备份时间
agent-pwd cred restore --from /path/to/data-dir/vault.dat.bak.2
agent-pwd unlock
```

## 5. 策略引擎

策略规则保存在加密 vault 中，自上而下判定；第一条同时匹配凭据 pattern、动作与条件的规则胜出。若无任何规则匹配，决策为 `confirm`（既不静默 `allow`，也不会硬 `deny` 导致新凭据完全不可用）。pattern 是 Go `path.Match` glob，匹配对象是 `<kind>:<id>`，例如 `server:cred_ab12...`，而不是 label。

`init` 安装的默认规则，按优先级排列：

| # | Rule ID | `cred_pattern` | `action` | `condition` | Decision |
|---|---|---|---|---|---|
| 1 | `default-ssh` | `server:*` | `ssh` | — | `allow` |
| 2 | `default-sql-read` | `server:*` | `sql` | `sql_readonly` | `allow` |
| 3 | `default-api-read` | `apikey:*` | `api` | `http_readonly` | `allow` |
| 4 | `default-fill` | `*` | `fill` | — | `allow` |
| 5 | `default-env` | `*` | `env` | — | `allow` |
| 6 | `default-sql-write` | `*` | `sql` | — | `confirm` |
| 7 | `default-api-write` | `*` | `api` | — | `confirm` |
| 8 | `default-fallback` | `*` | `*` | — | `confirm` |

条件说明：

- `sql_readonly`：去掉注释并裁剪后，查询以 `SELECT`、`SHOW`、`DESCRIBE`、`DESC` 或 `EXPLAIN` 开头，且不包含第二条语句。允许单个结尾 `;`；`WITH` 被视为写操作，需要确认。
- `http_readonly`：HTTP 方法为 `GET` 或 `HEAD`。其他方法落到写规则，需要确认。
- 无条件表示规则适用于所有匹配动作。

新规则插入到**最前面**，因此最新规则优先级最高：

```bash
agent-pwd policy add --cred-pattern 'server:*' --action ssh --decision deny
agent-pwd policy add --cred-pattern 'apikey:*' --action api --condition http_readonly --decision allow
agent-pwd policy add --cred-pattern 'server:cred_ab12cd34ef567890' --action sql --condition sql_readonly --decision allow --id allow-one-sql
agent-pwd policy add --cred-pattern 'apikey:*' --action '*' --decision confirm
agent-pwd policy list
agent-pwd policy rm allow-one-sql
```

注意事项：

- `--decision` 必须是 `allow`、`confirm` 或 `deny`；`--condition` 必须是空、`sql_readonly` 或 `http_readonly`。
- `--id` 可选；省略时 daemon 生成 `rule_...`，重复 ID 会被拒绝。`policy rm <id>` 使用的是规则 ID，不是凭据 ID。
- 优先级即列表顺序：`policy list` 显示的顺序就是判定顺序。宽泛的 `deny` 放在窄范围 `allow` 之前会立即终止判定。
- `risk: high` 的凭据会把命中的 `allow` 降级为 `confirm`。命中的 `deny` 绝对优先，不会被降级，也不会被后面的规则覆盖。
- `action` 取值为 `ssh`、`sql`、`api`、`fill`、`env` 或 `*`。被拒绝的动作同样会以 `denied` 结果写入审计日志。

## 6. 动作

所有动作命令都接受 `--cred <label|id>`，默认输出 JSON，返回脱敏结果。请用 `--` 分隔 CLI 选项与远程命令或 SQL 文本。动作执行超时为 30 秒；输出上限 1 MiB（截断时 `truncated: true`），SQL 结果上限 1000 行。

### SSH 执行

`agent-pwd ssh exec --cred <label|id> -- <command...>` 需要 `protocol=ssh` 的 `server` 凭据、密码和/或密钥，并且默认要求数据目录中有 `known_hosts`。默认端口 22。密码、私钥与口令永远不会出现在结果中。

```bash
agent-pwd ssh exec --cred bastion -- 'uptime'
agent-pwd ssh exec --cred prod-db -- 'systemctl --user status app'
agent-pwd ssh exec --cred bastion --insecure-host-key -- 'uname -a'   # 仅用于测试
```

主机密钥：默认严格校验 `known_hosts`。若文件不存在，动作会失败并提示其路径；请先添加主机密钥到 `<data-dir>/known_hosts`。`--insecure-host-key` 会完全关闭校验，允许中间人攻击——仅限测试，使用时须告知他人。结果包含 `output`、`exit_code` 与 `truncated`。

### SQL：MySQL、PostgreSQL、Redis

`agent-pwd sql query --cred <label|id> -- "<SQL>" [--arg v]...` 适用于使用 `mysql`、`postgres` 或 `redis` 的 `server` 凭据。`--arg` 将值绑定为查询参数（多个值请重复该标志），而不是拼接进 SQL 文本。

```bash
# MySQL：默认被 default-sql-read 放行
agent-pwd sql query --cred prod-mysql -- 'SELECT count(*) FROM users'

# PostgreSQL：绑定参数，不做字符串拼接
agent-pwd sql query --cred analytics -- 'SELECT id, email FROM users WHERE id = $1' --arg 42

# MySQL 占位符风格，两个绑定参数
agent-pwd sql query --cred prod-mysql -- 'SELECT id FROM users WHERE team = ? AND active = ?' --arg ops --arg 1

# 写操作命中 default-sql-write，返回退出码 10 等待确认
agent-pwd sql query --cred prod-mysql -- 'UPDATE users SET active = 0 WHERE id = 42'

# Redis 使用同一命令，但只有只读命令白名单
agent-pwd sql query --cred cache -- 'GET session:42'
agent-pwd sql query --cred cache -- 'HGETALL user:42'
```

只读路径规则：

- `SELECT`、`SHOW`、`DESCRIBE`、`DESC`、`EXPLAIN` 对 `server:*` 自动放行；其他起始动词（包括 `WITH`）需要确认。
- 多语句输入不算只读：允许一个结尾分号，出现第二个 `;` 则拒绝。分类前会去掉注释，因此注释花招无法伪装成读操作。MySQL 连接还设置了 `multiStatements=false`。
- Redis 只允许只读命令 `GET`、`MGET`、`HGET`、`HGETALL`、`HMGET`、`LRANGE`、`LLEN`、`SMEMBERS`、`SISMEMBER`、`ZRANGE`、`ZSCORE`、`TTL`、`PTTL`、`EXISTS`、`TYPE`、`INFO`、`DBSIZE`。若凭据中有密码，会自动发送 `AUTH`；默认端口 6379。即使放宽策略，执行器仍会拒绝写命令。
- PostgreSQL 使用 TLS（`sslmode=require`），默认端口 5432。

### HTTP API 调用

`agent-pwd api call --cred <label|id> --method GET --path /v1/x` 需要 `apikey` 凭据。daemon 把 `token` 注入配置的 `header_name`（默认 `Authorization`）；调用方不接触 token。默认方法为 `GET`。

```bash
agent-pwd api call --cred github --method GET --path /user
agent-pwd api call --cred github --method HEAD --path /v1/status
agent-pwd api call --cred openai --method POST --path /v1/chat/completions \
  --header 'Content-Type: application/json' \
  --body '{"model":"example","messages":[]}'
agent-pwd api call --cred github --method GET --path /orgs/x/repos --allow-redirect
```

- `--path` 必须是绝对路径（以 `/` 开头），且不得包含 `..` 或 `//`；它会拼接到 `base_url` 后。`--method` 接受 `GET`、`HEAD`、`POST`、`PUT`、`PATCH`、`DELETE`、`OPTIONS`。
- `GET` 与 `HEAD` 通过 `http_readonly` 自动放行；`POST` 等方法命中 `default-api-write`，在人工批准前返回退出码 `10`。
- `--header k:v` 可重复使用。不能覆盖已配置的认证 header、`Authorization`、`Cookie`、`Host` 或 `Proxy-Authorization`。
- 默认不跟随重定向（返回原始响应）。`--allow-redirect` 只允许同源主机内的重定向；跨源重定向会被阻止。
- 结果包含 `status`、少量允许出现的响应 header（`Content-Type`、`Content-Length`、`Date`、`Server`、`Cache-Control`、`ETag`）、`body` 与 `truncated`。响应体在输出前会脱敏。

### 环境变量注入

`agent-pwd env run --cred <label|id> -- <command...>` 在子进程中只注入一个环境变量（`key=value`），返回脱敏后的合并输出及 `exit_code`。

```bash
agent-pwd env run --cred deploy-token -- ./deploy.sh
agent-pwd env run --cred deploy-token -- python3 -c 'import os; print("token set:", bool(os.getenv("DEPLOY_TOKEN")))'
```

没有任何命令可以打印已存储的值。如果子进程回显该值，daemon 的脱敏器会把它替换为 `[REDACTED]`。子进程本身已经信任该值：它主动外传数据不在代理的保证范围内。

### 用 `fill` 填写网页密码与 TOTP

`fill` 绝不返回密钥本身，而是签发一个不透明、一次性、会过期的 handle；`fill apply` 消费该 handle，并通过 `stdin` 和 `AGENT_PWD_FILL` 把值注入受信任的子进程。

```bash
agent-pwd fill --cred github --field password
# -> {"handle":"<handle>","field":"password","expires_at":"...","note":"..."}

agent-pwd fill apply --handle <handle> -- ./browser-login.sh alice@example.com

# 2FA：从 web 凭据的 totp_seed 生成当前 6 位验证码
agent-pwd fill --cred github --field totp
agent-pwd fill apply --handle <handle> -- ./enter-otp.sh
```

字段映射：

| 凭据 Kind | 支持的 `--field` 值 |
|---|---|
| `web` | `password`（默认）、`totp`、`username` |
| `apikey` | `token` |
| `server` | `password` |
| `envvar` | `value` |

handle 默认 60 秒过期（daemon 启动时通过 `AGENT_PWD_FILL_TTL` 配置），与申请它的会话和客户端绑定，且即使子进程随后失败也会被消费掉。`fill apply` 要求在 `--` 后提供命令。浏览器导航与 DOM 交互由子自动化脚本负责，不属于 `agent-pwd`。

### 为什么没有命令会打印明文

- `cred list` 与 `cred show` 返回完全没有 `Secret` 字段的 `PublicCredential` 对象；JSON 输出只包含已声明字段。
- daemon 把所有已存储密钥注册到脱敏器，将明文、URL 转义、hex、base64 与带引号形式在 SSH 输出、SQL 单元格、HTTP header/body 和子进程输出中替换为 `[REDACTED]`；`Bearer ...`、`password=...` 等模式同样会被脱敏。
- fill 值与一次性 token 只存在于 daemon 内存中；跨 CLI 边界的只有不透明 handle 和结果引用。
- 大小上限与行数限制可减少超大响应带来的意外泄漏。这些是纵深防御，不能替代审查子进程的行为。

## 7. 确认流程

高风险动作不会静默失败。CLI 打印待确认 JSON 对象并以退出码 `10` 退出：

```json
{
  "status": "pending",
  "confirmation_id": "c_0123456789abcdef",
  "poll_token": "<poll_token>",
  "expires_at": "2026-11-27T12:34:56Z",
  "reason": "rule:default-api-write",
  "summary": "apikey/github api POST /v1/...",
  "next": "tell the human the confirmation_id and poll token; then run agent-pwd confirm result <id> --token <poll_token>"
}
```

Agent 必须在此停止：不要重试、屏蔽或自动批准，应把 `confirmation_id` 与 `poll_token` 交给人类。

```bash
# 人类在另一个终端操作：Touch ID / Windows Hello，失败后回退主密码
agent-pwd confirm approve c_0123456789abcdef

# 强制走主密码路径（Linux 没有系统生物识别 API 时适用）
agent-pwd confirm approve c_0123456789abcdef --no-biometric
# 可用 --password；推荐交互式提示

# 拒绝动作
agent-pwd confirm deny c_0123456789abcdef

# 仅取一次最终结果
agent-pwd confirm result c_0123456789abcdef --token <poll_token>

# 轮询直到做出决定
agent-pwd confirm result c_0123456789abcdef --token <poll_token> --wait
```

细节：

- `confirm approve` 在平台支持时使用原生生物识别（macOS Touch ID、Windows Hello，Linux 无），然后回退到主密码证明。客户端把密码派生为 HMAC proof，密码本身永不发送给 daemon。`--no-biometric` 跳过原生验证。
- `confirm result` 对**最终**结果是一次性的：第一次取到终态结果后 poll token 即失效。状态仍为 `pending` 时反复轮询是安全的，取最终结果则不行。
- `--wait` 每 500 ms 轮询一次直到有决定（客户端上限约 6 分钟）。AI 契约还记载了在原动作上使用 `--wait` 让动作阻塞到人类决定；否则按上面的方式使用 `confirm result`。
- 待确认操作仅存在于 daemon 内存，默认 5 分钟后过期（`AGENT_PWD_CONFIRM_TTL`）。过期时审计记录 `timeout` 与 `TIMEOUT`。`lock` 会取消待确认操作；daemon 重启会取消所有待确认操作，之后 `confirm approve` 返回 `NOT_FOUND`。
- 审计记录依次标记 `started`，然后 `approved` / `denied` / `timeout`，执行完成后标记 `completed`。已批准的动作仍可能在执行阶段失败；该错误由 `confirm result` 返回并使用对应退出码。
- 因此无人值守任务按设计会在高风险动作处停下。系统不支持自动批准。

## 8. 审计与运维

### 审计日志

```bash
agent-pwd audit tail --n 50         # 最新记录，默认 50 条，不含敏感内容
agent-pwd audit verify              # 校验完整哈希链
```

`audit.log` 每行都是 JSON，字段包括 `seq`、`ts`、`credential_id`、`action`、`decision`、`confirm_source`、`outcome`、`result_code`、`detail_hash`、`policy_rule`、`client`、`prev_hash` 与 `hash`。哈希是前一条哈希与规范化记录的 SHA-256。`audit verify` 可检测格式错误的行、序号缺口、`prev_hash` 不一致、哈希不匹配及时间戳倒退；正常时返回 `ok`、`records`、`first_seq`、`last_seq`，异常时给出 `problem`。daemon 启动时校验链，若已损坏则拒绝 `unlock` 并返回 `CORRUPT`。写入采用追加、加锁与 fsync；审计写入失败会阻止动作执行，若发生在执行之后则会锁定会话。

### 健康检查与密码轮换

```bash
agent-pwd doctor                    # 权限、备份、审计链、daemon、平台加固状态
agent-pwd rotate-password           # 提示输入当前主密码与新主密码
agent-pwd rotate-password --new-salt
```

`doctor` 报告 `data_dir`、`dir_permissions`、`vault_exists`、`audit_ok`、`audit_records`、`audit_problem`、`backups`、`newest_backup`、`daemon`、`service`、`platform`、`security_note` 与 `unlocked`。轮换需要会话已解锁并校验旧密码（输错同样计入第 3 节所述退避）。不加 `--new-salt` 时只用新 KEK 重新包裹 DEK；加 `--new-salt` 时会派生全新的 DEK 并重新加密所有凭据。

### 服务、备份与 i18n

```bash
agent-pwd service install
agent-pwd service status
agent-pwd service uninstall

agent-pwd doctor                    # 确认备份轮转状态
agent-pwd cred restore --from /path/to/data-dir/vault.dat.bak.1

agent-pwd --lang zh-CN status
agent-pwd status --lang zh-CN
AGENT_PWD_LANG=zh-CN agent-pwd docs
agent-pwd docs --format json
```

语言优先级：`--lang` > `AGENT_PWD_LANG` > `en`。`--lang zh-CN` 与 `--lang=zh-CN` 等价；别名 `zh`、`zh-cn`、`zh_cn`、`zh-hans`、`zh_hans` 映射到 `zh-CN`，不支持的值回退到 `en`。系统区域设置永远不会被读取。只翻译面向人的文字：命令、标志、JSON key、枚举、决策值、退出码/错误码、ID、token 和文件路径都保持原样。`make docs` 重新生成两份契约文件；`make docs-check` 校验它们是否同步。`--json` 是默认输出模式，`--human` 切换为紧凑文本。

## 9. 故障排查

| 现象 | 可能原因 | 处理方式 |
|---|---|---|
| 退出码 `20`，错误 `NO_DAEMON`，提示 "daemon is not running" | 没有 daemon 监听 Unix socket / 命名管道 | 执行 `agent-pwd service install` 或 `agent-pwd daemon`；用 `agent-pwd service status` 检查。CLI 不会代你启动。 |
| 退出码 `20`，"vault is locked; run agent-pwd unlock" | 会话空闲（8h）或硬上限（24h）超时、调用过 `lock`，或 daemon 重启清空内存 | 执行 `agent-pwd unlock`；启动时检查 `AGENT_PWD_IDLE_TIMEOUT` / `AGENT_PWD_HARD_TIMEOUT`。 |
| "too many failed attempts; retry in Ns" | 主密码错误触发指数退避 | 等待提示的秒数；`status` 显示 `failures`。失败 5 次后 daemon 停止，请有意重启。 |
| `rotate-password` 报 "wrong master password" | 提供的当前密码错误 | 输入真正的当前密码；该失败同样计入退避。 |
| 退出码 `30` / `DENIED`，"policy denied this action (rule ...)" | 命中 `deny` 规则，或动作被策略阻止 | 如实报告拒绝；用 `agent-pwd policy list` 检查。不要尝试绕过。 |
| 退出码 `40` / `NOT_FOUND`，"credential not found" | `--cred` 值错误，或凭据被删除/覆盖 | 查看 `agent-pwd cred list`；使用当前 label 或 `cred_...` ID。 |
| unlock 报 `CORRUPT`、`audit_ok: false` | 审计哈希链损坏 | 执行 `agent-pwd audit verify` 并用 `doctor` 检查；必要时恢复已核对的备份。不要编辑 `audit.log`。 |
| "unknown, used or expired fill handle" | handle 已被消费、已过期（默认 60s），或会话/客户端改变 | 重新申请 `fill` handle，并在 TTL 内由同一客户端 apply；检查 `AGENT_PWD_FILL_TTL`。 |
| `confirm result` 报 "confirmation not found" | 待确认操作已过期（5m）、被锁定/取消，或 daemon 重启 | 在人类可决策后重新执行原动作；待确认状态仅在内存中。 |
| "invalid poll token" | token 错误，或最终结果已被取走一次 | 使用原退出码 10 响应中的 `poll_token`；最终结果只能取一次。 |
| 确认一直处于 `pending` | 尚无人批准；系统有意不支持无人值守批准 | 请人类运行 `confirm approve <id>`；需要阻塞时用 `--wait`。 |
| 一个 label 出现两条记录，或发生意外覆盖 | 旧 vault 有重复 label；现在 label 是主键 | 用期望的数据重新执行 `cred add <kind> <label>` 以去重，再用 `cred list` 核对。 |
| SSH 报 "no known_hosts at ..." | 严格主机密钥校验未找到信任库 | 把主机密钥添加到 `<data-dir>/known_hosts`；测试之外避免使用 `--insecure-host-key`。 |
| SSH/SQL/API 返回退出码 `1` 及执行器错误 | 凭据协议与动作不匹配、HTTP 方法不支持、path 非法或远端报错 | 让凭据类型匹配动作（`server`+`ssh`、`server`+`mysql|postgres|redis`、`apikey`+`api`、`envvar`+`env`）；查看 `error.message`。 |
| Linux：确认时没有生物识别弹窗 | 没有系统级生物识别 API | 使用 `confirm approve --no-biometric` 并输入主密码。 |
| macOS：服务反复重启或丢失会话 | 服务启动了会脱离终端的 daemon 形式 | 重新安装服务；它必须运行 `daemon --foreground`（LaunchAgent 已自动处理）。 |
| Windows：连不上 daemon | 命名管道 ACL 仅允许当前用户与 SYSTEM | 用启动 daemon 的同一用户运行 CLI；检查 `service status`。 |

平台说明：macOS 使用 LaunchAgent 与 Touch ID helper；Linux 使用 systemd user unit（或手动启动）和主密码确认，并通过 `PR_SET_DUMPABLE=0` 阻止 core dump，但无法阻止同用户 `ptrace`；Windows 使用计划任务、命名管道 ACL 以及可用的 Windows Hello。数据目录与路径见第 1 节；`AGENT_PWD_HOME` 可覆盖它们。

## 10. 安全边界

设计上受到保护：

- 明文密钥不会进入 CLI 输出、`cred show`、`vault.dat`、`audit.log` 或 AI 上下文；所有对外文本都经过脱敏。
- 高风险动作无法静默完成；拒绝、批准与超时均有审计。
- 复制走的 vault 若无主密码无法读取；Argon2id 提供离线保护。
- 审计被改写、删除或重排都可通过哈希链发现。
- 会话密钥在锁定、空闲超时、硬超时、daemon 停止及审计写入失败后清零。

明确不在保护范围内：

- 以**同一操作系统用户**身份运行的恶意软件：它可以读取进程内存、用 `ptrace` 附加、替换二进制，或使用本机 socket/pipe 与数据目录。
- 同一用户的其他进程直接访问 daemon socket/命名管道或数据目录；这些进程位于信任边界之内。
- 账户被攻陷、物理接触、胁迫，或人类明确授权接收注入密钥的恶意子进程。
- 已被允许运行的子进程滥用 `fill` handle。`fill` 是一种能力（capability）：保证的是 `agent-pwd` 自身不打印值，且动作可审计、可吊销（`lock`、`cred rm`）。

二次确认防的是注入误导，不是入侵：它阻止被误导的 agent 悄悄完成高风险动作，但不能阻止同一用户的恶意进程伪造确认。操作系统账户才是真正的安全下限。疑似漏洞请按 [SECURITY.zh-CN.md](../SECURITY.zh-CN.md) 中的方式私密报告。

## 11. 常用命令速查与链接

| 任务 | 命令 |
|---|---|
| 构建 / 升级 | `make build`；把 `bin/agent-pwd` 复制到 `PATH` 中 |
| 查看版本 | `agent-pwd version` |
| 安装 / 移除自启动 | `agent-pwd service install` / `agent-pwd service uninstall` / `agent-pwd service status` |
| 启动 daemon | `agent-pwd daemon --foreground` 或 `agent-pwd daemon` |
| 初始化 / 解锁 / 锁定 / 查看 | `agent-pwd init` / `agent-pwd unlock` / `agent-pwd lock` / `agent-pwd status` |
| 添加凭据 | `agent-pwd cred add <web|apikey|server|envvar> <label> [--secret JSON] [--risk normal|high] [--tags a,b] [--port N]` |
| 列表 / 查看 / 删除 / 恢复 | `agent-pwd cred list` / `agent-pwd cred show <label|id>` / `agent-pwd cred rm <label|id>` / `agent-pwd cred restore --from <backup>` |
| 策略 | `agent-pwd policy add --cred-pattern '<glob>' --action '<glob>' --decision allow|confirm|deny [--condition ...] [--id ...]`；`agent-pwd policy list`；`agent-pwd policy rm <id>` |
| SSH | `agent-pwd ssh exec --cred <label|id> -- '<command>' [--insecure-host-key]` |
| SQL / Redis | `agent-pwd sql query --cred <label|id> -- '<SQL 或命令>' [--arg v]...` |
| HTTP API | `agent-pwd api call --cred <label|id> --method GET --path /v1/x [--header k:v] [--body ...] [--allow-redirect]` |
| 环境变量注入 | `agent-pwd env run --cred <label|id> -- <command...>` |
| fill / apply | `agent-pwd fill --cred <label|id> --field password|totp|username|token`；`agent-pwd fill apply --handle <h> -- <command...>` |
| 确认 | `agent-pwd confirm approve <id> [--no-biometric]` / `agent-pwd confirm deny <id>` / `agent-pwd confirm result <id> --token <poll_token> [--wait]` |
| 审计 / 健康 | `agent-pwd audit tail [--n 50]` / `agent-pwd audit verify` / `agent-pwd doctor` |
| 修改主密码 | `agent-pwd rotate-password [--new-salt]` |
| 机器契约 / 帮助 / 语言 | `agent-pwd docs [--format md|json]`；`agent-pwd help`；`agent-pwd --lang en|zh-CN <command>` |

相关文档：

- [AI 集成契约](contract.zh-CN.md)——由 `agent-pwd docs` 生成，包含精确的命令注册表与 JSON 契约。
- [本地化指南](i18n.zh-CN.md)——支持的语言与不可翻译的机器 token。
- [agent-pwd 技能](../skills/agent-pwd/SKILL.zh-CN.md)——面向 agent 的硬性规则与用法。
- [可运行示例](../samples/README.zh-CN.md)——网页登录、SSH、SQL、API、env、fill、确认流程与运维的多组案例。
- [README](../README.zh-CN.md)——安装快速上手与项目概览。
- [安全政策](../SECURITY.zh-CN.md)——信任边界与漏洞私密报告方式。
