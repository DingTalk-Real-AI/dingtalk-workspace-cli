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
| 文档正文读取、创建、更新、块编辑、媒体和导出 | [`dingtalk-doc`](../../dingtalk-doc/SKILL.md) |
| 文件搜索、上传下载、复制移动、重命名、权限 | [`dingtalk-drive`](../../dingtalk-drive/SKILL.md) |
| 邮件查询、搜索、读取和发送 | [`dingtalk-mail`](../../dingtalk-mail/SKILL.md) |
| 听记列表、摘要、转写、关键字和标题 | [`dingtalk-minutes`](../../dingtalk-minutes/SKILL.md) |
| 待办创建、查询、更新、完成和删除 | [`dingtalk-todo`](../../dingtalk-todo/SKILL.md) |
| 知识库/钉盘空间、空间节点和成员管理 | [`dingtalk-wiki`](../../dingtalk-wiki/SKILL.md) |
| 姓名模糊找人、负责人、上下级、工号、手机号语义线索、企业知识和行为记录搜索 | [`dingtalk-aisearch`](../../dingtalk-aisearch/SKILL.md) |
| 完整手机号精确反查，或已有 userId 后查人员详情、部门和角色 | [`dingtalk-contact`](../../dingtalk-contact/SKILL.md) |
| Markdown / `.md` 内容读取、创建、覆盖、局部修改或版本差异比较 | [`dingtalk-misc`](../../dingtalk-misc/SKILL.md) → [`markdown.md`](../../dingtalk-misc/references/markdown.md) |
| 审批、考勤、会议、电子表格、日志、DING、直播、宜搭、开放平台等长尾产品 | [`dingtalk-misc`](../../dingtalk-misc/SKILL.md) |

选择 `dingtalk-misc` 后，先读取其 `SKILL.md` 产品索引，再只读取命中产品的单个
reference，不要加载全部长尾产品文档。

## 高频边界

- `aisearch person`：按姓名、职责、上下级、工号或手机号线索语义找人；`contact`：
  完整手机号精确反查，或拿到 userId 后查详情、部门和角色；`mail`：邮件内容与收发。
- `drive`：对任何文件都成立的存储操作；`doc`：文档正文和块内容；`wiki`：空间与
  节点组织。
- `.md` 文件按普通文件走 `drive`：先 `dws drive download --node <ID> --output <PATH> --format json`
  下载到本地读取或修改，再用 `dws drive upload` 回传；复制、移动、删除同样走
  `drive`。
- `aitable`：字段/记录式数据表；`sheet`：单元格、公式、多工作表。`sheet` 位于
  [`dingtalk-misc`](../../dingtalk-misc/references/sheet.md)。
- `calendar`：日历事件、参会人和会议室；`conference`：预约/发起视频会议及会控；
  `minutes`：会后听记内容。
- `report`：钉钉日志系统中的日报/周报；`doc`：普通文档创作；`todo`：个人任务。
- `chat`：普通会话和消息；`ding`：强提醒；`a2a`：Agent 协作协议。后两者位于
  `dingtalk-misc`。
- 请假、加班、外出、出差、补卡等考勤业务审批优先走 `attendance`；其他通用审批
  走 `oa`。两者均位于 `dingtalk-misc`。

边界仍无法判断时，只读取 [intent-guide.md](intent-guide.md) 的对应章节。
