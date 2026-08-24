# IM 51 个增量 Shortcut 参数幻觉静态审计

审计日期：2026-08-05
审计基线：`fix/param-hallucination` 当前工作树，HEAD `b1d422dc`（已包含当时最新 `main`）
审计方法：`specs/product-cli-param-hallucination-analysis-spec.md`

## 结论

1. 当前主分支的 51 个目标 Shortcut 均是可执行 Cobra leaf，也都能从最终 Runtime Schema 解析到；51/51 的公开 Cobra 参数与 Schema 参数完全一致。
2. 这 51 个命令当前都没有生成的 `param_concepts` 兜底项，即直接覆盖为 0/51。已有 concept 定义可以复用，但 `commands` 范围尚未包含这批 Shortcut。
3. 51 个命令中，Chat Skill 只精确点名 8 个，另外 43 个没有逐条展开。根 Skill 明确把完整低频清单交给 Runtime Schema/Catalog，并提供 `dws shortcut list --service chat` 最后回退，因此这不是 43 个公开契约缺失；它说明低频选路高度依赖正确的 leaf 发现。
4. 四个命令已经存在隐藏的原生兼容参数，不应再伪装成中央别名能力：
   - `+chat-members-list`：`--chat`、`--open-conversation-id`
   - `+messages-list`：`--conversation-id`、`--id`、`--size`
   - `+messages-recall`：`--chat`、`--group`、`--id`、`--message-id`、`--message-ids`
   - `+messages-resource-url`：`--msg-id`、`--open-message-id`
5. 可安全落到现有别名表能力的主要是“同实体、同角色、同基数、同值域、值原样传递”的精确命令级映射。数字 groupId/CID 转换、群名解析、单复数转换、源/目标角色选择、`before → time + direction` 和 `page-all` 翻页都不应由别名表完成。

## 八类静态问题

| 问题 | 影响 | 结论 |
|---|---:|---|
| 会话 ID 参数名碎片化且 `--group` 语义漂移 | 36 个命令表现 | 单角色稳定 CID 可扩展 `open_conversation_id`；群名/CID 混合与双角色必须 scoped/block |
| 数字群号与 openConversationId 值域冲突 | 1 个命令 | `+chat-get-by-id --group-id` 不得接收 CID 别名，只能拦截并提示换命令/先转换 |
| 消息 ID 名称、角色与 processQueryKey 混杂 | 13 个命令表现 | 普通消息单角色可归一；ref/src/keys 必须保留角色或隔离 |
| 用户 ID 值域、角色和单复数混杂 | 8 个命令表现 | mixed-ID 参数可命令级绑定；applicant/inviter 等多角色不能猜 |
| robotCode 与 openBotId 混淆 | 6 个命令表现 | 扩展两个独立 concept，并双向 block 交叉拼法 |
| 单值/列表与分页口径混用 | 5 个命令表现 | 只落严格等价项；单复数、page/cursor、count/size 不自动互转 |
| 时间、方向与全量能力不能单 flag 改写 | 1 个命令 | `start→time` 可审；`before/page-all` 超出现有能力 |
| 低频能力依赖 Schema 发现 | 全部 51 个 | 补选择与回归，不把 51 条完整参数复制进根 Skill |

完整逐命令事实、问题明细和不可解决项见静态审计工作簿。

## 推荐的参数兜底设计

### 1. 扩展已有会话 ID concept

把审核后只有一个稳定 CID 角色的命令加入 `open_conversation_id.commands`，包括：

- 会话分类增加/移除会话；
- 加机器人、移机器人、按 ID 查成员；
- 群审批、禁言、退出、身份移除、转让群主、头像和设置；
- 会话清消息、清红点、隐藏、已读/未读、免打扰；
- 消息标记、emoji/文字表情、机器人群发/撤回、资源、Pin/Top。

真实参数是宽泛 `--id` 的命令使用 `command_overrides.bind` 明确绑定，不把 `id` 加入全局 concept。

### 2. 对混合 `--group` 做命令级处理

