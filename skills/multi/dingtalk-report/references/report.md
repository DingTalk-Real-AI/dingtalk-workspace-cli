# 日志 (report) 命令参考

> **注意**：钉钉日志 = 「OA 周报应用」（按模版填报 / 收件箱 / 已发列表 / 统计），不是通用日志或记录系统。`dws report` 与其别名 `dws log` 自动由 envelope 注册——`dws report --help` 输出会列出全部别名，文档不再单独声明。

## 载体辨义（首屏必读）

`dws report` 管理「钉钉日志」OA 应用（按模版填报、收到列表、已发列表、统计），**不是**钉钉在线文档。Agent 在拿到用户 query 时按下表选择命令族：

| 用户原话信号 | 命令族 | 不走 |
|-------------|--------|------|
| 钉钉日志 / OA 周报 / 周报模板 / 日报模板 / 我的钉钉日志 / 写日志 / 提交周报 / 填模版 | `dws report` | 不走 dws doc |
| 在线文档 / 写一篇文档 / 整理成文档 / 周报文档 / 月报文档 / 用文档保存 | `dws doc` | 不走 dws report |
| （无强信号）写日报 / 写周报 / 写月报 | **默认** `dws doc` | 仅当用户后续明确指定钉钉日志时才切到 dws report |

- 默认走 `dws doc`：长文本编辑、富文本、可分享链接的场景。
- 切到 `dws report`：query 中出现「钉钉日志 / OA 周报模板 / 钉钉日报应用 / 我的钉钉日志」等强信号时（不依赖词典扩张，依赖语义判断 + 必要时反问澄清）。
- 信号歧义时应先反问「您指的是钉钉日志（OA 周报应用）还是钉钉在线文档？」而不是默认选一个。

## 查日志快速调度（Agent 首选）

当用户说「查日志 / 看日志 / 找日报 / 查看周报 / 我发过的日志 / 收到的日志」且语义指向钉钉日志 OA 应用时，Agent 不要先解释概念，直接按下面指令调度：

- `dws report inbox list` = 列出**我收到**的日报（别人发给我的）。
- `dws report outbox list` = 列出**我发出**的日报（我创建或提交的）。
- `dws report entry get --report-id <reportId>` = 读取单份日报正文 + 钉钉跳转链接。
- `dws report entry stats --report-id <reportId>` = 读取单份日报的已读统计。
- `dws report entry submit --template-id ... --contents-file ...` = 按模版提交一份新日报。
- `dws report template list` = 列出可用日报模版。
- `dws report template get --name "<模版名>"` = 读取单个模版的字段定义（contents 拼装来源）。

| 用户意图 | 第一条有效指令 | 后续动作 |
|----------|----------------|----------|
| 查我发过的日志 / 我创建的日志 | `dws report outbox list --cursor 0 --size 20 --format json` | 从返回里取 `reportId`，再执行 `dws report entry get --report-id <reportId> --format json` |
| 查我收到的日志 / 别人发给我的日志 | `dws report inbox list --start "<YYYY-MM-DDT00:00:00+08:00>" --end "<YYYY-MM-DDT23:59:59+08:00>" --cursor 0 --size 20 --format json` | 必须先按用户时间词补齐完整 ISO 起止时间；取 `reportId` 后调用 `entry get` |
| 查看某条日志正文 / 日志详情 | `dws report entry get --report-id <reportId> --format json` | 如果用户没给 `reportId`，先用 `outbox list` 或 `inbox list` 找候选 |
| 查某条日志统计 / 已读统计 | `dws report entry stats --report-id <reportId> --format json` | 如果用户没给 `reportId`，先用 `outbox list` 或 `inbox list` 找候选 |
| 查日志模版 / 有哪些周报模板 | `dws report template list --format json` | 需要字段定义时继续 `dws report template get --name "<模版名>" --format json` |
| 提交 / 填写 / 创建钉钉日志 | `dws report template list --format json` | 再 `template get` 取字段定义，最后 `entry submit`；禁止直接编 contents |

效率约束：

- 不要先调用 `dws report --help` / `dws report inbox list --help`。本页已经给出可执行命令。
- 查询收到的日志统一使用 `dws report inbox list`；查询发出的日志统一使用 `dws report outbox list`。
- 不要为了格式化结果创建脚本。直接从 JSON 里抽取关键字段，返回用户可读列表。
- 对"最近一周"这类常用查询，直接把当前日期换算成 7 天窗口后执行一次 `inbox list`；如返回分页标记再继续翻页。

