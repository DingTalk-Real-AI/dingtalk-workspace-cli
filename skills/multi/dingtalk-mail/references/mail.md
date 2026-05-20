# 邮箱 (mail) 命令参考

## 命令速查目录

| 命令 | 功能简述 |
|------|----------|
| `dws mail mailbox list` | 查询**当前用户自己**的可用邮箱列表 |
| `dws mail message search` | 搜索邮件（KQL 语法，按主题/发件人/日期等） |
| `dws mail message get` | 查看邮件完整内容（含正文） |
| `dws mail message send` | 发送邮件（支持附件/内联图片） |
| `dws mail message reply` | 回复邮件（支持附件/内联图片） |
| `dws mail message reply-all` | 回复所有人（支持附件/内联图片） |
| `dws mail message forward` | 转发邮件（支持附件/内联图片） |
| `dws mail message batch-move` | 批量移动邮件到指定文件夹 |
| `dws mail message batch-delete` | 批量删除邮件 |
| `dws mail draft create` | 创建草稿（保留在草稿箱，不发送） |
| `dws mail draft update` | 更新草稿内容（保留在草稿箱，不发送） |
| `dws mail draft send` | 发送草稿箱中已有的草稿 |
| `dws mail folder list` | 列举邮件文件夹 |
| `dws mail attachment list` | 列举指定邮件的所有附件 |
| `dws mail attachment download` | 下载邮件附件到本地（**仅支持逐个下载，不支持批量下载**） |
| `dws mail tag list` | 列举邮件标签 |
| `dws mail thread get` | 获取会话详情 |
| `dws mail user search` | 搜索通讯录用户（**按姓名查他人邮箱**，不是搜邮件） |

> **查找他人邮箱**（如「获取严龙的邮箱」）→ **不要用 `mailbox list`**，应走三路并发查询，详见「查找他人邮箱地址」章节。

---

## 默认邮箱选择规则（重要）

所有 mail 相关命令，**除非用户明确要求使用个人邮箱，否则一律默认使用企业邮箱**。

**适用范围：** 任何需要传入 `--email` / `--from` / `--sender` 参数的 mail 子命令一律适用。

**默认选择策略：**

1. 调用 `dws mail mailbox list --format json` 获取当前用户的所有邮箱。
2. 从返回的 `mailboxes` 中**优先选择企业邮箱**（账号类型为企业邮箱、域名非 `@dingtalk.com` 的邮箱），将其作为 `--email` / `--from` 的默认值。
3. 仅当用户在指令中**明确指定**「用我的个人邮箱」「用 dingtalk.com 邮箱」「用我的私人邮箱」等表述时，才选择个人邮箱（`@dingtalk.com` 域名）。
4. 若用户同时拥有多个企业邮箱（如分属多家公司），优先选择与当前会话上下文匹配的企业邮箱；若仍无法判断，向用户确认后再操作。
5. 若用户**仅拥有个人邮箱**（无企业邮箱），可直接使用个人邮箱，但需注意 `mail user search` 等仅企业邮箱可用的命令会因权限报错，需走「查找他人邮箱地址」章节的替代路径。

**触发个人邮箱的关键词举例：** 「我的个人邮箱」「私人邮箱」「dingtalk.com 邮箱」「@dingtalk 的邮箱」「我的 personal 邮箱」。

> 该规则覆盖文档后续所有命令示例：示例中虽以 `user@company.com` 等占位邮箱书写，实际执行时**必须按上述策略动态选择企业邮箱**，不要直接照抄示例中的邮箱字面量，更不要默认使用 `@dingtalk.com` 个人邮箱。

---

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

# 路径 2：mail user search（仅企业邮箱可用，个人邮箱会报权限错误可忽略）
dws mail user search --email <当前邮箱> --keyword "姓名" --format json
# → 提取 users[].email

