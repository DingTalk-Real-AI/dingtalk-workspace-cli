# Lite Recipe 完整步骤

> 核心流程步骤 3 判定为 lite 后，按本文件中对应 recipe 的步骤**直接执行**。
> 所有命令均须加 `--format json`（下文省略）。

## #1 消息沟通

所有消息沟通相关的命令详情、参数说明、意图路由和复合工作流，请查阅 `dingtalk-chat`。

## #2 任务管理

### create-todo

1. 确定执行者：指定姓名 → `aisearch person --query "<姓名>" --dimension name` → `userId`；未指定 → `contact user get-self` → `userId`；多人 → 逐个搜索逗号拼接。
2. 创建：`todo task create --title "<标题>" --executors <userId>[,<userId2>...] --priority <优先级>`（可选 `--due "<截止ISO>"`）→ `todoTaskId`

### todo-query-ops

- 查询：`todo task list [--status false|true]`（不传=全部）
- 详情：`todo task get --task-id <id>`
- 完成/重开：`todo task done --task-id <id> --status <true|false>`
- 按主题筛选：list 后按标题关键词过滤

## #3 会议日程

### list-today-meetings

**优先**：加载 `dingtalk-calendar` sub-skill 后按其「今天/明天/本周日程」recipe 执行；该自动化脚本只在 `dingtalk-calendar` 包内可用。
备选：`dws calendar event list --start "<今日起始ISO>" --end "<今日结束ISO>"`（须加 `--format json`）

### check-users-busy

查询多人在某时段内的闲忙（**busy**，不是用 `event list` 扫日程）：

1. 解析用户：对每个姓名执行 `aisearch person --query "<姓名>" --dimension name` → `userId`；多人将 `userId` 用英文逗号拼接（无空格或按 `dingtalk-calendar` `busy search` 要求）。
2. 确认时段：用户须给出或可收敛为明确的 `--start` / `--end`（ISO-8601）；若未给出，**先追问**起止时间，禁止用任意默认全天窗口代替用户意图。
3. 执行：`dws calendar busy search --users <userId1,userId2,...> --start "<ISO>" --end "<ISO>" --format json`

详见 `dingtalk-calendar` 中「查询用户闲忙状态」。

## #4 文档知识

### query-doc

1. 全局搜索：`drive search --query "<关键词>"` → `nodeId`（聚合钉盘+文档空间）
2. 空间内搜索：`wiki node search --workspace <WS_ID> --query "<关键词>"` → `nodeId`
3. `doc read --node <nodeId>`（按需；大文档只抽章节）

### list-folder-docs

`drive list --workspace <WS_ID>` 或 `wiki node list --workspace <WS_ID>`

## #5 工作汇报

### query-report-list

1. 收到的日志：先把用户时间词转成起止时间，再执行 `report inbox list --start "<YYYY-MM-DDT00:00:00+08:00>" --end "<YYYY-MM-DDT23:59:59+08:00>" --cursor 0 --size 20 --format json`。用户只说“最近/近期/最近收到”时默认最近 7 天。
2. 我发过/我创建的日志：首条查询必须用 `report outbox list --cursor 0 --size 20 --format json`；如用户指定时间，补 `--start "<YYYY-MM-DDT00:00:00+08:00>" --end "<YYYY-MM-DDT23:59:59+08:00>"`。
3. 按发件人过滤收件箱：先 `aisearch person --query "<姓名>" --dimension name --format json` 取 `userId/staffId`，再加 `--sender-user-ids <id>`；空结果必须说明未找到该发件人的日志，不得改选其他人。
4. 面向用户时必须基于 `result[]` 拼 Markdown 表，表头固定为 `日期 | 标题 | 发送人 | 状态 | 钉钉链接`；每条 `result[]` 都会带这五个中文字段，不要把 `reportId` / `日志ID` 作为主列。
5. 用户要正文、详情、统计、汇总或总结多篇日志时，必须用内部保留的 `reportId` 逐篇执行 `report entry get --report-id <reportId> --format json` 或 `report entry stats --report-id <reportId> --format json`；选前 5 篇时调用次数应等于实际选中篇数。

