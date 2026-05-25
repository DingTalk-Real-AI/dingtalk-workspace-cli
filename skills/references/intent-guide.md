# 意图路由指南

当用户请求难以判断归属哪个产品时，参考本指南。

## 易混淆场景快速对照表

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|-----------|----------|--------|--------|------|
| "搜一下 OAuth2 接入文档" | 搜索开发文档 | `devdoc` | — | 搜索开放平台技术文档，不是钉钉内部内容 |
| "帮我建一个项目跟踪表" | 创建数据表格 | `aitable` | `todo` | 涉及结构化数据/行列操作，不是个人待办 |
| "帮我记一下明天要做的事" | 创建个人待办 | `todo` | `aitable` | 个人待办提醒，非数据表 |
| "帮我建一个明天下午的日程" | 日历日程 | `calendar` | — | 日历日程管理（可含参与者/会议室）|
| "帮我看看收到的日报" | 日志收件箱 | `report` | `todo` | 钉钉日志系统（日报/周报），不是待办 |
| "帮我创建一个待办提醒" | 个人待办 | `todo` | `report` | 个人任务提醒，不是日志汇报 |
| "帮我提交请假审批" | 发起审批 | `oa` | — | 审批流程，不是待办或日志 |
| "帮我建一个项目群" | 创建群聊 | `chat group create` | — | 群聊管理，不是日历日程 |
| "把张三拉进群" | 添加群成员 | `chat group members add` | — | 先查 userId，再添加 |
| "让机器人在群里发个通知" | 机器人群发 | `chat message send-by-bot` | `chat message send-by-webhook` | 企业内部机器人发消息，需 robotCode |
| "通过 Webhook 发告警到群里" | Webhook 告警 | `chat message send-by-webhook` | `chat message send-by-bot` | 自定义机器人 Webhook，需 token |
| "给张三发一条机器人单聊消息" | 机器人单聊 | `chat message send-by-bot --users` | — | 机器人批量单聊，先查 userId |

---

## 典型场景详解

### 1. aitable vs todo — 表格数据 vs 待办任务

**用 `aitable` 的场景**：
- "创建一个表格记录团队成员信息" — 结构化数据，有行列
- "在表格里加一列'状态'字段" — 字段/列操作
- "查一下表格里所有优先级为高的记录" — 数据筛选和查询
- "用项目管理模板建一个表" — 模板创建
- 用户提到"多维表"、"Base"、"数据表"、"记录"

**用 `todo` 的场景**：
- "帮我记一下这周要做的事" — 个人任务管理
- "创建一个待办提醒" — 任务提醒

**判断关键**：有没有行列/字段/记录概念？有→ `aitable`；个人任务清单 → `todo`

---

### 2. devdoc — 开发文档搜索

**用 `devdoc` 的场景**：
- "API 调用报错 403 怎么解决" — 开发调试问题
- "搜一下 OAuth2 接入文档" — 开放平台技术文档
- "CLI 命令出错了怎么办" — CLI 使用错误
- 用户提到"开发"、"API"、"调用错误"

---

### 3. report vs todo — 日志 vs 待办

**用 `report` 的场景**：
- "帮我看看收到的日报" — 日志收件箱
- "帮我写/提交今天的日报（钉钉日志模版）" — 先 `report template list` / `template detail`，再 `report create`
- "有什么日志模版" — 查看模版
- "看看这个日志的已读统计" — 阅读状态
- "我发过的日志有哪些" — 已发送列表 (`report sent`)
- 用户提到"日报"、"周报"、"日志"

**用 `todo` 的场景**：
- "记一下这周要做的事" — 个人任务管理

**判断关键**：钉钉日志系统(日报/周报模版，含按模版创建汇报)→ `report`；任务清单→ `todo`

---

### 4. chat 内部 — 两种消息发送方式

**用 `chat message send-by-bot` 的场景**：
- "让机器人在群里发一条通知" — **机器人身份**发群消息
- "给张三发一条机器人单聊消息" — 机器人批量单聊

**用 `chat message send-by-webhook` 的场景**：
- "通过 Webhook 发告警到群里" — 自定义机器人 Webhook
- 用户有 Webhook Token

**判断关键**：企业内部机器人→ `send-by-bot`（需 robotCode）；有 Webhook Token→ `send-by-webhook`

---

