---
name: dws
description: 管理钉钉产品能力(AI表格/AI搜问/日历/通讯录/群聊与机器人/待办/审批/考勤/日志/DING消息/开放平台文档/钉钉文档/钉钉云盘/AI听记/邮箱/在线电子表格/知识库等)。当用户需要操作表格数据、管理日程会议、模糊找人/查谁负责某事项、查询通讯录、管理群聊、机器人发消息、创建待办、提交审批、查看考勤、提交日报周报（钉钉日志模版）、读写钉钉文档、上传下载云盘文件、查询听记纪要、收发邮件、读写在线电子表格(axls)、管理钉钉知识库时使用。
cli_version: ">=1.0.15"
---

# 钉钉全产品 Skill

通过 `dws` 命令管理钉钉产品能力。


> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。`--help` 决定 Cobra 实际接受的 flags；基础命令的 leaf Schema 决定 Agent 选择、参数映射/约束和安全确认语义，`+` shortcut 则读取独立 shortcut catalog。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。

## 严格禁止 (NEVER DO)
- 不要使用 dws 命令以外的方式操作（禁止 curl、HTTP API、浏览器）
- 不要编造 UUID、ID 等标识符，必须从命令返回中提取
- 不要猜测字段名/参数值，操作前必须先查询确认

## 严格要求 (MUST DO)
- 所有命令必须加 `--format json` 以获取可解析输出
- 危险操作必须先向用户确认，用户同意后才加 `--yes` 执行
- 单次批量操作不超过 30 条记录
- 所有命令必须**严格遵循**对应产品参考文档里面规定的参数格式（如：如果有参数值，则参数和参数值之间至少用一个空格隔开）
- **脚本优先**：[scripts/](./scripts/) 下的 `python scripts/<name>.py` 已封装翻页/轮询/批量逻辑，遇到对应场景（如 AI 表格批量导入导出、AI 应用创建轮询、文档创建后写内容、钉盘目录树等）**优先调用脚本**而非手写多步命令。脚本均支持 `--dry-run` 预览、`--format json` 输出，失败时回退到手动步骤
- **实时个人消息事件例外**：用户要监听消息、订阅事件、自动回复消息或事件驱动 Agent 时，必须走 `dws event consume ... --flatten` 长连接，不要写脚本轮询消息历史

## Shortcut 与原子命令的使用原则

`shortcut` 是对常用操作的高层封装，适合优先承担用户意图；产品参考文档和本 skill 负责判断意图、风险、跨产品流程和复杂参数，CLI 帮助负责声明当前版本真正可调用的命令。

- 先按产品参考、意图表和 recipe 路由。存在精确覆盖场景的专用脚本/recipe 时继续遵循“脚本优先”；否则用户意图可由可见 shortcut 满足时，优先使用 `dws <service> +<verb> ... --format json`，不要手写等价的多步原子命令。
- shortcut 有独立 catalog，按设计不进入 Runtime Schema。调用前用 `dws shortcut list --service <service> --format json` 读取完整 flags、required/enum、跨参数 constraints、risk/confirmation 和 examples；不要对 `+` 路径调用 `dws schema`。
- 真正组装参数前用叶子帮助 `dws <service> +<verb> --help` 核对当前 Cobra 接受的 flags。父级 `dws <service> --help` 只能发现子命令，不能替代叶子参数帮助。
- shortcut catalog 中 `confirmation=user_required` 时，必须先获得用户确认，确认后才加 `--yes`；`not_required` 不额外确认。
- 如果 shortcut 不在 help / list 中，改用产品参考里的原子命令、脚本或标准流程；不要猜测未展示的 `+` 命令。
- shortcut 失败时按“错误处理”流程先加 `--verbose` 复查；若仍失败，应记录具体输入、输出、trace / endpoint / tool 信息。


<!-- VISIBLE_SHORTCUTS_OVERVIEW_START -->
## Shortcut 总览

下面统计当前公开 catalog 中的 shortcut。mono 模式不展开 200+ 行明细，避免 skill 过重；需要执行时先按产品路由，再用 `dws shortcut list --service <service> --format json` 读取参数、约束、风险和示例，最后用 `dws <service> +<shortcut> --help` 核对当前 Cobra flags。multi 模式的各产品 skill 会展开该产品的 shortcut 表。

