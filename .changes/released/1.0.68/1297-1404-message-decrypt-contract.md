---
category: Fixed
---

- **消息解密失败原因透传** — 策略查询失败与策略关闭不再共用一个笼统原因：查询失败记为 `policy_query_failed`，策略关闭透传服务端 `reason`（如 `org_crypto_disabled`、`crypto_disabled_by_switch`），仅在旧服务端未给出原因时回退 `policy_disabled`；批量解密整体失败按消息逐条记为 `batch_decrypt_failed` 并保留 `messageId`/`conversationId`，不再输出一条无归属的 Go 错误文本（#1404）。
- **分页聚合保留正文缺口** — 单聊全量读取（`+messages-list-direct --page-all`）与智能读取快捷入口在写完分页诊断后不再覆盖解密账本的 `partial`，解密失败但分页成功时结果仍标记为不完整；`chat +messages-mget` 等原子入口补齐 `decryptAllowedCount`，与快捷入口的账本字段一致（#1297）。
- **策略响应缺失 mode 时不再静默降级** — 仅当响应是不含任何策略字段的纯成功回执时才按 `mode=off` 兼容处理；缺少 `mode` 却带有 `reason`、`friendlyHint` 或错误字段的响应直接失败，避免把契约缺字段读成"组织未开通"。
