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

## 玉澜域核心混淆场景

> 内容生产域 6 个产品之间最容易选错的 4 类场景。命中率直接决定 LLM
> 第一步是否走对路径。

### aitable vs doc vs sheet — 数据表格 vs 文档内容 vs 电子表格

**用 `aitable` 的场景**：
- "创建一个表格记录团队成员信息" — 结构化数据，有行列
- "在表格里加一列'状态'字段" — 字段/列操作
- "查一下表格里所有优先级为高的记录" — 数据筛选和查询
- "用项目管理模板建一个表" — 模板创建
- 用户提到"多维表"、"Base"、"数据表"、"记录"

**用 `doc` 的场景**：
- "帮我写个会议纪要" — 富文本内容创作
- "看一下这个文档链接的内容" — 阅读文档
- "在知识库创建一个文件夹" — 文档空间管理
- 用户提到"文档"、"知识库"、"写文档"

**用 `sheet` 的场景**：
- "创建一个电子表格" — 创建 Excel 式在线表格
- "帮我读一下这个表格 A1 到 D10 的数据" — 按单元格区域读取
- "在 B2 写入一个 SUM 公式" — 写入公式/值到单元格
- "帮我看看这个表格有哪些工作表" — 工作表管理
- 用户提到"电子表格"、"Excel"、"工作表"、"Sheet"、"单元格"、"公式"

**三者判断关键**：
- 有字段定义/记录增删改查/数据筛选 → `aitable`
- 纯文本/Markdown/富文本编辑 → `doc`
- 单元格区域读写/公式/多工作表 → `sheet`

**易误判场景**：
- "在知识库中新建一个表格" — 指在钉钉文档空间创建表格类型节点 → `doc`（不是 `aitable`）
- "帮我建个表记录项目进度" — 指创建结构化数据表 → `aitable`

### xlsx vs axls — 本地表格文件 vs 在线电子表格

alidocs 链接表面长得一样（`https://alidocs.dingtalk.com/i/nodes/{id}`），
但节点类型完全不同。sheet 产品线只服务 axls（在线电子表格），
xlsx / xls / xlsm / csv 等本地表格文件必须走 `dws doc download`，
严禁错路由。

**用 `sheet` 的场景（axls，钉钉在线电子表格）**：
- `dws doc info --node <URL>` 返回 `contentType=ALIDOC` + `extension=axls`
- 用户在钉钉文档空间直接"新建电子表格"得到的节点
- 所有 sheet 子命令（`list` / `range read` / `range write` / `export` 等）仅服务这类节点

**用 `dws doc download` 的场景（xlsx / xls / xlsm / csv 本地表格文件）**：
- `dws doc info --node <URL>` 返回 `contentType=DOCUMENT` + `extension=xlsx` / `xls` / `xlsm` / `csv`
- 用户把本地 Excel 文件上传到文档空间得到的节点，本质是"文件 + 预览"，非在线表格
- sheet 命令直接调用会报错，必须先 `dws doc download --node <URL>` 下载到本地再解析处理

**判断关键**：
- 未知 alidocs URL → 必须先 `dws doc info --node <URL> --format json` 探测 `contentType` 与 `extension`
- `contentType=ALIDOC` + `extension=axls` → `sheet`
- `contentType=DOCUMENT` + `extension=xlsx` / `xls` / `xlsm` / `csv` → `dws doc download`
- 用户说"把在线表格导出为 xlsx 文件" → `dws sheet export`（axls → xlsx 的格式转换，不是读取 xlsx）

**易误判场景**：
- 用户粘贴一个 alidocs 链接说"读一下这个表格" — 不能直接调 `sheet range read`，必须先 probe 再按 `extension` 路由
- 用户说"读一下这个 xlsx 文件里的数据" — 走 `dws doc download` 下载后本地解析，不要走 `sheet`
- 用户说"把这个在线表格导出为 xlsx" — 走 `dws sheet export`，不要走 `dws doc download`（后者只能下载已有的 xlsx 节点，无法从 axls 生成）

详见 [url-patterns.md](./url-patterns.md) 和 [sheet.md 适用范围](./products/sheet.md)。

### devdoc vs doc search — 两种搜索

**用 `devdoc` 的场景**：
- "API 调用报错 403 怎么解决" — 开发调试问题
- "搜一下 OAuth2 接入文档" — 开放平台技术文档
- "CLI 命令出错了怎么办" — CLI 使用错误
- 用户提到"开发"、"API"、"调用错误"

**用 `doc search` 的场景**：
- "在我的文档里搜一下'项目方案'" — 搜索文档标题和内容
- 用户明确说"我的文档"、"知识库里搜"

**判断关键**：搜开发文档→ `devdoc`；搜用户自己的文档→ `doc search`

### drive vs doc — 文件存储 vs 文档内容

**用 `drive` 的场景**：
- "把这个 PDF 传到钉盘" — 上传文件
- "下载那个 Excel 附件" — 下载文件
- "看一下钉盘根目录有什么文件" — 浏览文件列表
- 用户提到"钉盘"、"网盘"、"上传"、"下载"

**用 `doc` 的场景**：
- "读一下这个文档的内容" — 读取文档 Markdown
- "帮我写入一段话到文档里" — 编辑文档内容
- "在知识库里搜索会议纪要" — 搜索文档
- 用户提到"文档内容"、"知识库"

**判断关键**：文件存储/传输→ `drive`；文档内容读写→ `doc`

---

## 玉澜域专项路由

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