| 服务 | shortcut 数 | multi skill | 发现命令 |
|---|---:|---|---|
| `aitable` | 29 | `dingtalk-aitable` | `dws shortcut list --service aitable --format json` |
| `attendance` | 19 | `dingtalk-attendance` | `dws shortcut list --service attendance --format json` |
| `calendar` | 20 | `dingtalk-calendar` | `dws shortcut list --service calendar --format json` |
| `chat` | 42 | `dingtalk-chat` | `dws shortcut list --service chat --format json` |
| `contact` | 14 | `dingtalk-contact` | `dws shortcut list --service contact --format json` |
| `devapp` | 19 | `dingtalk-dev` | `dws shortcut list --service devapp --format json` |
| `ding` | 4 | `dingtalk-ding` | `dws shortcut list --service ding --format json` |
| `doc` | 17 | `dingtalk-doc` | `dws shortcut list --service doc --format json` |
| `drive` | 7 | `dingtalk-drive` | `dws shortcut list --service drive --format json` |
| `mail` | 10 | `dingtalk-mail` | `dws shortcut list --service mail --format json` |
| `minutes` | 6 | `dingtalk-minutes` | `dws shortcut list --service minutes --format json` |
| `oa` | 7 | `dingtalk-oa` | `dws shortcut list --service oa --format json` |
| `report` | 2 | `dingtalk-report` | `dws shortcut list --service report --format json` |
| `sheet` | 2 | `dingtalk-sheet` | `dws shortcut list --service sheet --format json` |
| `todo` | 11 | `dingtalk-todo` | `dws shortcut list --service todo --format json` |
| `wiki` | 1 | `dingtalk-wiki` | `dws shortcut list --service wiki --format json` |
<!-- VISIBLE_SHORTCUTS_OVERVIEW_END -->

## 多组织 / 多账号

- `dws profile list --format json` 默认返回全部账号。自动化只使用每项稳定的 `profile=corpId:userId`；`status/expiresAt/refreshExpAt` 来自真实身份 Token，列表不触发刷新。
- 输入支持 `corpId:userId`、`corpId:userName`、`corpName:userId`、`corpName:userName`，也兼容单独的 corpId、唯一 corpName 和本地 profile 名。名称只用于输入；重名时必须按报错候选改用 `corpId:userId`。
- 只传组织时使用该组织明确记录的 `isOrgCurrent=true` 账号。多账号组织没有默认账号时必须让用户指定账号；禁止选择第一项、最近登录或最近使用账号。
- 不传 `--profile` 使用全局 `isCurrent=true` 账号。`primaryProfile/isPrimary` 仅兼容输出，不参与选择；`previousProfile` 只用于 `profile switch -`。
- 跨组织读 / 搜：按 `corpId` 去重；每个组织使用唯一 `isOrgCurrent=true` 的 `profile`。组织存在多个账号且没有默认账号时先询问用户。写 / 发 / 删 / 撤回及持久切换前先确认目标组织和账号。

## 产品总览

