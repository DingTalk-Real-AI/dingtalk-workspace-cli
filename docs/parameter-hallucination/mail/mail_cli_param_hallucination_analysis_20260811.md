# DWS 邮箱 CLI 参数幻觉分析

## 1. 结论摘要

本轮只分析当前提交中的邮箱产品参数事实，没有使用历史 badcase、`dws-eval`、历史工作簿或固定 Schema 快照。分析基线为分支 `codex/fix-im-reliability`、提交 `e8ca78fe49f3a0aefd3d43f84687d299c67e3bd6`，二进制从该提交重新构建，Schema 来源为 `runtime-assembled`。

邮箱产品共有 **73 个官方可执行叶子命令**：其中 **46 个进入运行时 Schema**，另有 **27 个精确审核排除项**；73 个命令中包含 **10 个仓库内置 Shortcut**。可见本地业务参数共出现 **251 次**，形成 **49 个不同参数名**。46 个 Schema 工具与同提交真实 Help 的参数名集合逐项一致，差异为 **0**，因此当前主要风险不是 Schema 发布了错误 flag，而是同一产品内部的参数命名、业务角色和值域较复杂。

最突出的风险是：**31 个命令使用宽泛的 `--id` 或 `--ids`，但实际分别表示邮件 messageId、会话 conversationId、草稿 messageId、文件夹 ID、模板 ID、联系人 ID、规则 ID、标签 ID、召回任务 ID 或日历文件夹 ID**。模型从相邻命令迁移参数经验时，既可能生成当前命令不接受的更明确拼写，也可能把不同值域的 ID 原样传到错误命令。该问题不能通过一个全局 `id` concept 解决，必须按精确命令绑定和保护。

第二类风险来自邮箱业务角色：`--email` 在 63 个命令中通常表示操作所属邮箱，`--from` 在发送、回复和草稿动作中表示发件邮箱；模板创建、更新同时暴露 `--email` 和 `--from`，两者不能合并。收件人 `--to`、抄送人 `--cc` 又是逗号分隔邮箱地址文本，不能与钉钉用户 ID 或人员名称互转。

搜索和分页也存在明显分裂：5 个搜索入口分别使用 `query` 或 `keyword`，其中邮件搜索的 `query` 是完整 KQL，人员搜索的 `query/keyword` 才是普通关键词；16 个分页命令使用 `limit`、`size`、`cursor` 或 `offset`。数量同义名可以在精确命令内安全归一，但 KQL 条件拼装、字符串 cursor 与整数 offset 的互转都超出当前“只改参数名、不改参数值”的能力。

产品 Skill 覆盖了 **68/73** 个当前官方命令，遗漏 5 个命令：`mail calendar list`、`mail calendar-event list`、`mail mailbox shared-with-me`、`mail message export`、`mail message share-to-chat`。这属于公开契约文档缺项，应修改 Skill，不能伪装成别名能力。

本轮已在 `docs/parameter-hallucination/mail/param_concepts.json` 生成完整候选别名表。候选相对正式表新增 **9 个 concept**、扩展 **5 个已有 concept 的邮箱命令范围**、新增 **46 个邮箱 command override**、修改 2 个既有邮箱 override，并新增 36 条、移除 1 条 validation fixture。结构化复核确认：非邮箱 override、fixture 以及已有 concept 的非邮箱字段均未变化。候选规则已通过生成、CLI/PreParse、guard、生成确定性和 Schema policy；但 complete-command 最终 payload 门禁仍缺 **19 个命令、22 条 alias 的完整模板**，且需要删除或更新已失效的 `mail message search` 旧模板。因此该文件当前状态是：**规则设计合理、可以作为评审草稿，但补齐最终 payload 模板前不能直接替换正式别名表。**

## 2. 分析范围与事实来源

本轮逐项检查了：

