# Ding 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交重新构建的
`dws`、运行时 Schema、Cobra Help、内置 Shortcut、DING Skill 与正式
`internal/cli/param_concepts.json`。当前工作区在任务期间发生了分支推进，但未被本分析
回退或改写；候选仍是冻结提交正式表的独立完整副本。

DING 有 7 个原子叶和 5 个注册 Shortcut，共 12 个可执行路径；运行时 Schema 发布
6 个 Agent 工具。`ding +send-by-message` 虽在运行时 Shortcut 注册表中可执行，却未挂载
到冻结提交的 reviewed Agent/Cobra source tree，参数生成器会拒绝把它作为作用域；候选
因此只治理其原子路径 `ding message send-by-message`，并把 Shortcut 差异列为待修复的
命令面问题。

主要风险是机器人 `userId` 与个人 DING `openDingTalkId` 混用、DING 的
`openDingId` 与聊天 `openMessageId` 混用、已有消息提醒与自由文本提醒混用、发送提醒
枚举与列表过滤枚举混用，以及整数 cursor 与不透明 page token 混用。候选已通过真实
生成器、PreParse、alias/canonical dry-run、block、非目标差异审计、`internal/cli`、
`internal/pipeline`、generated drift 和 Schema Catalog 政策。完整 `internal/app` 的唯一
产品相关失败是冻结仓库缺 6 个 DING 命令的 complete-command E2E 模板；正式状态为
“规则及运行链路已验证，补齐模板后方可落地”。

## 参数问题

### 1. 机器人发送与个人发送的接收人 ID 值域不同

- `ding message send --users` 是机器人 DING 的接收人 `userId` 列表，并要求
  `--robot-code`。
- `ding message send-personal`、`ding +send-personal` 和
  `ding message send-by-message` 的 `--users` 是 `openDingTalkId` 列表，不需要
  `robot-code`。
- 两组命令的真实 flag 都叫 `users`，但值域和发送身份不同，不能用通用
  `--user-ids` 在两者间无条件归一。

候选在机器人发送上只接受 `receiver-user-ids`、`recipient-user-ids`、`user-ids`；在
个人发送上只接受 `receiver-open-dingtalk-ids`、`recipient-open-dingtalk-ids`、
`open-dingtalk-ids`。相反值域、staffId 和单数 ID 均在派发前拦截。

### 2. `openDingId`、`openMessageId` 与其他 ID 容易串用

机器人撤回、个人撤回和接收状态都需要一个 DING 的 `openDingId`，但真实参数分别是
`--id` 或 `--ding-id`。消息转 DING 同时需要会话 `openConversationId` 和聊天消息
`openMessageId`，这两个值不能当作撤回用的 DING ID。

候选扩展既有 `ding_id`，只在撤回和接收状态路径映射 `id`、`ding-id`、
`open-ding-id`；`message-id`、`task-id`、`uuid`、`request-id` 被拦截。消息转 DING
则把 `conversation-id`/`open-conversation-id` 精确映射到 `group`，把
`open-message-id`/`msg-id` 映射到 `message-id`。

### 3. 机器人身份只属于机器人发送和撤回

`robot-code` 是机器人应用身份，在 `ding message send` 与
`ding message recall` 上值原样传递。个人发送、个人撤回和消息转 DING 都不应接受
`robot-code`。候选把 `robot` 安全映射到机器人命令的 `robot-code`，并在个人身份路径
明确 block `robot`、`robot-code` 及 bot/robot ID 猜测。

### 4. 自由文本发送与“基于已有消息发送”不是同一输入形态

机器人发送和个人发送有 `--content`，因此 `text`、`body`、`message-content` 可在
精确命令内改名。`send-by-message` 没有正文参数，它引用一个已经存在的聊天消息；
`content`、`body`、`text`、`message` 在该命令上必须停止，不能静默改成
`message-id`。

### 5. `type` 在发送与列表命令中的枚举域不同

发送命令的 `--type` 是 `app/sms/call` 提醒方式；列表命令的 `--type` 是
`ALL/UNREAD/SEND/NEW_COMMENT/DELETED` 过滤器。候选允许发送命令使用
`remind-type`，列表命令使用 `message-type`，并相互 block；不得仅凭 flag 同名推断
枚举可互换。

### 6. 列表 cursor 是整数，不是不透明分页令牌

`ding message list` 与 `ding +list` 的 `--cursor` 是整数。候选只增加
`next-cursor → cursor` 的命令级别名；`page-token`、`next-token`、`offset`、`page`
被拦截，没有扩展通用 `page_cursor` concept。

### 7. Schema、原子命令、Shortcut 注册与 reviewed source tree 不完全同构

运行时 Schema 发布原子 `send/recall` 和 4 个 Shortcut；其他 5 个原子兼容叶仍可执行。
`+send-by-message` 已注册且可运行，但缺少 Contract/Identity，未进入 reviewed source
tree，因而不能出现在 `param_concepts.json` 作用域中。该缺口需要修复 Shortcut 声明，
不能靠候选表绕过生成器闭环校验。