时间 flag 硬约束：只允许 `--start` / `--end`；禁止 `--start-date` / `--end-date` / `--date`。不要只传 `2026-05-04`，必须展开成 `2026-05-04T00:00:00+08:00` 这种完整 ISO；禁止 UTC `Z` / `date -u`。

硬约束：`report inbox list` 是收到的日志（别人发给我），`report outbox list` 是我创建/发出的日志（我发给别人）。不要混淆方向；不要回答"API 不支持收到的日志"。

> 旧命令兼容：`report list` / `report inbox` / `report sent` / `report created` / `report detail` / `report stats` 仍可执行，但已 deprecated，stderr 会打废弃提醒，新计划一律使用 `inbox list` / `outbox list` / `entry get` / `entry stats`。

禁止：不要先查 help，不要为了格式化列表创建脚本；不要传 `--size 50/100`。`report inbox` 可作为兼容入口使用，但新计划优先写规范命令 `report inbox list --start "<YYYY-MM-DDT00:00:00+08:00>" --end "<YYYY-MM-DDT23:59:59+08:00>" --cursor 0 --size 20 --format json`。

### check-report-read-status

`report entry stats --report-id <reportId>` → 已读/未读

## #7 听记与会后

> 产品命令完整参考见 `dingtalk-minutes`。full recipe 见 `dingtalk-minutes/references/07-minutes.md`。

### minutes-query（查询与获取）

> **scope 选择铁律（P2 真实 badcase）**：`list` 后的 scope 决定查询范围，最高频误判是把"我能访问的所有听记"错选成 `mine`：
> - `mine` = **仅我自己创建/发起**的听记（范围最窄）
> - `shared` = **仅他人共享给我**的听记
> - `all` = **我可访问的全部**（= mine ∪ shared，范围最广）
> - **判定口诀**：query 含"访问/权限/可见/能看到/所有/我的"等覆盖范围语义 → 一律走 `all`；**仅当**明确说"我创建的/我发起的/我录的" → 才走 `mine`。**不要因为句子里有"我"字就退化成 `mine`。**
> - 错误：`我能访问的所有听记` → `list mine`（漏掉共享给我的，判定不通过）
> - 正确：`我能访问的所有听记` → `dws minutes list all --format json`

> **选对象铁律（0605 P2 EDD badcase 提炼，命令对了但选错听记 = 整任务失败）**：list/搜索拿到结果后，必须按语义精准锁定目标听记，详见 `dingtalk-minutes`「选对象铁律 S1~S6」。速记：
> - **S1 跨组织汇总**：以 list 返回的 `taskUuid + title + organizationName` 三元组为准逐条照抄，组织与听记不可张冠李戴。
> - **S2 "最近一次某类会议"**：先 `--query "<主题词>"`（如周会）过滤出该类，再在候选里取时间最新；主题匹配优先级高于时间。
> - **S3 比时长最长**：必须读 `durationMicros` 字段做数值比较，禁止凭印象/标题猜，口头结论与操作的 taskUuid 须自洽。
> - **S4 内容为空**：锁定 taskUuid 后所有 get/update 复用同一 id；某字段为空就如实说，**禁止偷偷切换到另一条听记**。
> - **S5 模糊日期匹配不到**：日期可能是"会议主题日期"而非"创建日期"，按标题关键词搜，精确日期没命中就放宽 ±7 天/同主题候选请用户确认，**禁止直接报"找不到"**。搜索回退策略：① 先 `--query "<主题关键词>"` 不带日期搜 → ② 若结果过多则加 `--start/--end` 扩大到 ±7 天 → ③ 列出候选让用户确认。
> - **S6 给标题没给 id**：必须先 `list all --query "<标题关键词>"` 定位 taskUuid 再 update/get，禁止凭记忆直接填 `--id` 跳过定位。

