# agent-pwd samples

Runnable examples for every action type. **No secrets are stored here.** Every script takes a credential **label** (or reads it from `AGENT_PWD_CRED`) and lets the daemon inject the secret.

See [简体中文](README.zh-CN.md).

## Prerequisites

```bash
agent-pwd status          # daemon running and unlocked
agent-pwd cred list       # find the label you want to use
python3 --version         # needed for the browser examples
pip install playwright    # only for samples/web-login
```

All actions return JSON by default and use these exit codes: `0` success, `10` pending confirmation, `20` locked/daemon absent, `30` denied, `40` credential not found.

## Scenarios

| Directory | What it demonstrates |
|---|---|
| [`web-login/`](web-login/) | Jenkins login and a generic form, driven by Playwright with a one-shot `fill` handle |
| [`ssh/`](ssh/) | Run uptime, tail logs, inspect Docker via stored SSH password/key |
| [`sql/`](sql/) | MySQL reads/writes, PostgreSQL EXPLAIN, Redis read-only commands |
| [`api/`](api/) | GET/POST with injected token, extra headers, body payloads |
| [`env/`](env/) | Inject one env var into a child, verify presence without printing it, nested processes |
| [`fill/`](fill/) | Issue/apply handles for password and TOTP, consume from stdin |
| [`confirm/`](confirm/) | Handle exit `10`, approve/deny flows, collect results |
| [`ops/`](ops/) | doctor, audit verify, backup restore, password rotation |

Each scenario directory contains its own bilingual `README.md` / `README.zh-CN.md` with per-script usage and notes.

## Safety

- Replace placeholder labels and URLs before running.
- Username examples are plain arguments; if a username is sensitive, store it as a credential and use `fill --field username` too.
- Scripts never print secret values. `fill apply` redacts child stdout/stderr regardless.
- `agent-pwd` does not navigate browsers or locate elements; the Playwright scripts are the trusted child processes that do it.

Run a sample:

```bash
AGENT_PWD_CRED=my-jenkins ./samples/web-login/run_jenkins.sh my-jenkins alice https://jenkins.example.com/
```
