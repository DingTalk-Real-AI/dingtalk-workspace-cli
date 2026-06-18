# 事件订阅

> 把应用关心的事件推到回调地址；见 SKILL.md 概念地图。

`dws dev app event list/subscribe/unsubscribe`，按 `--unified-app-id` 定位，订阅/退订用 `--event-codes`（逗号分隔，一次多个）。参数查 `dws schema dev.app.event.<method>`。

前置条件（业务规则）：订阅/退订前必须先建立长连（`dev connect`，见 connect.md）。长连未在线时服务端报「长链接未在线」（errorCode -1），这是预期，不是故障——先建联再订阅。

规则：
- 写操作先 `--dry-run` 预览，确认后 `--yes`。
- 一次可订阅多个事件码，共用同一回调。
- 事件码取值以开放平台文档为准；不确定走 `dws dev doc search`。
- 退订前先 `event list` 确认当前订阅，避免退不存在的。
- 返回看 `events`（已订阅列表）和 `pushType`（推送方式）。