| 产品                | 用途                                                   | 参考文件                                                           |
|-------------------|------------------------------------------------------|----------------------------------------------------------------|
| `aiapp`           | AI应用：创建/查询/修改AI应用                                       | [aiapp.md](./references/products/aiapp.md)                     |
| `aisearch`        | AI搜问（搜人首选）：按姓名/部门/职位/职责/上级/下级/手机号/工号维度找人，"谁负责 XX/XX 的负责人/某事项/某项目的人"统一走本产品 | [aisearch.md](./references/products/aisearch.md)               |
| `aitable`         | AI表格：Base/数据表/字段/记录/视图/附件/图表/仪表盘/导入导出/模板搜索            | [aitable.md](./references/products/aitable.md)                 |
| `attendance`      | 考勤：打卡结果/打卡流水/考勤组查询/考勤规则/汇总统计/假期类型/假期余额（P0 已落地，部分管理类命令仍属 P1） | [attendance.md](./references/products/attendance.md)           |
| `calendar`        | 日历：日历列表/日程/参与者/附件/响应/会议室/闲忙查询/时间建议                  | [calendar.md](./references/products/calendar.md)               |
| `chat`            | 群聊与机器人：搜索群/建群/群成员管理/改群名/消息发送(文本/Markdown/图片/文件)/拉取消息/消息收藏/@我/特别关注/机器人群发/单聊/撤回/转发/引用回复/Webhook/机器人搜索 | [chat.md](./references/products/chat.md)                       |
| `contact`         | 通讯录：用户查询/部门/角色/花名册/离职员工/特别关注，以及创建企业、企业账号和邀请员工              | [contact.md](./references/products/contact.md)                 |
| `devdoc`          | 开放平台文档：搜索开发文档                                        | [devdoc.md](./references/products/devdoc.md)                   |
| `ding`            | DING消息：发送/撤回（应用内/短信/电话）                              | [ding.md](./references/products/ding.md)                       |
| `doc`             | 钉钉文档：搜索/浏览/读写/块级编辑/评论/文件创建/复制/移动/重命名/**删除/导出 docx/权限管理/媒体上传下载**       | [doc.md](./references/products/doc.md)                         |
| `drive`           | 钉钉云盘：文件列表/元数据/文件夹/上传(两步)/下载                        | [drive.md](./references/products/drive.md)                     |
| `minutes`         | AI听记：听记列表/摘要/关键词/转写/待办/思维导图/发言人/发言人段落总结/热词/录音控制/成员权限/上传 | [minutes.md](./references/products/minutes.md)                 |
| `oa`              | OA审批：待处理/详情/同意/拒绝/撤销/记录/已发起/任务/转交/评论/抄送              | [oa.md](./references/products/oa.md)                           |
| `pat`             | PAT 行为授权：浏览器策略/scope 预览/一次性、会话或永久授权                    | [pat.md](./references/products/pat.md)                         |
| `report`          | 日志：按模版创建/收件箱/已发送/模版查看/详情/已读统计                         | [report.md](./references/products/report.md)                   |
| `mail`            | 邮箱：邮箱地址查询/邮件搜索(KQL)/邮件详情/发送邮件                        | [mail.md](./references/products/mail.md)                       |
| `sheet`           | 在线电子表格(axls)：工作表 CRUD/区域读写/CSV 批量写入/行列增删/合并/查找替换/筛选视图/全局筛选/排序/下拉列表/条件格式/浮动图片/浮动图表/模板/导出 xlsx(单命令一站式) | [sheet.md](./references/products/sheet.md)                     |
| `todo`            | 待办：创建(含优先级/截止时间/循环)/查询/修改/标记完成/删除                   | [todo.md](./references/products/todo.md)                       |
| `wiki`            | 知识库：空间创建/详情/列表/搜索 + 成员管理                                | [wiki.md](./references/products/wiki.md)                       |
| `event`           | 个人 IM 事件：监听消息接收、指定发送人、已读、撤回、表情回应，NDJSON 输出（实时驱动 Agent）| [event.md](./references/products/event.md)                     |

## 意图判断决策树

用户提到"AI应用/创建应用/生成系统/做工具/管理后台/低代码" → `aiapp`
用户提到"目标管理/Agoal/战略解码/经营合约/计分卡/目标模板/周月报提交统计" → `agoal`
用户提到"找人/搜人/谁负责 XX/某事项的负责人/某项目的人/团队成员/上级/下级/按工号找人/按手机号找人" → `aisearch`
用户提到"表格/多维表/AI表格/记录/数据/视图/图表/仪表盘" → `aitable`
用户提到"考勤/打卡/排班" → `attendance`
用户提到"日程/日历/会议室/约会/时间建议" → `calendar`
用户提到"群聊/建群/群成员/群管理/发消息/发图片消息/发文件消息/发 Markdown 消息/截图发钉钉/转发消息/引用回复/@我/特别关注消息/机器人发消息/Webhook/机器人群发/机器人单聊/通知" → `chat`
用户提到"通讯录/同事/部门/组织架构/子部门/部门多少人/离职员工/离职名单/离职花名册/花名册/员工档案/学历/家庭/银行卡/紧急联系人/合同/角色/主管角色/管理员角色/财务/HR/特别关注/星标联系人/创建企业/企业账号/邀请员工/新员工入职" → `contact`
用户提到"开发/API/调用错误 文档" → `devdoc`
用户提到"DING/紧急消息/电话提醒" → `ding`
用户提到"钉钉文档/云文档/知识库/读写文档/块级编辑/文档评论/文档复制移动" → `doc`
用户提到"云盘/文件存储/文件上传下载/文件夹" → `drive`
用户提到"听记/AI听记/会议纪要/转写/摘要/思维导图/发言人/热词" → `minutes`
用户提到"邮箱/邮件/发邮件/收邮件/搜邮件/查邮件/邮件草稿/转发邮件/回复邮件/邮件附件/抄送" → `mail`
用户提到"审批/请假/报销/出差/加班/同意/拒绝/撤销审批" → `oa`
用户提到"PAT 授权/行为权限/scope 授权/批量授权/一次性授权/会话授权/永久授权/授权浏览器策略" → `pat`
用户提到"日志/日报/周报/日志统计/写日报/提交周报/发日志/填日志" → `report`
用户提到"在线电子表格/钉钉表格/axls/工作表/单元格读写/合并单元格/筛选视图/导出 xlsx" → `sheet`
用户提到"待办/TODO/任务提醒/循环待办" → `todo`
用户提到"创建知识库/知识库列表/搜索知识库空间/wiki/团队空间/知识库成员管理/我的文档个人空间" → `wiki`
用户提到"监听有人@我/监听单聊或群消息/监听某人发送的消息/监听消息已读/监听消息撤回/监听消息贴表情或表情回应/订阅个人 IM 事件/实时接收钉钉事件/个人事件流/event consume user_im_message_*/监听并自动回复消息/驱动 Agent 处理消息" → `event`

