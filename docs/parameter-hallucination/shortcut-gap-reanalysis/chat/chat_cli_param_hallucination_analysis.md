# Chat CLI 参数幻觉复核报告

## 1. 结论

冻结基线为 `origin/main@f4474b57eb1db23b1638b9574be2f5dca368a360`。本轮覆盖 18 个此前未进入正式参数兜底范围的公开 Shortcut；识别出 2 个可直接自动兜底、14 个只能部分兜底、2 个只应增加保护的命令，其中高风险写入或敏感读取 8 个。

本报告中的修订候选已合入正式 `internal/cli/param_concepts.json`，并重放到最新 main。所有映射均满足：源参数不是该命令真实 flag、目标是该命令真实 flag、实体/角色/值域/基数一致、值原样传递、不绕过确认。

## 2. 参数问题明细

| # | 命令 | 真实参数焦点 | 幻觉风险 | 候选处理 | 落地能力 | 风险 |
|---:|---|---|---|---|---|---|
| 1 | `chat +conversation-list` | `limit,cursor` | 整数 cursor 与 token/page 模型不等价 | limit concept；next-cursor 命令级映射；token/page 阻断 | 部分兜底 | 中 |
| 2 | `chat +conversation-list-top` | `limit,cursor,type` | 整数 cursor + 本地会话类型枚举 | 分页有限兜底；泛化 conversation-type 保持歧义 | 部分兜底 | 中 |
| 3 | `chat +chat-search` | `query,limit,page-size,cursor,page-token` | 两组公开同义 real flags 并存，额外 generic alias 无法选 canonical | query 可兜底；size/next-cursor 等设歧义 | 部分兜底 | 中 |
| 4 | `chat +chat-list-mine` | `role,limit` | limit 是总量上限且没有 cursor；role 是枚举 | 仅 `max-results` / `max-result` 归一到总量上限；分页式命名阻断，generic size/type 保持歧义 | 部分兜底 | 中 |
| 5 | `chat +chat-list-all` | `limit,cursor` | opaque cursor 与页码/offset 冲突 | 复用 pagination_size/page_cursor；page/offset 阻断 | 可自动兜底 | 低 |
| 6 | `chat +chat-list-join-requests` | `limit,cursor` | opaque cursor 与页码/offset 冲突 | 复用 pagination_size/page_cursor；page/offset 阻断 | 可自动兜底 | 低 |
| 7 | `chat +chat-list` | `types,page-size,limit,page-token,cursor` | 两组真实兼容参数并存；types 是列表枚举 | 不再造第三 canonical；generic size/cursor/type 歧义 | 仅保护 | 中 |
| 8 | `chat +messages-recall` | `conversation-id,msg-id` | 会话 ID、消息 ID、发送 openTaskId、threadId 角色冲突 | 复用 open conversation/message concepts；其他 ID 域阻断 | 部分兜底 | 高 |
| 9 | `chat +messages-query-send-status` | `open-task-id` | openTaskId 不是 openMessageId，也不是 Todo taskId | 不新建单命令 concept；错误 ID 域阻断 | 仅保护 | 高 |
| 10 | `chat +messages-create-text-emotion` | `emotion-name,text,background-id` | 创建名称/正文与既有 emotionId/backgroundId 易混 | 正文 concept + 名称角色别名；existing ID 阻断 | 部分兜底 | 中 |
| 11 | `chat +messages-send-card` | `group,receiver,receiver-open-dingtalk-id,at-open-dingtalk-ids,content,flow-status` | 三个目标域 + @列表 + 卡片状态并存 | ID concept 仅绑定同值域；openDingTalkId 只允许角色完整别名 | 部分兜底 | 高 |
| 12 | `chat +messages-update-card` | `biz-id,content,flow-status` | 卡片 bizId/状态不是消息 ID/投递状态 | 正文可兜底；卡片角色别名；错误 ID 阻断 | 部分兜底 | 高 |
| 13 | `chat +messages-send` | `identity,group,groups,user,users,open-dingtalk-id,open-dingtalk-ids,robot-code,text,markdown,media-id,file,uuid,@参数` | 身份能力矩阵包含多目标域、多基数、多内容类型 | 只映射明确 user/robot/CID 角色；generic content/recipient/at-ids 歧义 | 部分兜底 | 高 |
| 14 | `chat +at-me` | `group,days,limit,cursor,output-dir` | group 可为 CID 或名称；当前用户是隐式目标 | CID/群名原值映射；用户 ID 歧义；分页复用 | 部分兜底 | 中 |
| 15 | `chat +broadcast` | `to(list names),content` | 自然姓名列表不是 userId/openDingTalkId 列表 | 仅姓名列表角色别名；稳定 ID 阻断 | 部分兜底 | 高 |
| 16 | `chat +dm` | `to(single name),content` | 自然姓名单值不是稳定 ID | 仅姓名角色别名；稳定 ID 阻断 | 部分兜底 | 高 |
| 17 | `chat +my-groups` | `type,limit,cursor` | type 是客户端过滤；opaque cursor 与页码不同 | 分页 concept；页码阻断；group-type 歧义 | 部分兜底 | 中 |
| 18 | `chat +thread-replies` | `group,message-id,thread-id,topic-id,limit,page-size,order,sort,time` | 会话/主消息/thread 三类 ID；两组真实兼容参数并存 | 会话/消息 concept；plural/task ID 阻断；generic size/id 歧义 | 部分兜底 | 高 |

