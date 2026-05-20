# 邮箱 (mail) 命令参考

## 命令总览

### 查询可用邮箱地址
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

### 搜索邮件 (KQL 语法)
```
Usage:
  dws mail message search [flags]
Example:
  dws mail message search --email user@company.com --query "subject:\"周报\"" --size 20
  dws mail message search --email user@company.com --query "from:alice AND date>2025-06-01T00:00:00Z" --size 10
Flags:
      --cursor string   邮件的起始偏移标识, 其值取自响应中的nextCursor字段。""表示从头开始
      --email string    搜索目标邮箱地址 (必填)
      --query string    KQL 查询表达式 (必填), 其中 date 格式需遵循 ISO8601 规范
      --size string     每页返回数量(最大限制 100, 默认 20) (必填)，别名: --limit, --page-size
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
| `message` | `object` | 邮件完整信息，包含主题、发件人、收件人、正文、附件等 |

### 发送邮件
```
Usage:
  dws mail message send [flags]
Example:
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "周报" --body "本周完成任务A和任务B"
Flags:
      --body string      邮件正文，支持 Markdown 格式 (必填)
      --cc string        抄送人列表
      --from string      发件人邮箱 (必填)，别名: --sender
      --subject string   邮件标题 (必填)
      --to string        收件人列表 (必填)
```

## 通用错误说明

以下错误适用于所有 mail 命令。

| 错误标识 | 含义 | 处理建议 |
|----------|------|----------|
| `domain.notFound` | 该用户的邮箱不是由钉钉邮箱托管，无法完成操作 | 确认邮箱是否已开通钉钉企业邮箱服务 |

## 意图判断

用户说"我的邮箱/邮箱地址" → `mailbox list`
用户说"找邮件/搜邮件/查邮件" → `message search`
用户说"看邮件/打开邮件/邮件内容" → 先 `message search` 获取 messageId，再 `message get`
用户说"发邮件/写邮件" → 先 `mailbox list` 获取发件地址，再 `message send`
用户说"给(某人名字)发邮件" → 先 `aisearch person` 获取 userId，再 `contact user get` 获取收件人邮箱，再 `message send`


## 严格禁止 (NEVER DO)
- 明确禁止猜测、假设、推断发件人和收件人邮箱
- 无法获取邮箱时，强引导ask_human，由用户确认，不要通过假设或其他方式继续执行

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
| `mailbox list` | 邮箱地址 | message search/get/send 的 --email/--from |
| `message search` | `messageId` | message get 的 --id |
| `aisearch person` → `contact user get` | 用户邮箱 (orgAuthEmail) | message send 的 --to/--cc (跨产品) |

## 注意事项

- `mailbox list` 返回用户所有邮箱（含个人和企业），每条记录包含邮箱地址、账号类型、所属企业。选择邮箱时优先匹配用户当前所在企业的企业邮箱；若有多个可选，向用户确认后再操作
- `message search` 返回邮件 ID 和元信息（不含正文），需 `message get` 获取完整内容
- KQL 查询支持 AND/OR/NOT 组合，字段值含空格时需用双引号
- `--cc` 抄送人支持多人，逗号分隔
- 收件人邮箱获取：用户只知道同事名字时，先通过 `dws aisearch person --keyword "名字" --dimension name` 获取 userId，再 `dws contact user get --ids <userId>` 从返回中提取 orgAuthEmail 字段