关键区分: aitable(数据表格) vs todo(待办任务)
关键区分: report(钉钉日志/日报周报) vs todo(待办任务)
关键区分: chat send-by-bot(机器人身份发消息) vs send-by-webhook(自定义机器人Webhook告警)
关键区分: doc(钉钉文档/富文本协同) vs drive(钉钉云盘/二进制文件)
关键区分: oa tasks(审批 taskId，审批/拒绝用) vs oa list-pending(收件箱 processInstanceId，查看用)


> 更多易混淆场景见 [intent-guide.md](./references/intent-guide.md)

## 危险操作确认

以下操作为不可逆或高影响操作，执行前**必须先向用户展示操作摘要并获得明确同意**，同意后才加 `--yes` 执行。

| 产品 | 命令 | 说明 |
|------|------|------|
| `aitable` | `base delete` | 删除整个 AI 表格，含全部数据表和记录 |
| `aitable` | `table delete` | 删除数据表（含全部字段/视图/记录） |
| `aitable` | `field delete` | 删除字段（该列所有值同步清空） |
| `aitable` | `view delete` | 删除视图 |
| `aitable` | `record delete` | 删除记录（支持批量） |
| `aitable` | `chart delete` / `dashboard delete` | 删除图表/仪表盘 |
| `calendar` | `event delete` | 删除日程，所有参与者同步取消 |
| `calendar` | `participant delete` | 移除日程参与者 |
| `calendar` | `room delete` | 取消会议室预定 |
| `chat` | `group members remove` | 移除群成员 |
| `chat` | `message recall-by-bot` | 撤回机器人已发消息 |
| `doc` | `delete` | **删除整篇文档/文件**到回收站（与 `block delete` 不同，本命令删除整个 node） |
| `doc` | `block delete` | 删除文档单个块（不可恢复） |
| `doc` | `permission update` | 修改协作者权限（降权可能影响他人访问） |
| `ding` | `message recall` | 撤回已发 DING 消息 |
| `oa` | `approval revoke` | 撤销自己发起的审批实例 |
| `oa` | `approval reject` | 拒绝待审批（需加明确理由） |
| `todo` | `task delete` | 删除待办 |
| `minutes` | `replace-text` | 全文批量替换转写与摘要 |

### 确认流程
```
Step 1 → 展示操作摘要（操作类型 + 目标对象 + 影响范围）
Step 2 → 用户明确回复确认（如 "确认" / "好的"）
Step 3 → 加 --yes 执行命令
```

## 核心流程
作为一个智能助手，你的首要任务是**理解用户的真实、完整的意图**，而不是简单地执行命令。在选择 `dws` 的产品命令前，必须严格遵循以下四步流程：

