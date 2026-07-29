# IM / Chat 产品对齐修改说明

## 1. 文档信息

| 项目 | 内容 |
|---|---|
| 修改分支 | `codex/im-chat-align` |
| 基线提交 | `6ab01a36` |
| GitHub 仓库 | `DingTalk-Real-AI/dingtalk-workspace-cli` | |
| Hint 参考记录 | `/Users/fuyongchang.fyc/Desktop/dws/dws_optimization_record_im.xlsx` |
| 当前状态 | 工作区已修改，尚未提交 |

## 2. 原始目标

本次修改聚焦 `multi` 模式下的 IM / Chat 产品，目标包括：

1. 将 Chat Skill 改为树结构，参考 `nanrun` 的组织方式。
2. 对齐 Chat 的 Skill、reference 和 scripts。
3. 参考 Excel 与 `nanrun` 修改运行时 Hint 和执行前 Guidance。
4. 将 Schema 暴露能力、Agent Selection、Safety Metadata 与 Skill / Cobra 实际能力对齐。

## 3. 实现原则

### 3.1 当前 Cobra 是命令事实来源

所有命令路径和 flag 均以当前主干 Cobra 树为准。没有直接复制 `nanrun` 中已经过时的路径或参数。

本次校正的典型差异包括：

- 群成员管理使用 `--id <openConversationId>`。
- 退群、解散群、转让群主使用 `--group <openConversationId>`。
- 只有解散群需要显式确认参数 `--yes`。
- 共同群的公开规范路径为 `dws chat search-common`。
- 单聊历史脚本使用 `dws chat message list-direct`。

### 3.2 Hint 保持单一文案源

`nanrun` 在 `internal/helpers/chat.go` 中维护独立的
`chatAgentGuidance` map。当前主干已经采用统一的 Schema Metadata 架构：

```text
schema_hints/selection/chat.json
        │
        ▼
生成 Agent Metadata / Catalog
        │
        ▼
ResolveMeta(cliPath)
        │
        ▼
Chat --help 运行时 Guidance
```

因此没有把 `chatAgentGuidance` map 原样复制进来，避免 Selection JSON 和
Go map 形成两套可能漂移的 Hint 文案。

### 3.3 生成物只通过生成器更新

`schema_catalog/` 和 `schema_agent_metadata/` 均为生成物，没有手工编辑。
本次先修改 Registry、Metadata 和 Selection 等源输入，再执行 Schema
生成流程。

### 3.4 奥卡姆剃刀边界

- 不修改无关产品。
- 不新增重复的 Chat 命令实现。
- 不改变 Schema JSON wire format。
- 不复制已下线的 `extract_media_id.py`。
- 不恢复已经退役的 `chat media upload` 真实上传行为。
- Go Hint 只补运行时消费、结构化恢复建议和必要的本地校验。

## 4. Skill 修改

### 4.1 意图表

修改文件：

- `skills/multi/dingtalk-chat/SKILL.md`

意图表新增或强化以下路由：

| 用户意图 | 路由 |
|---|---|
| 收藏或取消收藏消息 | `message add-favorite` / `remove-favorite` |
| 查看收藏消息 | `message list-favorites` |
| 编辑已发送消息 | `message edit` |
| 普通群升级外部群 | `group upgrade-to-external` |
| 清除当前用户群昵称 | `group update-nick`，省略 `--nick` |
| 按会话反查分组 | `category list-by-conv` |
| 批量查询分组 | `category batch-info` |
| 导出群聊消息 | `chat_export_messages.py` |
| 查询单聊历史 | `chat_history_with_user.py` |
| 机器人多群群发 | `bot_broadcast.py` |
| 按发送者查消息 | `message list-by-sender` |
| 查询特别关注消息 | `message list-focused` |
| 查询共同群 | `chat search-common` |
| 退群 / 解散群 | `group quit` / `group dismiss` |
| 转让群主 | `group transfer-owner` |
| 群公告 | `group notice create/edit/get/list` |
| 群身份 | `group-role` 分支 |
| 消息已读状态 | `message read-status` |
| 简单关键词搜索 | `message search` |
| 组合条件搜索 | `message search-advanced` |
| 回复、转发、合并转发 | `reply/forward/combine-forward/forward-topic` |
| 置顶、钉住消息 | `set-top-msg/set-pin-msg` 及取消命令 |
| 会话状态 | `mute/hide/mark-read/mark-unread` |

### 4.2 命令树

当前命令树如下：

