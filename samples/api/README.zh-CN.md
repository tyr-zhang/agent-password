# api 示例

本场景用 `apikey` 凭据调用 HTTP API。daemon 把存储的 token 注入配置的认证 header（默认 `Authorization`），脚本从不接触 token。

See [English](README.md).

## 前置条件

- `agent-pwd status` 必须显示 daemon 正在运行且已解锁（必要时执行 `agent-pwd unlock`）；`agent-pwd cred list` 可查看要用的 label。
- 需要一个含 `base_url` 和 `token` 的 `apikey` 凭据（`header_name` 可选，默认 `Authorization`）。示例假定 API 提供 `/user`、issues 端点和分页列表端点。
- `create_issue.sh` 构造 JSON body 需要 `python3`。
- `GET` 和 `HEAD` 自动放行；`POST` 等写方法命中写操作策略，在人类批准前返回退出码 `10`。
- 所有 `agent-pwd` 动作默认输出 JSON。退出码：`0` 成功、`10` 待确认、`20` 未解锁/daemon 未运行、`30` 策略拒绝、`40` 凭据不存在。

## 脚本

| 文件 | 用法 | 预期行为 |
|---|---|---|
| `get_user.sh` | `get_user.sh <apikey-label>` | 执行 `agent-pwd api call --cred "$label" --method GET --path /user`；自动放行的带认证 GET。 |
| `create_issue.sh` | `create_issue.sh <apikey-label> <owner> <repo> <title>` | 用 `{"title": ...}` 构造 JSON body，向 `/repos/${owner}/${repo}/issues` 发起 POST，预期退出 `10`；`handle_pending` 打印待确认 JSON 和人类批准命令。 |
| `list_with_header.sh` | `list_with_header.sh <label> <path>` | 带 `--header 'Accept: application/json'` 和 `--header 'X-Client: agent-pwd-sample'` GET 指定 path；`path` 默认 `/v1/items?page=1&limit=20`。 |

## 示例

```bash
./samples/api/get_user.sh '<label>'
./samples/api/create_issue.sh '<label>' '<owner>' '<repo>' '<title>'
./samples/api/list_with_header.sh '<label>' '<path>'
./samples/api/list_with_header.sh '<label>' '/v1/items?page=1&limit=20'
```

## 安全与注意事项

- 脚本中没有明文 token，也不会打印 token；响应 body 和 header 在输出前会被脱敏。不要把 token 写进 `--header` 或 `--body`。
- `POST` 退出 `10` 时必须停止：确认只能由人类完成，不支持自动批准。其他场景中的 `fill` handle 是一次性的，默认 60 秒过期。
- `--cred` 接受凭据 label 或 ID；优先匹配 label，因此 label 胜出。
- `--path` 必须是绝对路径（以 `/` 开头），且不能包含 `..` 或 `//`；重定向默认关闭，这些脚本没有传 `--allow-redirect`。
- 脚本会 source `../common.sh`；未传 label 时，该脚本也接受 `AGENT_PWD_CRED` 作为 label 回退值。
