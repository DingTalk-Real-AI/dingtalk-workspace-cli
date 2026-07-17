# 邮箱与邮件命令

## 命令总览

### 查询可用邮箱地址
> **注意：** 仅返回当前登录用户**自己的**邮箱列表，不能用于查找他人邮箱。查找他人邮箱请使用三路并发流程（见"查找他人邮箱地址"章节）。
```
Usage:
  dws mail mailbox list [flags]
Example:
  dws mail mailbox list
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `mailboxes` | `List[]` | 邮箱列表，每条包含邮箱地址、账号类型、所属企业 |

### 获取用户邮箱信息

```
Usage:
  dws mail mailbox profile [flags]
Example:
  dws mail mailbox profile --email user@company.com
Flags:
      --email string   用户的邮箱地址 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `email` | `string` | 邮箱地址 |
| `emailAliases` | `string[]` | 邮件地址别名列表 |
| `name` | `string` | 用户名 |
| `nickname` | `string` | 用户昵称 |
| `displayName` | `string` | 用户显示名 |
| `mboxSize` | `string` | 邮箱容量（字节） |
| `mboxSizeUsed` | `string` | 已使用的邮箱容量（字节） |
| `createdTime` | `string` | 创建时间 |
| `modifiedTime` | `string` | 修改时间 |

### 查找他人邮箱地址（通讯录查人）

> **这不是 `mailbox list`。** 当需要获取**某人**的邮箱地址时，必须走以下三路并发查询，取最先返回有效邮箱的结果。禁止臆测邮箱地址。

**触发场景：** 用户说「获取/查找/得到 某人的邮箱地址」、「给某人发邮件」、「某人发给我的邮件」等任何涉及按姓名找邮箱的场景。

**三路并发查询流程：**

```bash
# 同时发起以下三路，取最先返回有效邮箱的结果
# 路径 1：aisearch + contact user get
dws aisearch person --keyword "姓名" --dimension name --format json
# → 取 userId，再执行：
dws contact user get --ids <userId> --format json
# → 提取 orgAuthEmail 字段

# 路径 2：mail user search（仅企业邮箱可用，个人邮箱会报权限错误可忽略；已知工号时可用 --employee-no 替代 --keyword）
dws mail user search --email <当前邮箱> --keyword "姓名" --format json
# 或按工号查询：dws mail user search --email <当前邮箱> --employee-no "工号" --format json
# → 提取 users[].email

# 路径 3：contact user search
dws contact user search --query "姓名" --format json
# → 提取用户邮箱字段
```

若三路均无有效邮箱，必须请用户手动提供，**严禁臆测**。

### 列出文件夹中的邮件
> **注意：** `message list` 用于按文件夹列出邮件；若需根据主题/发件人/日期等条件精确搜索，请使用 `message search`。
```
Usage:
  dws mail message list [flags]
Example:
  dws mail message list --email user@company.com
  dws mail message list --email user@company.com --folder-id 1
  dws mail message list --email user@company.com --folder-id 2 --limit 50
  dws mail message list --email user@company.com --cursor <nextCursor>
Flags:
      --email string      邮件所属邮箱地址 (必填)
      --folder-id string  文件夹 ID（1=已发送, 2=收件箱, 3=垃圾邮件, 5=草稿, 6=已删除），默认为收件箱，别名: --folder
      --limit string      每页返回数量(最大限制 100, 默认 20)，别名: --size, --page-size
      --cursor string     邮件的起始偏移标识, 其值取自响应中的nextCursor字段。""表示从头开始
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `messages` | `List[]` | 邮件列表，每条包含邮件 ID 及元信息（不含正文） |
| `total` | `int32` | 符合条件的总邮件数 |
| `nextCursor` | `string` | 下一页游标，传入 `--cursor` 翻页；值为 `$` 表示已到达列表尾部 |

### 搜索邮件 (KQL 语法)
```
Usage:
  dws mail message search [flags]
Example:
  dws mail message search --email user@company.com --query "subject:\"周报\"" --limit 20
  dws mail message search --email user@company.com --query "from:alice AND date>2025-06-01T00:00:00Z" --limit 10
