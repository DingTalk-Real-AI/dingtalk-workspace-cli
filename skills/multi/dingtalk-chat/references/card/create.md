# 创建流式卡片

使用 `dws chat +messages-send-card`。群目标传 `--group`；单聊 userId 传 `--receiver`，
Runtime 会唯一解析为 openDingTalkId；已有 openDingTalkId 时传
`--receiver-open-dingtalk-id`。三种目标严格三选一。

- 只传目标：创建卡片并从真实结果取得 `bizId`，供后续更新。
- 同时传 `--content`：Runtime 串行执行 create → 从返回提取 `bizId` → update；默认
  `--flow-status 3`。
- 群聊可传 `--at-open-dingtalk-ids` 或 `--at-all`；艾特对象只进入初始
  `create_and_send_card`。同一次调用带 `--content` 时，Runtime 将 create 返回的
  `atTag` 自动加在正文前，再调用 `update_streaming_card`；调用方不要拼 ID
  或艾特占位符。
- `--dry-run` 仍执行只读 userId 解析，只输出两步计划，不执行写入。

创建成功后保留真实 `bizId`。自动更新返回 `verified=true` 时已有明确生效证据；返回
`accepted=true, verified=false` 时仅表示服务端已接受请求但未提供独立更新证据，应如实说明，
不要重复创建或重复执行相同更新。只有错误明确标记 `retryable=true` 时，才使用原 `bizId`
重试；明确未应用或 `bizId` 不一致时停止并保留真实错误。若结果中已经包含 `openTaskId`，
可以按用户需要查询一次投递状态；该查询只确认消息投递，不代表卡片正文已经更新成功。

当前内容仅为 streaming text，不接受 Lark Card JSON、组件树或按钮 callback。

```bash
dws chat +messages-send-card --group <openConversationId> --at-open-dingtalk-ids <mentionedOpenDingTalkId> --content "请确认"
dws chat +messages-send-card --group <openConversationId> --at-all --content "请大家确认"
```