## 3. 可直接解决的方案

### 3.1 Concept 变更

| Concept | 处理 | 命令范围 |
|---|---|---|
| `search_query` | 扩展既有 concept 命令范围 | `chat +chat-search` |
| `pagination_size` | 扩展既有 concept 命令范围 | `chat +conversation-list`、`chat +conversation-list-top`、`chat +chat-list-all`、`chat +chat-list-join-requests`、`chat +at-me`、`chat +my-groups` |
| `page_cursor` | 扩展既有 concept 命令范围 | `chat +chat-list-all`、`chat +chat-list-join-requests`、`chat +at-me`、`chat +my-groups` |
| `content_text` | 扩展既有 concept 命令范围 | `chat +messages-create-text-emotion`、`chat +messages-send-card`、`chat +messages-update-card`、`chat +broadcast`、`chat +dm` |
| `open_conversation_id` | 扩展既有 concept 命令范围 | `chat +messages-recall`、`chat +messages-send-card`、`chat +thread-replies` |
| `open_message_id` | 扩展既有 concept 命令范围 | `chat +messages-recall`、`chat +thread-replies` |
| `user_id` | 扩展既有 concept 命令范围 | `chat +messages-send-card`、`chat +messages-send` |
| `user_ids` | 扩展既有 concept 命令范围 | `chat +messages-send` |
| `robot_code` | 扩展既有 concept 命令范围 | `chat +messages-send` |

### 3.2 命令级 Override

| 命令 | 处理原则 |
|---|---|
| `chat +conversation-list` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +conversation-list-top` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +chat-search` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +chat-list-mine` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +chat-list-all` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +chat-list-join-requests` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +chat-list` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +messages-recall` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +messages-query-send-status` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +messages-create-text-emotion` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +messages-send-card` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +messages-update-card` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +messages-send` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +at-me` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +broadcast` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +dm` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +my-groups` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `chat +thread-replies` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |

正式规则新增 67 条 PreParse 验证 fixture，覆盖 alias/canonical、block 与 ambiguous 三类路径。完整命令模板位于 `internal/app/param_alias_payload_equivalence_test.go`。

## 4. 当前不能自动解决

