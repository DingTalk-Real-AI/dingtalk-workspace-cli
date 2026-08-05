# 创建流式卡片

使用 `dws chat +messages-send-card`。群目标传 `--group`；单聊 userId 传 `--receiver`，
Runtime 会唯一解析为 openDingTalkId；已有 openDingTalkId 时传
`--receiver-open-dingtalk-id`。三种目标严格三选一。

- 只传目标：创建卡片并从真实结果取得 `bizId`，供后续更新。
- 同时传 `--content`：Runtime 串行执行 create → 从返回提取 `bizId` → update；默认
  `--flow-status 3`。
- 群聊可传 `--at-open-dingtalk-ids` 或 `--at-all`；艾特对象只进入初始
  `create_and_send_card`，不会进入后续 `update_streaming_card`。
- `--dry-run` 仍执行只读 userId 解析，只输出两步计划，不执行写入。

创建成功但自动更新失败时，错误会保留真实 `bizId`。不要重复创建；使用该 `bizId` 继续
`+messages-update-card` 或人工处理。

当前内容仅为 streaming text，不接受 Lark Card JSON、组件树或按钮 callback。

```bash
dws chat +messages-send-card --group <openConversationId> --at-open-dingtalk-ids <mentionedOpenDingTalkId> --content "请确认"
dws chat +messages-send-card --group <openConversationId> --at-all --content "请大家确认"
```
