# 邮箱 (mail) 命令参考

## 命令速查目录

| 命令 | 功能简述 |
|------|----------|
| `dws mail mailbox list` | 查询**当前用户自己**的可用邮箱列表 |
| `dws mail message search` | 搜索邮件（KQL 语法，按主题/发件人/日期等） |
| `dws mail message get` | 查看邮件完整内容（含正文） |
| `dws mail message send` | 发送邮件 |

> **查找他人邮箱**（如「获取严龙的邮箱」）→ **不要用 `mailbox list`**，应走两路并发查询，详见「查找他人邮箱地址」章节。

> **范围说明：** 当前 `dws mail` 只实现了 `mailbox list` 与 `message search / get / send` 四条命令。诸如 `reply` / `reply-all` / `forward` / `batch-move` / `batch-delete` / `draft *` / `folder list` / `attachment list / download` / `tag list` / `thread get` / `mail user search` 等命令**目前尚未实现**，禁止编造这些命令——遇到 LLM 误生成时立即报错或回退到 ask_human。

---

## 默认邮箱选择规则（重要）

所有 mail 相关命令，**除非用户明确要求使用个人邮箱，否则一律默认使用企业邮箱**。

**适用范围：** 任何需要传入 `--email` / `--from` 参数的 mail 子命令一律适用。

**默认选择策略：**

1. 调用 `dws mail mailbox list --format json` 获取当前用户的所有邮箱。
2. 从返回的 `emailAccounts` 中**优先选择企业邮箱**（账号 `type` 为 `ORGANIZATION`、域名非 `@dingtalk.com` 的邮箱），将其作为 `--email` / `--from` 的默认值。
3. 仅当用户在指令中**明确指定**「用我的个人邮箱」「用 dingtalk.com 邮箱」「用我的私人邮箱」等表述时，才选择个人邮箱（`type` 为 `PERSONAL`，通常是 `@dingtalk.com` 域名）。
4. 若用户同时拥有多个企业邮箱（如分属多家公司），优先选择与当前会话上下文匹配的企业邮箱；若仍无法判断，向用户确认后再操作。
5. 若用户**仅拥有个人邮箱**（无企业邮箱），可直接使用个人邮箱。

**触发个人邮箱的关键词举例：** 「我的个人邮箱」「私人邮箱」「dingtalk.com 邮箱」「@dingtalk 的邮箱」「我的 personal 邮箱」。

> 该规则覆盖文档后续所有命令示例：示例中虽以 `user@company.com` 等占位邮箱书写，实际执行时**必须按上述策略动态选择企业邮箱**，不要直接照抄示例中的邮箱字面量，更不要默认使用 `@dingtalk.com` 个人邮箱。

---

## 命令总览

### 查询可用邮箱地址
> **注意：** 仅返回当前登录用户**自己的**邮箱列表，不能用于查找他人邮箱。查找他人邮箱请使用两路并发流程（见"查找他人邮箱地址"章节）。
```
Usage:
  dws mail mailbox list [flags]
Example:
  dws mail mailbox list
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `emailAccounts` | `List[]` | 邮箱列表，每条包含 `email` / `type` / `orgName`；type 取值 `PERSONAL`（个人邮箱）/ `ORGANIZATION`（企业邮箱） |

### 查找他人邮箱地址（通讯录查人）

> **这不是 `mailbox list`。** 当需要获取**某人**的邮箱地址时，必须走以下两路并发查询，取最先返回有效邮箱的结果。禁止臆测邮箱地址。

**触发场景：** 用户说「获取/查找/得到 某人的邮箱地址」、「给某人发邮件」、「某人发给我的邮件」等任何涉及按姓名找邮箱的场景。

**两路并发查询流程：**

```bash
# 同时发起以下两路，取最先返回有效邮箱的结果
# 路径 1：aisearch + contact user get
dws aisearch person --keyword "姓名" --dimension name --format json
# → 取 userId，再执行：
dws contact user get --ids <userId> --format json
# → 提取 orgAuthEmail 字段

# 路径 2：contact user search
dws contact user search --keyword "姓名" --format json
# → 提取用户邮箱字段
```

若两路均无有效邮箱，必须 `ask_human` 请用户手动提供，**严禁臆测**。

### 搜索邮件 (KQL 语法)
```
Usage:
  dws mail message search [flags]
Example:
  dws mail message search --email user@company.com --query "subject:\"周报\"" --size 20
  dws mail message search --email user@company.com --query "from:alice AND date>2025-06-01T00:00:00Z" --size 10
Flags:
      --cursor string   分页游标 (首页留空)
      --email string    搜索目标邮箱地址 (必填)
      --query string    KQL 查询表达式 (必填)
      --size string     每页返回数量 1-100 (默认 20)