时间窗口默认规则：

- `outbox list` 不传 `--start` / `--end` 时 CLI 默认最近 20 天，适合"我发过的日志 / 最近日志"。
- `inbox list` 必须传 `--start` / `--end`；用户说"最近一周/今天/本周/昨天"时，Agent 必须先转成 `YYYY-MM-DDT00:00:00+08:00` 到 `YYYY-MM-DDT23:59:59+08:00`。
- 查更早日志时每次窗口不要超过 20 天，按窗口滚动查询。

时间参数硬约束：

- 只允许使用 `--start` 和 `--end` 两个 flag；禁止写 `--start-date`、`--end-date`、`--date`、`startDate`、`endDate`。
- 时间值推荐使用完整 ISO-8601 + 时区格式：`YYYY-MM-DDTHH:mm:ss+08:00`。
- Agent 不要只传裸日期；即使 CLI 能兼容 `YYYY-MM-DD`，生成命令时也必须展开成当天起止时间，避免工具层把日期误解析成普通字符串。
- "最近一周"固定展开为：起始日 `T00:00:00+08:00`，结束日 `T23:59:59+08:00`。

错误示例（不要生成）：

```bash
dws report inbox list --start-date 2026-05-04 --end-date 2026-05-11 --format json
dws report inbox list --date 2026-05-11 --format json
```

正确示例：

```bash
dws report inbox list --start "2026-05-04T00:00:00+08:00" --end "2026-05-11T23:59:59+08:00" --cursor 0 --size 20 --format json
```

快速决策：

```bash
# 我发过 / 我创建的日志
dws report outbox list --cursor 0 --size 20 --format json
dws report entry get --report-id <reportId> --format json

# 我收到的日志
dws report inbox list --start "<YYYY-MM-DDT00:00:00+08:00>" --end "<YYYY-MM-DDT23:59:59+08:00>" --cursor 0 --size 20 --format json
dws report entry get --report-id <reportId> --format json

# 已知 reportId
dws report entry get --report-id <reportId> --format json
```

## 日志列表展示规范

面向用户展示时，默认不要把 `reportId` / `report_id` / `report_Id` 作为主列。日志 ID 只给 Agent 后续调用 `detail` / `stats` 使用，用户一般不需要看。

优先展示这些字段：

| 展示列 | 取值优先级 |
|--------|------------|
| 日期 | `createTime` / `modifiedTime` / `gmtCreate`，转成本地可读时间 |
| 标题 | `report_name` / `reportName` / `title` / `summary` / `report_template_name`，没有标题时用「<发送人>的日志」 |
| 发送人 | `creatorName` / `senderName` / `userName` |
| 已读状态 | `readStatus` / `isRead` / `hasRead` / `read`；字段缺失时不展示，不要编造 |
| 模板 | `report_template_name` / `templateName`，用户关心分类时展示 |
| 查看详情 | 从 `_internalDetailCommands[].command` 取内部详情命令；面向用户展示为“查看详情”，不要展示命令本身 |
| 钉钉链接 | 详情接口返回 `dingtalkOpenMarkdownLink` / `dingtalkOpenUrl` / `result.url` 后，展示成 markdown 链接 |

CLI 列表命令只返回 JSON-first 数据，不把 Markdown 表作为裸文本直接输出。`report inbox list` / `report outbox list` 会对当前页 `result[]` 中所有带 `reportId` 的日志逐条调用 `dws report entry get --report-id <reportId> --format json` 补齐展示字段；返回 JSON 会携带 `agentDisplayRequired=true`、`agentDisplayRequiredColumns`、`agentDisplayLinkColumn=钉钉链接`、`agentDisplayMarkdownRequired=true`、`agentDisplayMarkdown`、`agentDisplayMarkdownHeader` 等字段，这些是面向 Agent 的强制展示合同，不是可选建议。Agent 必须在 final reply 直接原样输出 `agentDisplayMarkdown`，让客户端按 Markdown 渲染出带可点击 `钉钉链接` 列的表格；不要自行重组列，不要改表头。

