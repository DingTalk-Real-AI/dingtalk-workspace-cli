# message-query：消息读取、搜索与查询

> 返回入口：[DingTalk Chat Skill](../../SKILL.md)

用于浏览或导出指定会话、按条件搜索消息、按消息 ID 读取详情、查看 @我、话题回复、
Favorite、Pin 和 reaction。只读任务优先使用 Shortcut；只有 Shortcut 未发布所需底层字段、
原始响应或手工 continuation 时才读取精确原子 leaf Schema。

## 入口选择

| 用户终点 | 唯一推荐入口 |
|---|---|
| <!-- dws-intent: chat.read.conversation -->浏览或导出一个指定群聊/单聊 | `dws chat +chat-messages` |
| <!-- dws-intent: chat.read.reactions -->筛选指定会话中存在 reaction 的消息 | `dws chat +search-msg --group <群名或ID> --has-reactions --page-all` |
| <!-- dws-intent: chat.search.filtered -->发送者、关键词、@对象或消息类型是主要条件 | `dws chat +search-msg` |
| <!-- dws-intent: chat.conversation.active-since -->最近 24 小时或指定时间以来哪些会话有新消息，只要会话摘要 | `dws chat +recent-conversations`；可选 `--start/--end` |
| 已知消息 IDs 读取详情 | `dws chat +messages-mget` |
| 查看 @我的消息 | `dws chat +at-me` |
| 查看 Favorite | `dws chat +flag-list` |
| 已知话题主消息或 thread/topic ID 读取回复 | `dws chat +thread-replies` |

`+chat-messages` 是指定会话的粗粒度读取；`+search-msg` 是目标条件明确的单/跨会话检索。
不要先读完整会话再补跑搜索，也不要把群名或姓名直接填入只接受稳定 ID 的参数。

旧名 `+active-conversations` 保留为隐藏但可执行的兼容入口；新调用使用 `+recent-conversations`。
两者共用实现、参数和结果，使用旧名不会额外告警。

`+recent-conversations` 不返回消息正文。它自动沿 `nextCursor` 翻页，按 `openConversationId`
去重，并返回 `conversationId`、`name`、
`nameKnown`、`direct/group/unknown` 类型及时间窗内的最大消息时间。名称未知时仍返回
`name=""`、`nameKnown=false`。两个时间参数都省略时查询最近 24 小时；仅传 `--end` 时，
默认 `start=end-24h`，不是当天零点。显式 `--start` 保持原有时间解析规则，空值或纯空白仍报错；
`--page-delay` 默认 200ms，范围 0–60000ms，分页等待可被取消。`--total-timeout` 默认
300 秒（1–3600），约束本次所有页、重试和等待共用的查询预算；全局 `--timeout` 仍是单次请求上限。

查询边界 `--start/--end` 仅支持整秒；显式传入非零小数秒会在参数校验时失败。省略
`--end` 时，将本次查询取到的当前时间向下取整秒后固定，因此不包含当前尚未结束的
这一秒，再从有效 `end` 倒推默认 `start`。最终有效整秒区间必须满足 `end > start`；同一 `start/end` 用于所有页请求、结果返回
和续查。该限制只作用于查询边界，消息 `latestMessageTime` 仍保留毫秒精度。

成功时会话摘要位于 `data`，其中 `pageSize` 记录本次 `--limit`。达到 `--page-limit` 仍有
下一页时，保留 `data.complete=false` 和 `meta.pagination.next_token`。后续页失败时返回
`outcome=partial_failure`（退出码 7）：`data.succeeded[0]` 的 `id=completed-pages`，其余
字段是已成功页面的同形聚合摘要，`complete=false`；`data.failed[0]` 的 `id=page:N`，
其 `error` 保留原始错误诊断，`error.details.failedPage/failedCursor` 标明失败页及输入
游标，`meta.pagination.next_token` 也指向该失败页。首个请求页失败时返回普通 `failure`。
遇到分页错误先按真实错误排查，不自动重试；不能只因拿到部分数据就报告查询完成。