**列表查询**（`list` 后**必须**跟 scope：`mine`/`shared`/`all`，默认补 `all`）：

```bash
# 我可访问的所有听记（默认）
dws minutes list all --format json
# 按关键词服务端搜索（严禁全量拉取后本地 grep）
dws minutes list all --query "周会" --format json
# 按时间范围筛选（ISO-8601 格式）
dws minutes list mine --start "2026-05-01T00:00:00+08:00" --end "2026-05-25T23:59:59+08:00" --format json
# 关键词 + 时间组合
dws minutes list all --query "需求评审" --start "2026-05-25T00:00:00+08:00" --end "2026-05-25T23:59:59+08:00" --format json
# 限制条数
dws minutes list mine --limit 5 --format json
# 共享给我的听记
dws minutes list shared --query "ROI" --format json
```

| 参数 | 说明 |
|------|------|
| `--query "<关键词>"` | 服务端关键词搜索 |
| `--start "<ISO-8601>"` | 开始时间 |
| `--end "<ISO-8601>"` | 结束时间 |
| `--limit <N>` | 每页条数，默认 10（`--max` 为兼容别名） |
| `--cursor "<token>"` | 分页 token，首页留空（`--next-token` 为兼容别名） |

**获取详情**：

- 批量基础信息：`minutes get batch --ids <uuid1,uuid2,...>`
- 单篇摘要：`minutes get summary --id <taskUuid>`
- 转写原文（自动翻页）：`minutes get transcription --id <taskUuid>`（返回 `cursor` / `nextToken` / `nextCursor` 时用 `--cursor <token>` 继续）
- 关键词：`minutes get keywords --id <taskUuid>`
- 待办事项：`minutes get todos --id <taskUuid>`
- 基础信息：`minutes get info --id <taskUuid>`
- 音频地址：`minutes get audio --id <taskUuid>`

> `--id`/`--uuid`/`--task-uuid` 三者等价。推荐 `--id`。

### minutes-edit（编辑与替换）

- **替换转写文字**：`minutes replace-text --id <taskUuid> --search "旧文字" --replace "新文字"`
  - 执行前检查特殊字符（引号/书名号/括号等），若包含先提示用户确认去除
  - 替换成功后追问是否加热词：`minutes hot-word add --words "新文字"`
- **替换发言人**：先统一搜人取得 dingUid → `minutes speaker replace --id <taskUuid> --from "发言人X" --to "姓名" --target-uid <userId>`
  - 查询 dingUid：`aisearch person --query "姓名" --dimension name --format json` → 取 `userId`
  - 多个匹配 → 列出候选让用户选；无匹配 → 不带 `--target-uid` 执行
- **修改标题**：`minutes update title --id <taskUuid> --title "新标题"`
- **修改摘要**：`minutes update summary --id <taskUuid> --content "新内容"`
- **热词管理**：`minutes hot-word add --words "词1,词2"` / `minutes hot-word list`
- **思维导图**：`minutes mind-graph create --id <taskUuid>` → `mind-graph status --id <taskUuid>` 轮询至完成

### minutes-tag（标签/分组查询）

- 查询标签列表：`minutes tag list` → 返回用户在听记页面创建的所有标签/分组（含 tagId 和名称）
- 按标签查听记：`minutes tag query --tag-id <tagId> [--limit 20] [--cursor <token>]`
  - tagId 来自 `tag list` 返回值，不可编造
  - 支持分页，`--cursor` 传入上一次返回的 nextToken

**典型链路**：用户说"帮我看看'周会'标签下的听记" →
1. `dws minutes tag list --format json` → 按名称匹配找到 tagId
2. `dws minutes tag query --tag-id <tagId> --format json`

### minutes-permission（权限管理）

- 添加成员：`minutes permission add --ids <uuid1,uuid2> --member-uids <uid1,uid2> --policy 4`
  - 需先通过 `aisearch person --query "<姓名>" --dimension name` 获取目标 userId
  - policy：0=不可见 / 1=仅查看 / 2=查看+下载 / 3=查看+下载+编辑 / 4=全部权限