展示列按方向区分：`inbox list` 是收件箱，只展示 `日期 | 标题 | 发送人 | 状态 | 钉钉链接`，**禁止返回或展示日志正文、完整内容或日志内容摘要**；`outbox list` 是发件箱，可展示 `日期 | 标题 | 发送人 | 状态 | 日志内容 | 钉钉链接`，其中 `日志内容` 来自详情补齐，方便用户快速确认自己发出的内容。凡用户说"列出 / 找到 / 查询 / 搜索 / 看看日志"，默认都要在 final reply 渲染 Markdown 表；只有用户明确表示"不关心列表内容 / 只要原始 JSON / 只要数量 / 只要 ID"时，才可以不渲染表。推荐用户可读输出：

| 日期 | 标题 | 发送人 | 状态 | 钉钉链接 |
|------|------|--------|------|----------|
| 2026-05-09 23:08 | 张成强的周报 | 张成强 | 未读 | [在钉钉中查看日志](...) |

操作列规则：

- `inbox list` 列表阶段：每条 `result[]` 只带 `日期` / `标题` / `发送人` / `状态` / `钉钉链接` 五个展示字段；使用 `result[].钉钉链接` 作为可点击操作列。禁止额外返回或展示 `日志内容` / `日志内容摘要`。
- `outbox list` 列表阶段：每条 `result[]` 可带 `日志内容`，表头固定为 `日期 | 标题 | 发送人 | 状态 | 日志内容 | 钉钉链接`。如果用户点名某一条或说"打开第 N 条/看正文"，Agent 用 `_internalDetailCommands[N].command` 调 `dws report entry get --report-id ... --format json`。
- 详情阶段：`entry get` 返回里如果有 `dingtalkOpenMarkdownLink`，优先把操作列替换为该 markdown 链接；否则用 `dingtalkOpenUrl` 或 `result.url` 包成 `[在钉钉中查看日志](url)`。
- `钉钉链接` 是强制列：禁止省略、改名、合并到标题里，也禁止改成不含链接列的摘要表。
- 不要在用户表格里展示 `_internalDetailCommands`、raw `reportId`、raw `dingtalk://...`。链接必须是 markdown 可点击文本。
- `dws report inbox list` 默认会对当前页所有可查看日志自动补 `entry get`，但只返回日期、标题、发送人、状态与可点击钉钉链接；需要正文时必须显式调用 `dws report entry get --report-id ... --format json`。
- `dws report outbox list` 默认会对当前页所有可查看日志自动补 `entry get`，并可把 `日志内容` 纳入发件箱表格展示。

只有在用户明确要求"给我日志 ID / 方便我后续查询"时，才额外展示 `reportId`。否则 final reply 应保留可读信息，并说明"需要看正文或打开钉钉，我可以继续打开某一条"。

## 提交链路硬约束（必读）

当用户意图涉及"填模板 / 提交日志 / 提交日报 / 提交周报"等需要 submit 的场景时，**必须**按以下步骤执行，**禁止跳步**：

1. `dws report template list --format json` — 取 `report_template_id` 与可见模版名
2. `dws report template get --name "<模版名>" --format json` — 取 `result.report_template_fields[]`，每项含 `field_name` / `field_sort` / `field_type`
3. `dws report entry submit --template-id <id> --contents-file <tmp.json> --format json` — contents 数组按上面「字段映射」严格对齐第 2 步：`field_name → key`，`field_sort → sort`，`field_type → type`，再填 `content` 与 `contentType`；CLI 提交成功后会自动反查详情并追加钉钉打开链接字段，返回中直接取 `reportId` 与 `dingtalkOpenMarkdownLink` / `dingtalkOpenUrl`
4. 仅当第 3 步返回中缺少 `dingtalkOpenUrl` 时，执行 `dws report entry get --report-id <reportId> --format json` 补取 `result.url`（`dingtalk://...` 协议深链接）。final reply 中优先使用 `dingtalkOpenMarkdownLink`，否则用 `[在钉钉中查看日志](dingtalkOpenUrl)`。**禁止把 raw `dingtalk://...` URL 原样写进回复**，必须包成 markdown link 让用户可点击跳转钉钉客户端

跳步风险（已实证）：