- `+chat-members-list`：保留原生 `--group` 的群名/CID resolver；只增加 `chat-id/id → conversation-id`，`--query` 不是成员过滤能力，需 block/unsupported。
- `+chat-update`：`conversation-id/open-conversation-id/chat-id → group` 可以原样传 CID；`id` 过于宽泛，建议先 block，除非增加 CID 本地值域校验。`title/new-title → name` 可作为同命令内严格等价的名称字段。

### 3. 严格隔离数字群号

`+chat-get-by-id --group-id` 的值是数字 groupId。建议对 `group`、`conversation-id`、`chat-id`、`open-conversation-id`、`id` 全部配置 block。系统只能告诉调用方：传数字群号，或改用接受 openConversationId 的 leaf；不能自动查询转换。

### 4. 扩展消息 ID concepts，保留角色

- 普通单消息角色：将会话已读边界、emoji、资源、Pin/Top 等命令纳入 `open_message_id`。
- 消息列表：单值与 `open_message_ids` 分开，不启用全局单复数规则。
- 引用回复：`+messages-reply` 可做 `msg-id/open-message-id → ref-msg-id`，但不把引用角色抹成全局普通消息 ID。
- 转发：只在精确命令中将角色明确的拼法映射到 `msg-id/src-msg-id/msg-ids`；存在源/目标双会话时，generic `chat-id` 返回 ambiguous。
- 机器人撤回：`keys` 是 processQueryKey，不是 openMessageId；block 所有 message-id 拼法。

### 5. 用户与机器人值域保护

- `+chat-members-get --users` 只接受 openDingTalkId 列表，可 bind 到 `open_dingtalk_ids`，并将 `open-dingtalk-ids → users`。
- mixed-ID 列表或单值只做命令级 scoped alias，不提升为全局等价。
- `applicant/inviter`、成员/群主、发送者/接收者等不同业务角色保持隔离；generic `user-id` 有多个目标时返回 ambiguous。
- `robot-code` 与 `bot-id(openBotId)` 各自扩展命令范围，并在对方命令 block。

### 6. 只落严格等价的分页和时间项

- `+flag-list`：`limit → size` 可以命令级归一；`max/max-results/max-size` 不足以证明是“每页数量”。
- `+messages-list`：`start → time` 可以原样传；`before/before-time/end` 需要方向或边界重写，`page-all` 需要循环，均 block 并提示使用 `+chat-messages`。
- `+chat-list` 已真实支持 `page-size/limit` 与 `page-token/cursor`，保持原生即可。

## 明确不做的自动兜底

- 群名查询成 openConversationId；
- 数字 groupId 与 CID 互转；
- 单值与列表拆分/合并；
- `before` 生成 `time + direction older`；
- `page-all` 自动翻页并伪造完整性；
- 在 applicant/inviter、src/dest 等多个角色中猜一个；
- 把机器人 processQueryKey 当 openMessageId；
- 把资源输出文件与下载目录互换。

## 版本边界

实验目录里确认过提交的二进制不是严格的 51/51：`b8b55834`、`ee943d9b`、`f050fbde` 的真实 Cobra 面都只含 50 个目标 Shortcut，缺少 `chat +chat-list`。`+chat-list` 因此只有当前静态审计证据，没有对应实验 badcase 证据；后续要用合成 CLI/单元测试覆盖，不能把它计入“实验已验证”。

## 验证要求

1. 每条 alias 验证来源不是命令真实 flag、目标是真实 flag、值原样传递。
2. 对每个 concept 验证单值/列表、userId/openDingTalkId、robotCode/openBotId、groupId/CID 不交叉。
3. 写命令验证归一化前后目标对象、消息角色、确认门禁和最终参数组装不变。
4. 原生隐藏兼容 flag 直接走 Cobra，不经中央别名二次改写。
5. 为 block/ambiguous 断言错误在本地发生且给出正确 leaf/参数提示。
6. 对 51 个 leaf 跑 Schema/Help 一致性、生成漂移和全部静态 fixture。

## 事实来源

- `app.NewRootCommand()` 构建的当前 Cobra 命令树和 flag；
- `DeliverySchemaAllPayloadForTest()` 生成的最终 Runtime Schema；
- `skills/multi/dingtalk-chat`；
- `internal/cli/param_concepts.json` 与生成 lookup；
- `internal/cli/command_path_fallbacks.json`；
- 具体 Shortcut 的声明、约束与实现。
