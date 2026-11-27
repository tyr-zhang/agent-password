# web-login samples

This scenario logs in to a website with a `web` credential. The shell wrappers issue a one-shot `fill` handle for the `password` field, and the trusted Playwright child process fills the form; the agent never sees the secret.

See [简体中文](README.zh-CN.md).

## Prerequisites

- `agent-pwd status` must show the daemon running and unlocked (`agent-pwd unlock` if needed); `agent-pwd cred list` shows the label to use.
- `python3` and Playwright's Python package (`pip install playwright`) with a Chromium browser available to it; the scripts launch `p.chromium.launch(headless=False)`, so a display is required.
- A `web` credential with a `password`; the browser scripts use the username passed as a plain argument. For TOTP, use the separate `samples/fill/run_totp.sh` demo and extend the browser script accordingly.
- All `agent-pwd` actions emit JSON by default. Exit codes: `0` success, `10` pending confirmation, `20` locked/daemon absent, `30` denied, `40` credential not found.

## Scripts

| File | Usage | Expected behavior |
|---|---|---|
| `run_jenkins.sh` | `run_jenkins.sh <label|id> <username> [url]` | Issues `agent-pwd fill --cred "$label" --field password`, applies the handle to `jenkins.py`, and prints only the browser page title and URL. `url` defaults to `https://jenkins.example.com/`. |
| `run_generic.sh` | `run_generic.sh <label> <username> <url> <user_sel> <pass_sel> [submit_sel]` | Issues a `password` fill handle and applies it to `generic_form.py` with caller-supplied selectors; `submit_sel` defaults to `button[type=submit]`. |
| `jenkins.py` | `python3 jenkins.py <url> <username> [user_selector] [pass_selector] [submit_selector]` (invoked through `agent-pwd fill apply --handle <h> -- ...`) | Opens Chromium, fills the form via `AGENT_PWD_FILL`, clicks submit, waits for `networkidle`, and prints `title:` and `url:`. Selectors default to `input[name="j_username"]`, `input[name="j_password"]`, and `text=Sign in`; exits `2` if arguments are missing or `AGENT_PWD_FILL` is empty. |
| `generic_form.py` | `python3 generic_form.py <url> <username> <user_selector> <pass_selector> [submit_selector]` (invoked through `agent-pwd fill apply --handle <h> -- ...`) | Same browser flow with all selectors supplied by the caller; submit selector defaults to `button[type="submit"]`; exits `2` if arguments are missing or `AGENT_PWD_FILL` is empty. |

## Examples

```bash
./samples/web-login/run_jenkins.sh '<label>' '<username>' '<url>'
AGENT_PWD_CRED='<label>' ./samples/web-login/run_jenkins.sh '<label>' '<username>'
./samples/web-login/run_generic.sh '<label>' '<username>' '<url>' '<user-selector>' '<pass-selector>'
./samples/web-login/run_generic.sh '<label>' '<username>' '<url>' '<user-selector>' '<pass-selector>' '<submit-selector>'
```

## Safety and notes

- No script contains a plaintext secret, and the scripts never print one; `fill apply` redacts child stdout/stderr. `agent-pwd` does not navigate browsers or locate elements — the Playwright scripts are the trusted children that do so.
- `fill` handles are one-shot and expire after 60 seconds (consumed even if the child fails); apply them immediately with the same client. If an action exits `10`, stop and ask a human to approve — confirmation is human-only.
- `--cred` accepts a credential label or ID; labels are matched first, so the label wins.
- The `generic_form.py` module docstring omits `<username>`, but its argument handling requires it; pass the username as shown above.
- The wrappers source `../common.sh`, which also accepts `AGENT_PWD_CRED` as a label fallback when no label is passed.