- 跳过第 1 步直接编 templateId → 服务端返回 `PARAM_ERROR`，且**不告诉你哪个 ID 错**；
- 跳过第 2 步用 LLM 经验编 `key` 名 → 服务端返回 `PARAM_ERROR`，且**不告诉你哪个字段错**；服务端 PARAM_ERROR 信号弱，事后无法定位，**只能靠前置 schema 同步避免**；
- 未取到 `dingtalkOpenUrl` 且不补查 `entry get` → 用户拿不到跳转链接，无法在钉钉客户端打开刚提交的日志查看 / 修改；
- 用 `--contents` 直传长 JSON → shell 引号转义破坏 JSON → `INPUT_INVALID_JSON`。**长内容务必走 `--contents-file <path>` 或 `--contents -` (stdin)**。
- contents JSON 大小限制为 10MB，**不支持分批次提交**。超过限制需精简内容或拆分为多个独立日志提交。

推荐：Agent 在多轮场景中应在内存里持久化第 1/2 步的结果，避免每轮重新跑。

## 命令总览

### 获取日志模版列表
```
Usage:
  dws report template list [flags]
Example:
  dws report template list
```

### 读取单个日志模版的字段定义
```
Usage:
  dws report template get [flags]
Example:
  dws report template get --name <templateName>
Flags:
      --name string   模版名称 (必填)
```


### 提交日报（按模版）
```
Usage:
  dws report entry submit [flags]
Example:
  # 推荐：长内容走文件，避免 shell 引号问题
  dws report entry submit --template-id <templateId> --contents-file ./report.json --format json

  # stdin 输入
  cat report.json | dws report entry submit --template-id <templateId> --contents - --format json

  # 内联（短内容）
  dws report entry submit --template-id <templateId> \
    --contents '[{"key":"今日完成","sort":"0","content":"完成了需求评审","contentType":"markdown","type":"1"}]' \
    --format json
Flags:
      --template-id string    日志模版 ID (必填)，从 template list 返回中取
      --contents string       日志内容 JSON 数组 (必填，或用 --contents-file)；传 `-` 表示从 stdin 读取
      --contents-file string  从文件读取 contents JSON（推荐用于含中文/换行/Markdown 的长内容）
      --dd-from string        创建来源标识 (默认 dws)
      --to-chat               是否发送到日志接收人单聊 (默认 false，传本 flag 则为 true)
      --to-user-ids string    接收人 userId，逗号分隔 (可选)
```


**`contents` 数组元素**（与 MCP `create_report` 一致）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `key` | string | 控件名，**与 `template get` 返回的 `field_name` 完全一致**（不要自己改写） |
| `sort` | string | 控件排序，对齐 `template get` 的 `field_sort`（建议传字符串 `"0"`/`"1"` 等） |
| `type` | string | 控件类型，对齐 `template get` 的 `field_type`：`1` 文本 / `2` 数字 / `3` 单选 / `5` 日期 / `7` 多选 |
| `content` | string | 填写值；`type=1` 文本类支持 Markdown |
| `contentType` | string | `type=1` 时通常用 `markdown`，其余用 `origin` |

**字段名对齐**：`template get` 返回 `result.report_template_fields[].{field_name, field_sort, field_type}`（snake_case），拼 `--contents` 时 **逐一映射**：`field_name → key`、`field_sort → sort`、`field_type → type`，再填 `content` 与 `contentType`。**不要自己编 key 名**，必须从 get 返回值取，否则服务端会返回不可定位的 `PARAM_ERROR`。

**长内容传参优先级**：`--contents-file <path>` > `--contents -` (stdin) > `--contents '<json>'`。任何含中文换行 / Markdown / 引号的场景都应走 `--contents-file` 避免 shell 引号转义。

### 读取单份日报正文（含字段明细 + 跳转链接）
```
Usage:
  dws report entry get [flags]
Example:
  # 先通过 report inbox list / outbox list / entry submit 取得 reportId，再查正文与跳转链接
  dws report entry get --report-id <reportId>
Flags:
      --report-id string   日志 ID (必填)
```


**关键返回字段**：

| 字段 | 含义 | Agent 用法 |
|------|------|-----------|
| `result.url` | `dingtalk://dingtalkclient/action/openapp?...` 协议深链接 | **必须包成 markdown link**：`[查看日报](result.url)`。点击后会在钉钉客户端打开该日志详情页。**禁止**把 raw `dingtalk://...` URL 直接粘到回复里——多数终端 / 聊天界面无法对裸协议自动可点击 |
| `result.report_content[]` | 各控件正文（`key` / `value` / `richTextValue` 等）| 需要展示正文时读这里；`richTextValue` 是富文本编码，普通展示用 `value` |
| `result.report_name` / `result.creatorName` / `result.createTime` | 日志元信息 | final reply 给用户摘要时一并展示 |

