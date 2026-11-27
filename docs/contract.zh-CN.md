# agent-pwd — AI 集成契约

由 `agent-pwd docs` 生成；请勿手动编辑。

所有操作命令默认输出 JSON。密钥绝不会出现在输出中。


## 命令


### 操作（AI）

- `agent-pwd api call --cred <label|id> --method GET --path /v1/x` — 发起 HTTP 请求，由 daemon 注入已存储的 token
  flags: --cred --method --path --body --header --allow-redirect
- `agent-pwd env run --cred <label|id> -- <command...>` — 将 envvar 凭据注入子进程并返回脱敏后的输出
  flags: --cred
- `agent-pwd fill --cred <label|id> --field password|totp|username|token` — 签发一次性、会过期的 handle；绝不返回密钥本身
  flags: --cred --field
- `agent-pwd fill apply --handle <h> -- <command...>` — 消费 fill handle，并通过 stdin/env 将值注入子进程
  flags: --handle
- `agent-pwd sql query --cred <label|id> -- "<SQL>" [--arg v]...` — 执行 SQL（mysql/postgres）或只读 redis 命令
  flags: --cred --arg
- `agent-pwd ssh exec --cred <label|id> -- <command...>` — 使用已存储的凭据通过 SSH 执行命令，并返回脱敏后的 stdout
  flags: --cred --insecure-host-key

### 确认

- `agent-pwd confirm approve <id> [--no-biometric]` — 通过 Touch ID / Windows Hello 批准待确认操作，失败时回退到主密码
  flags: --no-biometric --password
- `agent-pwd confirm deny <id>` — 拒绝待确认操作
- `agent-pwd confirm result <id> --token <poll_token> [--wait]` — 获取已确认操作的结果（一次性最终结果）
  flags: --token --wait

### 管理（人工）

- `agent-pwd audit tail [--n 50]` — 显示最新的审计记录（不含敏感内容）
  flags: --n
- `agent-pwd audit verify` — 校验完整的审计哈希链
- `agent-pwd cred add <web|apikey|server|envvar> <label>` — 以交互方式添加凭据；敏感字段不会进入 shell 历史
  flags: --secret --risk --tags --port
- `agent-pwd cred list` — 列出凭据元数据
- `agent-pwd cred restore --from <backup>` — 经过明确的人工验证后恢复 vault 备份
  flags: --from
- `agent-pwd cred rm <label|id>` — 删除凭据
- `agent-pwd cred show <label|id>` — 仅显示元数据；没有任何路径会打印密钥
- `agent-pwd docs [--format md|json]` — 打印由命令注册表生成的 AI 集成契约
  flags: --format
- `agent-pwd doctor` — 检查权限、备份、审计链、daemon 和平台加固
- `agent-pwd init` — 初始化 vault 并设置主密码
  flags: --password --new-password
- `agent-pwd lock` — 清零 session 密钥，并使 fill handle 和待确认操作失效
- `agent-pwd policy add --cred-pattern '<glob>' --action '<glob>' --decision allow|confirm|deny` — 在最前面插入策略规则（新规则优先级最高）
  flags: --cred-pattern --action --decision --condition --id
- `agent-pwd policy list` — 按优先级列出规则
- `agent-pwd policy rm <id>` — 按 id 删除规则
- `agent-pwd rotate-password [--new-salt]` — 修改主密码（除非指定 --new-salt，否则只重新封装 DEK）
  flags: --new-salt --password --new-password
- `agent-pwd service install` — 安装当前用户的 LaunchAgent / systemd unit / 计划任务
- `agent-pwd service status` — 报告自启动状态
- `agent-pwd service uninstall` — 删除当前用户的自启动项
- `agent-pwd status` — 显示 daemon、session 和 KDF 状态（不含密钥）
- `agent-pwd unlock` — 解锁正在运行的 daemon 中的 session
  flags: --password
- `agent-pwd version` — 打印 agent-pwd 的构建版本
## AI 集成契约

agent-pwd 是凭据代理：你可以使用（USE）凭据，但不能读取（READ）凭据。
切勿尝试打印、转存、猜测或外传明文密钥。不存在 `cred show --secret`，没有 HTTP API、MCP 服务，也没有任何命令会返回已存储的值。

### 你可以做什么
- 执行 SSH 命令、SQL 查询、API 调用，以及注入了环境变量的子进程。
- 为受信任的子进程申请一次性 `fill` handle；该 handle 对人工读取者毫无用处。
- 当高风险操作需要确认时，请求确认。
- `--cred` 使用凭据 label；ID 仅为兼容保留，且 label 优先匹配。

### 你不能做什么
- `cred show` 只返回元数据；读取 vault.dat 或 audit.log 永远得不到明文。
- 探测 daemon 内存或 IPC socket 以寻找密钥不属于 CLI 契约，也不会有帮助。
- 你不得绕过、屏蔽或自动批准确认请求。如果某个操作以退出码 10 结束，请停止（STOP），
  将 confirmation_id 和 poll token 交给人类，在对方批准之前不要重试。

### JSON 输出结构
CLI 直接打印结果对象，例如 {"status": "unlocked", ...}。失败时打印
{"ok": false, "error": {"code": 20, "message": "..."}}。在 daemon IPC 线路上，信封格式为
{"ok": true, "result": {...}}，但调用方永远看不到这个信封。
`--human` 切换为紧凑文本；默认输出 JSON。未声明的字段绝不会输出。

### 退出码
0 成功；10 等待确认（返回 confirmation_id）；20 vault 已锁定或 daemon 未运行；
30 策略拒绝；40 未找到凭据；1 其他错误（见 error.code）。

### 待确认流程
1. 操作命令以 10 退出，并打印 confirmation_id + poll_token。不要重试。
2. 由人类运行 `agent-pwd confirm approve <id>`（Touch ID / Windows Hello，或回退到主密码）。
3. 运行一次 `agent-pwd confirm result <id> --token <poll_token>` 获取最终结果。
   如果希望原命令一直阻塞到人类做出决定，请在原操作上使用 --wait。
待确认操作只存在于 daemon 内存中：daemon 重启会取消这些操作。

### 安全注意事项
- 你可能会被允许执行的命令产生副作用（例如 SQL 写入或 HTTP POST）；
  策略层会要求确认，但无法判断意图。切勿把这些工具当作执行无关破坏性操作的许可。
- 所有操作都会追加到哈希链审计日志中。凭据可以立即删除或轮换。

