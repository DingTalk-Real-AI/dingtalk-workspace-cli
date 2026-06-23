---
name: dws
description: 管理钉钉产品能力(AI表格/AI搜问/日历/通讯录/群聊与机器人/待办/审批/考勤/日志/DING消息/开放平台文档/钉钉文档/钉钉云盘/AI听记/邮箱/在线电子表格/知识库等)。当用户需要操作表格数据、管理日程会议、模糊找人/查谁负责某事项、查询通讯录、管理群聊、机器人发消息、创建待办、提交审批、查看考勤、提交日报周报（钉钉日志模版）、读写钉钉文档、上传下载云盘文件、查询听记纪要、收发邮件、读写在线电子表格(axls)、管理钉钉知识库时使用。
cli_version: ">=1.0.15"
---

# 钉钉全产品 Skill

通过 `dws` 命令管理钉钉产品能力。


> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。

## 严格禁止 (NEVER DO)
- 不要使用 dws 命令以外的方式操作（禁止 curl、HTTP API、浏览器）
- 不要编造 UUID、ID 等标识符，必须从命令返回中提取
- 不要编造 URL、Email、手机号等结构化信息，必须从命令返回中提取或由用户明确提供
- 不要猜测字段名/参数值，操作前必须先查询确认
- 禁止编造命令路径、子命令或 flag；产品参考缺失、路径/flag 不确定，或报 `unknown command` / `unknown flag` 时，必须先运行对应层级的 `dws <path> --help` 查证后再执行或重试

## 严格要求 (MUST DO)
- DWS 命令合法性协议：执行 `dws` 前必须用当前 skill 资料确认命令；产品参考已覆盖时直接按参考执行，缺失或不确定时必须先用 `--help` 查证
- 所有命令必须加 `--format json` 以获取可解析输出
- 危险操作必须先向用户确认，用户同意后才加 `--yes` 执行
- 单次批量操作不超过 30 条记录
- 所有命令必须**严格遵循**对应产品参考文档里面规定的参数格式（如：如果有参数值，则参数和参数值之间至少用一个空格隔开）
- **脚本优先**：[scripts/](./scripts/) 下的 `python scripts/<name>.py` 已封装翻页/轮询/批量逻辑，遇到对应场景（如 AI 表格批量导入导出、AI 应用创建轮询、文档创建后写内容、钉盘目录树等）**优先调用脚本**而非手写多步命令。脚本均支持 `--dry-run` 预览、`--format json` 输出，失败时回退到手动步骤
- **业务域最佳实践优先**：文档类多步任务先读 [04-document.md](./references/best_practices/04-document.md)；AI 表格读取/统计/写入/导入导出先读 [06-data-analytics.md](./references/best_practices/06-data-analytics.md)。本仓库只迁入这些业务域 best practices，不引入其它产品行动指南。
- 知识库容器只用 `dws wiki space/member`；知识库内文件/文档的浏览、搜索、读取、创建、移动、复制统一切到 `dws doc`。`workspaceId` 只能传给 `wiki --workspace`、`doc --workspace` 或 `doc search --workspace-ids`，禁止传给 `doc list --folder`，也不要使用不存在的 `--space-id`。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按本文件「错误处理」执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。


## 产品总览

> 若用户意图涉及多步操作、汇总/整理/归纳/分析、文档创建后写入内容、知识库内文档处理、AI 表格批量读写/统计/导入导出，**先匹配下方「行动指南」**；仅当行动指南无匹配且明确是单一产品单步操作时，按本表路由。

