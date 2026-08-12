# IM EventKey 与底层消费

高频消息、reaction、已读和撤回优先 `dws event +listen-im`。本页只用于群生命周期、
显式 EventKey 或高级底层控制。

## 16 个 EventKey

| EventKey | 范围 | 目标参数 |
|---|---|---|
| `user_im_message_receive_at` | 被 @ 消息 | 无 |
| `user_im_message_receive_o2o` | 指定单聊消息 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_receive_group` | 指定群消息 | `--group` |
| `user_im_message_receive_user` | 指定发送人在单聊/群聊中的消息 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_receive_o2o_all` | 全部单聊消息 | 无 |
| `user_im_message_receive_group_all` | 全部群消息 | 无 |
| `user_im_message_read_o2o` | 指定单聊已读 | user 二选一 |
| `user_im_message_read_group` | 指定群已读 | `--group` |
| `user_im_message_recall_o2o` | 指定单聊撤回 | user 二选一 |
| `user_im_message_recall_group` | 指定群撤回 | `--group` |
| `user_im_message_reaction_o2o` | 指定单聊 reaction | user 二选一 |
| `user_im_message_reaction_group` | 指定群 reaction | `--group` |
| `user_im_group_updated` | 群标题变化 | `--group` |
| `user_im_group_member_added` | 成员加入 | `--group` |
| `user_im_group_member_exited` | 成员退出 | `--group` |
| `user_im_group_disbanded` | 群解散 | `--group` |

`--user` 只接收 userId；明确 openDingTalkId 时用对应 flag。群目标必须是
openConversationId。不要把两种身份混传或选择搜索第一项。

## 精确模板

```bash
dws event consume user_im_group_member_added \
  --group <openConversationId> --flatten -f ndjson

dws event consume user_im_message_receive_user \
  --open-dingtalk-id <openDingTalkId> --flatten -f ndjson

dws event consume user_im_message_receive_group \
  user_im_message_reaction_group \
  user_im_message_recall_group \
  --group <openConversationId> --flatten -f ndjson
```

多事件只能共享同一 target/filter。用户类与群类不能混在一个命令中；不同人、群或过滤条件
启动不同 consume。只有全部 EventKey 都是接收消息时才能共享 `--query/--filter-json`。
多事件不支持 `--subscribe-id`、`--rule`、`--event-types`、`--filter`、`--foreground`、
`--force` 或 `--debug-raw-events`。

## 意图消歧

- “我和某人的单聊” → `receive_o2o`；“某人发给我的消息” → `receive_user`。
- 执行撤回/添加 reaction 走 `dws chat`；监听它们才走 Event。
- 群改名/成员进退/解散使用四个生命周期 EventKey；解散自测只能用已确认可销毁测试群。
