# 全局运行时参考

只在认证、profile、公开全局 flag 或输出格式不确定时读取本文件。命令与参数事实以当前
分支的 leaf Schema 和 Cobra Help 为准。

## 认证与 profile

日常业务命令会使用本地 OAuth 登录态，并在明确的 access-token 拒绝信号下自动刷新、
最多重放当前调用一次。不要从错误文本猜测 token 状态，也不要自行循环重试。

```bash
# 查看当前 profile 的登录态；读取 authenticated/token_valid 等真实字段
dws auth status --format json

# 列出全部账号及 isCurrent/isOrgCurrent
dws profile list --format json

# 本机浏览器登录；无头环境使用 --device
dws auth login --recommend --format json
dws auth login --device --recommend --format json

# 只指定本次组织/账号，不持久切换默认 profile
dws auth status --profile <corpId>:<userId> --format json
```

- 同一条 IM 链路的目标解析、读取与写入必须使用同一 `--profile`。
- `profile list` 本身不刷新 token；`auth status --profile ...` 只检查/刷新选中的 token
  slot，不修改 `currentProfile`。
- `auth logout` 默认退出全部账号；传 `--profile` 才缩小范围。`auth reset` 会清除本地
  token，只有确认登录态需要重建时才使用。
- 宿主已注入认证时不要向用户索要 token、refresh token、AppSecret 或 webhook token。

### 可迁移认证包

只有用户明确要求迁移登录态时才使用：

```bash
dws auth export --output dws-auth.tar.gz
dws auth import --input dws-auth.tar.gz
dws auth status --format json
```

认证包包含敏感凭据和解密材料；不得打印其内容，迁移完成后按用户的安全策略删除。
平台/密钥后端限制以 `dws auth export --help` 与命令返回为准，不自行绕过。

## 公开全局 flag

| flag | 语义 | 默认/约束 |
|---|---|---|
| `--format`, `-f` | `json\|table\|raw\|pretty\|ndjson\|csv` | `json` |
| `--fields` | 逗号分隔的输出字段筛选 | 空 |
| `--jq` | jq 表达式过滤输出 | 空 |
| `--profile` | 一次性指定组织或账号；支持 CSV 多选的命令由运行时决定 | 当前 profile |
| `--timeout` | HTTP 请求超时秒数 | `30` |
| `--dry-run` | 请求预览 | 只有 leaf 明确支持时才代表可执行预览 |
| `--yes`, `-y` | 通过 Runtime confirmation gate | 仅在用户确认且最终 Schema 要求时追加 |
| `--verbose`, `-v` | 增加诊断信息 | `false` |
| `--debug` | 输出内部调试诊断 | `false`；不得泄露凭据 |
| `--mock` | 开发调试 Mock | `false`；不得当作业务成功 |
| `--client-id` / `--client-secret` | OAuth 应用凭据覆盖 | 必须成对、仅在明确配置任务使用 |

公开 Help 不提供通用业务 `--token` flag。Webhook token 等凭据只传给声明该 leaf
参数的命令；不要把某个 leaf 的 `--token` 推广成全局认证方式。

## 输出契约

### 成功输出

`--format json` 直接序列化命令的真实 payload；不存在适用于所有产品的固定
`{"success":true,"body":...}` 包装。读取 leaf 声明的字段，并保留以下完整性信号：

- 分页：`hasMore`、`nextCursor`、`nextPageToken` 等当前响应字段；
- 批量/编排：`ok`、`partial`、`failures`、`results` 等当前 Shortcut ledger；
- 写入：返回的稳定资源 ID、投递状态或逐项失败，不用退出码代替结果验证。

`table`/`pretty` 面向人工阅读，`raw` 保留原始文本，`ndjson`/`csv` 面向列表流转。
Agent 默认使用 JSON；只有确定列表形状与下游需求时才切换格式。

### 错误输出

JSON 模式下，失败写到 stderr，稳定外层结构为：

```json
{
  "error": {
    "code": 3,
    "category": "validation",
    "message": "...",
    "reason": "...",
    "retryable": false,
    "hint": "...",
    "actions": ["..."]
  }
}
```

除 `code/category/message` 外的字段按错误类型出现。优先消费 `reason`、`retryable`、
`retry_after_seconds`、`next_retry_at`、`details`、`available_flags`、`trace_id`、
`server_error_code`、`hint` 和 `actions`；不要依赖自由文本子串做自动修复。

## 相关环境变量

| 变量 | 用途 |
|---|---|
| `DWS_CONFIG_DIR` | 覆盖默认 `~/.dws` 配置目录 |
| `DWS_CLIENT_ID` + `DWS_CLIENT_SECRET` | 成对覆盖 OAuth 应用凭据；只设置一个时不会作为完整凭据对使用 |
| `DWS_CHANNEL` | 受控分发渠道；仅在明确渠道任务中按 [channel-login.md](channel-login.md) 使用 |

不要把内部调试/兼容环境变量写进普通业务流程，也不要把凭据持久写入 shell profile。

## 命令自省

```bash
# 决定 Agent 选路、参数映射与确认语义
dws schema --cli-path "chat +messages-send" --compact --format json

# 核对当前二进制真实接受的 flag
dws chat +messages-send --help
```

已知 leaf 直接执行。只有 selection、安全或参数映射不确定时查精确 Schema；只有 flag
拼写不确定时查精确 Help。不要为一个 leaf 加载产品级全量 Catalog。