```

KQL 查询字段: date, size, tag, folderId, isRead, hasAttachments, subject, attachname, body, from, to
常用文件夹 ID: 1=已发送, 2=收件箱, 3=垃圾邮件, 5=草稿, 6=已删除

### KQL 查询字段说明

| 字段 | 类型 | 说明 | 正确示例 | 错误示例 |
|------|------|------|----------|----------|
| `date` | ISO8601 日期时间 | 邮件日期，支持 `>` `<` `>=` `<=` 比较运算符 | `date>2025-06-01T00:00:00Z` | `date>2025-06-01`（缺少时间部分） |
| `size` | 整数（字节数） | 邮件大小，支持 `>` `<` `>=` `<=` 比较运算符 | `size>1024` | `size>"1024"`（值不需要引号） |
| `tag` | 字符串 | 邮件标签 | `tag:important` | `tag:""` |
| `folderId` | 整数 | 文件夹 ID（1=已发送, 2=收件箱, 3=垃圾邮件, 5=草稿, 6=已删除） | `folderId:2` | `folderId:"收件箱"`（必须用数字 ID） |
| `isRead` | 布尔 `true`/`false` | 是否已读 | `isRead:false` | `isRead:0`、`isRead:"false"`（不支持数字或字符串形式） |
| `hasAttachments` | 布尔 `true`/`false` | 是否有附件 | `hasAttachments:true` | `hasAttachments:yes` |
| `subject` | 字符串 | 邮件主题，含空格须加双引号 | `subject:周报`、`subject:"项目 进展"` | `subject:项目 进展`（含空格未加引号） |
| `attachname` | 字符串 | 附件文件名，含空格须加双引号 | `attachname:report.pdf`、`attachname:"月度 报告.xlsx"` | `attachname:月度 报告.xlsx`（含空格未加引号） |
| `body` | 字符串 | 邮件正文内容，含空格须加双引号 | `body:会议纪要`、`body:"Q1 总结"` | `body:Q1 总结`（含空格未加引号） |
| `from` | 字符串（邮件地址或名称） | 发件人，支持：纯邮件地址、纯名称（含空格须加双引号）、`"名称<邮件地址>"` 格式 | `from:alice@company.com`、`from:"张 三"`、`from:"alice<a@b.com>"` | `from:张 三`（含空格未加引号） |
| `to` | 字符串（邮件地址或名称） | 收件人，支持：纯邮件地址、纯名称（含空格须加双引号）、`"名称<邮件地址>"` 格式 | `to:bob@company.com`、`to:"李 四"`、`to:"alice<a@b.com>"` | `to:李 四`（含空格未加引号） |

**组合查询说明：**
- 支持 `AND` / `OR` / `NOT` 逻辑运算符（大写）
- 括号用于分组：`(from:alice OR from:bob) AND folderId:2`
- 排除特定文件夹：`(NOT folderId:3) AND (NOT folderId:6)`

### message search 返回值说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `messages` | `List[]` | 邮件列表，每条包含邮件 ID 及元信息（不含正文） |
| `total` | `int32` | 符合条件的总邮件数 |
| `nextCursor` | `string` | 下一页游标，传入 `--cursor` 翻页；值为 `$` 表示已到达列表尾部 |

**翻页示例：**
```bash
# 第一页
dws mail message search --email user@company.com --query "folderId:2" --size 20 --format json
# 取返回中的 nextCursor，传入下一次请求（nextCursor="$" 时停止）
dws mail message search --email user@company.com --query "folderId:2" --size 20 --cursor <nextCursor> --format json
```

### 查看邮件完整内容
```
Usage:
  dws mail message get [flags]
Example:
  dws mail message get --email user@company.com --id <messageId>