- 当前提交重新构建的临时二进制及递归 `dws mail ... --help`；
- 同一二进制运行时组装的完整 Schema leaf；
- `NewSchemaSourceRootCommand()` 的官方命令树、27 个精确 Schema 排除项和 10 个内置 Shortcut；
- `skills/mono/references/products/mail.md`、邮箱最佳实践以及对应 multi Skill；
- `internal/helpers/mail.go` 与邮箱 Shortcut 实现，用于确认隐藏原生兼容 flag 和最终业务角色；
- 当前正式 `internal/cli/param_concepts.json` 及其生成、PreParse、保护和测试链路。

未纳入用户自定义 Shortcut、插件、历史实验、线上出现次数、模型通过率或底层 RPC 字段改名。

## 3. 参数问题

### 3.1 `id/ids` 在多个实体和值域间复用

25 个命令使用 `--id`，6 个命令使用 `--ids`。代表性差异包括：

- `mail message get --id`：内部邮件 messageId；
- `mail thread get --id`：邮件 conversationId/threadId；
- `mail folder delete --id`：邮件文件夹 ID；
- `mail template get --id`：模板 ID；
- `mail rule delete --id`：收件规则 ID；
- `mail sent-message recall-detail --id`：召回任务 ID；
- `mail calendar-event list --id`：邮箱日历文件夹 ID；
- `mail message batch-delete --ids`：邮件 messageId 列表；
- `mail thread batch-update --ids`：conversationId 列表。

风险不只是 unknown flag。若把 messageId、conversationId、internetMessageId 或 recall task ID 只按名称改写成 `id`，命令会接受参数名，却可能把错误值域送入后端。候选表因此没有创建全局邮件 ID concept，而是使用精确 command override、实体专用 concept、单复数 block 和 `attachment download --id` 的 ambiguous 提示。

### 3.2 邮件文件夹 ID 的参数名和角色不统一

10 个代表命令在 `folder`、`folder-id` 和 `id` 之间切换；其中 `folder` 始终要求 ID，不是文件夹名称，但又可能承担父文件夹、目标文件夹或列表筛选角色。

- `mail folder list`、`mail thread list` 已有隐藏原生 `--folder-id`；
- `mail message list` 以 `--folder-id` 为主，并原生接受隐藏 `--folder`；
- `mail folder create --folder` 表示父文件夹 ID；
- `mail message batch-move --folder` 表示目标文件夹 ID；
- `mail folder delete/update --id` 表示被操作文件夹 ID；
- `mail +folder-list/+recent-mail/+thread-list` 只暴露 `--folder`。

相同值可通过精确命令把 `folder-id` 归一到真实 flag；文件夹名称不能直接变成 ID，仍需先运行 folder list 查询。

### 3.3 所属邮箱、发件邮箱和收件人角色容易混淆

发送、回复、转发和草稿动作使用 `--from` 表示发件邮箱，但大量读取命令使用 `--email` 表示操作所属邮箱。对 7 个只暴露 `from` 的发送动作，`email → from` 可以按命令安全归一；模板 create/update 同时存在 `email` 和 `from`，必须保持两个角色，不能自动转换。

`--to`、`--cc` 是逗号分隔的邮箱地址文本。候选可以接受 `recipients/to-addresses` 和 `cc-recipients/cc-addresses` 等同值拼写，但会拦截 `user-id/user-ids` 和 BCC 拼写：前者值域不同，后者在当前 CLI 中没有真实目标 flag。

### 3.4 普通人员关键词与邮件 KQL 共用 `query` 名称

人员搜索的 `query/keyword` 是普通文本，可以在 `mail +find-mail-user`、`mail user search`、`mail +user-search` 间安全归一。邮件搜索的 `mail message search --query` 和 `mail +search-mail --query` 则要求完整 KQL，例如 `subject:"周报"`、`from:"alice"`；把 `--subject 周报` 只改成 `--query 周报` 会改变语义，不能视为等价。

正式表原有 `mail message search: subject → query` 因此是不安全的。本候选移除了该自动映射，改为在 dispatch 前提示必须编写完整 KQL。`mail +unread-mail` 内部固定 `isRead:false`，额外 query/keyword 也不应被别名表接受。

