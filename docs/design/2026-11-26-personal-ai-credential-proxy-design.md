# 个人凭据代理（Personal AI Credential Proxy）设计方案

日期：2026-11-26
状态：待审阅

## 1. 背景与目标

构建一个**个人自用**的凭据管理工具。它参考 1Password 的加密存储模型，但定位完全不同：核心消费者不是人，而是**AI 代理**。

关键差异：传统密码管理器的第一需求是"人方便地读出密码"，本项目的第一需求是"**AI 能使用凭据完成任务，但永远读不到凭据本身**"。

### 目标

- 安全存储四类凭据：网站账号密码、API Key/Token、服务器与中间件连接信息、本地程序的配置密钥
- 向 AI 暴露**动作型接口**：AI 请求"做某事"，系统执行并返回结果，明文凭据不进入 AI 上下文
- 按风险分级放行：低风险自动执行，高风险强制人工确认，未知默认拒绝
- 全量审计：所有动作可追溯，凭据可即时吊销
- 跨平台：macOS、Windows、Linux，单一 Go 二进制

### 非目标（明确不做）

- 不做 MCP Server，不做本地 HTTP API，**AI 的唯一接入方式是 CLI**
- 不做 GUI（第一阶段）；后续如需，另立设计
- 不做跨设备同步、团队共享、浏览器扩展自动填充
- 不实现浏览器生命周期管理（打开页面、导航、定位元素由既有的浏览器自动化工具负责）

## 2. 威胁模型

| 资产 | 威胁 | 对策 |
|---|---|---|
| 明文凭据 | AI 上下文被注入（访问恶意网页/恶意文件中的指令） | 明文永不出现在 AI 可见的输出中；所有命令输出默认脱敏 |
| 明文凭据 | AI 被诱导执行越权动作（改密码、转账、删除） | 分级策略 + 高风险二次确认 + 未知默认拒绝 |
| Vault 文件 | 磁盘被拷贝、备份泄漏、误上传 | 全库加密，密钥不落盘；仅泄露"存在哪些凭据"的元数据 |
| 会话密钥 | 同机其他用户进程读取内存或调用 CLI | socket/临时目录 0700 权限 + 会话超时锁定 |
| 审计日志 | 事后篡改掩盖行为 | 哈希链（每条记录含前一条的 SHA-256） |

**明确不防御**：以当前用户身份运行的恶意软件（可读取进程内存、替换二进制），以及能直接读写 socket 的同用户进程。这是本机信任模型的边界，必须在使用说明中告知。

因此二次确认的作用是**防误导，而非防入侵**：它保证"AI 被注入后无法静默完成高风险操作"，但不保证"恶意进程无法伪造确认"。安全的真正下限是操作系统账户本身。

## 3. 架构

单一 Go 二进制，多子命令形态：

```
agent-pwd              // CLI 客户端（默认行为）
agent-pwd daemon       // 后台常驻进程，持有会话密钥（内部模式）
```

### 进程模型

```
AI (Claude Code 等)
   │  exec: agent-pwd ssh exec --cred prod-db -- psql -c "..."
   ▼
agent-pwd (CLI 客户端进程)
   │  Unix domain socket (macOS / Linux) / Named pipe (Windows)，仅当前用户可访问
   ▼
agent-pwd daemon (常驻)
   ├── session    : 已解锁状态下持有 DEK，超时自动清空
   ├── policy     : 规则解析，产出 allow / confirm / deny
   ├── executor   : 按凭据类型分派执行
   └── audit      : 写入哈希链日志
```

不使用 HTTP 端口，避免任何网络暴露面。

### 平台适配层

三平台共享全部业务逻辑，只有以下面需要按平台实现（`internal/platform/{darwin,windows,linux}`，每个面一个窄接口）：