# 路径 3：contact user search
dws contact user search --keyword "姓名" --format json
# → 提取用户邮箱字段
```

若三路均无有效邮箱，必须 `ask_human` 请用户手动提供，**严禁臆测**。

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
      --size string     每页返回数量(最大限制 100, 默认 20)，别名: --limit, --page-size
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
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "周报" --body "见附件" --attachment ./report.pdf
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "周报" --body "见附件" --attachment ./a.pdf --attachment ./b.xlsx
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "图表周报" --body "图表如下：[inline:chart.png]" --inline-attachment ./chart.png
  dws mail message send --from user@company.com --to colleague@company.com \
    --subject "带图文档" --body "见附件，图表：[inline:img.png]" --attachment ./doc.pdf --inline-attachment ./img.png
Flags:
      --body string                     邮件正文 (必填)
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
- 在 `--body` 中使用占位符 `[inline:文件名]` 引用图片，CLI 自动替换为 `<img src="cid:...">` 标签
- 若 body 中没有对应占位符，内联图片会自动追加到正文末尾
- 非图片类型（PDF、视频、音频等）请改用 `--attachment`

### 列举邮件文件夹
```
Usage:
  dws mail folder list [flags]
Example:
  dws mail folder list --email user@company.com
  dws mail folder list --email user@company.com --folder-id <folderId>
Flags:
      --email string      邮件所属邮箱地址 (必填)
      --folder-id string  父文件夹唯一标识，不传则返回顶层文件夹 (可选)
```

不传 `--folder-id` 返回顶层文件夹列表；传入则返回该文件夹的子文件夹列表。

**返回字段（`folders` 数组）：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | `string` | 文件夹唯一标识 |
| `displayName` | `string` | 文件夹显示名称 |
| `parentFolderId` | `string` | 父文件夹 ID |
| `childFolderCount` | `int` | 子文件夹数量 |
| `totalItemCount` | `int` | 邮件总数 |
| `unreadItemCount` | `int` | 未读邮件数量 |

### 列举邮件附件

> **重要：** 不存在 `attachment download_batch` / `download_all` 等批量下载命令。如需下载多封邮件的所有附件，必须按以下流程逐个下载：1) `message search` 搜索邮件获取 messageId 列表 → 2) 对每封邮件 `attachment list` 获取 attachmentId + name → 3) 对每个附件逐个调用 `attachment download`。

```
Usage:
  dws mail attachment list [flags]
Example:
  dws mail attachment list --email user@company.com --id <messageId>
Flags:
      --email string   用户邮箱地址 (必填)
      --id string      邮件唯一标识 messageId (必填)
```

列出指定邮件的所有附件信息。

**返回字段（`attachments` 数组）：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | `string` | 附件唯一标识 |
| `name` | `string` | 附件文件名 |
| `contentType` | `string` | 附件 MIME 类型 |
| `size` | `int` | 附件大小（字节） |

### 下载邮件附件

> **重要：** `attachment download` 每次只能下载**一个**附件。不存在 `download_batch` / `download_all` / `batch_download` 等批量下载命令，不要编造不存在的命令。如需下载多封邮件的所有附件，必须循环执行：对每封邮件先 `attachment list` 获取附件列表，再对每个附件逐个调用 `attachment download`。

```
Usage:
  dws mail attachment download [flags]
Example:
  # 先列出附件获取 id 和 name
  dws mail attachment list --email user@company.com --id <messageId>
  # 再下载指定附件到当前目录（每次只能下载一个附件）
  dws mail attachment download --email user@company.com --message-id <messageId> --attachment-id <attachmentId> --name report.pdf
  # 下载到指定目录
  dws mail attachment download --email user@company.com --message-id <messageId> --attachment-id <attachmentId> --name img.png --output /tmp
Flags:
      --email string           用户邮箱地址 (必填)
      --message-id string      邮件唯一标识 messageId (必填)
      --attachment-id string   附件唯一标识，取自 attachment list 的 id 字段 (必填)
      --name string            保存到本地的文件名，取自 attachment list 的 name 字段 (必填)
      --output string          保存目录，默认为当前目录
```

下载指定邮件的某个附件到本地。CLI 自动执行以下编排流程：

1. 调用 `create_download_session`，从响应的 `downloadUrl` 字段获取完整下载地址
2. 通过 HTTP GET 下载附件内容并保存到本地

> **注意：** `--name` 和 `--attachment-id` 均来自 `attachment list` 的返回结果，建议先执行 `attachment list` 再执行 `attachment download`。

### 列举邮件标签
```
Usage:
  dws mail tag list [flags]
Example:
  dws mail tag list --email user@company.com
