# 事件订阅

> 把应用关心的事件推到回调地址；见 SKILL.md 概念地图。

`dws dev app event list/subscribe/unsubscribe`，按 `--unified-app-id` 定位，订阅/退订用 `--event-codes`（逗号分隔，一次多个）。参数查 `dws schema dev.app.event.<method>`。

规则：
- 写操作先 `--dry-run` 预览，确认后 `--yes`。
- 一次可订阅多个事件码，共用同一回调。
- 事件码取值以开放平台文档为准；不确定走 `dws dev doc search`。
- 退订前先 `event list` 确认当前订阅，避免退不存在的。
- `event list` 支持 `--keyword/--cursor/--page-size`；`--keyword` 按事件码或事件名称搜索；返回 `events/hasMore/nextCursor/pageSize`；翻页继续传 `nextCursor`。
- 返回看 `events[].subscribed` 和 `pushType=STREAM`。
- `subscribe/unsubscribe` 的 `--event-codes` 必填，返回 `success/operation/unifiedAppId/eventCodes/needsPublish/versionRequiredAction`；失败时补 `errorCode/errorMsg/reason/retryable/action`。
- 如果服务端返回 `errorCode=STREAM_NOT_CONNECTED`、`reason=STREAM_NOT_CONNECTED`、`retryable=false`、`action=run connect`，先执行 `dev connect` 建联，再重试订阅。
