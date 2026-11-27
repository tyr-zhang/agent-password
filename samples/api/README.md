# API samples

This scenario calls HTTP APIs with an `apikey` credential. The daemon injects the stored token into the configured auth header (default `Authorization`); the scripts never handle the token.

See [简体中文](README.zh-CN.md).

## Prerequisites

- `agent-pwd status` must show the daemon running and unlocked (`agent-pwd unlock` if needed); `agent-pwd cred list` shows the label to use.
- An `apikey` credential with `base_url` and `token` (optional `header_name`, default `Authorization`). The examples assume an API with `/user`, an issues endpoint, and a paginated list endpoint.
- `python3` is needed to build the JSON body in `create_issue.sh`.
- `GET` and `HEAD` are auto-allowed; `POST` and other methods match the write policy and return exit `10` until a human approves.
- All `agent-pwd` actions emit JSON by default. Exit codes: `0` success, `10` pending confirmation, `20` locked/daemon absent, `30` denied, `40` credential not found.

## Scripts

| File | Usage | Expected behavior |
|---|---|---|
| `get_user.sh` | `get_user.sh <label|id>` | Runs `agent-pwd api call --cred "$label" --method GET --path /user`; an auto-allowed authenticated GET. |
| `create_issue.sh` | `create_issue.sh <label|id> <owner> <repo> <title>` | Builds the JSON body with `{"title": ...}`, POSTs to `/repos/${owner}/${repo}/issues`, and expects exit `10`; `handle_pending` prints the pending JSON and the human approval commands. |
| `list_with_header.sh` | `list_with_header.sh <label> <path>` | GETs the given path with `--header 'Accept: application/json'` and `--header 'X-Client: agent-pwd-sample'`; `path` defaults to `/v1/items?page=1&limit=20`. |

## Examples

```bash
./samples/api/get_user.sh '<label>'
./samples/api/create_issue.sh '<label>' '<owner>' '<repo>' '<title>'
./samples/api/list_with_header.sh '<label>' '<path>'
./samples/api/list_with_header.sh '<label>' '/v1/items?page=1&limit=20'
```

## Safety and notes

- No script contains a plaintext token and no script prints one; response bodies and headers are redacted before output. Do not put tokens in `--header` or `--body`.
- A `POST` that exits `10` must stop: confirmation is human-only and automatic approval is unsupported. `fill` handles in other scenarios are one-shot and expire after 60 seconds.
- `--cred` accepts a credential label or ID; labels are matched first, so the label wins.
- `--path` must be absolute (start with `/`) and contain no `..` or `//`; redirects are off by default and these scripts do not pass `--allow-redirect`.
- The scripts source `../common.sh`, which also accepts `AGENT_PWD_CRED` as a label fallback when no label is passed.