### 收件箱：列出我收到的日报
```
Usage:
  dws report inbox list [flags]
Example:
  dws report inbox list --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --cursor 0 --size 20
  dws report inbox list --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --sender-user-ids staffId1,staffId2
  # 禁止：--start-date / --end-date / --date
  # 从列表结果提取 reportId 后，再查正文
  dws report entry get --report-id <reportId>
Flags:
      --cursor int                分页游标，首次传 0 (默认 0)
      --end string                结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)
      --sender-user-ids strings   发送人 staffId 列表，逗号分隔，用于过滤指定发送人的日志 (可选)
      --size int                  每页条数，最大 20 (默认 20)，别名: --limit
      --start string              开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)
```


`inbox list` 默认返回 JSON-first 结构：不会把 Markdown 表作为裸文本直接输出，但 JSON 里会返回 `agentDisplayMarkdown`。Agent 必须在 final reply 直接原样输出 `agentDisplayMarkdown`，让客户端按 Markdown 渲染出表格；不要基于个人偏好重新摘要。表头固定且必须逐字为 `| 日期 | 标题 | 发送人 | 状态 | 钉钉链接 |`，并按返回顺序展示（CLI 已按创建时间从近到远排序）。返回 JSON 中的 `agentDisplayRequired=true`、`agentDisplayRequiredColumns`、`agentDisplayLinkColumn=钉钉链接`、`agentDisplayMarkdownRequired=true`、`agentDisplayMarkdownHeader` 都是强制展示合同。凡用户说"列出 / 找到 / 查询 / 搜索 / 看看日志"，final reply 默认必须渲染这张 Markdown 表；只有用户明确表示"不关心列表内容 / 只要原始 JSON / 只要数量 / 只要 ID"时，才可以不渲染表。`result[]` 每项都包含且只展示这五个字段：`日期`、`标题`、`发送人`、`状态`、`钉钉链接`。`钉钉链接` 是强制可点击 markdown 链接列，禁止省略、改名、合并到标题里；`inbox list` 禁止返回或展示 `日志内容` / `日志内容摘要`。`reportId` 不在主结果里，只通过 `_internalDetailCommands` 保留给 Agent 后续调用 `entry get` / `entry stats`；final reply 禁止展示 `_internalDetailCommands`、毫秒时间戳、raw ID 或日志正文。

### 读取单份日报的已读统计
```
Usage:
  dws report entry stats [flags]
Example:
  dws report entry stats --report-id <reportId>
Flags:
      --report-id string   日志 ID (必填)
```


### 发件箱：列出我发出的日报
```
Usage:
  dws report outbox list [flags]
Example:
  dws report outbox list --cursor 0 --size 20
  dws report outbox list --cursor 0 --size 20 --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00"
  dws report outbox list --cursor 0 --size 20 --template-name "日报"
  # 从列表结果提取 reportId 后，再查正文
  dws report entry get --report-id <reportId>
Flags:
      --cursor int            分页游标，首次传 0 (默认 0)
      --size int              每页条数，最大 20 (默认 20)，别名: --limit
      --start string          创建开始时间 ISO-8601 (默认最近 20 天；服务端单次查询跨度上限 20 天)
      --end string            创建结束时间 ISO-8601 (默认最近 20 天；服务端单次查询跨度上限 20 天)
      --modified-start string 修改开始时间 ISO-8601 (可选)
      --modified-end string   修改结束时间 ISO-8601 (可选)
      --template-name string  日志模版名称 (可选，不传查全部)
```


`outbox list` 同样主要返回已发送日志的 ID 和摘要；要查看正文，继续用 `entry get`。

**默认时间窗口**：`--start` / `--end` 未传时，CLI 自动回退到最近 20 天（服务端 `get_send_report_list` 单次查询跨度**上限 20 天**，超过会被服务端拒绝），并在 stderr 输出一行 informational：

```
# info: --start / --end not provided, defaulting to last 20 days (<start_iso> ~ <end_iso>); server caps single-query span at 20 days, pass explicit --start to shift the window
```

查更早数据需**多次调用**并显式滚动 `--start` / `--end`（每次跨度 ≤ 20 天），不能一次性传超过 20 天范围；不要假定 `outbox list` 不传时间窗口就是全量。