0. **URL 预检**：输入含 `alidocs.dingtalk.com` URL 时，该域名下存在多种路径格式（`/i/nodes/...`、`/i/p/...`、`/spreadsheetv2/...`、`/document/edit|preview?dentryKey=...` 等），每种的处理流程不同。**必须先读取 [url-patterns.md](./references/url-patterns.md) 中的「alidocs URL 分流决策」**，按其中规则识别 URL 类型后再选择对应产品。含 `shanji.dingtalk.com` URL 时直接路由到 `minutes`。URL 已识别后直接进入对应产品流程，无需后续步骤。
1. 意图分类：首先，判断用户指令的核心 动词/动作 属于哪一类。这比关注名词更重要。
2. 歧义处理与信息追问：如果用户指令模糊或包含多个产品的关键字，严禁猜测。必须主动向用户追问以澄清意图。这是你作为智能助手而非命令执行器的核心价值。
3. 精准产品映射：在完成前两步，意图已经清晰后，参考产品总览和意图判断决策树 来选择产品。
4. 充分阅读产品参考文件，通过编写代码或直接调用指令实现用户意图。

## 命令发现（Schema 渐进查询 + --help 互为补充）

### Schema 渐进查询（Agent 选命令首选）

`dws schema` 内嵌当前二进制公开命令面的结构化契约。**Agent 选择命令、读取参数映射/约束和安全语义时必须优先渐进查询 leaf Schema**；真正组装执行参数前，用 `--help` 确认当前 Cobra 接受的 flags：

本节适用于进入 Runtime Schema 的基础/原子命令。`+` shortcut 是明确例外：按“Shortcut 与原子命令的使用原则”读取 `dws shortcut list`，不得把 Schema lookup 失败误判为 shortcut 不可执行。

稳定 command identity、主 CLI path 和 alias 已在构建时由 reviewed registry 与真实 Cobra tree 精确绑定。Agent 不应读取 Catalog 文件、native annotation 或其他生成 JSON 来重新推断命令；所有运行时查询都以当前二进制交付的 Schema 投影为准。

```bash
# 第 1 层：产品概览（~4.5KB，列出全部产品 + 工具数 + 用途摘要）
dws schema

# 第 2 层：产品级（列出该产品下全部工具的 cli_path + description + effect/risk）
dws schema calendar

# 第 3 层：分组级（按命令分组列出工具摘要）
dws schema "calendar event"

# 第 4 层：完整 leaf（参数契约：type/required/description/constraints/examples）
dws schema "calendar event create"

# --compact：当前支持；去除 provenance/debug 字段，仅保留 Agent 选参所需
dws schema "calendar event create" --compact

# --all：导出所有工具的完整 leaf Schema，仅用于 CI / 审计 / 参数 baseline
dws schema --all --format json
```

**`--all` 使用边界（强制）**：`--all` 会返回每个工具的完整参数、约束和安全语义，输出体积很大。仅在用户明确要求全量导出，或执行 CI、Catalog 审计、参数防丢 baseline 时使用。普通业务任务严禁使用 `--all` 做命令发现，也不要把全量结果直接注入 Agent 上下文；必须按“产品概览 → 产品/分组 → leaf”渐进查询。完整兼容性 baseline 必须使用未裁剪的 `schema --all`；`schema --all --compact` 会移除 provenance 和接口映射字段，不得作为完整 baseline。

同一个工具的 leaf 查询与 `--all` 条目是同一份 `ToolSpec` 契约的投影，参数、安全和接口语义必须一致。Alias 查询只改变路径视图；不得根据 alias 重写或补猜参数。若观察到内容差异，应作为契约漂移报告，而不是选择其中一份继续执行。

**--compact 模式**去掉的字段：`agent_metadata_source`、`agent_source_refs`、`agent_summary_source`、`effect_source`、`metadata_source`、`interface_ref`、`interface_description`、`property`、`primary_cli_path`、`parameter_count` 等 provenance/debug 字段。保留的字段：`cli_path`、`canonical_path`、`description`、`effect`、`risk`、`confirmation`、`interface_mode`、`availability`、`interface_reason`、`parameters`（含 `type`/`required`/`description`/`default`/`enum`）、`constraints`、`examples`、`use_when`、`avoid_when`。

`--compact` 是 Schema 展示能力。当前版本支持；若兼容旧二进制时收到 `unknown_flag: --compact`，仅去掉 `--compact` 重跑同一个 Schema 查询。不要因此判定 leaf 不存在，也不要改用 Schema 查询业务数据。

### Schema 字段速查

