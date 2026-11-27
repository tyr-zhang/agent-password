# 参与 agent-pwd 开发

感谢你帮助改进 agent-pwd。这是一个个人自用的凭据代理：其核心契约是 AI 代理可以*使用*凭据，但永远读不到明文。请在每一次改动中都守住这条边界。

语言：[English](CONTRIBUTING.md) · [简体中文](CONTRIBUTING.zh-CN.md)

## 开发环境

- **Go 1.25.4**（`go.mod` 中固定的版本；CI 通过 `go-version-file: go.mod` 安装 Go）。
- 需要 `git` 和 `make`。重新构建内置的 macOS Touch ID 辅助程序还需要 `swiftc`（`make helper`）；Windows Hello 在运行时通过 PowerShell/WinRT 调用。
- 克隆仓库后构建：

  ```bash
  make build                 # 生成 bin/agent-pwd，CGO_ENABLED=0
  bin/agent-pwd version      # 查看版本、提交号与构建时间
  ```

- Makefile 会导出 `CGO_ENABLED=0`，因此常规构建与测试不依赖 CGO。`make race` 会显式设置 `CGO_ENABLED=1`，因为竞态检测器需要它。
- daemon 不会被隐式拉起。手动运行使用 `bin/agent-pwd daemon`（或 `bin/agent-pwd daemon --foreground`）；常驻服务使用 `bin/agent-pwd service install`。
- `AGENT_PWD_HOME` 可覆盖数据目录，测试也会使用它。开发时请指向临时目录，切勿对真实 vault 运行测试。
- 平台相关行为（Unix socket 与 named pipe、launchd/systemd/Windows 服务、Touch ID/Windows Hello、`PR_SET_DUMPABLE`）只能在对应平台的真实机器上验证；交叉编译只证明代码能构建，不证明运行时行为正确。

## 常用检查

| 命令 | 作用 |
|---|---|
| `make build` | 构建带版本信息的 `bin/agent-pwd`。 |
| `make test` | 运行全部单元测试，以及真实 daemon 的集成测试和安全回归测试。 |
| `make race` | 运行竞态检测（`CGO_ENABLED=1`）。 |
| `make vet` | 运行 `go vet ./...`。 |
| `make cross` | 交叉编译 darwin/arm64、darwin/amd64、linux/amd64 和 windows/amd64。 |
| `make docs` | 从命令注册表重新生成 `docs/contract.md` 与 `docs/contract.zh-CN.md`。 |
| `make docs-check` | 若生成的契约文件不同步则失败。 |
| `make i18n-check` | 运行 i18n 与 CLI 测试包（`go test ./internal/i18n ./internal/cli`）。 |
| `make helper` | 重新构建内嵌的 macOS Touch ID 辅助程序（仅 macOS，需要 Swift）。 |
| `make clean` | 删除 `bin/`。 |

CI 在 macOS、Linux 和 Windows 上运行 `go test ./...`、`go vet ./...`、交叉编译与文档同步检查。提交 PR 前请让本地检查全部通过，并说明哪些平台无法在本地验证。

## 提交与 Pull Request 约定

- 倾向于小而聚焦的提交，提交标题使用英文祈使句。欢迎使用 Conventional Commits 前缀（`feat:`、`fix:`、`docs:`、`refactor:`、`test:`、`chore:`），但不强制。原因不明显时请在正文说明*为什么*。
- 不要提交密钥、vault 文件、审计日志、本地数据目录、构建产物，或包含真实凭据的生成物。粘贴示例输出到提交信息或 PR 描述前请先脱敏。
- PR 应说明问题、行为变化、测试方式（含平台）以及后续工作。有对应 issue 时请链接。
- 保持 PR 可审阅；在可行的情况下把行为改动与格式化或纯翻译改动分开。
- 所有 CI 检查必须通过，生成文件必须最新。存在未解决的评审意见，或已知破坏了“输出不含明文”与“审计失败即拒绝”的保证时，不要合并。

## 重新生成 AI 契约文档

`agent-pwd docs` 由 `internal/cli` 中的命令注册表和翻译目录生成；契约文本位于 `internal/cli/docs.go`。请勿手工编辑 `docs/contract.md` 或 `docs/contract.zh-CN.md`。

1. 按需修改命令注册、摘要、标志、退出码映射或契约文本。
2. 运行 `make docs`（会先构建二进制，再写入两种语言的文件）。
3. 运行 `make docs-check`，并将两个生成文件与源码改动一起提交。
4. 如果改动涉及已翻译的人可读文本，请按下文更新 `internal/i18n/catalog.go`。

## 贡献翻译

英文是规范源语言。`internal/i18n/catalog.go` 中的每个键都是传给 `i18n.T` 的精确英文字符串，值是简体中文译文。

1. 在调用处添加或修改英文字符串，并以该规范英文原文作为键。
2. 在 [`internal/i18n/catalog.go`](internal/i18n/catalog.go) 的 `catalogZHCN` 中加入完全相同的键及其中文译文。修改英文措辞会产生新键，因此必须同步更新目录。
3. 精确保留占位符、反引号内的命令/标志/字面值以及机器 token；参见 [`docs/i18n.md`](docs/i18n.md)。
4. 运行 `make i18n-check`，以及 `make docs` / `make docs-check`。新增键或新增翻译命令时，请补充或更新测试。
5. 修改面向贡献者的文档时，保持成对文档（`*.md` / `*.zh-CN.md`）同步。

## 安全敏感改动

涉及 crypto、store、session、policy、audit、executor、确认流程、IPC 或 CLI 输出的改动都属于安全敏感改动，必须满足以下要求：

- **输出不含明文。** 密钥不得出现在 stdout、stderr、JSON 或人类可读输出、错误信息、`--human` 输出或审计详情中。`cred show` 只能显示元数据，`fill` 只返回一次性不透明句柄。每新增一条输出路径，都要添加回归测试断言秘密值没有泄漏。
- **审计失败即关闭（fail-closed）。** 动作必须先追加到带哈希链的审计日志再执行。审计写入或校验失败时必须拒绝动作；不得添加绕过、静默降级或 best-effort 审计路径，也不得削弱 `audit verify` 的链校验。
- **确认必须由人掌控。** 高风险动作返回退出码 10 以及 `confirmation_id` 和 poll token，并等待批准。不要加入自动批准，不要在代码中重试待确认动作，待确认状态只能保存在 daemon 内存中。
- **机器 token 保持英文。** JSON 键、枚举值、退出码、错误码、命令与标志名、审计字段以及面向机器的诊断信息是稳定契约，不得翻译或改名。翻译只适用于人可读的叙述文本。
- **尊重信任边界。** 同用户恶意软件（包括 `ptrace` 和替换二进制）以及可直接访问 socket/named pipe 的同用户进程明确不在防御范围内。不要声称某个改动能防御它们；参见 [SECURITY.md](SECURITY.md)。
- 发现漏洞请按 [SECURITY.md](SECURITY.md) 私下报告，绝不要提交公开 issue。