```text
dingtalk-chat
├── message
│   ├── send / send-by-bot / send-by-webhook
│   ├── send-card → update-card
│   ├── edit / recall / recall-by-bot
│   ├── reply / forward / forward-topic / combine-forward
│   ├── list / list-direct / list-all
│   ├── list-by-sender / list-mentions / list-focused
│   ├── list-unread-conversations / list-topic-replies / list-by-ids
│   ├── search / search-advanced
│   ├── query-send-status / read-status
│   ├── add-favorite / remove-favorite / list-favorites
│   ├── add-emoji / remove-emoji
│   ├── add-text-emotion / remove-text-emotion / create-text-emotion
│   ├── list-emotion-replies
│   ├── set-top-msg / unset-top-msg
│   ├── set-pin-msg / unset-pin-msg / list-pin-msg
│   └── download-media
├── group
│   ├── create / rename / dismiss / quit / transfer-owner
│   ├── get-by-group-id / invite-url / share-invite
│   ├── update-icon / update-settings / update-alias / update-nick
│   ├── upgrade-to-external / set-history / set-admin
│   ├── bots / list-my-groups / list-all
│   ├── list-join-validations / audit-join-validation
│   ├── members
│   │   ├── [list] / list-by-ids
│   │   ├── add / remove
│   │   └── add-bot / remove-bot
│   └── notice
│       └── create / edit / get / list
├── group-role
│   └── list / add / update / remove / set-user / remove-user / query-user
├── bot
│   └── search / find
├── category
│   ├── list / list-by-conv / batch-info / list-conversations
│   ├── create / create-smart / rename / delete
│   └── add-conv / remove-conv
├── search / search-common
├── conversation-info / list-top-conversations / list-all-conversations
├── mute / set-top / hide / mark-unread / mark-read
├── clear-red-point / clear-all-red-point / clear-messages
├── mute-at-all / mute-red-envelope
├── group-mute / group-mute-member
├── text
│   └── translate
├── data-auth
│   └── cross-org
├── chmod
├── +shortcut
│   └── 见 Skill Shortcuts 表
└── scripts
    ├── chat_export_messages.py
    ├── chat_history_with_user.py
    └── bot_broadcast.py
```

命令树只保留公开规范路径。曾发现 `search-common` 同时出现在根节点和
`group` 分支，最终删除重复节点，只保留公开路径。

### 4.3 Skill 约束

新增或强化的约束包括：

- 群成员命令使用 `--id`，不要套用消息命令的 `--group`。
- `message list` 按时间拉消息，不用于关键词审计。
- 简单关键词优先 `message search`。
- 发送者、@、多会话组合条件使用 `search-advanced`。
- 产品对象 ID 不能代替真实 `openMessageId`。
- `send-card` 创建通用流式卡片，不是原生审批、日历或待办卡片。
- `update-card --biz-id` 只能使用 `send-card` 返回的真实 `bizId`。
- 会话分组标题最多 15 个字符，不得静默截断或改写用户原文。
- 群 ID、消息 ID、发送者 ID 等多步参数必须来自同一真实上游结果。

## 5. Reference 和 Scripts 对齐

### 5.1 新增错误恢复 Reference

新增文件：

- `skills/multi/dingtalk-chat/references/chat-error-recovery.md`

内容覆盖：

- Chat 错误的读取顺序。
- 可自行修正的参数、路径和 ID 错误。
- 只能修正一次的原则。
- 权限不足、资源不存在、目标歧义等停止条件。
- `openConversationId`、`openMessageId`、`bizId` 的跨步骤传递规则。

### 5.2 新增 Workflow

新增文件：

- `skills/multi/dingtalk-chat/references/workflows/01-onboarding.md`

Workflow：

```text
查找新人
  → 搜索目标群
  → 添加群成员
  → 发送欢迎消息
  → 创建待办
  → 创建欢迎会议
  → 汇总结果
```

该文件只负责编排原子命令，没有新增底层产品能力。

### 5.3 Reference 脚本索引

修改文件：

- `skills/multi/dingtalk-chat/references/chat.md`

新增 `bot_broadcast.py` 索引，使 reference、Skill 和实际 scripts 三者一致。

### 5.4 单聊历史脚本

修改文件：

- `skills/multi/dingtalk-chat/scripts/chat_history_with_user.py`

修复内容：

```diff
- dws chat message list --user <userId>
+ dws chat message list-direct --user <userId>
```

原因：

- `list-direct` 是当前单聊历史的明确叶子命令。
- 避免脚本和 Skill / reference 使用不同路径。

