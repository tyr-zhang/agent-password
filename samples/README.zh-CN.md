# agent-pwd 示例

每个动作类型都有可运行的例子。仓库里**不含任何密钥**：脚本接收凭据 **label**，由 daemon 注入 secret。

See [English](README.md).

## 前置条件

```bash
agent-pwd status          # daemon 正在运行且已解锁
agent-pwd cred list       # 找到要用的 label
python3 --version         # 浏览器示例需要
pip install playwright    # 仅 web-login 示例需要
```

动作默认输出 JSON，退出码：`0` 成功、`10` 待确认、`20` 未解锁/daemon 未运行、`30` 策略拒绝、`40` 凭据不存在。

## 场景

| 目录 | 演示内容 |
|---|---|
| [`web-login/`](web-login/) | 用 Playwright + 一次性 `fill` handle 登录 Jenkins 和通用表单 |
| [`ssh/`](ssh/) | 用存储的密码/私钥执行 uptime、查看日志、检查 Docker |
| [`sql/`](sql/) | MySQL 读写、PostgreSQL EXPLAIN、Redis 只读命令 |
| [`api/`](api/) | 注入 token 的 GET/POST、附加 header、请求体 |
| [`env/`](env/) | 向子进程注入一个环境变量、不打印值地验证存在性、多层子进程 |
| [`fill/`](fill/) | 申请/消费 password 与 TOTP 句柄、从 stdin 读取 |
| [`confirm/`](confirm/) | 处理退出码 10、批准/拒绝流程、取回结果 |
| [`ops/`](ops/) | doctor、审计校验、备份恢复、主密码轮换 |

每个场景目录中都有独立的中英双语 `README.md` / `README.zh-CN.md`，说明每个脚本的用法和注意事项。

## 安全说明

- 运行前替换占位 label 和 URL。
- 示例中的用户名是普通参数；如果用户名也敏感，请同样存入凭据并用 `fill --field username`。
- 脚本绝不打印密钥；`fill apply` 会对子进程 stdout/stderr 强制脱敏。
- `agent-pwd` 不负责打开浏览器或定位元素；Playwright 脚本是被信任的子进程。

运行示例：

```bash
./samples/web-login/run_jenkins.sh my-jenkins alice https://jenkins.example.com/
```