Flags:
      --cursor string   邮件的起始偏移标识, 其值取自响应中的nextCursor字段。""表示从头开始
      --email string    搜索目标邮箱地址 (必填)
      --query string    KQL 查询表达式 (必填), 其中 date 格式需遵循 ISO8601 规范
      --limit string    每页返回数量(最大限制 100, 默认 20)，别名: --size, --page-size
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
dws mail message search --email user@company.com --query "folderId:2" --limit 20 --format json
# 取返回中的 nextCursor，传入下一次请求（nextCursor="$" 时停止）
dws mail message search --email user@company.com --query "folderId:2" --limit 20 --cursor <nextCursor> --format json
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
| `message` | `object` | 邮件完整信息，包含主题、发件人、收件人、正文、附件等 |

### 批量获取邮件详情

```
Usage:
  dws mail message batch-get [flags]
Example:
  dws mail message batch-get --email user@company.com --ids <id1>,<id2>
  dws mail message batch-get --email user@company.com --ids <id1>,<id2>,<id3>
Flags:
      --email string   邮件所属邮箱地址 (必填)
      --ids string     要获取的邮件 ID 列表，逗号分隔，最多 20 个 (必填)
```

单次最多获取 20 封邮件。

**返回 JSON：**

```json
{
  "success": true,
  "messages": [
    { "subject": "...", "from": "...", "to": [...], "body": "...", ... },
    { "subject": "...", "from": "...", "to": [...], "body": "...", ... }
  ]
}
```

> **注意：** 如果某个邮件 ID 获取失败，整个命令会报错并中止。建议先通过 `message search` 或 `message list` 确认邮件 ID 有效后再批量获取。

### 发送邮件
```
Usage:
  dws mail message send [flags]
Example:
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "周报" --content "本周完成任务A和任务B"
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "周报" --content "见附件" --attachment ./report.pdf
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "周报" --content "见附件" --attachment ./a.pdf --attachment ./b.xlsx
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "图表周报" --content "图表如下：[inline:chart.png]" --inline-attachment ./chart.png
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "带图文档" --content "见附件，图表：[inline:img.png]" --attachment ./doc.pdf --inline-attachment ./img.png
Flags:
      --content string                   邮件正文 (必填)，别名: --body
      --cc string                       抄送人列表
      --from string                     发件人邮箱 (必填)，别名: --sender
      --subject string                  邮件标题 (必填)
      --to string                       收件人列表 (必填)
      --attachment stringArray          附件文件路径，可多次指定 (可选)
      --inline-attachment stringArray   内联图片路径，可多次指定，cid 自动生成 (可选)
```

**附件发送说明：**

当指定 `--attachment` 或 `--inline-attachment` 时，CLI 自动执行以下编排流程：

1. 创建邮件草稿（若有内联图片，正文自动转为 HTML 并注入 `<img>` 标签）
2. 为每个普通附件调用 `create_upload_session`（`isInline=false`），从响应的 `uploadUrl` 字段获取完整上传地址，HTTP POST 上传文件内容
3. 为每个内联图片调用 `create_upload_session`（`isInline=true`，传入 contentId），从响应的 `uploadUrl` 字段获取完整上传地址，HTTP POST 上传文件内容
4. 调用 `send_draft` 发送草稿

> **注意：** 附件必须通过 `--attachment` / `--inline-attachment` 参数传入，**严禁使用钉钉媒体存储（media upload）上传附件**。

**内联图片说明（`--inline-attachment`）：**

- 仅支持图片类型：`jpg` / `jpeg` / `png` / `gif` / `webp` / `bmp` / `svg`
- CLI 自动生成 contentId，格式：`inline-{文件名(不含扩展名)}-{序号}@alimail.com`，例：`inline-chart-1@alimail.com`
- 在 `--content` 中使用占位符 `[inline:文件名]` 引用图片，CLI 自动替换为 `<img src="cid:...">` 标签
- 若 content 中没有对应占位符，内联图片会自动追加到正文末尾
- 非图片类型（PDF、视频、音频等）请改用 `--attachment`