## 当前别名表可以实施的方案

1. 扩展既有 `ding_id`、`robot_code`、`user_ids`、`open_dingtalk_ids`、
   `open_conversation_id`、`open_message_id`、`content_text` 的精确命令范围。
2. 对接收人 ID 域、发送身份、ID 角色、正文输入、type 枚举和 cursor 类型增加命令级
   alias 与 block。
3. 保持所有映射为“参数名变化、值原样传递”；不查询人员、不转换 ID、不改单复数，
   不把自由文本包装为聊天消息。
4. 只治理生成器确认的 11 个 reviewed runnable 路径；不伪造
   `ding +send-by-message` 的候选条目。
5. 补齐 6 个 active 命令的 complete-command 模板并修复 Shortcut Contract 后再评审
   正式替换。

## 当前能力支持不了的事项

- 将姓名、手机号、staffId、userId、openDingTalkId 自动互转；
- 将单个接收人 ID 自动扩成列表，或把多个值隐式合并；
- 从 `openMessageId` 推导发送完成后的 `openDingId`；
- 把自由文本自动创建为聊天消息，再调用 send-by-message；
- 在机器人身份和个人身份之间自动选择发送路径；
- 在 `app/sms/call` 与列表过滤枚举之间转换；
- 把不透明 page token 或 offset 转为整数 cursor；
- 通过别名表为 `ding +send-by-message` 补 Contract/Identity 和 reviewed source tree 挂载。

这些场景应停止并提示真实参数，或先调用人员搜索、联系人查询、消息查询等显式命令；
不得只改参数名继续写操作。

## 第一轮改造建议

第一轮建议落地值域明确的 DING ID、机器人代码、接收人列表、会话/消息 ID、正文、
提醒方式和整数 cursor 别名，同时启用所有反向值域与角色保护。落地 PR 必须同步为以下
6 个 active 命令补 complete-command E2E 模板：`ding message recall`、
`ding +recall-personal`、`ding +send-personal`、`ding message send-personal`、
`ding message send-by-message`、`ding +list`。另行给 `ding +send-by-message` 补完整
Contract/Identity 后，才能把同一治理规则扩展到该 Shortcut。

## 候选 `param_concepts.json` 改动与审核

候选文件是冻结正式表的完整副本，不是增量片段。相对冻结正式文件：

- 修改 7 个既有 concept 的精确命令范围；
- 新增或修改 11 个 DING command override；
- 新增 15 个审核 fixture；
- `go generate ./internal/cli` 从 569 个命令作用域变为 577 个；
- 生成差异仅在 `ding` 条目，`command_path_fallbacks_generated.go` 无变化；
- alias/canonical 的机器人发送、个人发送、消息转 DING、撤回和列表 dry-run payload
  完全一致；
- 值域、角色、单复数、输入形态或类型不一致的写法全部进入 block，未做不安全推断。

候选位置：`docs/parameter-hallucination/ding/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与真实生成器 | 通过 | `go generate ./internal/cli`，577 个命令作用域 |
| PreParse 与 alias/canonical payload | 通过 | 5 组代表命令 dry-run 最终 tool arguments 完全一致 |
| block/ambiguous | 通过 | ID 值域、身份、正文、枚举、分页混用均在 dispatch 前 `blocked_flag` |
| 原生参数 | 通过 | canonical flags 继续走 Cobra 原生路径 |
| 非目标回归 | 通过 | 生成差异仅为 DING；fallback 生成文件无变化 |
| `internal/cli`、`internal/pipeline` | 通过 | 隔离冻结副本执行，CLI 96.288 秒 |
| generated drift | 通过 | 双次别名生成和 Schema 组装 hash 一致 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具 |
| complete-command payload 门禁 | 未通过 | 200/206 个活跃命令已有模板；DING 缺 6 个命令、9 个 active fixture 模板 |
| reviewed Shortcut 完整性 | 未通过 | `ding +send-by-message` 未进入 reviewed source tree，候选不能声明该路径 |

正式替换前必须补齐 6 个命令模板、重跑完整 `internal/app` 和政策门禁；如需治理
`ding +send-by-message`，还必须先完成其 Contract/Identity 声明。未完成前，本候选只
作为完整待审核草稿。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00。
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`。
- 候选 SHA-256：
  `382e6c16c913b78bf325cf7193cca32eb8b035439af970b9df58735b133e398c`。
- Help/实现：`internal/helpers/ding.go`；Shortcut：`internal/shortcut/ding/ding.go`；
  Skill：`skills/multi/dingtalk-misc/references/ding.md`。
- Schema 来源：同一冻结二进制的运行时声明组装；未使用固定 Catalog、历史 badcase、
  用户 Shortcut 或已安装插件。
