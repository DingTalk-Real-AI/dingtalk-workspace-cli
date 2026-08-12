# 产品路由

仅在用户泛称 DWS/钉钉操作，或者无法从意图直接选择单一产品 skill 时读取。调度器
已经命中清晰产品 skill 时不要读取本文件。

## 一级产品选择

| 用户目标 | 读取目标 |
|---|---|
| AI 表格、多维表、字段、记录、视图、仪表盘 | [`dingtalk-aitable`](../../dingtalk-aitable/SKILL.md) |
| 日程、参会人、会议室、闲忙 | [`dingtalk-calendar`](../../dingtalk-calendar/SKILL.md) |
| 群聊、消息、机器人、Webhook、群成员 | [`dingtalk-chat`](../../dingtalk-chat/SKILL.md) |
| 已有 userId 的用户详情、部门、角色、组织关系 | [`dingtalk-contact`](../../dingtalk-contact/SKILL.md) |
| 在线文档正文读取、创建、更新、块编辑、正文媒体和导出；未指定本地路径的“生成/写一篇文档” | [`dingtalk-doc`](../../dingtalk-doc/SKILL.md) |
| 文件搜索、上传下载、节点复制/移动/重命名、权限；包括在线 adoc 节点和普通“文档空间/我的文档”中的文件夹、元信息操作 | [`dingtalk-drive`](../../dingtalk-drive/SKILL.md) |
| 邮件查询、搜索、读取和发送 | [`dingtalk-mail`](../../dingtalk-mail/SKILL.md) |
| 听记列表、摘要、转写、关键字和标题 | [`dingtalk-minutes`](../../dingtalk-minutes/SKILL.md) |
| 待办创建、查询、更新、完成和删除 | [`dingtalk-todo`](../../dingtalk-todo/SKILL.md) |
| 用户明确说知识库/wiki/命名团队空间、知识库节点层级或空间成员管理 | [`dingtalk-wiki`](../../dingtalk-wiki/SKILL.md) |
| 姓名模糊找人、负责人、上下级、工号、手机号语义线索、企业知识和行为记录搜索 | [`dingtalk-aisearch`](../../dingtalk-aisearch/SKILL.md) |
| 完整手机号精确反查，或已有 userId 后查人员详情、部门和角色 | [`dingtalk-contact`](../../dingtalk-contact/SKILL.md) |
| 明确要求本地/原生 Markdown 文件或给出 `.md` 路径后的读取、创建、覆盖、局部修改或版本差异比较 | [`dingtalk-misc`](../../dingtalk-misc/SKILL.md) → [`markdown.md`](../../dingtalk-misc/references/markdown.md) |
| 组织大脑、人才池、员工档案专项、职业历程、绩效、结构化人才搜索 | [`dingtalk-misc`](../../dingtalk-misc/SKILL.md) → [`hrbrain.md`](../../dingtalk-misc/references/hrbrain.md) |
| PAT 行为授权、scope 授权、授权浏览器策略 | [`dingtalk-misc`](../../dingtalk-misc/SKILL.md) → [`pat.md`](../../dingtalk-misc/references/pat.md) |
| 切换组织、跨组织、多组织、profile 管理 | [`dingtalk-misc`](../../dingtalk-misc/SKILL.md) → [`profile.md`](../../dingtalk-misc/references/profile.md) |
| 审批、考勤、会议、电子表格、日志、DING、直播、开放平台应用、技能市场安装、个人 IM 事件等长尾产品 | [`dingtalk-misc`](../../dingtalk-misc/SKILL.md) |
| 宜搭 / AI 应用创建脚本 / 财务辅助脚本（无稳定产品面） | [`dingtalk-misc`](../../dingtalk-misc/SKILL.md) → [`unsupported-scripts.md`](../../dingtalk-misc/references/unsupported-scripts.md)；默认说明未产品化，勿当正式 CLI |

选择 `dingtalk-misc` 后，先读取其 `SKILL.md` 产品索引，再只读取命中产品的单个
reference，不要加载全部长尾产品文档。

## 高频边界

- `aisearch person`：按姓名、职责、上下级、工号或手机号线索语义找人；`contact`：
  完整手机号精确反查，或拿到 userId 后查详情、部门和角色；`mail`：邮件内容与收发。
- `drive`：对任何文件（包括在线 adoc 节点）都成立的复制、移动、重命名等存储操作；`doc`：文档正文和块内容；`wiki`：空间与
  节点组织。
- 未指定本地路径的“生成文档/写一篇文档”在钉钉/DWS 语境中创建在线 adoc；明确要求本地/原生 `.md` 或给出 `.md` 路径时才走 `markdown`（`dingtalk-misc`）。复制、移动、重命名、删除等实体操作走 `drive`。
- 本地文件“在线编辑/协作编辑/转在线”：Word/Markdown/Text 走 `doc +import`，Excel 走电子表格导入；只有要求保留原文件对象时才走 `drive upload`。
- “文档空间/我的文档”本身不触发 wiki：正文操作走 `doc`，文件夹、移动、元信息等存储操作走 `drive`；只有明确知识库/wiki/命名团队空间语义才走 `wiki`。
- `aitable`：字段/记录式数据表；`sheet`：单元格、公式、多工作表。`sheet` 位于
  [`dingtalk-misc`](../../dingtalk-misc/references/sheet.md)。
- `calendar`：日历事件、参会人和会议室；视频会议（conference）当前 CLI 不支持，请在钉钉客户端操作；
  `minutes`：会后听记内容。
- `report`：钉钉日志系统中的日报/周报；`doc`：普通文档创作；`todo`：个人任务。
- `chat`：普通会话和消息；`event`：个人 IM 事件长连接监听；`ding`：强提醒。
  `event` 与 `ding` 位于 `dingtalk-misc`。
- `hrbrain` / `markdown` / `pat` / `profile` 均位于 `dingtalk-misc`。
- 请假、加班、外出、出差、补卡等考勤业务审批优先走 `attendance`；其他通用审批
  走 `oa`。两者均位于 `dingtalk-misc`。

边界仍无法判断时，只读取 [intent-guide.md](intent-guide.md) 的对应章节。