```jsonc
// leaf 级输出（dws schema "calendar event create" --compact）
{
  "cli_path": "calendar event create",
  "canonical_path": "calendar.create_calendar_event",
  "description": "创建新的日程...",
  "effect": "write",           // read | write | destructive
  "risk": "medium",            // low | medium | high
  "confirmation": "not_required", // not_required | user_required
  "interface_mode": "mcp",     // mcp | local | composite
  "availability": "available", // available | unavailable
  "interface_reason": "",
  "parameters": {
    "title": { "type": "string", "required": true, "description": "..." },
    "start": { "type": "string", "required": true, "format": "date-time" }
    // ...
  },
  "constraints": { "require_together": [["recurrence-type", "recurrence-interval", "recurrence-range-type"]] },
  "examples": ["dws calendar event create --title ..."]
}
```

- `confirmation=user_required` → 必须先向用户确认再加 `--yes`；不要根据 `effect` 或 `risk` 的值自行重写最终 confirmation winner
- `availability=unavailable` → 不执行该工具；向用户说明 `interface_reason`。`interface_mode` 只描述实现机制，不能覆盖 availability
- `parameters.<flag>.required=true` → Agent 应提供该参数；Cobra 是否硬拒绝以 `--help`/实际命令契约为准
- `parameters.<flag>.cli_required=true` → Cobra 将该 flag 标记为硬必填
- `constraints.require_together` → 列出的 flag 必须同时提供

### Schema、Help 与业务数据的边界

| 信息 | 事实源 |
|------|--------|
| 命令是否存在、当前 Cobra 接受哪些 flags | `dws <cli_path> --help` |
| Agent 选择、参数映射/required/组合约束、risk/confirmation（原子/基础命令） | `dws schema "<cli_path>"`（按需加 `--compact`） |
| shortcut 的参数、组合约束、risk/confirmation、示例 | `dws shortcut list --service <service> --format json` |
| 人类可读用法 | `dws <cli_path> --help` |
| 钉钉中的文档、文件、日程、消息等实际数据 | 真正执行对应的 `read` / `search` / `list` 命令 |

Schema 与 Help 冲突是**契约漂移**，不得静默猜测或把两边字段随意拼接：

- 执行参数只使用 Cobra/Help 接受的 flags，并把漂移报告出来。
- 安全语义冲突时不能选择更宽松行为；先采用更保守的确认方式，无法确认安全执行方式时停止并报告。
- leaf Schema 是已经按来源 precedence 解析后的契约；不要根据值的“严格程度”自行改写 winner。只有发现它与 Help/实际执行契约冲突时，才进入上述安全降级。

`dws schema` 只查询命令契约，不搜索钉钉文档或业务数据。完成命令发现后，必须继续执行真实命令，例如 `dws doc read`、`dws drive search`；不要把 Schema 查询结果当成业务查询结果。

### Helper-only 与本地 Cobra 命令

`dev.*` 包含 helper-only 执行面，其中远端 helper 未进入 pinned metadata 时标记为 `composite`，不能伪装成 `local`。`event list` / `event schema` 读取内置目录和 payload 定义，属于 `local`；`event consume` / `event status` / `event stop` 同时编排远端个人订阅控制面与本地 bus/consume，属于 `composite`。实现来源不同，不改变统一查询边界：进入全局 `dws schema` 的命令必须先进入 reviewed CommandRegistry，并由同一 `ToolSpec` 投影到 leaf、产品/分组、`--all` 与 Catalog。不得在查询时重新调用 MCP `tools/list`，也不得把 Cobra 临时合成结果作为第二条 Schema 数据路径。

事件需要区分两种 Schema：`dws event schema <event_key> --flatten` 查询 Agent 要消费的顶层业务字段；`dws schema "event consume"` 查询 CLI 命令参数。前者是真实业务命令，后者只读取最终内嵌 SchemaRegistry；不能相互替代。

`source` 表示最终命令 identity 的来源，不表示运行时 backing；helper/local/MCP 实现机制读取 `interface_mode`、`availability` 和 provenance，不要假定 `dev.*` 必然是 `source=mcp:<server>`，也不要假定本地命令必然是 `source=cobra`。