## 两步读取正文

读取已有日志内容时，统一按下面两步走，不要把列表接口当正文接口：

1. `dws report inbox list ...` 或 `dws report outbox list ...`，先拿到目标日志的 `reportId`
2. `dws report entry get --report-id <reportId>`，再读取正文和字段明细

适用场景：

- "看我今天收到的某条周报正文"
- "把我发过的日报正文拉出来继续汇总"
- "先按时间范围筛日志，再读取具体内容"

## 意图判断

用户说"查日志/看日报/看正文" → `inbox list` 或 `outbox list` 获取列表，再 `entry get`
用户说"写日报/提交周报/发日志/填日志" → 先 `template list` / `template get` 取 `templateId` 与各控件 `key`/`sort`/类型，拼 `--contents` JSON，再 `entry submit`
用户说"日志统计/已读统计" → `entry stats`
用户说"有什么日志模版" → `template list` 或 `template get`
用户说"我发过的日志/我创建的日志" → `outbox list`
用户说"别人发给我的日志/我收到的日志" → `inbox list`

关键区分: report(钉钉日志模版汇报，含提交) vs doc(文档编辑) vs todo(待办任务)

## 核心工作流

```bash
# 1. 获取当前用户可用的日志模版
dws report template list --format json

# 2. 按名称读取模版字段定义
dws report template get --name "日报" --format json

# 2b. 提交日志（从步骤 1/2 取 templateId 与 contents 字段）— 推荐 --contents-file 传入避免 shell 引号
dws report entry submit --template-id <templateId> --contents-file ./report.json --format json
# submit 成功会自动反查详情并追加 dingtalkOpenMarkdownLink / dingtalkOpenUrl；
# final reply 直接使用 dingtalkOpenMarkdownLink: [在钉钉中查看日志](dingtalk://...)

# 2c. 仅当 submit 返回中缺少 dingtalkOpenUrl 时，手动补取 dingtalk:// 跳转链接
dws report entry get --report-id <submit 返回的 reportId> --format json
# → 取 result.url，final reply: [在钉钉中查看日志](result.url)

# 3. 查看收到的日报列表 — 提取 reportId
dws report inbox list --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" \
  --cursor 0 --size 20 --format json

# 4. 查看日报详情（正文/字段明细）
dws report entry get --report-id <reportId> --format json

# 5. 查看日报已读统计
dws report entry stats --report-id <reportId> --format json

# 6. 查看我发出的日报列表
dws report outbox list --cursor 0 --size 20 --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `template list` | template 名称（`items[].report_template_name`；顶层键只有 `items` 和 `success`，**无 `result` 包裹**） | `template get` 的 --name |
| `template list` | `items[].report_template_id` | `entry submit` 的 --template-id |
| `template get` | `result.report_template_fields[].field_name` / `field_sort` / `field_type` | 拼 `entry submit` 的 --contents JSON（按下表映射）|
| `entry submit` | `reportId`、`dingtalkOpenMarkdownLink`、`dingtalkOpenUrl`（CLI 自动反查详情后追加）| final reply 优先直接使用 `dingtalkOpenMarkdownLink`；需要结构化展示时用 `dingtalkOpenLink.title` + `dingtalkOpenLink.url` |
| `inbox list` / `outbox list` | `reportId` | `entry get` / `entry stats` 的 --report-id |
| `entry get` | `result.url`（`dingtalk://...`）| `entry submit` 未返回 `dingtalkOpenUrl` 或查看已有日志时，final reply 中以 markdown link 形式给用户：`[在钉钉中查看日志](result.url)` |

**`template get` → `entry submit --contents` 字段映射**（必须严格对齐，不要自己改写字段名）：

| `template get` 返回 | `entry submit --contents` 字段 |
|------------------------|--------------------------|
| `field_name`（string）| `key` |
| `field_sort`（number）| `sort`（string，例如 `"0"`） |
| `field_type`（number）| `type`（string，例如 `"1"`） |
| —（用户填写）| `content` |
| —（推断）| `contentType`（`type=1` 用 `markdown`，其余用 `origin`） |

## 注意事项

