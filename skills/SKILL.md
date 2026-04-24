---
name: dws
description: 管理钉钉产品能力(AI表格/日历/通讯录/群聊与机器人/待办/审批/考勤/日志/DING消息/开放平台文档/钉钉文档/钉钉云盘/AI听记等)。当用户需要操作表格数据、管理日程会议、查询通讯录、管理群聊、机器人发消息、创建待办、提交审批、查看考勤、提交日报周报（钉钉日志模版）、读写钉钉文档、上传下载云盘文件、查询听记纪要时使用。
cli_version: ">=1.0.15"
---

# 钉钉全产品 Skill

通过 `dws` 命令管理钉钉产品能力。

## 严格禁止 (NEVER DO)
- 不要使用 dws 命令以外的方式操作（禁止 curl、HTTP API、浏览器）
- 不要编造 UUID、ID 等标识符，必须从命令返回中提取
- 不要猜测字段名/参数值，操作前必须先查询确认

## 严格要求 (MUST DO)
- 所有命令必须加 `--format json` 以获取可解析输出
- 危险操作必须先向用户确认，用户同意后才加 `--yes` 执行
- 单次批量操作不超过 30 条记录
- 所有命令必须**严格遵循**对应产品参考文档里面规定的参数格式（如：如果有参数值，则参数和参数值之间至少用一个空格隔开）


## 产品总览

| 产品                | 用途                                                   | 参考文件                                                           |
|-------------------|------------------------------------------------------|----------------------------------------------------------------|
| `aitable`         | AI表格：Base/数据表/字段/记录/视图/附件/图表/仪表盘/导入导出/模板搜索            | [aitable.md](./references/products/aitable.md)                 |
| `attendance`      | 考勤：打卡记录/排班查询/考勤规则/汇总统计                             | [attendance.md](./references/products/attendance.md)           |
| `calendar`        | 日历：日程/参与者/会议室/闲忙查询/时间建议                             | [calendar.md](./references/products/calendar.md)               |
| `chat`            | 群聊与机器人：搜索群/建群/群成员管理/改群名/机器人群发/单聊/撤回/Webhook/机器人搜索     | [chat.md](./references/products/chat.md)                       |
| `contact`         | 通讯录：用户查询(当前用户/搜索/详情/手机号)/部门查询(搜索/成员列表)              | [contact.md](./references/products/contact.md)                 |
| `devdoc`          | 开放平台文档：搜索开发文档                                        | [devdoc.md](./references/products/devdoc.md)                   |
| `ding`            | DING消息：发送/撤回（应用内/短信/电话）                              | [ding.md](./references/products/ding.md)                       |
| `doc`             | 钉钉文档：搜索/浏览/读写/块级编辑/评论/文件创建/复制/移动/重命名                | [doc.md](./references/products/doc.md)                         |
| `drive`           | 钉钉云盘：文件列表/元数据/文件夹/上传(两步)/下载                        | [drive.md](./references/products/drive.md)                     |
| `minutes`         | AI听记：听记列表/摘要/关键词/转写/待办/思维导图/发言人/热词/上传                | [minutes.md](./references/products/minutes.md)                 |
| `oa`              | OA审批：待办/我发起的/表单模板/详情/审批流水/同意/拒绝/撤销                   | [oa.md](./references/products/oa.md)                           |
| `report`          | 日志：按模版创建/收件箱/已发送/模版查看/详情/已读统计                         | [report.md](./references/products/report.md)                   |
| `todo`            | 待办：创建(含优先级/截止时间/循环)/查询/修改/标记完成/删除                   | [todo.md](./references/products/todo.md)                       |

## 意图判断决策树

用户提到"表格/多维表/AI表格/记录/数据/视图/图表/仪表盘" → `aitable`
用户提到"考勤/打卡/排班" → `attendance`
用户提到"日程/日历/会议室/约会/时间建议" → `calendar`
用户提到"群聊/建群/群成员/群管理/机器人发消息/Webhook/机器人群发/机器人单聊/通知" → `chat`
用户提到"通讯录/同事/部门/组织架构" → `contact`
用户提到"开发/API/调用错误 文档" → `devdoc`
用户提到"DING/紧急消息/电话提醒" → `ding`
用户提到"钉钉文档/云文档/知识库/读写文档/块级编辑/文档评论/文档复制移动" → `doc`
用户提到"云盘/文件存储/文件上传下载/文件夹" → `drive`
用户提到"听记/AI听记/会议纪要/转写/摘要/思维导图/发言人/热词" → `minutes`
用户提到"审批/请假/报销/出差/加班/同意/拒绝/撤销审批" → `oa`
用户提到"日志/日报/周报/日志统计/写日报/提交周报/发日志/填日志" → `report`
用户提到"待办/TODO/任务提醒/循环待办" → `todo`

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
| `doc` | `block delete` | 删除文档块（不可恢复） |
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

1. 意图分类：首先，判断用户指令的核心 动词/动作 属于哪一类。这比关注名词更重要。
2. 歧义处理与信息追问：如果用户指令模糊或包含多个产品的关键字，严禁猜测。必须主动向用户追问以澄清意图。这是你作为智能助手而非命令执行器的核心价值。
3. 精准产品映射：在完成前两步，意图已经清晰后，参考产品总览和意图判断决策树 来选择产品。
4. 充分阅读产品参考文件，通过编写代码或直接调用指令实现用户意图。

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
1. 遇到错误，加 `--verbose` 重试一次
2. 若 stderr 出现 `RECOVERY_EVENT_ID=<event_id>`，优先按 [recovery-guide.md](./references/recovery-guide.md) 执行 recovery 闭环
3. 仍然失败，报告完整错误信息给用户，禁止自行尝试替代方案
4. 认证失败时，参考 [global-reference.md](./references/global-reference.md) 中的认证章节处理
5. 各产品高频错误及排查流程见 [error-codes.md](./references/error-codes.md)


## 详细参考 (按需读取)

- [references/products/](./references/products/) — 各产品命令详细参考（flag 细节以 `--help` / `dws schema` 为准）
- [references/intent-guide.md](./references/intent-guide.md) — 意图路由指南（易混淆场景对照）
- [references/global-reference.md](./references/global-reference.md) — 全局标志、认证、输出格式
- [references/field-rules.md](./references/field-rules.md) — AI表格字段类型规则
- [references/error-codes.md](./references/error-codes.md) — 错误码 + 调试流程
- [references/recovery-guide.md](./references/recovery-guide.md) — recovery 闭环、`RECOVERY_EVENT_ID`、`execute/finalize` 规范
- [scripts/](./scripts/) — 各产品批量操作脚本（AI表格/日历/机器人消息/通讯录/考勤/日志/待办等）