### 5.5 未复制的旧脚本

`nanrun` 中存在 `extract_media_id.py`，本次没有恢复。

当前主干的规则是：

- 本地图片或文件统一通过
  `message send --msg-type file --file-path` 发送。
- 只有上游已经提供有效 `mediaId` 时才使用
  `message send --msg-type image --media-id`。
- `chat media upload` 保留为退役兼容入口，不再提供真实上传能力。

## 6. Go 运行时 Hint 修改

Excel 中的 Hint 修改不只是 Schema JSON，还包含 Go 运行时错误输出和
`--help` Guidance。本次已按当前主干架构完成迁移。

### 6.1 Chat Help Guidance

修改文件：

- `internal/app/root_help.go`

新增 `renderChatAgentSelectionHint`：

1. 根据真实 CLI path 调用 `cli.ResolveMeta`。
2. 只处理 `product_id=chat` 的命令。
3. 从统一 Selection 中读取：
   - `agent_summary`
   - `use_when`
   - `avoid_when`
   - `examples`
4. 将精炼 Guidance 输出到 stderr。
5. 明确 Agent 执行时补充 `--format json`。

输出形式：

```text
Agent guidance:
  Outcome: ...
  Use when: ...
  Avoid when: ...
  Example: dws chat ...
  Output: Agent execution should add --format json.
```

非 Chat 命令不会输出该 Guidance。

### 6.2 媒体参数歧义拦截

修改文件：

- `internal/helpers/chat.go`

新增 `validateChatMessageMediaSelection`。

以下错误输入不再静默进入普通文本发送：

```text
--media-id <mediaId>
--text "example.png"
未提供 --msg-type image
```

现在返回 validation error，并提供：

- `reason=ambiguous_media_message`
- 图片正确路径：
  `--msg-type image --media-id`
- 文件正确路径：
  `--msg-type file --file-path`
- 纯文本恢复路径：
  移除 `--media-id`
- 当前可用 flags

这对应 Excel 中“图片或 PDF 文件名被当成字面文本发送”的静默成功问题。

### 6.3 会话分组标题 Hint

修改文件：

- `internal/helpers/chat.go`

增强 `validatedConversationCategoryTitle`：

- 空标题返回 `reason=invalid_category_title`。
- 超过 15 个字符返回 `reason=category_title_too_long`。
- Hint 明确不得静默截断、缩写或改写。
- Actions 要求用户提供合法标题后重试。

### 6.4 退役媒体上传 Hint

修改文件：

- `internal/helpers/chat_media_upload.go`

`chat media upload` 仍保持退役状态，但错误输出新增：

- `reason=chat_media_upload_retired`
- 本地文件正确发送路径
- 已有图片 `mediaId` 时的发送路径
- 明确禁止用 `drive upload` 替代聊天媒体语义

### 6.5 业务错误 Hint 统一入口

修改文件：

- `internal/errors/pat.go`
- `internal/helpers/helpers.go`
- `internal/app/runner.go`

新增或强化 `SuggestBusinessHint` 统一入口：

- runner 的 MCP tool error 使用该入口。
- runner 的 business error 使用该入口。
- helper 的业务错误建议使用同一入口。
- 支持从以下结构提取错误文本：
  - `errorMsg`
  - `message`
  - 字符串 `error`
  - 对象 `error.code/error.message`
  - `summary`
  - 顶层 `code`

新增 Chat 业务错误模式：

| 服务端错误 | 恢复建议 |
|---|---|
| `listRoles null` | 从当前账号实际加入或管理的群重新选择目标 |
| `OpenId/OpendId is not in conversation` | 选择当前账号已加入的群并重新获取消息 ID |
| `The operator is not in this group chat` | 检查操作者是否在源群 |
| `targetOpenConversationId和receiverUid不能同时为空` | 在 `--target` 和 `--receiver` 中选择正确接收目标 |
| `moveConversationV3 error` | 重新获取真实 `categoryId` 和可访问会话 ID |

## 7. Go 测试修改

### 7.1 Chat Help 测试

新增文件：

- `internal/app/chat_selection_help_test.go`

覆盖：

- Chat help 的 stderr 包含 Outcome、Use when、Avoid when、Example。
- Chat help 明确要求 `--format json`。
- 非 Chat 命令不输出 Chat Guidance。

### 7.2 媒体歧义测试

新增文件：

- `internal/helpers/chat_message_media_validation_test.go`

覆盖：