- `--start` / `--end` 使用 ISO-8601 格式（如 `2026-03-10T00:00:00+08:00`）
- `template list` 不需要参数，直接返回当前用户可用的所有日志模版
- `entry submit` 前必须先查模版（参见「提交链路硬约束」），勿猜测 `templateId` 或 `contents` 中的 `key`/`sort`；多控件时数组须覆盖模版必填项
- `inbox list` / `outbox list` 默认用于筛选目标日志，不保证返回完整正文；读取正文请继续调用 `entry get`
- `outbox list` 不传时间窗口默认最近 20 天（服务端单次查询跨度上限 20 天，超过会被拒绝），CLI 会在 stderr 输出 informational 提示；查更早数据需多次滚动调用，每次跨度 ≤ 20 天
- `entry submit` 的 contents JSON 大小限制为 **10MB**，**不支持分批次提交**。超出限制时需精简内容或将内容拆分为多个独立日志分别提交

## 常见错误诊断（CLI Code → 真实含义 → 建议动作）

错误码均落到 errors.go 既有 `INPUT_*` / `MCP_*` / `RESOURCE_*` 体系，对应进程退出码见全局错误码文档。

| Code | ExitCode | 真实含义 | 建议动作 |
|------|---------|---------|---------|
| `INPUT_INVALID_JSON` | 3 | `--contents` 或 `--contents-file` 内容非合法 JSON | 检查 JSON 数组结构，每项必须是 object，含 `key`/`sort`/`content`/`contentType`/`type` 五个字段 |
| `INPUT_FILE_NOT_FOUND` | 3 | `--contents-file` 指向的文件不存在（路径必须在当前工作目录 CWD 之下）| 确认文件存在且位于 CWD 内；把 contents JSON 写到 CWD 下的相对路径再传 |
| `INPUT_INVALID_PATH` | 3 | `--contents-file` 路径越出当前工作目录（如 `/tmp`、`os.tmpdir()`、`../` 或指向目录外的符号链接）| **不要**用 `/tmp` / `os.tmpdir()` / `../`；把 contents JSON 写到当前工作目录之下再传 |
| `INPUT_MISSING_PARAM` | 3 | `--template-id` / `--contents` 必填缺失 | 显式传值；从 `template list` 取合法 templateId |
| `INPUT_TOO_LARGE` | 3 | contents JSON 超过 10MB 限制 | **不支持分批次提交**。需精简内容或拆分为多个独立日志分别提交 |
| `MCP_TOOL_ERROR` / `[UNCLASSIFIED] business error` | 1 | 服务端业务错，`success=false`、`code=1`。覆盖：submit 时 templateId 错 / 字段名错 / 字段值错 / contents 空（`server_error_code: PARAM_ERROR`）；`entry get` / `entry stats` 传了不存在的 reportId（`server_error_code: BUSINESS_ERROR`，technical_detail 含 `report not exist`）；`template get` 传了不存在的模版名（`server_error_code: PARAM_ERROR`）| 查看 `server_error_code` / `technical_detail`；reportId / 模版名找不到时先用 `inbox list` / `outbox list` / `template list` 重新取合法值，不要猜；submit 类错误按链路重走 `template list → template get → entry submit`，连续 ≥ 2 次仍失败必须停止重试，降级 final_reply |

## 何时停止重试

`dws report entry submit` 在以下情况下**必须停止重试**，转为降级 final_reply：

1. 同一 templateId 连续 ≥ 3 次返回 PARAM_ERROR / INVALID_CONTENTS 类错误，且每次重试只是改 contents 字段名 / 格式而未重读 schema；
2. 出现服务端不可读的 PARAM_ERROR（technical_detail 仅含 `root.success当前值`）—— 即使只 1 次也应停止；
3. 出现 `INPUT_FILE_NOT_FOUND` 后下一次重试仍未先 `ls` 验证路径存在性。

降级 final_reply 模板：

> 当前 `dws report entry submit` 在该模版下持续返回不可恢复错误（已尝试 N 次，错误码 `<Code>`）。建议您：
>
> 1. 在钉钉客户端打开「日志」应用，选择「<模版名>」模版；
> 2. 复制下面的内容粘贴到对应字段；
> 3. 提交。
>
> 我已记录本次失败 trace，会同步给 dws 团队修复。

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|



## 相关产品

- [doc](../../dingtalk-doc/references/doc.md) — 长文本文档创作（钉钉在线文档），不是日志模版
- [todo](../../dingtalk-todo/references/todo.md) — 个人任务管理，不是日志汇报