本命令不保存磁盘进度，进程强制终止后需要重新查询。`pagesFetched` 仅计本次已验证页面。
分页耗尽不保证服务端索引无延迟或提供一致性快照。

`--cursor` 用于手工续查，CLI 不能跨次校验参数绑定或游标循环，也不自动恢复历史聚合。
使用 `--cursor` 续查时，必须保持同一 profile，并显式复用上次摘要中的 `start`、`end`
和 `pageSize`（分别传给 `--start/--end/--limit`）；非首页游标缺少显式 `--start` 或 `--end` 会报错，不能重新使用滚动默认窗口。续页响应不包含此前批次，所以即使
`meta.pagination.endpoint_exhausted=true`，该批自身的 `complete` 仍为 `false`。调用方应将
前后批次按 `conversationId` 合并，保留更大的 `latestMessageTime`；只有从首页起无遗漏地
接续全部批次、处理完失败页且最终确认 endpoint 耗尽，才能将合并结果称为全量。游标过期时需从首页重查。

```bash
dws chat +recent-conversations --format json
dws chat +recent-conversations --end "2026-09-08T15:30:00+08:00" --format json
dws chat +recent-conversations --start "2026-09-07T00:00:00+08:00" --format json
```

## 指定会话读取

群聊 `--group` 可传群名或 `openConversationId`；也可用 `--chat-query` 显式解析群名、
用 `--conversation-id` 显式传稳定 ID。单聊使用 `--user` 或 `--open-dingtalk-id`。

```bash
dws chat +chat-messages --group <群名或openConversationId> --format json
dws chat +chat-messages --group <openConversationId> --page-all --page-limit 50 --format json
```

可附带非必填的 `--sender-query <姓名>`：未传时返回全部消息；唯一解析出
userId/openDingTalkId 后，按消息 `senderId` 筛选同一次读取结果，覆盖最终
`messages/count` 并返回 `resolvedFilters`。解析失败、不完整或存在歧义时抑制未过滤消息，
返回 `sender_resolution_failed`，不得把全量消息当作发送者筛选结果。

`--sender` 是姓名、userId 或 openDingTalkId 的混合入口。通讯录无法确认输入类型时，可按
原值 userId 精确过滤并交付命中消息，但结果必须保留 `identity_unverified`，不得把字符串命中
升级为已验证身份，也不得据此作完整否定结论。

```bash
dws chat +chat-messages --group "项目群" --sender-query "测试用户甲" --page-all --format json
```

时间范围使用公开可选的 `--start`、`--end`、`--order asc|desc`，兼容别名为
`--start-time/--end-time/--sort`。范围为 `[start,end)`；仅开始时间表示到本次执行当前时间；
仅结束时间只支持 `desc`，`asc` 必须提供开始时间。旧 `--time/--direction` 只用于兼容的
单边界模式，不能与范围模式混用。

```bash
dws chat +chat-messages --group <openConversationId> \
  --start "2026-08-01T00:00:00+08:00" --end "2026-08-02T00:00:00+08:00" \
  --order asc --page-all --format json
```

只需消息字段可判断的子集可在同一次调用中使用全局 `--jq`，但必须保留根信封并同步
改写 `messages/count`；发送者姓名仍用 `--sender-query` 解析稳定身份，不按展示名过滤。

要求导出时用 `--output <工作目录内相对.json>` 原子写入；需要资源时在读取命令上加
`--download-resources`，不要让 Agent 先输出全量 JSON 再手工遍历资源引用。

## 多维度搜索

