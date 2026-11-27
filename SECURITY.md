# Security Policy

Language: [English](SECURITY.md) · [简体中文](SECURITY.zh-CN.md)

agent-pwd is a single-user, local credential proxy. It is designed to let an AI agent use credentials without ever seeing their plaintext. Please read the trust boundary below before deciding whether a finding is a vulnerability in scope.

## Supported versions

Only the **latest release** is supported. Older releases do not receive security fixes or backports; upgrade to the latest release before reporting an issue. Builds from `main` are development snapshots and are not a supported release.

## Reporting a vulnerability

**Do not open a public issue for a suspected vulnerability.** Public reports can expose users before a fix exists.

Report privately through GitHub Security Advisories:

<https://github.com/tyr-zhang/agent-password/security/advisories/new>

Please include:

- the affected version or commit and the operating system/architecture;
- a description of the impact and the trust boundary you believe is crossed;
- minimal reproduction steps, with commands shown using placeholders;
- any relevant redacted CLI output or exit/error codes.

Do **not** include master passwords, real credentials, `vault.dat`, backups, audit logs, or unredacted tokens in a report. Keep the discussion private until a fix or mitigation is coordinated.

## Response expectations

We aim to acknowledge a private report within three business days and provide an update at least every five business days while it is being investigated. Timing depends on severity and reproducibility; these are targets, not a guarantee. For accepted reports we coordinate a fix, a release, and disclosure with the reporter.

Questions that are not vulnerabilities are welcome in [GitHub Discussions](https://github.com/tyr-zhang/agent-password/discussions) or [Issues](https://github.com/tyr-zhang/agent-password/issues); see [SUPPORT.md](SUPPORT.md).

## Trust boundary and out-of-scope threats

**In scope** are failures of the guarantees the project actually makes: plaintext credentials leaking into AI-visible CLI output, silent execution of high-risk actions without human confirmation, reading the vault file's encrypted contents without the master password, tampering with the audit hash chain going undetected, or continued use of session keys after timeout.

**Explicitly out of scope:**

- Malware running as the **same operating-system user**. Such code can read process memory, attach with `ptrace`, replace the binary, or read the user's files. On Linux, `PR_SET_DUMPABLE=0` blocks core dumps but does not stop same-user `ptrace`.
- Any same-user process with direct access to the daemon's **Unix socket or Windows named pipe**, or to the data directory. The socket/pipe and directory are restricted to the owner, but the owner's other processes are trusted.
- Compromise of the operating-system account itself, physical access, coercion, or a malicious child process that a human deliberately authorized to receive injected secrets.
- The `fill` handle being misused by a child process that was already allowed to run: the handle is a capability, and agent-pwd guarantees only that it does not print plaintext itself and that the action is auditable and revocable.

**Second confirmation is anti-injection, not anti-intrusion.** It exists so an injected AI cannot silently complete a high-risk action; it does not prevent a hostile same-user process from forging confirmation or reading memory. The real security floor is the operating-system account boundary. Unattended jobs therefore stop at high-risk actions by design, and automatic approval is deliberately not supported.
