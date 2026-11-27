# 支持与帮助

语言：[English](SUPPORT.md) · [简体中文](SUPPORT.zh-CN.md)

感谢使用 agent-pwd。本项目按尽力而为的方式维护，请先搜索已有讨论再发新帖，并尽量让问题聚焦、可复现。

## 在哪里提问

- **使用问题、帮助与想法：** [GitHub Discussions](https://github.com/tyr-zhang/agent-password/discussions)。
- **缺陷报告与具体的功能请求：** [GitHub Issues](https://github.com/tyr-zhang/agent-password/issues)。请先查重。
- **疑似漏洞：** 不要使用 Discussions 或公开 Issues。按照 [SECURITY.md](SECURITY.md) 通过 GitHub Security Advisories 私下报告。
- **行为准则相关问题：** 使用 [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) 中描述的私密联系方式。

## 报告时应提供的信息

提交缺陷报告时请提供：

1. `agent-pwd version` 的输出（版本、提交号、构建时间），以及二进制的构建或安装方式。
2. 操作系统及版本、架构，以及 daemon 是手动启动还是通过 `service install` 启动。
3. 实际执行的命令，credential ID 和其他具体值请替换为占位符。**切勿粘贴主密码、秘密、token、`vault.dat`、备份或原始审计日志。**
4. 退出码和 CLI 输出（默认是 JSON；可用 `--human` 获得紧凑文本）。分享前请脱敏。
5. 预期行为与实际行为，以及复现步骤。
6. 相关的 `agent-pwd doctor` 输出；如有需要，可提供 `agent-pwd status` 输出。不愿公开的内容请省略。
7. 当时 vault 是否锁定、会话是否已超时，或是否有动作在等待确认（退出码 10，带 `confirmation_id`）。

使用类问题请说明你想完成什么（例如通过 `agent-pwd docs` 接入 AI 代理、配置二次确认、设置 `AGENT_PWD_HOME` 与会话超时），并只包含非敏感信息。

## 响应预期

Issues 和 Discussions 按尽力而为方式回复，不保证响应时间。请给回复留出几天时间；有新诊断信息时请补充到原贴，而不是重复开帖。安全报告按 [SECURITY.md](SECURITY.md) 中所述目标单独处理。
