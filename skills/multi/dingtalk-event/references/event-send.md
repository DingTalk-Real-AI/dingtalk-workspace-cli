# 企业自建应用三方事件发送

`dws event send` 使用企业自建应用 Token 调用现有 `POST /v1.0/event/user/push`，向指定企业内用户发送该应用的三方事件。应用归属、业务版本、接收人、payload Schema 和事件 ID 的幂等校验由服务端执行。调用方应提供已发布事件及对应业务版本。

## 发送入口

应用凭证已通过 `DWS_CLIENT_ID` 和 `DWS_CLIENT_SECRET` 注入时，可直接执行以下命令。事件、版本、接收人和 payload 均替换为本次真实业务值。

```bash
dws event send \
  --event-key app_user_order_created \
  --event-id order-1001-created \
  --payload-schema-version 3 \
  --user-id '<企业内 userId>' \
  --payload '{"orderId":"order-1001"}' \
  -f json
```

| 参数 | 约定 |
| --- | --- |
| `--event-key` | 必填，应用下的真实事件键。 |
| `--event-id` | 必填，最长 64 字符。重发同一条事件沿用原 ID，发送新事件使用新 ID。 |
| `--payload-schema-version` | 必填，以字符串传给服务端，使用 payload 对应的已发布业务版本。 |
| `--user-id` | 必填，可重复传入，去重后最多 100 人。使用目标企业内的 `userId`。 |
| `--payload` | 必填 JSON 对象，支持直接 JSON、`@文件` 和 `-` 从标准输入读取。保留 JSON 数字精度。 |
| `--occurred-at` | 可选，带时区的 ISO 8601 时间。省略时请求省略该字段。 |
| `--dry-run` | 预览请求。直接 JSON 会解析校验，文件或标准输入仅展示来源。预览阶段不读取文件内容、stdin 或 Keychain，也不换 Token 或发送请求。 |

`--payload-schema-version` 使用业务 Schema 版本。`configRevision` 是配置草稿修订号，平台信封的 `schemaVersion` 是协议版本，三者分别取对应字段。

## 凭证来源

完整凭证对按以下顺序解析。

1. 全局 `--client-id` 与 `--client-secret`，分别对应 AppKey 与 AppSecret。
2. 环境变量 `DWS_CLIENT_ID` 与 `DWS_CLIENT_SECRET`。
3. 本机应用配置及对应 Keychain 密钥。

显式参数或环境变量只提供一项时返回错误，按提示补齐同一来源的凭证对。完整应用凭证即可发送。应用发现、凭证读取和事件消费分别使用各自已有的用户授权链路，按需调用。

Agent 仅在已明确目标应用且用户授权相应操作时读取或使用凭证。执行时将 Secret 留在受控进程和运行环境中。对外答复、日志和示例保持脱敏。

## 回执与重发

统一结果外层 `ok=true`、`outcome=success` 时，读取 `data.eventId`、`data.status` 和 `data.duplicate`。

| `data.status` | 含义与处理 |
| --- | --- |
| `ACCEPTED` | 本次事件已接纳，`duplicate=false`。 |
| `DUPLICATE` | 同一事件此前已接纳，`duplicate=true`，按成功处理。 |

这两种回执退出码均为 0。实际消费以订阅端收到对应业务事件为准。需要端到端验收时，先启动同组织的消费端并等待明确 ready，再发送并关联业务标识核对收件。

每次 CLI 调用上报一次。超时或结果不确定时保留原 `eventId` 和完整请求内容，按返回的错误与重试提示处理。对同一事件重发使用原 ID、版本、接收人、payload 和发生时间。同一 ID 对应不同内容时返回冲突，先核对业务请求。

凭证失效、事件归属不符、不可用版本、无效收件人和 Schema 不匹配应保留真实业务错误码及上下文，按具体错误修正。以实际环境中的请求回执、订阅及收件证据确认验收状态。
