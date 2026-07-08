# 事件订阅

> 把应用关心的事件推到回调地址；见 SKILL.md 概念地图。

`dws dev app event list/subscribe/unsubscribe`，按 `--unified-app-id` 定位，订阅/退订用 `--event-codes`（逗号分隔，一次多个）。参数用对应命令的 `--help` 查询。

规则：
- 写操作先 `--dry-run` 预览，确认后 `--yes`。
- 一次可订阅多个事件码，共用同一回调。
- **事件码定位优先用 `event list --keyword <关键词>` 搜索**（按事件码或事件名称模糊匹配）；只有用户明确要「全部事件」时才不带 `--keyword` 翻全量。
- 可订阅的事件码通过 `event list` 查询：返回 `events[]` 列出 `eventCode/eventName/subscribed`，不用查文档。
- 退订前先 `event list` 确认当前订阅，避免退不存在的。
- 翻全量时用 `--cursor/--page-size` 逐页处理；返回 `events/hasMore/nextCursor/pageSize`，翻页继续传 `nextCursor`。
- 批量或全量订阅前，先把候选 `eventCode` 列给用户确认，再 `--dry-run` → `--yes`。
- 返回看 `events[].subscribed` 和 `pushType=STREAM`（事件走 Stream 长连推送；connect 与事件订阅的关系见下方「Stream 长连」）。
- `subscribe/unsubscribe` 的 `--event-codes` 必填，返回 `success/operation/unifiedAppId/eventCodes/needsPublish/versionRequiredAction`；失败时补 `errorCode/errorMsg/reason/retryable/action`。
- 如果订阅失败、返回提示长链接未在线（是泛化错误：`reason=business_error`、`message` 含「长链接未在线」、`server_error_code=-1`；没有 STREAM_NOT_CONNECTED 这类结构化错误码，也没有 action 字段），先执行 `dev connect` 建联，再重试订阅。

## Stream 长连：connect 与事件订阅的关系

钉钉一个应用就一条 Stream 长连（WebSocket），上面同时承载机器人消息、事件、卡片回调等多种 topic。connect 和 event 在这条长连上分工不同：

- `dev connect`：用应用凭证把这条长连建起来并保活，但只注册了机器人消息处理（收 @机器人 → 转发本地 agent），**不消费事件**。
- `event subscribe/unsubscribe`：只是**配置**操作（配应用订阅哪些事件码），自己不建长连、不收事件；服务端要求应用的 Stream 长连已在线，否则报错「长链接未在线」（泛化 business_error，不是结构化错误码）。
- 两者唯一关联：先 `dev connect` 把长连建在线，`event subscribe` 才能成功——connect 负责「让长连在线」，subscribe 负责「配置订阅」。
- 当前限制：connect 的长连不处理事件，dws 也没有「收/消费事件」的运行时命令，event 只到「配置订阅」为止。订阅成功后真正的事件推送 dws 暂不消费——要消费得用注册了事件 handler 的 SDK 自接（事件结构走 `dws devdoc article search --query` 查）。

## Stream 长连怎么建的

`dev connect` 内部用 dingtalk-stream-sdk-go，凭 `clientId/clientSecret` 建一条 WebSocket 长连并保活——底层网关握手、ticket、加解密都由 SDK 封装，agent 跑 `dev connect` 即可，不用碰这些。

dws 当前只消费机器人消息、不消费事件。要自己写事件消费程序补这个 gap 时，SDK 用法以官方文档为准（走 `dws devdoc article search --query`，或看 github.com/open-dingtalk/dingtalk-stream-sdk-go）——注意事件用 `RegisterAllEventHandler`、connect 用的机器人是 `RegisterChatBotCallbackRouter`，两套别混；版本/接口会变，不在这里固化。

## 发现命令

调用任何方法前先查清楚再敲：

```
# 浏览命令组下的子命令与 flag
dws dev app event --help

# 查某方法的必填参数、类型、默认值
dws dev <command-path> --help
```

按 `--help` 输出构造 flag；不要凭旧 schema 名称猜参数。