- 关键词使用公开 `--query`。
- 已知稳定会话 ID 使用 `--group` / `--groups`；稳定发送者 ID 使用 `--senders`。
- 只有群名时使用 `--chat-query`，由 CLI 唯一解析会话。
- 只有发送者姓名时使用 `--sender-query`，由 CLI 唯一解析人员。
- 不传会话过滤时搜索全部会话；默认时间范围为最近 7 天。
- `--page-all` 只翻完当前时间范围内的游标页；精确范围使用成对的 `--start/--end`。
- `--order` 只稳定排列已经取得的结果；未全量或 `complete=false` 时不得称为完整范围全局排序。
- reaction 是消息原生谓词：使用 `--has-reactions --page-all`，不能与 `--no-enrich` 组合；CLI 在详情富化后过滤并返回 `reactionFilter` 证据。

```bash
dws chat +search-msg --chat-query "项目群" --sender-query "测试用户甲" --page-all --format json
dws chat +search-msg --chat-query "项目群" --query "发布计划" --page-all --format json
dws chat +search-msg --sender-query "测试用户甲" --page-all --format json
dws chat +search-msg --group "项目群" --has-reactions --page-all --format json
```

需要 Shortcut 未发布的原始过滤字段或响应时，才评估 `message search-advanced`。它支持
发送者、@对象、多个会话、消息类型、会话类型、机器人消息和时间范围，但不是默认入口。
至少提供一种真实过滤条件，完整遍历只有 `--page-all` 会触发。

## 其他查询

### 已知消息、@我与话题回复

- `+messages-mget --msg-ids <id...>`：最多 50 条；结果可直接用于回复、转发、撤回和资源下载。
- `+at-me [--group <群名或ID>] --page-all`：群内或跨全部会话查看 @我的消息。
- `+thread-replies --message-id <rootMessageId>`：自动只读解析 conversation/thread。
- `+thread-replies --group <cid> --thread-id <threadId>`：显式稳定上下文。

话题回复默认 `desc`；`asc` 必须与 `--page-all` 一起使用。自动续页使用下层毫秒级
`nextCursor`，不得使用只有秒精度的展示时间手工拼 continuation。检查 `complete`、
`hasMore`、`stopReason` 和 `failures`。

### Favorite、Pin 与 reaction 查询

| 任务 | 入口 |
|---|---|
| Favorite 列表 | `+flag-list`；要求全部时加 `--page-all`，页大小 1–30 |
| 消息 Pin 列表 | `message list-pin-msg --open-conversation-id <cid>` |
| 批量 reaction/文字回应 | `message list-emotion-replies --msg-ids <id...>` |
| 已读/未读状态 | `message read-status --group <cid> --message-id <id>` |

Favorite、消息 Pin、消息 Top 和会话 Top 是不同对象。写入或取消这些状态读取
[message-actions.md](message-actions.md)，这里只负责查询。

## 原子 fallback

| 原子命令 | 仅用于 |
|---|---|
| `message list` | 指定会话原始响应或显式手工 continuation |
| `message list-all` | 需要时间范围内全部会话的原始消息明细或手工 continuation；只要会话摘要使用 `+recent-conversations` |
| `message list-by-sender` | 已有稳定发送者 ID 且需要底层原始响应 |
| `message list-mentions` / `list-focused` | @我或特别关注的原始列表 |
| `message search` / `search-advanced` | Shortcut 未发布的真实过滤字段 |
| `message list-by-ids` | 已知消息 ID 的原始详情响应 |

Typed `chat message` 自动翻页只由 `--page-all` 触发；只传 `--page-limit`、`--max-items`
或 `--page-delay` 仍是单页。非第一页失败时保留 partial 结果、失败页和 continuation，不能
把 partial result 表述成完整成功。

## 完成与错误

- 查询必须检查 `complete`、`hasMore`、`stopReason`、`failures` 和下载 ledger。
- 发送者/群名零命中或多候选时停止，不选择第一项。
- `unknown flag` 时读取精确 leaf Help，修正后最多重试一次。
- 子消息优先使用自己的 `messageId`；只在缺会话 ID 时继承父消息的 `conversationId`。
- 查到真实消息后需要写操作时，使用 [message-actions.md](message-actions.md) 中的稳定 ID 规则。