Flags:
      --email string   用户的邮箱地址 (必填)
```

列出指定邮箱下的所有邮件标签，返回标签的 ID 和元信息。

**返回字段（`tags` 数组）：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | `string` | 标签唯一标识 |
| `name` | `string` | 标签显示名称 |
| `parentId` | `string` | 父标签 ID |
| `totalItemCount` | `int` | 标签下邮件总数 |
| `unreadItemCount` | `int` | 标签下未读邮件数量 |

### 获取会话详情
```
Usage:
  dws mail thread get [flags]
Example:
  dws mail thread get --email user@company.com --id <conversationId>
Flags:
      --email string   会话所属邮箱地址 (必填)
      --id string      会话唯一标识 conversationId (必填)
```

**返回字段（`conversation` 对象）：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | `string` | 会话唯一标识 |
| `subject` | `string` | 会话主题 |
| `summary` | `string` | 会话摘要信息 |
| `lastModifiedDateTime` | `string (date-time)` | 会话最后修改时间 |
| `messageCount` | `int32` | 会话邮件数量 |
| `tags` | `array[string]` | 会话 tag 信息 |
| `senders` | `List[{email, name}]` | 会话发件人列表 |
| `isRead` | `boolean` | 会话是否已读（全部已读/未读） |
| `priority` | `string` | 会话重要性，取会话内邮件最高优先级（`PRY_HIGH` / `PRY_NORMAL`） |
| `flag` | `string` | 会话标识，取会话内最近邮件的标识（`FLAG_NONE` / `FLAG_REPLY` / `FLAG_FORWARD`） |
| `hasAttachments` | `boolean` | 会话是否包含附件（不含 inline 资源） |
### 回复邮件
```
Usage:
  dws mail message reply [flags]
Example:
  dws mail message reply --from user@company.com --id <messageId>
  dws mail message reply --from user@company.com --id <messageId> --subject "Re: 周报" --body "已收到，谢谢！"
Flags:
      --from string                     发件人邮箱 (必填)，别名: --sender
      --to string                       收件人列表（可选）
      --id string                       要回复的邮件 ID (必填)
      --subject string                  回复邮件标题（可选）
      --body string                     回复正文（可选）
      --attachment stringArray          附件文件路径，可多次指定 (可选)
      --inline-attachment stringArray   内联图片路径，可多次指定，cid 自动生成 (可选)
```

**附件发送说明：**

当指定 `--attachment` 或 `--inline-attachment` 时，CLI 自动执行以下编排流程：

1. 调用 `create_reply_draft` 创建回复草稿（若有内联图片，正文自动转为 HTML 并注入 `<img>` 标签）
2. 为每个普通附件创建上传会话并上传（`isInline=false`）
3. 为每个内联图片创建上传会话并上传（`isInline=true`，传入自动生成的 contentId）
4. 发送草稿

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `messageId` | `string` | 新生成的回复邮件 ID |

### 回复所有人
```
Usage:
  dws mail message reply-all [flags]
Example:
  dws mail message reply-all --from user@company.com --id <messageId>
  dws mail message reply-all --from user@company.com --id <messageId> --subject "Re: 周报" --body "感谢大家的参与！"
Flags:
      --from string                     发件人邮箱 (必填)，别名: --sender
      --to string                       收件人列表（可选，包含发件人及所有原始收件人）
      --id string                       要回复的邮件 ID (必填)
      --subject string                  回复邮件标题（可选）
      --body string                     回复正文（可选）
      --attachment stringArray          附件文件路径，可多次指定 (可选)
      --inline-attachment stringArray   内联图片路径，可多次指定，cid 自动生成 (可选)
```

**附件发送说明：**

当指定 `--attachment` 或 `--inline-attachment` 时，CLI 自动执行以下编排流程：

1. 调用 `create_replyall_draft` 创建回复全部草稿（若有内联图片，正文自动转为 HTML 并注入 `<img>` 标签）
2. 为每个普通附件创建上传会话并上传（`isInline=false`）
3. 为每个内联图片创建上传会话并上传（`isInline=true`，传入自动生成的 contentId）
4. 发送草稿

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `messageId` | `string` | 新生成的回复邮件 ID |

### 转发邮件
```
Usage:
  dws mail message forward [flags]