- `chat +conversation-list`：整数 cursor 与 token/page 模型不等价。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +conversation-list-top`：整数 cursor + 本地会话类型枚举。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +chat-search`：两组公开同义 real flags 并存，额外 generic alias 无法选 canonical。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +chat-list-mine`：limit 是总量上限且没有 cursor；仅角色完整的 `max-results` / `max-result` 保留原值映射，`page-size` / `per-page` 阻断，generic `size` / `take` / `top` 保持歧义。
- `chat +chat-list`：两组真实兼容参数并存；types 是列表枚举。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +messages-recall`：会话 ID、消息 ID、发送 openTaskId、threadId 角色冲突。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +messages-query-send-status`：openTaskId 不是 openMessageId，也不是 Todo taskId。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +messages-create-text-emotion`：创建名称/正文与既有 emotionId/backgroundId 易混。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +messages-send-card`：三个目标域 + @列表 + 卡片状态并存。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +messages-update-card`：卡片 bizId/状态不是消息 ID/投递状态。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +messages-send`：身份能力矩阵包含多目标域、多基数、多内容类型。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +at-me`：group 可为 CID 或名称；当前用户是隐式目标。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +broadcast`：自然姓名列表不是 userId/openDingTalkId 列表。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +dm`：自然姓名单值不是稳定 ID。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +my-groups`：type 是客户端过滤；opaque cursor 与页码不同。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `chat +thread-replies`：会话/主消息/thread 三类 ID；两组真实兼容参数并存。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。

## 5. 第一轮推荐

1. 先合入值域明确、值不变的 concept/命令级别名。
2. 对 ID 域、单复数、两种真实兼容 flag 并存、文件传输编码和多角色参数优先保留 block/ambiguous。
3. 不通过别名表实现名称查 ID、URL 拼装、裸文件路径补 `@`、枚举值翻译或分页模型转换。
4. 写操作的确认语义由真实命令负责，候选 fixture 不注入 `--yes`；测试模板另行验证移除 `--yes` 后仍停在确认门禁。

## 6. 候选表差异与实体必要性审计

- 变更 concept：`search_query`、`pagination_size`、`page_cursor`、`content_text`、`open_conversation_id`、`open_message_id`、`user_id`、`user_ids`、`robot_code`。
- 新增 command override：18 条。
- 新增 fixture：67 条。
- 新 concept 仅在同一实体跨多个命令重复出现时建立；单命令或单角色字段全部下沉到 command override。
- 没有新增 role-free 的 `id`、`name`、`status`、`config`、`content` concept。

完整候选见同目录 `param_concepts.json`。

## 7. 事实依据与验证门禁

事实优先级：同提交 Help/Cobra > 同提交运行时 Schema > 产品 Skill > 当前正式别名表。

- Help/Cobra：冻结提交重新构建的 `./dws <command> --help`。
- Runtime Schema：同一二进制 `./dws schema --cli-path <path> --compact -f json`。
- Skill：`skills/multi` 下对应产品 reference。
- 正式表：评审候选基于冻结基线完整复制；当前分支已将修订候选合入 `internal/cli/param_concepts.json`。
- 已通过：JSON/Schema、`go generate ./internal/cli`、全部既有与新增 PreParse fixture、alias/canonical、block/ambiguous、candidate-only 完整命令模板、移除 `--yes` 的确认不可绕过测试、非目标别名回归、`check-generated-drift.sh`、`check-schema-catalog.sh`。
- 评审阶段在隔离副本验证；正式落地阶段重新生成并保留 main 的 Chat thread 命令迁移。

## 8. 可复用流程

1. 冻结 commit、构建二进制并导出命令/Help/Schema 证据。
2. 对每个参数标注实体、值域、角色、基数、真实/兼容/不可用状态。
3. 先判断是否已有可复用 concept；只在跨命令重复实体时新增 concept。
4. 角色局部差异下沉到 command override；需要查找/转换/扩展时拒绝 alias。
5. 为正向 alias、canonical 不变、block、ambiguous 与非目标命令补 fixture 和完整命令模板。
6. 在隔离副本验证候选；评审通过后合入正式表、重新生成并合并最新 main。