- `mediaId` 存在但没有 `msgType` 时返回结构化错误。
- Hint 同时包含图片和文件恢复路径。
- 显式 `image`、显式 `file` 和普通文本路径不被错误拦截。

### 7.3 业务错误测试

修改文件：

- `internal/errors/pat_test.go`

新增以下错误模式的测试：

- 嵌套 `error.code/error.message`
- 群身份不可用
- 当前账号不在会话
- 操作者不在源群
- 分享邀请缺少接收目标
- 会话移动到分组失败

## 8. Schema 能力对齐

### 8.1 Registry

修改文件：

- `internal/cli/schema_command_registry/products/chat.json`

Chat Registry 从 120 项增加到 150 项。

新增的 30 个 canonical identity：

```text
chat.add_conv_to_categories
chat.create_conv_category
chat.delete_conv_category
chat.remove_conv_from_categories
chat.rename_conv_category
chat.grant_permission
chat.clear_all_red_point
chat.clear_conversation_messages
chat.clear_conversation_red_point
chat.grant_cross_org_data_access
chat.audit_join_group
chat.list_my_groups_pagination
chat.list_apply_join_group_records
chat.list_group_member_by_ids
chat.create_group_notice
chat.edit_group_notice
chat.get_group_notice
chat.list_group_notices
chat.share_group_invite_url
chat.update_user_group_alias
chat.hide_conversation
chat.list_all_conversations
chat.mark_message_read
chat.mark_conversation_unread
chat.list_message_emotion_replies
chat.set_top_message
chat.unset_top_message
chat.update_at_all_notification_off
chat.update_red_env_notification_off
chat.translate
```

对应 CLI path：

| 能力组 | CLI path |
|---|---|
| 会话分组 | `chat category add-conv/create/delete/remove-conv/rename` |
| 权限 | `chat chmod`、`chat data-auth cross-org` |
| 会话状态 | `chat clear-all-red-point/clear-messages/clear-red-point/hide/mark-read/mark-unread` |
| 群管理 | `chat group audit-join-validation/list-all/list-join-validations/share-invite/update-alias` |
| 群成员 | `chat group members list-by-ids` |
| 群公告 | `chat group notice create/edit/get/list` |
| 会话列表 | `chat list-all-conversations` |
| 消息 | `chat message list-emotion-replies/set-top-msg/unset-top-msg` |
| 通知 | `chat mute-at-all/mute-red-envelope` |
| 文本 | `chat text translate` |

### 8.2 Exclusions

修改文件：

- `internal/cli/schema_command_exclusions.json`

上述 30 个真实公开 Cobra 命令进入 Registry 后，从
`compatibility-helpers-pending-review` 精确 exclusion 中删除。

没有使用 prefix 或 wildcard exclusion。

### 8.3 Safety / Interface Metadata

修改文件：

- `internal/cli/schema_hints/metadata/chat.json`

为新增 30 个能力补齐：

- `effect`
- `risk`
- `confirmation`
- `idempotency`
- `interface_mode`
- `availability`
- `interface_reason`
- `reviewed`
- `review_reason`
- `cli_path`
- `runtime_gate`

安全信息与当前 Runtime 一致：

- `confirmation=user_required` 只在 Runtime 确实有确认 gate 时使用。
- 没有通过高风险标签臆造 `--yes`。
- 没有改变 Cobra 的实际参数或确认行为。

Metadata 当前为 151 条，而 Registry / Selection / Catalog 为 150 条。
额外一条是修改前已经存在的 legacy
`chat.get_group_members` metadata。本次没有借机清理无关历史记录。

### 8.4 Agent Selection

修改文件：

- `internal/cli/schema_hints/selection/chat.json`

为新增 30 个能力补齐：

- `agent_summary`
- `use_when`
- `avoid_when`
- `examples`
- `reviewed`
- `review_reason`
- `source_refs`

同时修改 10 个既有能力：

```text
chat.create_and_send_card
chat.download_media
chat.forward_message
chat.get_conversation_info
chat.list_conversation_message_v2
chat.query_message_send_status
chat.reply_personal_message
chat.send_personal_message
chat.update_group_name
chat.update_streaming_card
```

修改重点：

- 通用流式卡片与原生产品卡片的边界。
- `openMessageId`、`bizId`、`openTaskId` 的来源和类型。
- 本地文件、已有图片 `mediaId` 和普通文本的发送分流。
- 群名称必须保持用户原文。
- 会话查询目标只能选择一个。
- 消息回复、转发的 ID 必须来自同一上游消息。

### 8.5 Runtime Surface 统计