| 产品                | 用途                                                   | 参考文件                                                           |
|-------------------|------------------------------------------------------|----------------------------------------------------------------|
| `aisearch`        | AI搜问（搜人首选）：按姓名/部门/职位/职责/上级/下级/手机号/工号维度找人，"谁负责 XX/XX 的负责人/某事项/某项目的人"统一走本产品 | [aisearch.md](./references/products/aisearch.md)               |
| `aitable`         | AI表格：Base/数据表/字段/记录/视图/附件/图表/仪表盘/导入导出/模板搜索            | [aitable.md](./references/products/aitable.md)                 |
| `attendance`      | 考勤：打卡结果/打卡流水/考勤组查询/考勤规则/汇总统计/假期类型/假期余额（P0 已落地，部分管理类命令仍属 P1） | [attendance.md](./references/products/attendance.md)           |
| `calendar`        | 日历：日历列表/日程/参与者/附件/响应/会议室/闲忙查询/时间建议                  | [calendar.md](./references/products/calendar.md)               |
| `chat`            | 群聊与机器人：搜索群/建群/群成员管理/改群名/消息发送(文本/Markdown/图片/文件)/拉取消息/@我/特别关注/机器人群发/单聊/撤回/转发/引用回复/Webhook/机器人搜索     | [chat.md](./references/products/chat.md)                       |
| `contact`         | 通讯录：用户查询(当前用户/搜索/详情/手机号)/花名册档案(学历/家庭/银行卡/合同)/离职员工查询(姓名/时间范围/部门)/部门查询(搜索/详情/子部门/成员)/角色查询(主管/管理员/财务/HR 等 label)/特别关注列表              | [contact.md](./references/products/contact.md)                 |
| `devdoc`          | 开放平台文档：搜索开发文档                                        | [devdoc.md](./references/products/devdoc.md)                   |
| `ding`            | DING消息：发送/撤回（应用内/短信/电话）                              | [ding.md](./references/products/ding.md)                       |
| `doc`             | 钉钉文档：搜索/浏览/读写/块级编辑/评论/文件创建/复制/移动/重命名/**删除/导出 docx/权限管理/媒体上传下载**；创建/编辑先按渐进式 doc 子文档与 JSONML 工作流决策       | [doc.md](./references/products/doc.md)                         |
| `drive`           | 钉钉云盘：文件列表/元数据/文件夹/上传(两步)/下载                        | [drive.md](./references/products/drive.md)                     |
| `minutes`         | AI听记：听记列表/摘要/关键词/转写/待办/思维导图/发言人/发言人段落总结/热词/录音控制/成员权限/上传 | [minutes.md](./references/products/minutes.md)                 |
| `oa`              | OA审批：待处理/详情/同意/拒绝/撤销/记录/已发起/任务/转交/评论/抄送              | [oa.md](./references/products/oa.md)                           |
| `report`          | 日志：按模版创建/收件箱/已发送/模版查看/详情/已读统计                         | [report.md](./references/products/report.md)                   |
| `mail`            | 邮箱：邮箱地址查询/邮件搜索(KQL)/邮件详情/发送邮件                        | [mail.md](./references/products/mail.md)                       |
| `sheet`           | 在线电子表格(axls)：工作表 CRUD/区域读写/CSV 批量写入/行列增删/合并/查找替换/筛选视图/全局筛选/排序/下拉列表/浮动图片/导出(两步) | [sheet.md](./references/products/sheet.md)                     |
| `todo`            | 待办：创建(含优先级/截止时间/循环)/查询/修改/标记完成/删除                   | [todo.md](./references/products/todo.md)                       |
| `wiki`            | 知识库：空间创建/详情/列表/搜索 + 成员管理                                | [wiki.md](./references/products/wiki.md)                       |

## 核心流程（每次请求必须执行，不得跳过）

作为一个智能助手，你的首要任务是**理解用户的真实、完整意图**，不是简单执行第一条看起来相关的命令。在选择 `dws` 产品命令前，必须按以下流程执行：

0. **URL 预检**：输入含 `alidocs.dingtalk.com` URL 时，必须先读取 [url-patterns.md](./references/url-patterns.md) 的「alidocs URL 分流决策」，识别 URL 是文档、文件夹、知识库、表格、分享短链还是其它格式，再选择对应产品。含 `shanji.dingtalk.com` URL 时直接路由到 `minutes`。
1. **意图拆解**：判断用户请求是否包含多个时序步骤（如“创建文件夹，然后创建文档并写入内容”“读文档，然后总结并评估”“建 AI 表格，再加字段和记录”）。若是，拆成多个子意图，按顺序执行，前一步产出的 `workspaceId` / `nodeId` / `baseId` / `tableId` 必须作为后一步输入。
2. **行动指南优先匹配**：将用户意图或拆解后的子意图与下方「行动指南」逐行做语义比对。命中文档知识或 AI 表格数据任一行时，必须先读取对应 best practice，再读取对应产品参考文件执行；文档知识场景还必须进入 [doc.md](./references/products/doc.md) 的渐进式文档索引按需加载子文件。
3. **recipe 分级执行**：当前开源版只迁入业务域 full recipe（`04-document.md`、`06-data-analytics.md`），未迁入悟空完整 `lite-recipes.md`。因此命中下方任一业务域行动指南时，一律按 full recipe 处理：先读行动指南，再读产品参考，不要只凭主 skill 摘要直接执行。文档创建/更新/块级编辑按 `doc.md` 前置条件继续读取 `doc/style/doc-create-workflow.md`、`doc/style/doc-update-workflow.md`、`doc/format/doc-jsonml-schema.md` 或 `doc/format/doc-jsonml-cookbook.md`，优先保真使用 JSONML。
4. **Fallback 单产品路由**：仅当行动指南未命中，且用户意图明确是单一产品单步操作时，才按「产品总览」和「意图判断决策树」选择产品，并读取对应 `references/products/*.md`。
5. **追问**：以上步骤都无法判断时，主动追问用户澄清，严禁猜测命令、flag、URL、ID 或字段名。

## 行动指南（优先匹配）

> 将用户意图与下表做**语义比对**，不要求字面包含关键词。命中后必须读取该行动指南文件，并按其中固定路线执行；多个场景同时命中时，按下方「消歧规则」选择。

| # | 场景 | 触发关键词 / 能力范围 | 行动指南 |
|---|------|----------------------|----------|
| 4 | 文档知识 | 搜索/浏览/读取/创建/更新/迁移/模板复用/导出钉钉文档；知识库内文档处理；文件夹下创建文档；块级编辑；JSONML 保真改写；图片/附件/PDF/Excel/PPT 嵌入正文；读文档后总结/评估/对照 | [04-document.md](./references/best_practices/04-document.md) |
| 6 | AI 表格数据 | AI 表格读取/统计/写入记录/更新记录/字段/视图/导入导出/模板建表/主文档读取 | [06-data-analytics.md](./references/best_practices/06-data-analytics.md) |

### 消歧规则

- "工作台应用/app001/appXYZ/钉钉工作台上有哪些应用" → `workbench app`。
- "开放平台接口文档/API 怎么调用/错误码/字段说明" → `devdoc`。
- "MCP 服务/connector/创建工具/HSF 映射/上架 MCP" → MCP 平台配置流程。
- 只说"应用"且缺少上下文时，先结合当前对话判断；仍不明确时追问是工作台应用、文档能力还是 MCP 工具。
- "知识库空间/成员管理" → `wiki` 产品参考；"知识库里的文档/文件/内容" → #4 文档知识，再切 `doc`。
- "创建表格/在线电子表格/单元格" → `sheet`；"AI 表格/多维表/base/记录/字段/视图" → #6 AI 表格数据。
- "写文档/读文档/总结文档/插入图片附件/块级编辑/JSONML 保真改写" → #4 文档知识，不要停在 `doc create` 或 `doc block --help`；创建/编辑必须继续按 [doc.md](./references/products/doc.md) 加载渐进式子文档和 JSONML workflow。
- "导出文档后归档/上传" → 先 #4 完成 `doc export`，再按产品路由继续 `drive upload` 等后续动作。

## 意图判断决策树

用户提到"找人/搜人/谁负责 XX/某事项的负责人/某项目的人/团队成员/上级/下级/按工号找人/按手机号找人" → `aisearch`
用户提到"表格/多维表/AI表格/记录/数据/视图/图表/仪表盘" → `aitable`
用户提到"考勤/打卡/排班" → `attendance`
用户提到"日程/日历/会议室/约会/时间建议" → `calendar`
用户提到"群聊/建群/群成员/群管理/发消息/发图片消息/发文件消息/发 Markdown 消息/截图发钉钉/转发消息/引用回复/@我/特别关注消息/机器人发消息/Webhook/机器人群发/机器人单聊/通知" → `chat`
用户提到"通讯录/同事/部门/组织架构/子部门/部门多少人/离职员工/离职名单/离职花名册/花名册/员工档案/学历/家庭/银行卡/紧急联系人/合同/角色/主管角色/管理员角色/财务/HR/特别关注/星标联系人" → `contact`
用户提到"开发/API/调用错误 文档" → `devdoc`
用户提到"DING/紧急消息/电话提醒" → `ding`
用户提到"钉钉文档/云文档/读写文档/知识库里的文档/浏览知识库内容/知识库内搜索文档/块级编辑/文档评论/文档复制移动" → `doc`
用户提到"云盘/文件存储/文件上传下载/文件夹" → `drive`
用户提到"听记/AI听记/会议纪要/转写/摘要/思维导图/发言人/热词" → `minutes`
用户提到"邮箱/邮件/发邮件/收邮件/搜邮件/查邮件/邮件草稿/转发邮件/回复邮件/邮件附件/抄送" → `mail`
用户提到"审批/请假/报销/出差/加班/同意/拒绝/撤销审批" → `oa`
用户提到"日志/日报/周报/日志统计/写日报/提交周报/发日志/填日志" → `report`
用户提到"在线电子表格/钉钉表格/axls/工作表/单元格读写/合并单元格/筛选视图/导出 xlsx" → `sheet`
用户提到"待办/TODO/任务提醒/循环待办" → `todo`
用户提到"创建知识库/知识库列表/搜索知识库空间/wiki/团队空间/知识库成员管理/我的文档个人空间" → `wiki`

关键区分: aitable(数据表格) vs todo(待办任务)
关键区分: report(钉钉日志/日报周报) vs todo(待办任务)
关键区分: chat send-by-bot(机器人身份发消息) vs send-by-webhook(自定义机器人Webhook告警)
关键区分: doc(钉钉文档/富文本协同) vs drive(钉钉云盘/二进制文件)
关键区分: wiki(知识库空间/成员管理) vs doc(知识库内文档内容读写)。用户要读/搜/列知识库里的文档时：先 `wiki space list/search` 拿 `workspaceId`，再 `doc list --workspace` / `doc search --workspace-ids` / `doc read --node`。
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

## 命令发现（flag / 参数以 binary 为准）

产品参考文档（`references/products/*.md`）里的 flag 列表是**便于理解用途的参考**，不是权威契约。参数名称、默认值、必填约束随服务发现动态变化，**以下两个命令的输出才是调用的事实源**：

```bash
# 1) 人读视图：看 Usage / Example / Flags
dws <command-path> --help
# 例：dws calendar event list --help

# 2) 机读视图：JSON Schema + flag 别名映射 + 必填字段
dws schema                                 # 列出所有产品及工具
dws schema <product>.<canonical_name>      # 规范路径（如 calendar.list_suggested_event_times）
dws schema "<product> <group> <cli_name>"  # CLI 路径（如 "calendar event list"）
dws schema <path> --jq '.tool.flag_overlay'  # 只看 flag 别名
dws schema <path> --jq '.tool.required'      # 只看必填字段
```

**何时用哪条路径：**
- 只需看某个命令怎么调用 → `dws <cmd> --help`
- 构造 `--params` / `--json` 时不确定字段类型、必填、别名 → `dws schema <path>`
- 参考文档和 `--help` 冲突时 → **以 `--help` / `dws schema` 为准**，文档视为过期

`dws schema` 输出的 `flag_overlay[key].alias` 就是实际生效的 flag 名（如 `attendeeUserIds → --attendee-user-ids`）；`parameters[key]` 是原始 JSON Schema；`required` 是必填字段数组；`sensitive: true` 表示写/删操作，须先向用户确认再加 `--yes`。

## 错误处理
1. 遇到错误，加 `--verbose` 重试**一次**
2. 若 stderr 出现 `RECOVERY_EVENT_ID=<event_id>`，优先按 [recovery-guide.md](./references/recovery-guide.md) 执行 recovery 闭环
3. 仍然失败，**立即停止**并报告完整错误信息，禁止自行尝试替代方案或反复变通
4. **严禁**连续重试超过 3 次相同或类似的命令；如果 3 次仍失败，必须停止并报告
5. 报 `unknown command` / `unknown flag` 时，先运行对应层级 `dws <path> --help` 查证，再修正一次；不要把自然语言同义词直接当命令或 flag
6. 逐条命令多次失败时，检查 [scripts/](./scripts/) 是否有对应业务域脚本可降级使用
7. 认证失败时，参考 [global-reference.md](./references/global-reference.md) 中的认证章节处理
8. 各产品高频错误及排查流程见 [error-codes.md](./references/error-codes.md)
9. 遇到 [capability-limits.md](./references/capability-limits.md) 中列出的「已知不支持操作」时，**直接告知用户不支持并建议在钉钉客户端操作**，不要重试或变通


## 详细参考 (按需读取)

- [references/products/](./references/products/) — 各产品命令详细参考（flag 细节以 `--help` / `dws schema` 为准）
- [references/intent-guide.md](./references/intent-guide.md) — 意图路由指南（易混淆场景对照）
- [references/url-patterns.md](./references/url-patterns.md) — URL 格式规范 + alidocs URL 分流决策与类型探测流程（含钉盘 `document/edit|preview?dentryKey=` 链接）
- [references/global-reference.md](./references/global-reference.md) — 全局标志、认证、输出格式
- [references/field-rules.md](./references/field-rules.md) — AI表格字段类型规则
- [references/error-codes.md](./references/error-codes.md) — 错误码 + 调试流程
- [references/recovery-guide.md](./references/recovery-guide.md) — recovery 闭环、`RECOVERY_EVENT_ID`、`execute/finalize` 规范
- [scripts/](./scripts/) — 各产品批量/复合操作脚本（AI表格批量导入导出、AI应用创建轮询、日历、机器人消息、通讯录、考勤、日志、待办、文档创建并写入、钉盘目录树等）
- [references/products/aitable/](./references/products/aitable/) — AI表格细分章节（单元格值/字段属性/公式/筛选排序/导入导出/仪表盘/记录增删改查/错误恢复）
- [references/products/aitable-record-ops.md](./references/products/aitable-record-ops.md) — AI表格记录操作专项说明
- [references/capability-limits.md](./references/capability-limits.md) — 已知能力限制（doc/aitable/chat/minutes，遇到时直接告知用户不支持）