## 跨产品工作流路由

以下场景需要多个产品配合完成，注意上下文传递顺序。

### 创建日程并邀请同事（contact → calendar）

用户说"约张三明天下午开会"：

```bash
# 1. 搜索同事 userId
dws contact user search --query "张三" --format json

# 2. 创建日程
dws calendar event create --title "会议" \
  --start "2026-03-15T14:00:00+08:00" --end "2026-03-15T15:00:00+08:00" --format json

# 3. 添加参与者
dws calendar participant add --event <EVENT_ID> --users <USER_ID> --format json
```

### 创建待办并指派（contact → todo）

用户说"给张三建个待办"：

```bash
# 1. 搜索同事 userId
dws contact user search --query "张三" --format json

# 2. 创建待办
dws todo task create --title "任务内容" --executors <USER_ID> --format json
```

---

## 玉澜域专项路由

> 内容生产域（aitable / doc / drive / wiki / aiapp / devdoc）的易混淆场景。
> 落地范围：dws-opensource 玉澜分支（feat/align-yuyuan）的 helpers。

### alidocs URL 路由（先 probe，再走对应产品）

| 用户说 / 给出的信息 | 真实意图 | 应该用 | 不要用 |
|---------------------|----------|--------|--------|
| 粘贴 `alidocs.dingtalk.com/i/nodes/<UUID>` 原始 URL | 先识别节点类型 | `dws doc info --node <URL>` → 按 `extension` 路由（adoc/axls/able） | 直接 `sheet` / `aitable` |
| "读一下这个 xlsx 附件" / xlsx 节点链接 | 下载本地表格文件 | `dws doc download --node <URL>` | `sheet range read` |
| "把这个在线表格导出为 xlsx" | axls → xlsx 格式转换 | `dws sheet export`（待吴淼 W-01 落地）| `dws doc download` |
| `/i/p/` 开头的分享短链 | 短链兜底 | `read_url` 工具 | 任何 `dws doc *` |
| "删了这个 alidocs 文档" | 节点删除 | `dws doc delete --node <URL> --yes` | 在客户端操作（已支持） |
| "把这份文档导出 docx" | 异步导出 + 下载 | `dws doc export --node <URL> --output ./x.docx` | 自己拼 docs.dingtalk OSS URL |
| "把这个文档分享给张三可编辑" | 节点级授权 | `dws doc permission add --node <URL> --user <UID> --role EDITOR` | `dws wiki member add`（容器级，不是节点级） |
| "下载文档里那张图" | 拿附件 OSS URL | `dws doc media download --node <URL> --resource-id <ID>` | `doc download` |
| "把这张截图插到文档" | 上传 + 插块 | `dws doc media insert --node <URL> --file ./x.png` | 自己 PUT |

### aitable 导入路由（用户原话决定走哪条链路）

| 用户说... | 真实意图 | 应该用 | 不要用 |
|-----------|----------|--------|--------|
| "把 Excel 导入 AI 表格" / "把这个 xlsx 变成多维表" | **文件导入任务**（新建表） | `python scripts/aitable_import_via_task.py <baseId> <file>` 或 `dws aitable import upload --file ./x.xlsx` + `dws aitable import data --import-id <ID>` | `import_records.py`（除非用户指明追加到已有表） |
| "把这批数据追加到『XXX』表" | 已有 tableId 的批量写入 | `python scripts/import_records.py <baseId> <tableId> <file>` | `aitable_import_via_task.py` |
| "Excel 列名和表字段对不上但要追加" | 文件导入 + 字段映射 | `dws aitable import data --import-id <ID> --table-id <TBL> --field-mapping '{"目标":"源"}'` | 手动改 Excel 表头 |

### aitable 列表 / 翻页路由

| 用户说... | 真实意图 | 应该用 | 不要用 |
|-----------|----------|--------|--------|
| "把这张表的全部记录列给我" / "列完" / "所有记录" | 全量翻页（数据驱动决策时不能漏数据） | `dws aitable record query --base-id B --table-id T --all` | 单次 `record query` 后凭直觉判断（90% 漏数据） |
| "导出某张表/某个视图为 xlsx" | 同步导出 + 自动落盘 | `dws aitable export data --base-id B --scope view --table-id T --view-id V --output ./v.xlsx` | 自己拿 taskId 后 GET downloadUrl |

