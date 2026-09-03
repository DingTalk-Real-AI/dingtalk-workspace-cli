# group-discovery：群发现、列表与成员读取

> 返回入口：[DingTalk Chat Skill](../../SKILL.md)

用于只读的群列表、群搜索、共同群、群成员、群机器人和邀请链接。建群、改群、成员增删、
邀请卡片分享、公告、禁言和其他群管理写操作读取 [group-admin.md](group-admin.md)。

## 入口选择

| 用户终点 | 唯一推荐入口 |
|---|---|
| 我加入的全部群 | `dws chat +my-groups --page-all` |
| 我创建或管理的群 | `dws chat +chat-list-mine` |
| 只看群主群或管理员群 | `+chat-list-mine --role OWNER|ADMIN` |
| 按关键词搜索群 | `dws chat +chat-search --query <关键词>` |
| 查看指定群全部成员 | `dws chat +chat-members-list --group <群名或ID>` |
| 已知成员 openDingTalkId 批量查群内详情 | `dws chat +chat-members-get --id <cid> --users <ids>` |
| 获取群邀请链接 | `dws chat +chat-invite-url --group <群名或ID>` |
| 查看群机器人 | `dws chat +chat-bots --group <群名或ID>` |

“全部群”与“全部会话”不同：`+my-groups` 只列当前用户加入的群；
`+conversation-list --page-all` 可能同时包含单聊和群聊，不能替代群成员关系。

## 群列表、分页与角色

`+my-groups` 返回当前用户加入的群，包括作为群主、管理员和普通成员加入的群：

- 要求完整列表时使用 `--page-all`；Runtime 沿真实 `nextCursor` 读取后续页，并按
  `openConversationId` 合并去重，读完后再应用可选 `--type` 本地过滤。
- `--limit` 是每页数量，不是最终结果上限；`--cursor` 只用于从已有 `nextCursor` 手工续读。
- `--page-limit` 只与 `--page-all` 一起使用，用于限制最多读取页数。达到上限后仍有下一页时，
  结果不完整。
- 只有 `complete=true` 且 `hasMore=false` 才能声称已经读取全部；否则保留 `nextCursor`、
  `stopReason` 和 `failures` 并说明结果不完整。

```bash
dws chat +my-groups --page-all --page-limit 50 --format json
```

`+chat-list-mine` 只返回当前用户作为群主或管理员的群。只要群集合时不传 `--role`，
一次取得 OWNER 和 ADMIN。要求逐项标明身份时，直接分别查询 `--role OWNER` 和
`--role ADMIN`，不先执行无角色查询或读取 Help；按 `openConversationId` 合并去重后，
再应用一次全局数量上限，不得把两个分支直接拼接。

```bash
dws chat +chat-list-mine --limit 20 --format json
dws chat +chat-list-mine --role OWNER --format json
dws chat +chat-list-mine --role ADMIN --exclude-muted --format json
```

`+my-groups` 不提供当前用户角色。用户明确要求普通成员群时，使用
`chat group list-all --limit 200`；返回 `hasMore=true` 时，必须把真实 `nextCursor`
传给下一次调用并继续读取，直到 `hasMore=false`，不得把继续翻页交给用户。读完后按
`openConversationId` 去重，仅筛选真实返回的 `myRole=普通成员`；不得给 `+my-groups`
编造 `--role MEMBER`，也不得用“全部群减去 OWNER/ADMIN 群”推断。

## 群搜索与稳定 ID

群搜索默认使用 `+chat-search`。要求全部候选时加 `--page-all`；可用
`--page-size/--page-token` 或兼容 `--limit/--cursor`。零命中或多候选时停止并展示候选，
不要选择第一项。

```bash
dws chat +chat-search --query "项目冲刺" --page-all --format json
```

只有数字群号时，使用 `chat group get-by-group-id --group-id <数字>` 转换为
`openConversationId`。需要搜索共同群时使用原子 `chat search-common`；`AND` 表示所有人
都在群里，`OR` 表示任一人在群里。自然人员必须先解析为当前 profile 的真实身份。

```bash
dws chat search-common --nicks "测试用户甲,测试用户乙" --match-mode AND --limit 20 --cursor 0
```

## 群成员

`+chat-members-list` 接受群名或 `openConversationId`，唯一解析后全量读取，并把用户与机器人
分桶。结果必须检查 `buckets/complete/failures`。

```bash
dws chat +chat-members-list --group "项目群" --format json
dws chat +chat-members-list --conversation-id <openConversationId> --format json
```

先检查 `+chat-members-list` 的稳定结果。只有结果未包含用户要求的群昵称、角色或其他群内字段时，
才使用其中的真实 `openDingTalkId` 批量调用：

```bash
dws chat +chat-members-get --id <openConversationId> \
  --users <openDingTalkId1>,<openDingTalkId2> --format json
```

不要为了群内详情默认切换到企业通讯录；只有用户明确要求部门、岗位、直属主管等企业资料时，
才把真实 userId 交给 `dingtalk-contact`。

## 邀请链接与机器人

`+chat-invite-url` 是只读获取链接，可选 `--expires-seconds`；`group share-invite` 会实际把
邀请卡片发送给另一个会话或用户，属于 [group-admin.md](group-admin.md)。

`+chat-bots` 返回稳定 `bots[]` 和 `openBotId`，供后续移除。搜索可用机器人、机器人发送和
撤回读取 [chat-bot.md](chat-bot.md)。

## 原子 fallback

| 原子命令 | 仅用于 |
|---|---|
| `chat search` / `search-common` | Shortcut 未发布的搜索字段或共同群 |
| `chat group get-by-group-id` | 数字群号转换 |
| `chat group members` | 需要原始成员分页响应 |
| `chat group members list-by-ids` | 需要原始批量成员详情 |
| `chat group list-all` / `list-my-groups` | 需要 Shortcut 未投影的真实底层字段 |

使用原子 fallback 前读取精确 leaf Schema；不得把 fallback 写成与 Shortcut 并列的默认路线。

## 完成与错误

- 分页完成只以真实 `complete/hasMore/nextCursor/failures` 判断，不看过滤后的 `count` 猜测。
- 所有稳定 ID 必须来自同一 profile 的真实返回。
- 找不到群或出现多候选时停止，不臆测 `openConversationId`。
- 任务从只读发现转为写操作时，使用 [group-admin.md](group-admin.md) 的目标和安全规则。