| 关注点 | macOS | Windows | Linux |
|---|---|---|---|
| 进程间通信 | Unix socket | Named pipe（`\\.\pipe\...`） | Unix socket |
| 存储目录 | `~/Library/Application Support/agent-pwd/` | `%APPDATA%\agent-pwd\` | `$XDG_DATA_HOME/agent-pwd/`（默认 `~/.local/share/agent-pwd/`） |
| 二次确认 | LocalAuthentication（Touch ID） | Windows Hello | **无系统级生物识别抽象**，降级为终端重输主密码 |
| 常驻方式 | LaunchAgent plist | 计划任务 / 服务 | systemd user unit |
| 内存保护 | `mlock`（尽力） | `VirtualLock`（尽力） | `mlockall` + `PR_SET_DUMPABLE=0`（禁止 core dump） |

**Linux 的两点特殊处理**：

1. Linux 上默认允许同用户进程 `ptrace` daemon，等于会话密钥可被同用户进程直接读取。daemon 启动时调用 `prctl(PR_SET_DUMPABLE, 0)` 至少阻断 core dump，并在使用说明中明示"Linux 上没有生物识别二次确认，安全下限弱于 macOS/Windows"。
2. 优先使用 systemd user unit（`systemctl --user enable --now agent-pwd`），无 systemd 的环境（如某些容器）退化为手动后台启动。

socket 文件由 daemon 创建时设置 0700 目录 + 仅属主可读写。

### 模块边界（每个模块可独立测试）

| 模块 | 职责 | 依赖 |
|---|---|---|
| `store` | Vault 文件的读写、原子落盘、备份轮转 | 无 |
| `crypto` | Argon2id 派生、DEK 包裹、AES-256-GCM 加解密、AAD 绑定 | 无 |
| `session` | 解锁状态生命周期、超时、失败计数 | crypto |
| `policy` | 规则匹配与决策（allow/confirm/deny） | store（只读） |
| `executor` | 四类凭据的动作执行 | session、policy |
| `audit` | 哈希链追加、校验 | 无 |
| `api` | CLI 参数解析、请求/响应编解码、退出码映射 | 全部 |

## 4. 数据模型

### 4.1 Vault 文件

位置：

- macOS：`~/Library/Application Support/agent-pwd/vault.dat`
- Windows：`%APPDATA%\agent-pwd\vault.dat`
- Linux：`$XDG_DATA_HOME/agent-pwd/vault.dat`（默认 `~/.local/share/agent-pwd/vault.dat`）

```
[ header ]
  magic            "PVAULT\0\0"
  version          uint8
  kdf              argon2id, time/memory/threads 参数
  salt             32B
  wrapped_dek      由主密码派生的 KEK 加密的 DEK（AES-256-GCM）
  dek_nonce        12B

[ body ]  AES-256-GCM(DEK)，AAD = header
  credentials[]    凭据条目（含明文 secret，加密后）
  policies[]       策略规则
  updated_at