Example:
  dws mail message forward --from user@company.com --id <messageId>
  dws mail message forward --from user@company.com --to colleague@company.com --id <messageId> --subject "Fwd: 周报"
Flags:
      --from string                     发件人邮箱 (必填)，别名: --sender
      --to string                       转发收件人列表（可选）
      --id string                       要转发的邮件 ID (必填)
      --subject string                  转发邮件标题（可选）
      --body string                     转发附言（可选）
      --attachment stringArray          附件文件路径，可多次指定 (可选)
      --inline-attachment stringArray   内联图片路径，可多次指定，cid 自动生成 (可选)
```

**附件发送说明：**

当指定 `--attachment` 或 `--inline-attachment` 时，CLI 自动执行以下编排流程：

1. 调用 `create_forward_draft` 创建转发草稿（若有内联图片，正文自动转为 HTML 并注入 `<img>` 标签）
2. 为每个普通附件创建上传会话并上传（`isInline=false`）
3. 为每个内联图片创建上传会话并上传（`isInline=true`，传入自动生成的 contentId）
4. 发送草稿

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `messageId` | `string` | 新生成的转发邮件 ID |

### 批量移动邮件到指定文件夹
```
Usage:
  dws mail message batch-move [flags]
Example:
  dws mail message batch-move --email user@company.com --ids <id1>,<id2> --folder 6
Flags:
      --email string    邮件所属邮箱地址 (必填)
      --ids string      要移动的邮件 ID 列表，逗号分隔 (必填)
      --folder string   目标文件夹 ID (必填)
```

常用文件夹 ID: 1=已发送, 2=收件箱, 3=垃圾邮件, 5=草稿, 6=已删除


### 批量删除邮件
```
Usage:
  dws mail message batch-delete [flags]
Example:
  dws mail message batch-delete --email user@company.com --ids <id1>,<id2>
Flags:
      --email string   邮件所属邮箱地址 (必填)
      --ids string     要删除的邮件 ID 列表，逗号分隔 (必填)
```

### 创建草稿
```
Usage:
  dws mail draft create [flags]
Example:
  dws mail draft create --from user@company.com --to colleague@company.com \
    --subject "草稿标题" --body "草稿正文"
  dws mail draft create --from user@company.com --subject "草稿标题"
  dws mail draft create --from user@company.com --subject "带附件草稿" \
    --body "见附件" --attachment ./report.pdf
  dws mail draft create --from user@company.com --subject "带图片草稿" \
    --body "图表：[inline:chart.png]" --inline-attachment ./chart.png
Flags:
      --from string                     发件人邮箱 (必填)，别名: --sender
      --subject string                  邮件标题 (必填)
      --to string                       收件人列表（可选，有确定收件人时才传）
      --cc string                       抄送人列表（可选，有确定抄送人时才传）
      --body string                     邮件正文（可选，有正文内容时才传）
      --attachment stringArray          附件文件路径，可多次指定 (可选)
      --inline-attachment stringArray   内联图片路径，可多次指定，cid 自动生成 (可选)
```

> **注意：** `--to`、`--cc`、`--body` 均为可选参数，**仅在用户明确提供对应信息时才传入**。若用户未指定收件人，不要传 `--to ""`（空字符串）。

**附件说明：**

指定 `--attachment` 或 `--inline-attachment` 时，CLI 自动完成草稿创建和附件上传，**草稿保留在草稿箱，不会发送**。内联图片用法同 `message send`（`--body` 中使用 `[inline:文件名]` 占位符）。

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `messageId` | `string` | 新建草稿的邮件 ID |

### 更新草稿
```
Usage:
  dws mail draft update [flags]
Example:
  dws mail draft update --from user@company.com --id <messageId> --subject "新标题" --body "新正文"
  dws mail draft update --from user@company.com --id <messageId> --body "见附件" --attachment ./report.pdf
  dws mail draft update --from user@company.com --id <messageId> \
    --body "图表：[inline:chart.png]" --inline-attachment ./chart.png
Flags:
      --from string                     发件人邮箱 (必填)，别名: --sender
      --id string                       草稿邮件 ID (必填)
      --to string                       收件人列表（可选）
      --cc string                       抄送人列表（可选）
      --subject string                  邮件标题（可选）
      --body string                     邮件正文（可选）
      --attachment stringArray          附件文件路径，可多次指定 (可选)
      --inline-attachment stringArray   内联图片路径，可多次指定，cid 自动生成 (可选)