- 移除成员：`minutes permission remove --ids <uuid1,uuid2> --member-uids <uid1,uid2>`

### minutes-upload（音频上传）

```bash
# 创建上传会话
dws minutes upload create --file-name "meeting.mp3" --file-size 61565431 --format json
# 上传完成后确认
dws minutes upload complete --session-id <sid> --format json
# 取消上传
dws minutes upload cancel --session-id <sid> --format json
```

### 最佳实践案例速查（详见 `dingtalk-minutes`）

| 案例 | 场景 | 正确链路 |
|------|------|----------|
| 案例 1 | 听记 URL + 创建思维导图 | 提取 taskUuid → `mind-graph create` → `mind-graph status` 轮询；**禁止**走 app-development 或前端库 |
| 案例 2 | 替换文字后未引导热词 | 检查特殊字符 → `replace-text` → 追问加热词 `hot-word add` |
| 案例 3 | 查听记拉了不必要的转写 | 用户只要列表 → `list` 即可，**不要**自动拉 `get transcription` |
| 案例 4 | 拉完转写只输出时间线原文 | 拉完后追问按发言人聚类 → 引导匹配 → 调用 `speaker replace` 写回 |
| 案例 5 | 查某人说了什么不引导替换 | 推断发言人 → **用户确认** → 结构化总结 → 引导 `speaker replace` |
| 案例 6 | 通讯录+部门+转写三路印证 | Step 3 画像 + Step 4 `aisearch person` 并发 → 置信度 ≥70% → 确认 → 替换 |
| 案例 7 | grep 花名误判未参会 | **禁止**在转写文本里 grep 人名判参会；**必须**调 `aisearch person` |
| 案例 8 | 听记类 query 不走 dws | **禁止**用 session_search/browser_use/activity:search 替代 dws；模糊请求先 `list mine` |
| 案例 9 | 按标签筛选听记 | `tag list` → 按名称匹配 tagId → `tag query --tag-id <tagId>`；**禁止**编造 tagId |

### 听记取数深度约束（0609 点踩 case 提炼）

> 详细说明见 `dingtalk-minutes`。

- **转写原文硬约束**：用户诉求含「聚焦原话/逐字/沟通细节/具体讨论了什么」等词时，**必须先调 `get transcription` 翻页拉全**，禁止仅凭 summary 出稿
- **数据源下钻**：听记维度**必须 `get summary`（或 `get transcription`）读正文**，严禁只取标题列表；scope 用 `all`；空时换窗重试或标注
- **听记链接解析**：聊天消息中遇到听记链接（`flash_minutes_detail`/`SHANJI`）→ 解析 `minutesId` → 调 `minutes get summary/transcription`，禁止把链接降级为关键词
- **忠实性约束**：源数据无某要素（行动项/责任人/数字）时禁止生成；统计字段基于实际取数计数，不得编造
- **多源全覆盖**：用户枚举多数据源时每个来源都必须调对应工具；瞬时错误重试；如实声明缺失来源，禁编无来源数字

### 间接意图识别铁律

query 未提"听记"但任务产出依赖会议讨论内容时（报告/总结/日报/复盘/商业分析/市场感知），听记采集是**必跑前置步骤**：

1. **铁律 A**：任务含"会议/讨论/沟通"信息需求 → `dws minutes list` 必跑
2. **铁律 B**：用户说"文档啥也没有" → 听记优先级更高（唯一结构化数据源）
3. **铁律 C**：多源聚合场景 → 每个被提及的数据源都必须有采集动作，听记侧 0 调用 = 严重失败

## #8 通讯录

### get-contact-self

`contact user get-self` → 当前用户 userId、部门、主管等

### search-person

**搜人首选入口**。凡是“找人/搜人/找同事/谁负责/上级/下级/负责人/团队成员”均优先用 `aisearch person`：