### 3.5 分页大小和翻页机制分裂

16 个命令涉及 `limit/size/cursor/offset`：

- 普通列表多以 `limit + cursor` 为主；
- `+search-mail`、`+unread-mail` 以 `size` 为真实参数；
- `mail mailbox shared-with-me` 使用 `limit + offset`，其中 offset 是整数，不是 opaque cursor；
- 部分原子命令已原生提供隐藏 `size/page-size`，这些保持原生，不重复依赖中央别名。

候选扩展已有 `pagination_size` 和 `page_cursor` 的精确命令范围。数量值和字符串 cursor 可原样传递；`cursor → offset`、页码计算或 token 解码不在当前能力范围，并对 shared-with-me 的 cursor 拼写进行保护。

### 3.6 标签、规则和操作参数具有角色或结构约束

标签 ID 同时出现在 `tag create --parent-id`、`tag delete/update --id`、`message batch-update --tags` 和 `thread update --tag-ids` 中。父标签、目标标签、单值和列表不能合并成一个无角色 `tag-id`。

规则 create/update 的 `conditions`、`actions` 是完整 JSON 数组。`subject/from/folder-id/tag-id` 等离散参数不能只通过改名变成 JSON；候选对这类输入进行安全拦截。`action` 等枚举值同样继续交给命令原生校验，不做值转换。

### 3.7 时间格式与字符串布尔值不能跨命令自动转换

`mail +thread-list`、`mail thread list` 和 `mail calendar-event list` 使用 UTC 时间范围；`mail auto-reply update` 使用 `YYYY/MM/DD HH:MM:SS +ZZZZ`。候选只在 `+thread-list` 内接受同格式的 `start-time/end-time` 名称，不跨格式转换。

`mail auto-reply update`、`mail rule create/update` 的 `enabled` 是字符串参数，而不是 Cobra bool flag。把缺少值的 `--enabled`、或任意 true/false 变体自动转成业务值，会越过当前别名链路的职责，因此保持原生校验。

### 3.8 Skill 漏写当前命令

Help 与 Schema 的公开 flag 集合没有差异，但 Skill 漏写 5 个官方命令。该问题会让模型无法发现命令或套用相邻命令参数，应更新 Skill 命令树和参数说明；别名表不能创建缺失的文档事实。

## 4. 当前别名表可以实施的方案

第一轮建议实施以下治理动作：

1. 用命令级 alias 明确邮件 messageId、conversationId、文件夹 ID 和日历文件夹 ID 的真实 `id/ids` 角色；不同值域和单复数做 block，双 ID 角色的 attachment download 对裸 `id` 提示 ambiguous。
2. 新增 `mail_template_id`、`mail_contact_id(s)`、`mail_rule_id`、`mail_tag_id(s)`、`mail_recall_task_id` 等精确 concept，只覆盖同实体、同角色、同单复数的命令。
3. 新增 `mail_to_recipients`、`mail_cc_recipients`，仅处理值可原样传递的邮箱地址列表拼写；不支持 BCC、人员名称或钉钉 userId 转换。
4. 扩展 `search_query`、`pagination_size`、`page_cursor`、`time_start/time_end` 的邮箱精确命令范围，不改变这些 concept 的非邮箱字段。
5. 删除不安全的 `mail message search --subject → --query`，改为 KQL 组合提示和 dispatch 前拦截。
6. 已存在的隐藏原生兼容参数继续保持原生行为，不用中央 alias 重复包装。

## 5. 当前能力支持不了或不应该做的事项

- 文件夹名、标签名、联系人名自动查询并转换为 ID；
- 人员名称、staffId、钉钉 userId 自动转换为邮箱地址列表；
- 把 `--subject/--sender/--date/--folder-id` 组合成一个 KQL `--query` 值；
- 把离散规则参数组合成 `conditions/actions` JSON 数组；
- 字符串 cursor、整数 offset 和页码之间的换算；
- 不同命令时间格式之间的转换；
- 将单值自动扩展为列表，或把列表静默取一个元素；
- 把 internal messageId 与 internetMessageId、conversationId、recall task ID 互转；
- 自动创建当前 CLI 不存在的 BCC flag，或把附件 CSV 与 repeatable flag 结构互转。