```

**附件说明：**

指定 `--attachment` 或 `--inline-attachment` 时，CLI 自动完成草稿更新和附件上传，**草稿保留在草稿箱，不会发送**。内联图片用法同 `message send`（`--body` 中使用 `[inline:文件名]` 占位符）。

### 发送草稿
```
Usage:
  dws mail draft send [flags]
Example:
  dws mail draft send --from user@company.com --id <messageId>
Flags:
      --from string   发件人邮箱 (必填)，别名: --sender
      --id string     草稿邮件 ID (必填)
```

将草稿箱中已有的草稿发送出去。草稿 ID 来自 `draft create` 或 `message search`（`folderId:5`）的返回结果。

### 搜索邮箱用户（通讯录）
```
Usage:
  dws mail user search [flags]
Example:
  dws mail user search --keyword "张三"
  dws mail user search --email user@company.com --keyword "张三"
  dws mail user search --email user@company.com --keyword "alice" --size 10
  dws mail user search --email user@company.com --keyword "alice" --cursor <nextCursor>
Flags:
      --email string    搜索目标邮箱地址 (可选)
      --keyword string  搜索关键词 (必填)
      --cursor string   分页游标，取自响应中的 nextCursor 字段（可选）
      --size string     每页返回数量（可选）
```

> **重要区别：**
> - `mail user search` — 搜索**通讯录联系人/邮箱用户**（按姓名/关键词找人），用于获取某人的邮箱地址
> - `mail message search` — 搜索**邮件内容**（按 KQL 语法搜邮件，如主题、发件人、日期等）
>
> 不要混淆：查找"某人的邮箱地址"用 `user search`；查找"某封邮件"用 `message search`。
>
> 仅企业邮箱（非 `@dingtalk.com` 个人邮箱）可使用 `user search`；使用个人邮箱调用将因无权限而报错。

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `users` | `List[]` | 匹配的用户列表，每条包含用户 ID、邮箱地址、姓名、昵称、工号、职位、工作地 |
| `nextCursor` | `string` | 下一页游标，传入 `--cursor` 翻页 |
| `hasMore` | `boolean` | 是否还有更多数据 |

**user 对象字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | `string` | 用户 ID |
| `email` | `string` | 展示使用的邮件地址 |
| `name` | `string` | 用户名（人名） |
| `nickname` | `string` | 用户昵称（或者花名） |
| `employeeNo` | `string` | 工号 |
| `jobTitle` | `string` | 职位 |
| `workLocation` | `string` | 工作地 |

## 通用错误说明

以下错误适用于所有 mail 命令。

| 错误标识 | 含义 | 处理建议 |
|----------|------|----------|
| `domain.notFound` | 该用户的邮箱不是由钉钉邮箱托管，无法完成操作 | 确认邮箱是否已开通钉钉企业邮箱服务 |

## 意图判断

用户说"我的邮箱/邮箱地址" → `mailbox list`（**仅限查询自己的邮箱，不能查他人**）
用户说"获取/查找/得到 某人的邮箱地址" → **不是 `mailbox list`**，走三路并发查询流程（见「查找他人邮箱地址」章节）
用户说"找邮件/搜邮件/查邮件" → `message search`
用户说"看邮件/打开邮件/邮件内容" → 先 `message search` 获取 messageId，再 `message get`
用户说"发邮件/写邮件" → 先 `mailbox list` 获取发件地址，再 `message send`
用户说“给(某人名字)发邮件” / “查询某人发给我的邮件” / “查询发给某人的邮件” / 任何涉及按人名查找邮箱的场景 →
  **第一步**：并发同时发起以下三路查询，取最先返回有效邮箱的结果；若三路均无有效邮箱，ask_human 请用户提供，禁止臆测：
    1. `aisearch person --keyword <姓名>` → `contact user get --ids <userId>`，提取 `orgAuthEmail`
    2. `mail user search --email <当前邮箱> --keyword <姓名>`，提取 `users[].email`（仅企业邮箱可用）
    3. `contact user search --keyword <姓名>`，提取用户邮箱字段
  **第二步**：用获得的目标邮箱拼入 KQL（如 `from:<email>` 或 `to:<email>`）执行 `message search`，或用于 `message send`
用户说"发带附件的邮件/发邮件附件" → 先 `mailbox list` 获取发件地址，再 `message send --attachment <文件路径>`
用户说"给(某人名字)发邮件" → 先 `aisearch person` 获取 userId，再 `contact user get` 获取收件人邮箱，再 `message send`
用户说"查看附件/邮件附件/有什么附件" → 先 `message search` 获取 messageId，再 `attachment list`
用户说"下载附件/保存附件/把附件存到本地/把所有附件下载到..." → 先 `message search` 获取 messageId，再 `attachment list` 获取 attachmentId 和 name，最后逐个 `attachment download`（**不支持批量下载，不存在 download_batch/download_all 命令，必须逐个下载**）
用户说"把XX邮件的所有附件都下载" / "批量下载附件" / "下载4月所有发票邮件的附件" → **不存在批量下载命令**，必须按以下流程循环执行：1) `message search` 搜索匹配邮件获取 messageId 列表 → 2) 对每封邮件 `attachment list` 获取 attachmentId + name → 3) 对每个附件逐个调用 `attachment download`。不要编造 `download_batch` / `download_all` / `batch_download` 等不存在的命令
用户说"查看会话/获取会话/看这封邮件的会话" → 先 `message search` 或 `message get` 获取邮件中的 `conversationId`，再 `thread get`
用户说"搜索/查找/联系 邮箱用户/联系人/某人的邮箱地址" → `user search`（搜索通讯录人员，不是搜邮件内容）
用户说"发送草稿/把草稿发出去/发这封草稿" → 先 `message search --query "folderId:5"` 找到草稿 messageId，再 `draft send`
用户说"翻页继续搜索联系人/通讯录" → `user search --cursor <nextCursor>`（注意：不是 `message search`）

**`user search` vs `message search` 关键区别：**
- `user search`：搜索的是**人**（通讯录联系人），入参是 `--keyword 姓名`，返回用户信息
- `message search`：搜索的是**邮件**（邮件内容），入参是 `--query KQL表达式`，返回邮件列表


## 严格禁止 (NEVER DO)
- 明确禁止猜测、假设、推断发件人和收件人邮箱
- 无法获取邮箱时，强引导ask_human，由用户确认，不要通过假设或其他方式继续执行
- **严禁在用户未明确指定使用个人邮箱时，默认选择 `@dingtalk.com` 个人邮箱作为 `--email` / `--from`**；默认必须从 `mailbox list` 中挑选企业邮箱
- **涉及带附件的邮件操作时，严禁上传到钉钉媒体存储（media upload）**；必须使用对应命令的 `--attachment` / `--inline-attachment` 参数，由 CLI 内部完成附件处理
- **严禁编造不存在的批量下载命令**（如 `attachment download_batch`、`attachment download_all`、`attachment batch_download` 等）。下载附件只有 `attachment download` 一条命令，每次只能下载一个附件；需要批量下载时必须循环调用

## 核心工作流

```bash
# 1. 查看可用邮箱 — 提取邮箱地址
dws mail mailbox list --format json