1. 从用户问题中提取 keyword（人名/业务关键词）和 dimension（维度），规则见 `dingtalk-aisearch`。
2. `aisearch person --query "<关键词>" --dimension <维度>`
3. 结果中提取 `userId` 和 `title`（姓名）展示给用户。
4. 若需要 userId 做后续操作（发消息/建待办），可直接使用结果中的 `userId`。
5. **重名消歧**：多人同名时禁止默认选第一个，须追加 `contact user get --ids` 获取部门/职位后请用户确认，详见 `dingtalk-contact/references/08-directory.md`「多命中」。

### search-user

仅在以下**精确查询**场景使用，搜人请优先用 `search-person`：

- 需要获取 userId 给其他产品使用（发消息/建待办/约日程）
- 已有 userId 需查完整详情（`contact user get --ids`）

1. `aisearch person --query "<姓名>" --dimension name` → `userId`；**多命中须列出候选请用户确认**。
2. **重名消歧**：多人同名时禁止默认选第一个，须追加 `contact user get --ids` 获取部门/职位后请用户确认，详见 `dingtalk-contact/references/08-directory.md`「多命中」。
3. 需详情时：`contact user get --ids <userId>`（多人可 `--ids id1,id2,...`）

## #9 邮件

### mail-list-mailbox

查询当前用户自己的可用邮箱地址列表。**仅返回自己的邮箱**，不能查他人邮箱（查他人邮箱请走 `dingtalk-mail` 中「查找他人邮箱地址」三路并发查询流程）。

`mail mailbox list`

### mail-search

搜索邮件。**必须使用 KQL 语法通过 `--query` 传递查询条件**，禁止臆造 `--subject`、`--from` 等不存在的 flag。

1. 获取邮箱地址：`mail mailbox list` → 取用户邮箱地址。若用户已提供邮箱可跳过。
2. 构造 KQL 查询：根据用户意图将搜索条件转为 KQL 表达式（详见 `dingtalk-mail` 中 KQL 查询字段说明）。
   - 按主题：`subject:周报`、`subject:"项目 进展"`（含空格须加双引号）
   - 按发件人：`from:alice@company.com` 或 `from:"张三"`
   - 按日期：`date>2025-06-01T00:00:00Z`（ISO8601 格式，必须含时间部分）
   - 按文件夹：`folderId:2`（2=收件箱, 1=已发送, 5=草稿, 6=已删除）
   - 按是否有附件：`hasAttachments:true`
   - 组合：`from:alice AND subject:周报 AND date>2025-06-01T00:00:00Z`
3. 执行搜索：`mail message search --email <邮箱> --query "<KQL表达式>" --limit 20`
4. 查看详情（按需）：`mail message get --email <邮箱> --id <messageId>`

### mail-send

发送邮件。

1. 获取邮箱地址：`mail mailbox list` → 取用户邮箱作为 `--from`。
2. 确定收件人：用户直接提供邮箱地址 → 直接使用；用户提供姓名 → 走「查找他人邮箱地址」三路并发流程（见 `dingtalk-mail`）。
3. 发送：`mail message send --from <发件邮箱> --to <收件邮箱> --subject "<主题>" --content "<正文>"`（可选 `--cc`、`--attachment`、`--inline-attachment`）。

### mail-reply-forward

回复或转发邮件。

1. 获取邮箱地址：`mail mailbox list` → 取用户邮箱。
2. 定位原始邮件：若用户未提供 messageId → 先用 `mail-search` 搜索定位。
3. 执行：
   - 回复：`mail message reply --from <邮箱> --id <messageId>`（可选 `--to`、`--subject`、`--content`）
   - 回复全部：`mail message reply-all --from <邮箱> --id <messageId>`（可选 `--to`、`--subject`、`--content`）
   - 转发：`mail message forward --from <邮箱> --to <收件邮箱> --id <messageId>`（可选 `--subject`、`--content`）
