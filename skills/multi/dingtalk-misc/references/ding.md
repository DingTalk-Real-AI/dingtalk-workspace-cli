# DING 消息

完成 shared 前置读取后，本页是 DING 唯一默认 Reference。已知路径直接执行；仅在
真实 `unknown command` / `unknown flag` 时读一次 leaf Help，契约不确定时读一次
compact leaf Schema。业务失败不是命令漂移，不能触发命令探索。

## 机器人写前门禁

用户明确指定企业机器人时，写入前固定主体和通道：

1. 用户给出真实 `robot-code` 时才传 `--robot-code`。用户指定某个机器人显示名但没给
   该机器人的 code 时，写前直接以 `robot_credentials_missing` blocked；显示名不是 code，
   也不能用环境中的默认机器人替代。
2. 用户只要求企业机器人、没有指定具体机器人时，省略 `--robot-code` 并执行一次；
   Runtime 只解析已配置的 `DINGTALK_DING_ROBOT_CODE`，Agent 不读取、打印或探测该变量。
3. 缺凭据返回 `robot_credentials_missing`；机器人无效或不在当前组织返回
   `robot_not_in_org`。两者都为 `retryable=false`；首次明确失败即 blocked，不执行
   `dws auth login`，也不再发第二次请求。
4. blocked 后禁止重试、`--verbose`、Help/Schema、dev/devapp、chat bot、配置目录、
   其他 profile、日志或评测文件搜索；禁止切换为个人身份、其他机器人或其他通道。
   CLI 不枚举候选 robot-code，也不使用另一个机器人的凭据补偿本次失败。
5. 只有用户提供新的有效 robot-code、明确切换组织或重新指定主体/通道，才开始新的
   逻辑请求。

## 意图与入口

| 用户意图 | 首选入口 | 身份/边界 |
|---|---|---|
| 查历史、未读、已发、评论或删除记录 | `dws ding message list` | 按 `openDingId` 识别消息 |
| 查一个 DING 的接收状态 | `dws ding +receiver-status` | 必须已有 `openDingId` |
| 发 DING，未指定机器人 | `dws ding message send-personal` | 当前用户；接收人用 `openDingTalkId` |
| 明确用企业机器人发送 | `dws ding message send` | 接收人用 `userId`；需要 `robot-code` |
| 把聊天消息转为 DING | `dws ding message send-by-message` | 使用目标会话和消息的稳定 ID |
| 撤回当前用户 DING | `dws ding message recall-personal` | 使用发送回执的 `openDingId` |
| 撤回机器人 DING | `dws ding message recall` | 使用同一 `robot-code` 和 `openDingId` |
| 普通聊天、建群、群消息和解散群 | `dingtalk-chat` | Chat 拥有群资源，DING 只负责强提醒 |

全链使用同一 profile。零/多候选、身份不明或 profile 不一致时停止，禁止取首项或
按内容猜 ID。成功必须有业务回执；失败、投递未知、未确认状态不得报成功。

## 完成条件

| 用户要求 | 完整成功必须具备 |
|---|---|
| 发送 | 指定主体/通道的成功回执和真实 `openDingId` |
| 发送并查状态 | 同一 `openDingId` 的精确接收状态 |
| 发送、查状态并撤回 | 前两项完成，且同一主体的 DING recall 返回成功终态 |
| 全部/多页列表 | 明确分页终止证据；否则只能报告 partial 和停止原因 |
| `sms` / `call` | 实际主体、实际通道、费用属性、`openDingId` 与请求后续终态 |

完整发送—查询—撤回链不得中途缩减；Runtime 再次确认时按结构化停点续接，保留
待执行动作和稳定 ID。

## 调用与上下文预算

- 每个姓名只解析一次；同一逻辑请求的 send、status、recall 各最多一次。仅用户明确
  要求等待时，status 才有界轮询。
- 先用单次响应完整校验，再无损投影；`{actor, channel, openDingId, status, recall,
  pagination}` 只是最小集合，正文、收件人、时间等用户要求字段必须原样保留。
- 投影只减少重复上下文，不删除业务信息，也不得为恢复已丢字段重发远端请求。最终
  报告所需事实、条数和完整性；禁止为整理输出追加 list、Help 或 Schema。

## 身份与稳定 ID

姓名接收人只解析一次：

```bash
dws aisearch person --query "<姓名>" --dimension name --format json
```

| 用途 | 参数 | 稳定来源 |
|---|---|---|
| 机器人发送 | `--users <userId,...>` | 人员解析的 `userId` |
| 个人发送/消息转 DING | `--users <openDingTalkId,...>` | 人员解析的 `openDingTalkId` |
| 消息转 DING | `--group` / `--message-id` | 目标聊天回执的 `openConversationId` / `openMessageId` |
| 状态查询/撤回 | `--ding-id` / `--id` | 发送回执或真实列表的 `openDingId` |

这些 ID 不可互换；机器人凭据、主体与 profile 按“机器人写前门禁”处理。

## 消息转 DING 的资源生命周期

`send-by-message` 是“源聊天消息 → 新 DING”转换。成功回执同时保存以下身份，禁止
覆盖到同一个变量：

| 字段 | 资源类型 | 后续用途 |
|---|---|---|
| `sourceMessageId` | chat message | 仅用于聊天消息查询或撤回 |
| `sourceConversationId` | chat conversation | 来源审计和会话定位 |
| `openDingId` | ding | DING 接收状态和撤回 |
| `recallTarget` | ding | `{resourceType: "ding", openDingId}` |