# 2. 搜索邮件 — 提取 messageId
dws mail message search --email user@company.com \
  --query "subject:\"周报\" AND date>2025-06-01T00:00:00Z" --size 10 --format json

# 3. 查看邮件详情
dws mail message get --email user@company.com --id <messageId> --format json

# 4. 发送邮件（纯文本）
dws mail message send --from user@company.com --to colleague@company.com \
  --subject "周报" --body "本周完成…" --format json

# 4b. 发送带附件的邮件（自动编排：创建草稿→上传附件→发送草稿）
dws mail message send --from user@company.com --to colleague@company.com \
  --subject "周报" --body "见附件" --attachment ./report.pdf --format json

# 4c. 发送带内联图片的邮件（正文自动转 HTML，<img> 标签自动注入）
dws mail message send --from user@company.com --to colleague@company.com \
  --subject "图表周报" --body "本周图表如下：[inline:chart.png]" \
  --inline-attachment ./chart.png --format json

# 5. 下载邮件附件到本地（每次只能下载一个附件，不支持批量下载）
# 步骤 5.1：搜索匹配的邮件，获取 messageId 列表
# 示例：下载4月所有发票邮件的附件
dws mail message search --email user@company.com \
  --query "subject:发票 AND date>2025-04-01T00:00:00Z AND date<2025-05-01T00:00:00Z AND hasAttachments:true" --size 50 --format json