这些事项不阻塞第一轮“参数名归一和危险值域保护”，但不能为了提高覆盖率强行写进 alias。

## 6. 候选别名表修改与审核

候选文件基于正式表完整复制后修改，正式 `internal/cli/param_concepts.json` 未被替换。

结构化 diff：

- concepts：31 → 40，新增 9 个邮箱专用 concept；
- 扩展 5 个已有 concept，仅追加邮箱精确命令范围；
- command overrides：129 → 175，新增 46 个邮箱 override，修改 2 个既有邮箱 override；
- validation fixtures：253 → 288，新增 36 条，移除 1 条不安全 subject→query fixture；
- 非邮箱 override 和 fixture 变化：0；
- 已有 concept 的非邮箱命令范围、成员、排除项和语义字段变化：0。

审核中曾发现复用 `folder_id` 并改写其说明/排除项会影响已有 Drive 范围，已收敛为邮箱命令级映射；最终草稿不改变 Drive 规则。邮件 messageId 和 thread conversationId 也没有复用 IM 的 `message-id/conversation-id` concept，而是限定在邮箱精确命令，避免值域跨产品合并。

## 7. 候选草稿验证结果

候选在独立分析副本 `/private/tmp/dws-mail-param-candidate.OoNKy0` 中临时作为正式输入验证，主工作区正式表未变。

已通过：

- JSON 解析和 `go generate ./internal/cli`；
- `internal/cli`、`internal/pipeline` 测试；
- validation fixture 经过最终 embedded delivery；
- 全量 reviewed guard 在 runtime contract/dispatch 前停止；
- generated drift、组装确定性和 Schema catalog policy；
- 非邮箱配置的结构化 diff 隔离检查。

尚未通过：

- complete-command 最终 payload 等价门禁缺少 19 个邮箱命令、22 条 alias 的完整模板；
- 旧的 `mail message search` payload 模板仍指向已移除的 subject alias，需要删除或改成 KQL block 模板。

缺模板的命令包括：`mail +user-search`、`mail +search-mail`、`mail +contact-list`、`mail +thread-list`、`mail +recent-mail`、`mail message get`、`mail attachment download`、`mail message batch-delete`、`mail thread get`、`mail thread batch-update`、`mail template get`、`mail contact update`、`mail contact batch-delete`、`mail rule delete`、`mail tag delete`、`mail message batch-update`、`mail sent-message recall-detail`、`mail message send`、`mail calendar-event list`。

所以候选不能表述为“全部测试通过、可直接替换”。正式落地前需补齐模板，证明 alias 与 canonical 输入进入相同最终 payload；写操作继续使用 dry-run 或 injected Runner。

## 8. 第一轮改造建议

1. 先合入本候选中的精确 ID/列表、收发件人、分页和 KQL 保护规则，但同步补全 19 个命令的最终 payload 模板。
2. 删除或更新 `mail message search` 的旧 subject alias 代码测试，确保自动转换不再掩盖 KQL 语义。
3. 更新邮箱 Skill，补入 5 个遗漏命令，并明确 messageId、conversationId、internetMessageId、folderId 和 recall task ID 的来源。
4. 对名称到 ID、KQL/JSON 组合、值格式转换继续保持显式“不支持”，不要扩大中央 PreParse 的职责。

## 9. 可复用到其他产品的流程

后续产品继续按同一顺序执行：从目标提交重建二进制；合并官方命令树、运行时 Schema、排除项和内置 Shortcut；逐命令对账 Help/Schema/Skill；按实体、角色、值域和单复数归并问题；再映射到 concept、命令级 alias、block、ambiguous、保持原生或当前不支持；最后生成完整候选表，在隔离副本中运行生成、最终 argv/payload、保护和非目标回归验证。