Flags:
      --email string   邮件所属邮箱地址 (必填)
      --id string      邮件 ID (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `message` | `object` | 邮件完整信息，包含主题、发件人、收件人、正文（`markdownBody`）等 |

### 发送邮件
```
Usage:
  dws mail message send [flags]
Example:
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "周报" --body "本周完成任务A和任务B"
Flags:
      --body string      邮件正文 (必填)
      --cc string        抄送人列表，逗号分隔 (可选)
      --from string      发件人邮箱 (必填)
      --subject string   邮件标题 (必填)
      --to string        收件人列表，逗号分隔 (必填)
```

> **附件能力**：当前 CLI 不支持通过 `dws mail message send` 直接附加附件或内联图片；如用户需要发带附件的邮件，先告知用户该限制，再让用户在钉钉客户端手动发送，**不要编造 `--attachment` / `--inline-attachment` 等参数**。

## 通用错误说明

以下错误适用于所有 mail 命令。

| 错误标识 | 含义 | 处理建议 |
|----------|------|----------|
| `domain.notFound` | 该用户的邮箱不是由钉钉邮箱托管，无法完成操作 | 确认邮箱是否已开通钉钉企业邮箱服务 |

## 意图判断

用户说"我的邮箱/邮箱地址" → `mailbox list`（**仅限查询自己的邮箱，不能查他人**）
用户说"获取/查找/得到 某人的邮箱地址" → **不是 `mailbox list`**，走两路并发查询流程（见「查找他人邮箱地址」章节）
用户说"找邮件/搜邮件/查邮件" → `message search`
用户说"看邮件/打开邮件/邮件内容" → 先 `message search` 获取 messageId，再 `message get`
用户说"发邮件/写邮件" → 先 `mailbox list` 获取发件地址，再 `message send`
用户说"给(某人名字)发邮件" / "查询某人发给我的邮件" / "查询发给某人的邮件" →
  **第一步**：并发同时发起以下两路查询，取最先返回有效邮箱的结果；若两路均无，ask_human：
    1. `aisearch person --keyword <姓名>` → `contact user get --ids <userId>`，提取 `orgAuthEmail`
    2. `contact user search --keyword <姓名>`，提取用户邮箱字段
  **第二步**：用获得的目标邮箱拼入 KQL（如 `from:<email>` 或 `to:<email>`）执行 `message search`，或用于 `message send`
用户说"发带附件的邮件" → 当前 CLI 不支持附件参数，告知用户限制并建议在钉钉客户端手动操作
用户说"回复邮件 / 转发邮件 / 批量移动邮件 / 删除邮件 / 草稿管理 / 列文件夹 / 列标签 / 下载附件 / 查看会话线程" → 当前 CLI 未实现这些命令，不要编造；告知用户在钉钉客户端处理

## 严格禁止 (NEVER DO)
- 明确禁止猜测、假设、推断发件人和收件人邮箱
- 无法获取邮箱时，强引导 ask_human，由用户确认，不要通过假设或其他方式继续执行
- **严禁在用户未明确指定使用个人邮箱时，默认选择 `@dingtalk.com` 个人邮箱作为 `--email` / `--from`**；默认必须从 `mailbox list` 中挑选企业邮箱（`type=ORGANIZATION`）
- **严禁编造不存在的命令**：当前 `dws mail` 顶层只有 `mailbox` 与 `message` 两个子命令，`message` 只支持 `search / get / send`；其余 `reply` / `reply-all` / `forward` / `batch-*` / `draft *` / `folder *` / `attachment *` / `tag *` / `thread *` / `user search` 等命令**均未实现**，遇到时报错而不是凭空生成

## 核心工作流

```bash
# 1. 查看可用邮箱 — 提取邮箱地址
dws mail mailbox list --format json

# 2. 搜索邮件 — 提取 messageId
dws mail message search --email user@company.com \
  --query "subject:\"周报\" AND date>2025-06-01T00:00:00Z" --size 10 --format json

# 3. 查看邮件详情
dws mail message get --email user@company.com --id <messageId> --format json

# 4. 发送邮件
dws mail message send --from user@company.com --to colleague@company.com \
  --subject "周报" --body "本周完成…" --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `mailbox list` | `emailAccounts[].email` | message search/get/send 的 --email/--from |
| `message search` | `messageId`（即返回的 `messages[].id`）| message get 的 --id |
| `aisearch person` → `contact user get` / `contact user search` | 用户邮箱 (orgAuthEmail / email) | message send 的 --to/--cc（两路并发，取先到结果） |

## 注意事项

- `mailbox list` 返回用户所有邮箱（含个人和企业），每条记录包含邮箱地址、账号类型、所属企业。**默认一律选择企业邮箱**（除非用户明确指定使用个人邮箱）；若有多个企业邮箱可选，优先匹配用户当前所在企业的那一个；仍无法判断时向用户确认后再操作。详见文档顶部「默认邮箱选择规则」章节
- `message search` 返回邮件 ID 和元信息（不含正文），需 `message get` 获取完整内容
- KQL 查询支持 AND/OR/NOT 组合，字段值含空格时需用双引号
- `--cc` 抄送人支持多人，逗号分隔
- 收件人邮箱获取：用户只知道同事名字时，**并发**同时执行以下两路查询，取最先返回有效邮箱的结果，无需等待另一路完成：
  1. `dws aisearch person --keyword "名字" --dimension name` → `dws contact user get --ids <userId>`，提取 `orgAuthEmail`
  2. `dws contact user search --keyword "名字"`，提取用户邮箱字段
  若两路均无有效邮箱，必须 ask_human 请用户手动提供收件人邮箱，严禁臆测和假设