# 步骤 5.2：对每封邮件，列出附件获取 attachmentId 和 name
# （对搜索结果中的每封邮件都要执行一次）
dws mail attachment list --email user@company.com --id <messageId> --format json

# 步骤 5.3：对每个附件逐个下载（没有批量下载命令，必须循环调用）
dws mail attachment download --email user@company.com \
  --message-id <messageId> --attachment-id <attachmentId> --name report.pdf --output ~/invoices/

# 5. 获取邮件所属会话详情（thread）
# 步骤 5.1：先通过 message search 或 message get 获取邮件中的 conversationId
dws mail message search --email user@company.com \
  --query "subject:\"周报\"" --size 5 --format json
# 从返回的邮件列表中提取 conversationId 字段

# 步骤 5.2：用 conversationId 获取会话详情
dws mail thread get --email user@company.com --id <conversationId> --format json

# 步骤 5.3（可选）：同时返回会话内所有邮件列表
dws mail thread get --email user@company.com --id <conversationId> --select messages --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `mailbox list` | 邮箱地址 | message search/get/send/thread get 的 --email/--from |
| `message search` | `messageId` | message get 的 --id |
| `message search` | `conversationId` | thread get 的 --id |
| `message search` | `messageId` | attachment list 的 --id |
| `attachment list` | `attachments[].id` / `attachments[].name` | attachment download 的 --attachment-id / --name |
| `message get` | `conversationId` | thread get 的 --id |
| `aisearch person` → `contact user get` / `contact user search` / `mail user search` | 用户邮箱 (orgAuthEmail / email) | message send 的 --to/--cc（三路并发，取先到结果） |
| `user search` | 用户邮箱 (email) | message send 的 --to/--cc |

## 注意事项

- `mailbox list` 返回用户所有邮箱（含个人和企业），每条记录包含邮箱地址、账号类型、所属企业。**默认一律选择企业邮箱**（除非用户明确指定使用个人邮箱）；若有多个企业邮箱可选，优先匹配用户当前所在企业的那一个；仍无法判断时向用户确认后再操作。详见文档顶部「默认邮箱选择规则」章节
- `message search` 返回邮件 ID 和元信息（不含正文），需 `message get` 获取完整内容
- KQL 查询支持 AND/OR/NOT 组合，字段值含空格时需用双引号
- `--cc` 抄送人支持多人，逗号分隔
- 收件人邮箱获取：用户只知道同事名字时，**并发**同时执行以下三路查询，取最先返回有效邮箱的结果，无需等待其他路完成：
  1. `dws aisearch person --keyword "名字" --dimension name` → `dws contact user get --ids <userId>`，提取 `orgAuthEmail`
  2. `dws mail user search --email <发件人邮箱> --keyword "名字"`，提取 `users[].email`（仅企业邮箱账号可调用，个人 @dingtalk.com 邮箱会报权限错误可忽略）
  3. `dws contact user search --keyword "名字"`，提取用户邮箱字段
  若三路均无有效邮箱，必须 ask_human 请用户手动提供收件人邮箱，严禁臆测和假设
- `thread get` 无法直接通过邮箱地址查询会话列表，**必须先有 conversationId**；conversationId 来自 `message search` 或 `message get` 返回的邮件字段 `conversationId`
- `thread get` 默认不返回邮件列表，如需查看会话内所有邮件，需加 `--select messages`；如需同时返回多个可选字段，用英文逗号分隔，如 `--select messages,internetMessageId`
- `thread get` 返回的 `messages` 列表中，邮件正文（`body`）、收件人（`toRecipients`）等字段默认不包含，需在 `--select` 中额外指定
- `user search` 仅支持企业邮箱（非 `@dingtalk.com` 个人邮箱），使用个人邮箱将因无权限报错；搜到的用户邮箱（`email` 字段）可直接用于 `message send` 的 `--to`/`--cc` 参数