## 错误处理
1. 遇到错误，加 `--verbose` 重试一次
2. 若 stderr 出现 `RECOVERY_EVENT_ID=<event_id>`，优先按 [recovery-guide.md](./references/recovery-guide.md) 执行 recovery 闭环
3. 仍然失败，报告完整错误信息给用户，禁止自行尝试替代方案
4. 认证失败时，参考 [global-reference.md](./references/global-reference.md) 中的认证章节处理
5. 各产品高频错误及排查流程见 [error-codes.md](./references/error-codes.md)
6. 遇到 [capability-limits.md](./references/capability-limits.md) 中列出的「已知不支持操作」时，**直接告知用户不支持并建议在钉钉客户端操作**，不要重试或变通


## 详细参考 (按需读取)

- [references/products/](./references/products/) — 各产品命令详细参考（Cobra 接受的 flag 以叶子 `--help` 为准；基础命令的 Agent 映射/约束/安全语义以 leaf Schema 为准，shortcut 以独立 catalog 为准）
- [references/intent-guide.md](./references/intent-guide.md) — 意图路由指南（易混淆场景对照）
- [references/url-patterns.md](./references/url-patterns.md) — URL 格式规范 + alidocs URL 分流决策与类型探测流程（含钉盘 `document/edit|preview?dentryKey=` 链接）
- [references/global-reference.md](./references/global-reference.md) — 全局标志、认证、输出格式
- [references/field-rules.md](./references/field-rules.md) — AI表格字段类型规则
- [references/error-codes.md](./references/error-codes.md) — 错误码 + 调试流程
- [references/recovery-guide.md](./references/recovery-guide.md) — recovery 闭环、`RECOVERY_EVENT_ID`、`execute/finalize` 规范
- [scripts/](./scripts/) — 各产品批量/复合操作脚本（AI表格批量导入导出、AI应用创建轮询、日历、机器人消息、通讯录、考勤、日志、待办、文档创建并写入、钉盘目录树等）
- [references/products/aitable/](./references/products/aitable/) — AI表格细分章节（单元格值/字段属性/公式/筛选排序/导入导出/仪表盘/记录增删改查/错误恢复/最佳实践）
- [references/products/aitable-record-ops.md](./references/products/aitable-record-ops.md) — AI表格记录操作专项说明
- [references/products/pat.md](./references/products/pat.md) — PAT 浏览器策略、行为 scope 预览与授权安全要求
- [references/capability-limits.md](./references/capability-limits.md) — 已知能力限制（doc/aitable/chat/minutes，遇到时直接告知用户不支持）
- [references/best_practices/](./references/best_practices/) — 全场景 recipe 行动指南（11 个编号场景 + lite 速查）
  - [01-messaging.md](./references/best_practices/01-messaging.md) — 消息沟通
  - [02-task.md](./references/best_practices/02-task.md) — 任务管理（todo）
  - [03-meeting.md](./references/best_practices/03-meeting.md) — 会议日程（日历 + 会议室）
  - [04-document.md](./references/best_practices/04-document.md) — 文档场景（write-doc / search-docs / migrate-doc / update-doc-section / doc-to-message / delete-old-doc / export-doc-as-docx / grant-doc-access / insert-image-to-doc / template-based-generation 等）
  - [05-reporting.md](./references/best_practices/05-reporting.md) — 工作汇报（钉钉日志 / 文档周报选路）
  - [06-data-analytics.md](./references/best_practices/06-data-analytics.md) — AI表格数据分析（read-aitable / generate-data-report / create-aitable-record / update-aitable-record / export-aitable-to-xlsx / primary-doc-from-record 等）
  - [07-minutes.md](./references/best_practices/07-minutes.md) — 听记与会后
  - [08-directory.md](./references/best_practices/08-directory.md) — 通讯录（组织架构）
  - [09-mail.md](./references/best_practices/09-mail.md) — 邮件
  - [10-minutes-speaker-match.md](./references/best_practices/10-minutes-speaker-match.md) — 听记发言人智能匹配
  - [11-minutes-speaker-correct.md](./references/best_practices/11-minutes-speaker-correct.md) — 听记发言人识别与标注
  - [lite-recipes.md](./references/best_practices/lite-recipes.md) — Lite Recipe 速查（核心流程判定为 lite 后直接执行）
  - [_common/conventions.md](./references/best_practices/_common/conventions.md) — 批量查询、多源并行采集、字段术语等通用规范
  - [_common/recipe-conventions.md](./references/best_practices/_common/recipe-conventions.md) — recipe 元规范