修改文件：

- `internal/cli/schema_hints/runtime-surface-completeness.json`

```diff
- source_tools: 813
+ source_tools: 843
```

## 9. Schema 生成物

以下文件通过生成器重新生成：

- `internal/cli/schema_agent_metadata/chat.json`
- `internal/cli/schema_agent_metadata/index.json`
- `internal/cli/schema_agent_metadata_audit.json`
- `internal/cli/schema_catalog/catalog.json`
- `internal/cli/schema_catalog/tools/chat.json`

生成结果：

| 指标 | 数量 |
|---|---:|
| 产品 | 26 |
| 全部工具 | 843 |
| Chat Registry | 150 |
| Chat Selection | 150 |
| Chat Catalog | 150 |
| Chat Metadata 输入 | 151 |

生成物 diff 较大是因为生成器会完整重建 Typed Metadata 和 Catalog，
不是手工扩写或运行时代码膨胀。

## 10. 全部涉及文件

### 10.1 Go 运行时和测试

```text
internal/app/root_help.go
internal/app/runner.go
internal/app/chat_selection_help_test.go
internal/errors/pat.go
internal/errors/pat_test.go
internal/helpers/chat.go
internal/helpers/chat_media_upload.go
internal/helpers/helpers.go
internal/helpers/chat_message_media_validation_test.go
```

### 10.2 Schema 源输入

```text
internal/cli/schema_command_exclusions.json
internal/cli/schema_command_registry/products/chat.json
internal/cli/schema_hints/metadata/chat.json
internal/cli/schema_hints/runtime-surface-completeness.json
internal/cli/schema_hints/selection/chat.json
```

### 10.3 Skill、Reference 和 Scripts

```text
skills/multi/dingtalk-chat/SKILL.md
skills/multi/dingtalk-chat/references/chat.md
skills/multi/dingtalk-chat/references/chat-error-recovery.md
skills/multi/dingtalk-chat/references/workflows/01-onboarding.md
skills/multi/dingtalk-chat/scripts/chat_history_with_user.py
```

### 10.4 生成物

```text
internal/cli/schema_agent_metadata/chat.json
internal/cli/schema_agent_metadata/index.json
internal/cli/schema_agent_metadata_audit.json
internal/cli/schema_catalog/catalog.json
internal/cli/schema_catalog/tools/chat.json
```

### 10.5 修改说明文档

```text
docs/im-chat-alignment-change-report.md
```

## 11. 验证结果

### 11.1 Skill 和脚本

- Skill 命令完整性检查通过：
  `993 executable command paths`。
- `chat_export_messages.py` dry-run 路由正确。
- `chat_history_with_user.py` dry-run 使用 `message list-direct`。
- `bot_broadcast.py` dry-run 按群调用机器人发送。
- Python scripts 通过 `py_compile`。

### 11.2 Go 测试

通过的定向测试包括：

```text
go test ./internal/app
  - Chat Help Selection 输出
  - 非 Chat Help 隔离
  - Help 相关测试

go test ./internal/helpers
  - Chat 媒体参数歧义
  - Chat 媒体上传退役入口
  - Chat 相关定向测试

go test ./internal/errors
  - SuggestBusinessHint
  - Chat 业务错误模式

go test ./internal/cli
  - ResolveMeta
  - Safety Annotation
  - Registry / Schema completeness
```

需要本机 `httptest` 端口的既有测试在沙箱外运行后通过。

### 11.3 Build 和 Policy

- `go build ./cmd` 通过。
- `git diff --check` 通过。
- `check-skill-commands.sh` 通过。
- `check-schema-catalog.sh` 通过：
  `26 products, 843 tools`。
- `check-generated-drift.sh` 通过。
- Schema Agent Metadata 和 Catalog 连续两次生成结果一致。

## 12. 当前状态与后续动作

当前所有修改均位于：

```text
branch: codex/im-chat-align
base:   6ab01a36
```

尚未执行：

- `git add`
- `git commit`
- `git push`
- 创建 Pull Request

提交前建议：

1. 由 Chat 产品负责人复核 30 个新增 Schema 能力是否全部允许公开给 Agent。
2. 复核 `clear-messages`、`chmod`、`data-auth cross-org` 等能力的 Runtime
   confirmation gate 是否需要单独增强。
3. 确认 Help Guidance 使用英文标签
   `Outcome / Use when / Avoid when / Example / Output` 是否需要本地化。
4. 确认后再统一提交生成物，避免漏掉 Catalog shard。
