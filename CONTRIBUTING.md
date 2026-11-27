# Contributing to agent-pwd

Thanks for helping improve agent-pwd. This CLI is a credential proxy for personal use: its contract is that an AI agent can *use* credentials but can never read their plaintext. Please keep that boundary in mind for every change.

Language: [English](CONTRIBUTING.md) · [简体中文](CONTRIBUTING.zh-CN.md)

## Development setup

- **Go 1.25.4** (the version pinned in `go.mod`; CI installs Go from `go.mod`).
- `git` and `make`. Rebuilding the bundled macOS Touch ID helper additionally requires `swiftc` (`make helper`); Windows Hello is invoked through PowerShell/WinRT at runtime.
- Clone the repository and build:

  ```bash
  make build                 # bin/agent-pwd, with CGO_ENABLED=0
  bin/agent-pwd version      # version, commit, build date
  ```

- The Makefile exports `CGO_ENABLED=0`, so normal builds and tests stay CGO-free. `make race` explicitly sets `CGO_ENABLED=1` because the race detector needs it.
- The daemon is never started implicitly. For manual runs use `bin/agent-pwd daemon` (or `bin/agent-pwd daemon --foreground`); for a persistent service use `bin/agent-pwd service install`.
- `AGENT_PWD_HOME` overrides the data directory and is used by tests. Point it at a scratch directory during development, and never run tests against your real vault.
- Platform-specific behavior (Unix socket versus named pipe, launchd/systemd/Windows service, Touch ID/Windows Hello, `PR_SET_DUMPABLE`) needs a real machine on that platform. Cross-compilation only proves the code builds, not that runtime behavior works.

## Checks to run

| Command | What it does |
|---|---|
| `make build` | Builds `bin/agent-pwd` with version metadata. |
| `make test` | Runs all unit tests plus the real-daemon integration and security-regression tests. |
| `make race` | Runs the race detector (`CGO_ENABLED=1`). |
| `make vet` | Runs `go vet ./...`. |
| `make cross` | Cross-builds darwin/arm64, darwin/amd64, linux/amd64 and windows/amd64. |
| `make docs` | Regenerates `docs/contract.md` and `docs/contract.zh-CN.md` from the command registry. |
| `make docs-check` | Fails if the generated contract files are out of sync. |
| `make i18n-check` | Runs the i18n and CLI test packages (`go test ./internal/i18n ./internal/cli`). |
| `make helper` | Rebuilds the embedded macOS Touch ID helper binaries (macOS, Swift required). |
| `make clean` | Removes `bin/`. |

CI runs `go test ./...`, `go vet ./...`, cross-builds, and a docs-sync check on macOS, Linux and Windows. Please make the local checks pass before opening a pull request, and mention any platform you could not verify locally.

## Commit and pull request conventions

- Prefer small, focused commits with an imperative English subject line. Conventional Commits prefixes (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`) are welcome but not required. Explain *why* in the body when the reason is not obvious.
- Never commit secrets, vault files, audit logs, local data directories, build artifacts, or generated material that contains real credentials. Redact sample output before pasting it into a commit message or PR description.
- A pull request should state the problem, the behavior change, how you tested it (including platforms), and any follow-up work. Link the relevant issue if there is one.
- Keep the PR scope reviewable. Separate behavioral changes from formatting or translation-only changes where practical.
- All CI checks must pass, and generated files must be up to date. Do not merge with unresolved review comments or known regressions in the no-plaintext output and audit-fail-closed guarantees.

## Regenerating the AI contract docs

`agent-pwd docs` is generated from the command registry in `internal/cli` and the translation catalog; the contract text lives in `internal/cli/docs.go`. Never hand-edit `docs/contract.md` or `docs/contract.zh-CN.md`.

1. Change the command registration, summary, flag, exit-code mapping, or contract text as needed.
2. Run `make docs` (it builds the binary first and writes both language files).
3. Run `make docs-check` and commit both generated files together with the source change.
4. If the change affects translated human-facing text, update `internal/i18n/catalog.go` as described below.

## Contributing translations

English is the canonical source language. Each key in `internal/i18n/catalog.go` is the exact English string passed to `i18n.T`; the map value is the Simplified Chinese rendering.

1. Add or update the English string at the call site, using the canonical English wording as the key.
2. Add the exact same key to `catalogZHCN` in [`internal/i18n/catalog.go`](internal/i18n/catalog.go) with its Chinese translation. Changing the English wording creates a new key, so update the catalog too.
3. Preserve placeholders, backticked commands/flags/values, and machine tokens exactly; see [`docs/i18n.md`](docs/i18n.md).
4. Run `make i18n-check` and `make docs` / `make docs-check`. Add or update tests for any new key or newly translated command.
5. Keep the paired documents (`*.md` / `*.zh-CN.md`) in sync when you change contributor-facing documentation.

## Security-sensitive changes

Changes to crypto, store, session, policy, audit, executors, confirmation, IPC, or CLI output are security-sensitive. They must meet these expectations:

- **No plaintext in output.** Secrets must never appear in stdout, stderr, JSON or human-readable output, error messages, `--human` output, or audit details. `cred show` stays metadata-only, and `fill` returns only an opaque one-time handle. Add a regression test asserting that secret values do not leak for every new output path.
- **Audit fails closed.** Actions must be appended to the hash-chained audit log before they run. If audit writing or verification fails, deny the action; never add a bypass, silent fallback, or best-effort audit path. Do not weaken `audit verify` chain checks.
- **Confirmation stays human-controlled.** High-risk actions return exit code 10 with `confirmation_id` and a poll token and wait for approval. Do not add automatic approval, do not retry pending actions in code, and keep pending state only in daemon memory.
- **Machine tokens stay English.** JSON keys, enum values, exit codes, error codes, command and flag names, audit fields, and machine-facing diagnostics are a stable contract and must not be translated or renamed. Translation applies to human-facing prose only.
- **Honor the trust boundary.** Same-user malware (including `ptrace` and binary replacement) and same-user processes with direct socket/named-pipe access are explicitly out of scope. Do not claim that a change defends against them; see [SECURITY.md](SECURITY.md).
- Report discovered vulnerabilities privately as described in [SECURITY.md](SECURITY.md), never in a public issue.
