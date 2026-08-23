# 更新流式卡片

使用 `dws chat +messages-update-card --biz-id <bizId> --content <文本> --flow-status <1..5>`。

状态为：1 processing、2 typing、3 completed、4 executing、5 error。Runtime 拒绝范围外的
状态；正常完成的最后一次更新应为 3。`bizId` 必须来自真实创建结果，不能用消息 ID 代替。

更新是写操作，confirmation 以精确 leaf Schema 与 Runtime gate 为准。失败后保留原
`bizId` 和状态，不创建新卡片来掩盖更新失败。

结果中 `verified=true` 表示已有明确更新证据；`accepted=true, verified=false` 仅表示服务端
接受了请求但未提供独立生效证据，应如实说明且不得重复执行相同更新。只有错误明确标记
`retryable=true` 时才重试；明确未应用或 `bizId` 不一致时停止。
