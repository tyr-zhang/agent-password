# Support

Language: [English](SUPPORT.md) · [简体中文](SUPPORT.zh-CN.md)

Thanks for using agent-pwd. This project is maintained on a best-effort basis, so please search existing threads before opening a new one and keep questions focused and reproducible.

## Where to ask

- **Questions, usage help, and ideas:** [GitHub Discussions](https://github.com/tyr-zhang/agent-password/discussions).
- **Bug reports and concrete feature requests:** [GitHub Issues](https://github.com/tyr-zhang/agent-password/issues). Please check for duplicates first.
- **Suspected vulnerabilities:** do **not** use Discussions or public Issues. Follow [SECURITY.md](SECURITY.md) and report privately through GitHub Security Advisories.
- **Code of Conduct concerns:** use the private contact described in [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## What to include

For a bug report, please provide:

1. `agent-pwd version` output (version, commit, build date) and how you built or installed the binary.
2. Operating system and version, architecture, and whether you started the daemon manually or via `service install`.
3. The exact command you ran, with credential IDs and other values replaced by placeholders. **Never paste master passwords, secrets, tokens, `vault.dat`, backups, or raw audit logs.**
4. The exit code and the CLI output (`JSON` is the default; you can use `--human` for compact text). Redact anything sensitive before sharing.
5. What you expected to happen and what happened instead, plus steps to reproduce.
6. Relevant output from `agent-pwd doctor` and, if useful, `agent-pwd status`. Omit anything you do not want to share.
7. Whether the vault was locked, the session had timed out, or an action was pending confirmation (exit code 10 with `confirmation_id`).

For usage questions, describe what you are trying to do (for example, wiring an AI agent through `agent-pwd docs`, setting up confirmation, or configuring `AGENT_PWD_HOME` and session timeouts) and include only non-sensitive details.

## Response expectations

Issues and discussions are answered on a best-effort basis; there is no guaranteed response time. Please allow a few days for a reply, and add any new diagnostic information to the existing thread rather than opening duplicates. Security reports are handled separately with the targets described in [SECURITY.md](SECURITY.md).