转换后撤回时，只执行：

```bash
dws ding message recall-personal --id <OPEN_DING_ID> --format json
```

撤回前根据回执字段和 `resourceType=ding` 校验来源，只使用 `openDingId`，不得拿
`sourceMessageId` 或会话 ID 替代。`openDingId` 是不透明标识，不依据 `msg` / `cid` 前缀
推断资源类型；裸 `--id` 的资源有效性由服务端校验，CLI 只拒绝空白值，不额外查询。
若结构化证据表明目标属于 Chat，立即停止并重新取得 DING 回执；不能改用 Chat 撤回
来宣称完成。服务端拒绝后也不切换资源、身份或重放。

## 临时群消息转 DING 的有界状态链

这类跨产品任务只为群生命周期加载 `dingtalk-chat` 的 `group-admin.md`；不加载聊天消息
查询、搜索或消息动作 Reference，除非用户另外要求操作源聊天消息。人员只使用
`dingtalk-aisearch` 根 Skill 的 person 入口，不加载其二级 Reference；按“身份与稳定 ID”
一次解析后，同时复用其 `openDingTalkId` 建群和作为 DING 接收人，禁止在 Chat 内再次按
姓名解析。

1. `dws chat +chat-create --name "<群名>" --users <OPEN_DINGTALK_IDS> --format json`；
   保留完整回执，后续只使用其中的 `openConversationId`，创建失败不继续。
2. `dws chat +messages-send --as user --group <OPEN_CONVERSATION_ID> --text "<正文>"
   --format json`；保存完整响应及 `sendReceipt`，不要改用只返回粗粒度结果的发送入口。
3. `sendReceipt.readyForMessageActions=true` 时直接读取
   `messageRef.openMessageId/openConversationId`；否则只用其中的 `openTaskId` 调用一次
   `dws chat +messages-query-send-status --open-task-id <OPEN_TASK_ID> --format json`。
   状态仍未给出完整 `messageRef` 时报告 pending/blocked，不执行 `+chat-messages`、内容
   搜索或重复发送来猜消息 ID。完整 `messageRef.openConversationId` 必须等于建群回执的
   `openConversationId`；不一致时立即 blocked。
4. 完整 `messageRef` 就绪后执行一次 `dws ding message send-by-message`。保存转换回执的
   `sourceMessageId`、`sourceConversationId`、`openDingId` 和 `recallTarget`；同一请求需要
   幂等控制时生成一次 `--uuid` 并始终复用。
5. 只有用户要求时才用 `openDingId` 查 DING 接收状态或撤回 DING；不得撤回源 Chat
   消息来代替 DING 撤回。临时群最后只用
   `dws chat +chat-dismiss --group <OPEN_CONVERSATION_ID> --format json` 解散一次。

默认调用预算是：每个不同人员一次解析、一次建群、一次群消息发送、至多一次发送状态
查询、一次消息转 DING、一次临时群解散；DING 状态和撤回仅按用户要求各一次。最终结果
保留人员映射、`openConversationId`、`openTaskId`、`sourceMessageId`、`openDingId`、投递/
撤回状态和清理结果；某一步失败时同时报告此前已提交的资源，不能用减少上下文掩盖部分成功。

## 查询与分页

```bash
dws ding message list --type <ALL|UNREAD|SEND|NEW_COMMENT|DELETED> \
  --cursor 0 --format json
dws ding +receiver-status --ding-id <OPEN_DING_ID> --format json
```

- list 项已含 `content`、`openDingId` 和状态；读取这些字段不追加详情调用。
- list 不传 `--type` 时按 `ALL` 查询；首次 `--cursor 0`，后续只用响应的 `nextCursor`。
- 只有 `hasMore=true` 且 `nextCursor` 是严格前进的正整数时继续；达到用户上限、
  `hasMore=false`、游标缺失、停滞或循环时立即停止并如实说明完整性。
- 跨页默认按 `openDingId` 去重。只有用户明确要求内容合并时才按规范化内容去重，
  并同时报告原始条数和合并后条数。

## 发送与撤回

```bash
# 当前用户身份
dws ding message send-personal \
  --users <OPEN_DINGTALK_ID> --content "<内容>" --type app --format json

# 企业机器人身份
dws ding message send \
  --users <USER_ID> --content "<内容>" --type app --format json

# 聊天消息转 DING
dws ding message send-by-message \
  --group <OPEN_CONVERSATION_ID> --message-id <OPEN_MESSAGE_ID> \
  --users <OPEN_DINGTALK_ID> --type app --format json

dws ding message recall-personal --id <OPEN_DING_ID> --format json
dws ding message recall --id <OPEN_DING_ID> --format json
```

- 企业机器人命令只有在用户提供真实 code 时才追加 `--robot-code <ROBOT_CODE>`；否则
  按“机器人写前门禁”决定省略参数执行或在写前 blocked。
- `app` 免费；`sms` / `call` 有通信费用。费用知情与 Runtime 写确认分别处理。
- 需要调用方控制幂等时，为个人发送或消息转 DING 生成一次稳定 `--uuid`；同一
  逻辑请求的安全重试必须复用，不能为每次尝试生成新键。
- 跨产品补偿由工作流所有者负责；DING 返回真实失败状态和已产生的稳定资源 ID。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "ding +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws ding <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service ding --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws ding +receiver-status` | read | 查询 DING 消息接收人已读状态 |
<!-- VISIBLE_SHORTCUTS_END -->