```

采用**单文件 + 字段级整体加密**，而非 SQLCipher：

- 避免 CGO 依赖与 SQLCipher 的商用许可约束，跨平台编译只需 `CGO_ENABLED=0`
- 个人凭据规模（数百条以内）无需数据库索引
- 写入用"临时文件 + fsync + rename"保证原子性，保留最近 N 份备份

主密码轮换：重新派生 KEK，仅重新包裹 DEK，不必重加密全部凭据。

### 4.2 凭据条目

```
id, kind, label, risk_level, tags[], created_at, updated_at
secret: {}            // 按 kind 区分结构，整体位于加密体内
```

| kind | secret 结构 | 可执行的 action |
|---|---|---|
| `web` | username, password, totp_seed?, url? | `fill`（返回一次性注入句柄） |
| `apikey` | base_url, header_name, token | `api`（代理发起 HTTP 请求） |
| `server` | protocol(ssh/mysql/redis), host, port, username, password/key, database? | `ssh`, `sql` |
| `envvar` | key, value | `export`（写入子进程环境变量） |

`envvar` 是唯一存在"明文进入 AI 可见输出"风险的类别（AI 可能要求打印环境变量）。设计上只允许通过 `agent-pwd env run --cred X -- cmd` 注入到子进程，**不提供"读取值"的子命令**。

### 4.3 审计记录（独立 append-only 文件）

```
{ seq, ts, credential_id, action, decision, confirm_source, result, client, prev_hash, hash }
```

`hash = SHA-256(前一条 hash || 本条规范化 JSON)`，`audit verify` 可检测断裂与删除。

`client` 记录调用方自报的标识（进程名 + 工作目录），仅用于区分"人手动执行"与"AI 调用"，不作为安全凭据——自报字段可伪造，不参与任何策略决策。

## 5. CLI 接口

### 5.1 管理命令（人使用）

| 命令 | 说明 |
|---|---|
| `agent-pwd init` | 初始化 vault，设置主密码 |
| `agent-pwd unlock` / `lock` / `status` | 会话管理 |
| `agent-pwd cred add <kind> <label>` | 交互式录入（从 stdin/tty 读，避免进 shell 历史） |
| `agent-pwd cred list` / `show <id>` | `show` 只显示元数据，永不显示 secret |
| `agent-pwd cred rm <id>` | 删除 |
| `agent-pwd policy add/list/rm` | 策略规则维护 |
| `agent-pwd audit tail` / `verify` | 审计查看与完整性校验 |
| `agent-pwd doctor` | 自检：权限、备份、daemon 存活 |
| `agent-pwd docs` | 输出"AI 接入契约"（见 6） |

### 5.2 动作命令（AI 使用）

| 命令 | 说明 |
|---|---|
| `agent-pwd ssh exec --cred <id> -- <cmd...>` | 建 SSH 连接执行命令，返回 stdout |
| `agent-pwd sql query --cred <id> -- "<SQL>"` | 建连接执行查询，返回结果集 |
| `agent-pwd api call --cred <id> --method GET --path /v1/x` | 代持 token 发请求，返回响应体 |
| `agent-pwd fill --cred <id> --field password` | 返回一次性注入句柄（非明文） |
| `agent-pwd env run --cred <id> -- <cmd...>` | 注入环境变量运行子进程 |
| `agent-pwd confirm approve <id>` | 批准挂起的待确认动作 |

所有动作命令默认输出 JSON（`--json` 为默认，`--human` 切换为人类可读），便于 AI 稳定解析。

### 5.3 退出码（机器可判定的契约）

| 码 | 含义 |
|---|---|
| 0 | 成功 |
| 10 | 待人工确认（返回 confirmation_id） |
| 20 | Vault 未解锁 |
| 30 | 策略拒绝 |
| 40 | 凭据不存在 |
| 1 | 其他错误 |

## 6. AI 接入契约

由于不使用 MCP，AI 无法自动发现能力，因此需要一个**稳定的、可自描述的文档**：

- `agent-pwd docs` 从命令定义自动生成（单一事实来源，避免文档漂移）
- 内容包含：能做什么、不能做什么（"你无法读取密码"）、命令语法、JSON 输出 schema、退出码语义、待确认流程怎么处理
- 建议用户把该文档内容嵌入其 Claude Code 的 `CLAUDE.md`／技能说明中

**关键约束需在契约中显式声明**：AI 不得尝试 `cred show`／直接读取 vault 文件／探测 daemon 内存；这些命令要么不存在，要么直接拒绝。

## 7. 策略引擎

规则形如 `(credential_pattern, action_pattern) → decision`，按优先级顺序匹配，命中即停。

默认策略（`init` 时写入）：

| 匹配 | 决策 |
|---|---|
| `server:*` + `ssh`/`sql` 且 SQL 为只读白名单（SELECT/SHOW/DESCRIBE） | allow |
| `apikey:*` + `api` 且 method ∈ GET/HEAD | allow |
| 任何凭据 + `fill` / `env run` | allow |
| 任何凭据 + 写操作（INSERT/UPDATE/DELETE/DROP、POST/PUT/PATCH） | confirm |
| 未匹配任何规则 | **confirm** |

"未匹配默认 confirm 而非 deny"是有意的：deny 会让新凭据完全不可用，并逼迫用户放宽规则；confirm 保留了安全下限，同时不挡路。规则按顺序匹配、命中即停，因此**规则顺序即优先级**。默认规则表由 `init` 写入，用户增删时新规则插入到默认规则之前。

风险分级还支持按凭据手工标记 `risk_level: high`，此时该凭据的**所有**动作一律 confirm。

## 8. 失败模式与错误处理

| 场景 | 行为 |
|---|---|
| daemon 未运行 | CLI 直接以 20 退出，并提示启动命令；不自动拉起（避免隐式启动持有密钥的进程） |
| 会话已超时锁定 | 动作命令以 20 退出；不需要在超时后泄露或缓存凭据 |
| vault 文件损坏 | 从最近备份恢复候选，报告差异，需用户确认后替换 |
| 审计写入失败 | **拒绝执行该动作**（审计不可用即不可信） |
| 主密码错误 | 指数退避（1s/2s/4s…上限 60s），连续 5 次后强制 daemon 退出 |
| 待确认超时（默认 5 分钟） | 动作自动作废，审计记 `timeout` |
| `fill` 句柄过期（默认 60 秒） | 句柄失效，需重新请求 |

## 9. 二次确认的实现

高风险动作返回退出码 10 与 `confirmation_id`，同时：

1. **交互场景**：daemon 通过平台原生方式弹出确认
   - macOS：LocalAuthentication（Touch ID），失败降级为终端重输主密码
   - Windows：Windows Hello（`Windows.Security.Credentials.UI`），降级同上
   - Linux：无系统级生物识别统一接口，**直接走终端重输主密码**
2. **无人值守场景**：确认请求会**挂起等待**，可由另一终端执行 `agent-pwd confirm approve <id>` 完成

这是 B+C 解锁模型的直接结果：**无人值守任务遇到高风险动作一定会停住**。这是有意的安全边界，需在契约与使用说明中明确告知使用者与 AI，避免 AI 反复重试。

## 10. 测试策略

| 层次 | 内容 |
|---|---|
| 单元 | crypto 往返与篡改检测（改密文 1 bit 必须失败）；KDF 参数固化用例；policy 决策表驱动测试覆盖全部规则组合；audit 链断裂检测 |
| 集成 | 起真实 daemon，跑完整 CLI 流程：init → add → unlock → 动作 → 审计校验 |
| 假外部服务 | 本地假 HTTP 服务验证 `api call`；本地 sshd（或 mock）验证 `ssh exec` |
| 跨平台 | macOS / Windows / Linux 三平台 CI；Windows 单独验证 named pipe 权限与 Windows Hello 降级路径；Linux 单独验证 socket 权限、`PR_SET_DUMPABLE` 生效、systemd unit 启停 |
| 安全回归 | 断言所有命令输出不含 secret 字段；断言 `cred show` 不存在 secret 输出路径 |

## 11. 实施阶段

| 阶段 | 范围 |
|---|---|
| P1 | crypto + store + session + CLI 骨架（init/unlock/lock/status/cred/policy/audit）+ `server` 类的 `ssh exec` |
| P2 | `sql query`、`api call`、`env run`、`fill` 句柄 |
| P3 | 二次确认（原生弹窗 + approve 流程）、`agent-pwd docs` 契约生成 |
| P4 | Windows 与 Linux 支持、CI 三平台、打包与 `doctor` 完善 |

四类凭据不按类别切阶段，而是按"框架能力 → 执行器"切分，避免框架未稳就铺开执行器。

## 12. 已知取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 不做 MCP，只做 CLI | 实现与调试成本最低，无额外协议层 | AI 无法自动发现能力，依赖文档；每次调用有进程启动开销 |
| 不做本地 HTTP API | 消除网络面与鉴权面 | 只能本机调用；无法与其他语言的库直接集成 |
| 不自动拉起 daemon | 避免隐式创建持有密钥的进程 | 使用者需自行保证 daemon 运行 |
| 单文件加密存储而非数据库 | 无 CGO、无许可约束、便于备份 | 规模上限约数千条；并发写入需加锁 |
| 高风险动作必停 | 安全下限 | 无人值守流程会被阻塞 |
| Linux 无生物识别二次确认 | Linux 缺乏统一的系统级生物识别接口 | Linux 上的高风险确认退化为重输主密码，安全体验低于 macOS/Windows |
| 跨三平台而非只做 macOS | 使用者明确需要 | 平台适配层与三平台 CI 带来额外维护成本 |