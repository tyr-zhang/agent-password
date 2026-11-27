# ops samples

This scenario contains operator commands rather than credential actions: a health check, a vault backup restore, and master-password rotation. These are human-facing commands; no credential label is passed to the scripts.

See [简体中文](README.zh-CN.md).

## Prerequisites

- A running daemon with an unlocked session is the general baseline (`agent-pwd status`, `agent-pwd unlock`). `status`, `doctor`, `audit verify`, and `cred restore` are public operations and do not require an unlocked session; `agent-pwd rotate-password` does.
- `restore_backup.sh` needs a backup file named `vault.dat.bak.N` in the data directory (for example `~/Library/Application Support/agent-pwd/` on macOS) and an explicit human verification before the restore.
- `rotate.sh` requires interactive input of the current and new master passwords; the daemon verifies the old password and applies backoff on wrong attempts.
- All `agent-pwd` commands emit JSON by default. Exit codes: `0` success, `10` pending confirmation, `20` locked/daemon absent, `30` denied, `40` credential not found; shell-level file checks can also exit `1`.

## Scripts

| File | Usage | Expected behavior |
|---|---|---|
| `health.sh` | `health.sh` | Runs `agent-pwd status`, `agent-pwd doctor`, and `agent-pwd audit verify` in that order; `set -e` stops at the first failing command. |
| `restore_backup.sh` | `restore_backup.sh <vault.dat.bak.N>` | Checks that the file exists (otherwise prints `backup not found: ...` and exits `1`), then runs `agent-pwd cred restore --from "$backup"` and `agent-pwd audit verify`. |
| `rotate.sh` | `rotate.sh` | Runs `agent-pwd lock`, `agent-pwd rotate-password` (interactive prompts for old and new master passwords), and `agent-pwd audit verify`. As written, `lock` clears the session keys, so `rotate-password` exits `20` and `set -e` skips `audit verify`. |

## Examples

```bash
./samples/ops/health.sh
./samples/ops/restore_backup.sh '<vault.dat.bak.N>'
./samples/ops/rotate.sh
```

## Safety and notes

- No script contains a plaintext secret and no script prints one; `status`, `doctor`, and `audit verify` never emit secret material. Never paste master passwords or secrets into these scripts.
- `lock` invalidates fill handles and pending actions: `fill` handles are one-shot and expire after 60 seconds, and confirmation is human-only. On exit `10`, stop and ask a human rather than auto-approving.
- These scripts do not pass `--cred`; in action commands, `--cred` accepts a label or ID and labels are matched first (the label wins).
- For a working rotation, run `agent-pwd rotate-password` directly in an unlocked session; `rotate.sh` demonstrates the sequence but stops at `20` after its own `lock` step.
- `restore_backup.sh` overwrites the vault; verify the backup by hand and keep `audit verify` passing afterwards.
