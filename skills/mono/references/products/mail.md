# 邮箱 (mail) 命令参考

## 命令速查目录

| 命令 | 功能简述 |
|------|----------|
| `dws mail mailbox list` | 查询**当前用户自己**的可用邮箱列表 |
| `dws mail message list` | 列出指定文件夹中的邮件（默认收件箱） |
| `dws mail message search` | 搜索邮件（KQL 语法，按主题/发件人/日期等） |
| `dws mail message get` | 查看邮件完整内容（含正文） |
| `dws mail message send` | 发送邮件（支持附件/内联图片） |
| `dws mail message reply` | 回复邮件（支持附件/内联图片） |
| `dws mail message reply-all` | 回复所有人（支持附件/内联图片） |
| `dws mail message forward` | 转发邮件（支持附件/内联图片） |
| `dws mail message batch-move` | 批量移动邮件到指定文件夹 |
| `dws mail message batch-delete` | 批量删除邮件 |
| `dws mail message batch-update` | 批量修改邮件状态（标记已读/未读/添加标签/移除标签） |
| `dws mail message verify` | 根据 internetMessageId 查询邮件发送状态 |
| `dws mail draft create` | 创建草稿（保留在草稿箱，不发送） |
| `dws mail draft update` | 更新草稿内容（保留在草稿箱，不发送） |
| `dws mail draft send` | 发送草稿箱中已有的草稿 |
| `dws mail folder list` | 列举邮件文件夹 |
| `dws mail folder create` | 创建邮件文件夹 |
| `dws mail folder delete` | 删除邮件文件夹 |
| `dws mail folder update` | 更新邮件文件夹名称 |
| `dws mail attachment list` | 列举指定邮件的所有附件 |
| `dws mail attachment download` | 下载邮件附件到本地（**仅支持逐个下载，不支持批量下载**） |
| `dws mail tag list` | 列举邮件标签 |
| `dws mail tag create` | 创建邮件标签 |
| `dws mail tag delete` | 删除邮件标签 |
| `dws mail tag update` | 更新邮件标签名称 |
| `dws mail thread list` | 列出指定邮箱、指定文件夹下的邮件会话 |
| `dws mail thread get` | 获取会话详情 |
| `dws mail thread update` | 修改单个邮件会话的状态或标签（标记已读/未读/添加标签/移除标签） |
| `dws mail thread batch-update` | 批量修改邮件会话的状态或标签（单次最多 100 个） |
| `dws mail thread trash` | 将单个邮件会话移动到已删除文件夹（不会永久删除） |
| `dws mail thread batch-trash` | 将多个邮件会话批量移动到已删除文件夹（单次最多 100 个，不会永久删除） |
| `dws mail user search` | 搜索通讯录用户（**按姓名查他人邮箱**，不是搜邮件） |
| `dws mail template create` | 创建邮件模板 |
| `dws mail template list` | 列举邮件模板 |
| `dws mail template get` | 获取邮件模板详情 |
| `dws mail template update` | 更新邮件模板 |
| `dws mail template delete` | 删除邮件模板 |
| `dws mail contact create` | 创建个人邮件联系人（添加到自己的联系人列表） |
| `dws mail contact list` | 列举个人邮件联系人（查看自己保存的联系人，**不是搜索通讯录用户**） |
| `dws mail contact update` | 更新个人邮件联系人信息 |
| `dws mail contact batch-delete` | 批量删除个人邮件联系人 |
| `dws mail auto-reply get` | 获取用户的自动回复配置 |
| `dws mail rule list` | 列出个人收信规则 |
| `dws mail rule create` | 创建个人收信规则 |
| `dws mail rule update` | 更新个人收信规则 |
| `dws mail rule delete` | 删除个人收信规则 |
| `dws mail rule adjust` | 调整收信规则排序 |

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
      --folder-id string  文件夹 ID（1=已发送, 2=收件箱, 3=垃圾邮件, 5=草稿, 6=已删除），默认为收件箱
      --limit string      每页返回数量(最大限制 100, 默认 20)
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

### 创建邮件文件夹
```
Usage:
  dws mail folder create [flags]
Example:
  dws mail folder create --email user@company.com --name "项目资料"
  dws mail folder create --email user@company.com --name "子文件夹" --folder <folderId>
Flags:
      --email string    邮件所属邮箱地址 (必填)
      --name string     新建邮件文件夹名称 (必填)
      --folder string   父文件夹 ID，不传则创建顶层文件夹 (可选)
```

不传 `--folder` 创建顶层文件夹；传入 `--folder` 时创建指定父文件夹下的子文件夹。

> **重要：** `--folder` 必须填写父文件夹 ID，不是文件夹名称。父文件夹 ID 来自 `dws mail folder list --email <邮箱>` 返回的 `folders[].id`。

**返回 JSON：**

```json
{
  "success": true,
  "result": {
    "folder": {
      "id": "104",
      "displayName": "项目资料",
      "parentFolderId": "0",
      "childFolderCount": 0,
      "totalItemCount": 0,
      "unreadItemCount": 0,
      "extensions": {}
    }
  }
}
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | `boolean` | 是否创建成功 |
| `result.folder.id` | `string` | 文件夹唯一标识 |
| `result.folder.displayName` | `string` | 文件夹显示名称 |
| `result.folder.parentFolderId` | `string` | 父文件夹 ID |
| `result.folder.childFolderCount` | `number` | 子文件夹数量 |
| `result.folder.totalItemCount` | `number` | 邮件总数 |
| `result.folder.unreadItemCount` | `number` | 未读邮件数量 |
| `result.folder.extensions` | `object` | 文件夹扩展信息 |

### 删除邮件文件夹
```
Usage:
  dws mail folder delete [flags]
Example:
  dws mail folder delete --email user@company.com --id <folderId>
Flags:
      --email string   邮件所属邮箱地址 (必填)
      --id string      要删除的邮件文件夹 ID (必填)
```

`--id` 必须填写要删除的文件夹 ID，不是文件夹名称。文件夹 ID 来自 `dws mail folder list --email <邮箱>` 返回的 `folders[].id`，或来自 `folder create` 返回的 `result.folder.id`。

**返回 JSON：**

```json
{
  "success": true,
  "result": {}
}
```

### 更新邮件文件夹
```
Usage:
  dws mail folder update [flags]
Example:
  dws mail folder update --email user@company.com --id <folderId> --name "新文件夹名"
Flags:
      --email string   邮件所属邮箱地址 (必填)
      --id string      要更新的邮件文件夹 ID (必填)
      --name string    更新后的邮件文件夹名称 (必填)
```

`--id` 必须填写要更新的文件夹 ID，不是文件夹名称；`--name` 是更新后的文件夹名称。若用户只给出原文件夹名称，必须先调用 `folder list` 找到对应 `folders[].id`，再执行 update。

**返回 JSON：**

```json
{
  "success": true,
  "result": {}
}
```

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

### 创建邮件标签
```
Usage:
  dws mail tag create [flags]
Example:
  dws mail tag create --email user@company.com --name "项目资料"
  dws mail tag create --email user@company.com --name "子标签" --parent-id <tagId>
Flags:
      --email string       用户的邮箱地址 (必填)
      --name string        新建邮件标签名称 (必填)
      --parent-id string   父标签 ID，不传则创建顶层标签 (可选)
```

不传 `--parent-id` 创建顶层标签；传入 `--parent-id` 时创建指定父标签下的子标签。

> **重要：** `--parent-id` 必须填写父标签 ID，不是标签名称。父标签 ID 来自 `dws mail tag list --email <邮箱>` 返回的 `tags[].id`。

**返回 JSON：**

```json
{
  "success": true,
  "result": {
    "tag": {
      "id": "tag-001",
      "name": "项目资料",
      "parentId": "0",
      "totalItemCount": 0,
      "unreadItemCount": 0,
      "extensions": {}
    }
  }
}
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | `boolean` | 是否创建成功 |
| `result.tag.id` | `string` | 标签唯一标识 |
| `result.tag.name` | `string` | 标签显示名称 |
| `result.tag.parentId` | `string` | 父标签 ID |
| `result.tag.totalItemCount` | `number` | 标签下邮件总数 |
| `result.tag.unreadItemCount` | `number` | 标签下未读邮件数量 |
| `result.tag.extensions` | `object` | 标签扩展信息 |

### 删除邮件标签
```
Usage:
  dws mail tag delete [flags]
Example:
  dws mail tag delete --email user@company.com --id <tagId>
Flags:
      --email string   用户的邮箱地址 (必填)
      --id string      要删除的邮件标签 ID (必填)
```

`--id` 必须填写要删除的标签 ID，不是标签名称。标签 ID 来自 `dws mail tag list --email <邮箱>` 返回的 `tags[].id`，或来自 `tag create` 返回的 `result.tag.id`。

只能删除用户自定义标签，系统标签不能删除。

**返回 JSON：**

```json
{
  "success": true,
  "result": {}
}
```

### 更新邮件标签
```
Usage:
  dws mail tag update [flags]
Example:
  dws mail tag update --email user@company.com --id <tagId> --name "新标签名"
Flags:
      --email string   用户的邮箱地址 (必填)
      --id string      要更新的邮件标签 ID (必填)
      --name string    更新后的邮件标签名称 (必填)
```

`--id` 必须填写要更新的标签 ID，不是标签名称；`--name` 是更新后的标签名称。若用户只给出原标签名称，必须先调用 `tag list` 找到对应 `tags[].id`，再执行 update。

只能更新用户自定义标签，系统标签不能更新。

**返回 JSON：**

```json
{
  "success": true,
  "result": {}
}
```

### 列出邮件会话

```
Usage:
  dws mail thread list [flags]
Example:
  dws mail thread list --email user@company.com --folder <folderId> --limit 10
  dws mail thread list --email user@company.com --folder 104 --limit 20 --cursor <nextCursor>
Flags:
      --email string       会话所属邮箱地址 (必填)
      --folder string      邮件文件夹 ID，不是文件夹名称 (必填)
      --limit int          本次列出的会话数，最大 100 (必填)
      --cursor string      分页游标，首次请求可不传 (可选)
      --start string       开始 UTC 时间字符串，如 2024-01-01T00:00:00Z (可选)
      --end string         结束 UTC 时间字符串，如 2024-12-31T23:59:59Z (可选)
      --ascending          是否按时间升序；不传由服务端默认排序 (可选)
```

`--folder` 必须填写文件夹 ID，不是文件夹名称。若用户只给出“收件箱/已删除/某个自定义文件夹”这类名称，必须先调用 `folder list` 找到对应 `folders[].id`，再执行 `thread list`。

**返回 JSON：**

```json
{
  "success": true,
  "result": {
    "conversations": [
      {
        "id": "conversationId",
        "subject": "会话主题",
        "summary": "会话摘要",
        "lastModifiedDateTime": "2024-02-06T01:05:07Z",
        "messageCount": 1,
        "tags": [],
        "senders": [
          {
            "email": "sender@example.com",
            "name": "发件人"
          }
        ],
        "isRead": true,
        "priority": "PRY_NORMAL",
        "flag": "FLAG_NONE",
        "hasAttachments": false
      }
    ],
    "nextCursor": "",
    "hasMore": false
  }
}
```

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

### 修改邮件会话状态

```
Usage:
  dws mail thread update [flags]
Example:
  dws mail thread update --email user@company.com --id <conversationId> --action markRead
  dws mail thread update --email user@company.com --id <conversationId> --action addTags --tag-ids 1,2
Flags:
      --email string     会话所属邮箱地址 (必填)
      --id string        会话唯一标识 conversationId (必填)
      --action string    操作类型：markRead、markUnread、addTags、removeTags (必填)
      --tag-ids string   标签 ID 列表，多个用英文逗号分隔；addTags/removeTags 时必填 (可选)
```

`--id` 必须填写会话 ID，不是邮件 ID。会话 ID 可通过 `thread list` 的 `conversations[].id` 获取。

支持的 `--action`：

| action | 说明 | 是否需要 `--tag-ids` |
|--------|------|----------------------|
| `markRead` | 标记会话为已读 | 否 |
| `markUnread` | 标记会话为未读 | 否 |
| `addTags` | 给会话增加标签 | 是 |
| `removeTags` | 从会话移除标签 | 是 |

成功时返回：

```json
{
  "success": true,
  "result": {}
}
```

### 批量修改邮件会话状态

```
Usage:
  dws mail thread batch-update [flags]
Example:
  dws mail thread batch-update --email user@company.com --ids <conversationId1>,<conversationId2> --action markUnread
  dws mail thread batch-update --email user@company.com --ids <conversationId1>,<conversationId2> --action removeTags --tag-ids 11
Flags:
      --email string     会话所属邮箱地址 (必填)
      --ids string       会话 ID 列表，多个用英文逗号分隔，最多 100 个 (必填)
      --action string    操作类型：markRead、markUnread、addTags、removeTags (必填)
      --tag-ids string   标签 ID 列表，多个用英文逗号分隔；addTags/removeTags 时必填 (可选)
```

`--ids` 必须填写会话 ID 列表，不是邮件 ID 列表，最多 100 个。

成功时返回：

```json
{
  "success": true,
  "result": {}
}
```

### [危险] 删除邮件会话

```
Usage:
  dws mail thread trash [flags]
Example:
  dws mail thread trash --email user@company.com --id <conversationId> --yes
Flags:
      --email string   会话所属邮箱地址 (必填)
      --id string      要删除的会话 ID (必填)
      --yes            确认执行此危险操作 (必填)
```

> ⚠️ **危险操作**：此命令会将邮件会话移动到已删除文件夹。建议先通过 `thread get` 确认目标会话后再执行。

将指定邮件会话移动到已删除文件夹，不会永久删除邮件。`--id` 必须填写会话 ID，不是邮件 ID。执行前需向用户确认，确认后传入 `--yes` 执行。

成功时返回：

```json
{
  "success": true,
  "result": {}
}
```

### [危险] 批量删除邮件会话

```
Usage:
  dws mail thread batch-trash [flags]
Example:
  dws mail thread batch-trash --email user@company.com --ids <conversationId1>,<conversationId2> --yes
Flags:
      --email string   会话所属邮箱地址 (必填)
      --ids string     要删除的会话 ID 列表，多个用英文逗号分隔，最多 100 个 (必填)
      --yes            确认执行此危险操作 (必填)
```

> ⚠️ **危险操作**：此命令会批量将邮件会话移动到已删除文件夹。建议先通过 `thread list` 确认目标会话后再执行。

将指定邮件会话批量移动到已删除文件夹，单次最多 100 个会话。不会永久删除邮件。`--ids` 必须填写会话 ID 列表，不是邮件 ID 列表。执行前需向用户确认，确认后传入 `--yes` 执行。

成功时返回：

```json
{
  "success": true,
  "result": {}
}
```

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

### 批量修改邮件状态

批量修改邮件的已读状态或标签，通过 `--action` 指定操作类型。

**支持的操作类型（--action）：**

| action | 说明 | 需要的额外参数 |
|--------|------|---------------|
| `markRead` | 标记邮件为已读 | 无 |
| `markUnread` | 标记邮件为未读 | 无 |
| `addTags` | 给邮件增加标签 | `--tags`（标签 ID 列表，必填） |
| `removeTags` | 从邮件移除标签 | `--tags`（标签 ID 列表，必填） |

**常用标签 ID：**

| 标签 ID | 名称 | 图标 |
|---------|------|------|
| `1` | 跟进事项 | 小红旗 |
| `2` | 完成事项 | 绿色小勾 |
| `11` | 重要 | 星标 |

```
Usage:
  dws mail message batch-update [flags]
Example:
  dws mail message batch-update --email user@company.com --ids <id1>,<id2> --action markRead
  dws mail message batch-update --email user@company.com --ids <id1>,<id2> --action addTags --tags 1,2
  dws mail message batch-update --email user@company.com --ids <id1>,<id2> --action removeTags --tags 11
Flags:
      --email string   邮件所属邮箱地址 (必填)
      --ids string     要修改的邮件 ID 列表，逗号分隔 (必填)
      --action string  操作类型: markRead/markUnread/addTags/removeTags (必填)
      --tags string    标签 ID 列表，逗号分隔 (action 为 addTags/removeTags 时必填)
```

> **查询标签 ID：** 使用 `dws mail tag list --email <邮箱>` 可查看所有可用标签及其 ID。

### 查询邮件发送状态
```
Usage:
  dws mail message verify [flags]
Example:
  dws mail message verify --email user@company.com --internet-message-id <internetMessageId>
Flags:
      --email string                邮件所属邮箱地址 (必填)
      --internet-message-id string  邮件的 internetMessageId (必填)，取自发送类命令返回值
```

根据 `internetMessageId` 查询某封邮件当前的发送投递状态。

> **internetMessageId 来源：** `message send` / `draft send` / `message reply` / `message reply-all` / `message forward` 等发送类命令的返回值中均会带 `internetMessageId` 字段，可直接传入此命令查询发送结果。

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `message` | `object` | 邮件完整信息 |
| `sendStatus` | `string` | 发送状态，取值见下表 |

**`sendStatus` 取值说明：**

| 值 | 含义 |
|------|------|
| `none` | 未发送 |
| `posting` | 投递中 |
| `partial_success` | 部分成功（部分收件人投递成功） |
| `success` | 发送成功 |
| `failed` | 发送失败 |
| `unknown` | 未知状态 |

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

### 创建邮件模板
```
Usage:
  dws mail template create [flags]
Example:
  dws mail template create --email user@company.com --name "周报模板" --subject "周报" --content "本周工作总结..."
  dws mail template create --email user@company.com --from user@company.com --name "通知模板" --subject "通知" --content "..." --to a@x.com,b@x.com --cc c@x.com
Flags:
      --email string      用户邮箱地址 (必填)
      --from string       模板发件人邮箱 (可选)
      --subject string    模板邮件标题 (必填)
      --content string    模板邮件正文 (必填)
      --name string       模板名称 (必填)
      --to string         模板收件人列表，逗号分隔 (可选)
      --cc string         模板抄送人列表，逗号分隔 (可选)
      --is-draft          是否为草稿模板 (可选，默认 false)
```

> **草稿模板说明：** 传入 `--is-draft` 创建的模板为草稿模板，草稿模板支持后续通过 `template update` 修改内容。**非草稿模板创建后不可修改**（`template update` 仅对草稿模板有效）。

### 列举邮件模板
```
Usage:
  dws mail template list [flags]
Example:
  dws mail template list --email user@company.com --limit 20
  dws mail template list --email user@company.com --limit 20 --cursor <nextCursor>
Flags:
      --email string    用户邮箱地址 (必填)
      --cursor string   分页游标，取自响应中的 nextCursor 字段 (可选)
      --limit string    每页返回数量 (必填)
```

### 获取邮件模板详情
```
Usage:
  dws mail template get [flags]
Example:
  dws mail template get --email user@company.com --id <templateId>
Flags:
      --email string   用户邮箱地址 (必填)
      --id string      模板唯一标识 (必填)
```

### 更新邮件模板

> ⚠️ 服务端限制：仅草稿模板（创建时带 `--is-draft`）可更新；非草稿模板更新会返回 1001 Invalid parameter。需要修改非草稿模板时，建议删除后用 `--is-draft` 重建。

> **重要限制：** `template update` **仅对草稿模板有效**。只有通过 `template create --is-draft` 创建的草稿模板才支持更新，非草稿模板调用 update 会返回 `Invalid parameter` 错误。

```
Usage:
  dws mail template update [flags]
Example:
  dws mail template update --email user@company.com --id <templateId> --subject "新标题" --content "新正文"
  dws mail template update --email user@company.com --id <templateId> --name "新模板名"
Flags:
      --email string      用户邮箱地址 (必填)
      --id string         模板唯一标识 (必填，必须是草稿模板的 ID)
      --from string       模板发件人邮箱 (可选)
      --subject string    模板邮件标题 (可选)
      --content string    模板邮件正文 (可选)
      --name string       模板名称 (可选)
      --to string         模板收件人列表，逗号分隔 (可选)
      --cc string         模板抄送人列表，逗号分隔 (可选)
```

### 删除邮件模板
```
Usage:
  dws mail template delete [flags]
Example:
  dws mail template delete --email user@company.com --id <templateId>
Flags:
      --email string   用户邮箱地址 (必填)
      --id string      模板唯一标识 (必填)
```

> **特殊字符注意：** `--contact-id` 等 ID 参数的值可能包含 `$`、`!` 等 shell 特殊字符。在终端手动执行时，**必须用单引号**包裹这类参数值（如 `--contact-id '101_0:DzzzzyJqO10$---.hp5uBuR'`），双引号会导致 `$` 被 shell 变量展开，使 ID 值被篡改从而报错。通过 MCP 协议（JSON 传参）调用时无此问题。

### 创建邮件联系人
```
Usage:
  dws mail contact create [flags]
Example:
  dws mail contact create --email user@company.com --contact-email colleague@company.com --display-name "张三"
  dws mail contact create --email user@company.com --contact-email colleague@company.com --first-name "三" --last-name "张"
Flags:
      --email string          用户邮箱地址 (必填)
      --contact-email string  联系人邮箱地址 (必填)
      --first-name string     联系人名 (可选)
      --middle-name string    联系人中间名 (可选)
      --last-name string      联系人姓 (可选)
      --display-name string   联系人显示名称 (可选)
```

### 列举邮件联系人
```
Usage:
  dws mail contact list [flags]
Example:
  dws mail contact list --email user@company.com --limit 20
  dws mail contact list --email user@company.com --limit 20 --cursor <nextCursor>
Flags:
      --email string    用户邮箱地址 (必填)
      --cursor string   分页游标，取自响应中的 nextCursor 字段 (可选)
      --limit string    每页返回数量 (必填)
```

> **重要区别：** `contact list` 列举的是**个人联系人**（用户自己保存/创建的联系人列表），不需要关键词；搜索**企业通讯录用户**（按姓名找人查邮箱）请用 `user search`。

### 更新邮件联系人
```
Usage:
  dws mail contact update [flags]
Example:
  dws mail contact update --email user@company.com --contact-id <contactId> --display-name "李四"
  dws mail contact update --email user@company.com --contact-id <contactId> --contact-email new@company.com --first-name "四" --last-name "李"
Flags:
      --email string          用户邮箱地址 (必填)
      --contact-id string     联系人唯一标识 (必填)
      --contact-email string  联系人邮箱地址 (可选)
      --first-name string     联系人名 (可选)
      --middle-name string    联系人中间名 (可选)
      --last-name string      联系人姓 (可选)
      --display-name string   联系人显示名称 (可选)
```

### 批量删除邮件联系人

> **特殊字符注意：** `--contact-ids` 的值可能包含 `$`、`!` 等 shell 特殊字符。在终端手动执行时，**必须用单引号**包裹这类参数值（如 `--contact-ids '101_0:DzzzzyJqO10$---.hp5uBuR'`），双引号会导致 `$` 被 shell 变量展开，使 ID 值被篡改从而报错。通过 MCP 协议（JSON 传参）调用时无此问题。

```
Usage:
  dws mail contact batch-delete [flags]
Example:
  dws mail contact batch-delete --email user@company.com --contact-ids <id1>,<id2>
Flags:
      --email string         用户邮箱地址 (必填)
      --contact-ids string   要删除的联系人 ID 列表，逗号分隔 (必填)
```

### 获取自动回复配置

获取当前用户的邮件自动回复配置，包括是否启用、生效时间、回复范围和回复内容。

```
Usage:
  dws mail auto-reply get [flags]
Example:
  dws mail auto-reply get --email user@company.com
Flags:
      --email string   用户的邮箱地址 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `enabled` | bool | 是否启用自动回复 (true=启用, false=禁用) |
| `startTime` | string | 自动回复开始时间 |
| `endTime` | string | 自动回复结束时间 |
| `scope` | string | 回复范围: "contact"(仅联系人) 或 "all"(所有人) |
| `content` | string | 自动回复内容 |

### 收信规则管理

#### 列出收信规则

列出当前用户的所有收信规则，包括规则名称、启用状态、条件、动作和排序。

```
Usage:
  dws mail rule list [flags]
Example:
  dws mail rule list --email user@company.com
Flags:
      --email string   用户的邮箱地址 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `total` | int | 规则总数 |
| `rules` | List[] | 规则列表 |
| `rules[].id` | string | 规则 ID |
| `rules[].name` | string | 规则名称 |
| `rules[].enabled` | bool | 是否启用 |
| `rules[].conditions` | List[] | 规则条件列表 |
| `rules[].actions` | List[] | 规则动作列表 |
| `rules[].order` | int | 规则排序 |

#### 创建收信规则

创建一条新的收信规则。支持设置规则名称、启用状态、匹配条件和执行动作。

> **`--conditions` 和 `--actions`** 为 JSON 数组字符串。

**界面与参数对应关系：**

| 界面元素 | CLI 参数 / JSON 字段 | 说明 |
|----------|---------------------|------|
| 规则名称 | `--name` | 必填，规则的显示名称 |
| 如果满足以下「全部」条件 | `--conditions` | 多个条件之间为 **AND** 关系（即所有条件都满足才触发） |
| ├ 对象下拉（发件人） | `object: "from"` | 条件匹配的对象，可选值见下方 |
| ├ 操作下拉（包含） | `operation: "include"` | 匹配方式，可选值见下方 |
| └ 关键词输入框 | `keyword: "a@test.com"` | 匹配的具体值 |
| 执行以下操作 | `--actions` | 条件满足后执行的动作列表 |
| ├ 动作下拉（移动到文件夹） | `action: "ActSavetoFolder"` | 动作类型，可选值见下方 |
| └ 参数选择（收件箱） | `parameters: ["2"]` | 动作的参数，如目标文件夹 ID |

**条件逻辑说明：**

- `--conditions` 数组中的多个条件之间为 **AND（且）** 关系，即所有条件都满足才触发规则
- 同一个条件对象（如 `from`）内部的 `or` 数组中多个表达式之间为 **OR（或）** 关系
- 同一个 `and` 数组中的多个子条件之间为 **AND（且）** 关系

**条件对象 (object) 与合法操作类型 (operation) 组合：**

| object | 合法 operation | 说明 |
|--------|---------------|------|
| `from` | `include`(包含), `exclude`(不包含), `oneof`(是联系人之一), `noneof`(不是联系人之一) | 匹配发件人地址或名称 |
| `to` | `include`(包含), `exclude`(不包含), `oneof`(是联系人之一), `noneof`(不是联系人之一) | 匹配收件人地址或名称 |
| `subject` | `include`(包含), `exclude`(不包含) | 匹配邮件主题 |
| `attachment` | `exist`(是否存在附件) | keyword="1" 表示有附件，keyword="0" 表示无附件 |
| `x-aliyun-size` | `greater`(大于), `less`(小于) | 邮件大小，单位为 **字节(Bytes)**（1KB=1024, 1MB=1048576）；可组合使用表示范围区间 |

**操作类型 (operation) 详细说明：**

| 值 | 界面显示 | 适用 object | 说明 |
|----|---------|------------|------|
| `include` | 包含 | from, to, subject | 字段包含关键词 |
| `exclude` | 不包含 | from, to, subject | 字段不包含关键词 |
| `oneof` | 是联系人之一 | from, to | 字段值在给定联系人列表中 |
| `noneof` | 不是联系人之一 | from, to | 字段值不在给定联系人列表中 |
| `greater` | 大于 | x-aliyun-size | 数值大于阈值，单位字节(Bytes) |
| `less` | 小于 | x-aliyun-size | 数值小于阈值，单位字节(Bytes) |
| `exist` | 存在 | attachment | keyword="1" 表示有附件，keyword="0" 表示无附件 |

**动作类型 (action) 可选值：**

| 值 | 界面显示 | parameters 说明 | 前置依赖 |
|----|---------|----------------|----------|
| `ActSavetoFolder` | 移动到文件夹 | 目标文件夹 ID，如 `["2"]`（2=收件箱） | 需先通过 `dws mail folder list` 获取文件夹 ID |
| `ActFlagMail` | 标记标签 | 标签 ID 列表，逗号分隔，如 `["102,11,1"]` | 需先通过 `dws mail tag list` 获取标签 ID |
| `ActFlagMail2` | 标记已读 | `"asread"`(标记已读)，服务端仅支持标记已读，不支持标记未读 | 无 |
| `ActReply` | 自动回复 | 回复内容文本，如 `["感谢您的来信"]` | 无 |

**条件 JSON 结构说明：**

每个条件由 `object`（匹配对象）和 `or`（OR 表达式列表）组成，`or` 内嵌 `and`（AND 条件列表）。

| 字段 | 说明 |
|------|------|
| `object` | 条件对象，取值及合法 operation 见上方组合表 |
| `or` | OR 表达式列表，同一 object 下多个 or 项之间为 **OR** 关系 |
| `and` | AND 条件列表，同一 or 项内多个 and 子条件之间为 **AND** 关系 |
| `operation` | 操作类型，必须与 object 合法组合（见上方组合表） |
| `keyword` | 关键词/阈值；attachment+exist 时 "1"=有附件/"0"=无附件；x-aliyun-size 时单位为字节(Bytes)，如 1KB=1024, 1MB=1048576 |
| `ignoreCase` | 是否忽略大小写（布尔值，仅 from/to/subject + include/exclude 时需要） |

**完整 conditions JSON 示例：**

```json
[
  {"object":"from","or":[
    {"and":[{"operation":"oneof","keyword":"a@test.com","ignoreCase":true}]},
    {"and":[{"operation":"oneof","keyword":"b@test.com","ignoreCase":true}]}
  ]},
  {"object":"subject","or":[{"and":[{"operation":"include","keyword":"报告","ignoreCase":true}]}]},
  {"object":"attachment","or":[{"and":[{"operation":"exist","keyword":"1"}]}]},
  {"object":"x-aliyun-size","or":[{"and":[{"operation":"greater","keyword":"1024"},{"operation":"less","keyword":"10240"}]}]}
]
```

> 上例表示：发件人是 a@test.com **或** b@test.com **且** 主题包含"报告" **且** 有附件 **且** 大小在 1KB(1024字节)~10KB(10240字节) 之间。
>
> **同一 object 下匹配多个值的 OR 写法：** 在 `or` 数组中放多个 `and` 项（每个 `and` 对应一个匹配值），而非在一个 `and` 中放多个条件。例如上方 `from` 条件中，两个邮箱地址分别作为独立的 `and` 项放在 `or` 数组中，表示"满足任一即可"。

**完整 actions JSON 示例：**

```json
[
  {"action":"ActSavetoFolder","parameters":["2"]},
  {"action":"ActFlagMail","parameters":["102,11,1"]},
  {"action":"ActFlagMail2","parameters":["asread"]},
  {"action":"ActReply","parameters":["感谢您的来信，我将尽快回复"]}
]
```

> **注意：** 使用 `ActSavetoFolder` 前需先通过 `dws mail folder list` 获取文件夹 ID；使用 `ActFlagMail` 前需先通过 `dws mail tag list` 获取标签 ID。

```
Usage:
  dws mail rule create [flags]
Example:
  dws mail rule create --email user@company.com --name "VIP邮件标记" --enabled true \
    --conditions '[{"object":"from","or":[{"and":[{"operation":"include","keyword":"vip@company.com","ignoreCase":true}]}]}]' \
    --actions '[{"action":"ActFlagMail2","parameters":["asread"]}]'
  dws mail rule create --email user@company.com --name "大附件归档" --enabled true \
    --conditions '[{"object":"x-aliyun-size","or":[{"and":[{"operation":"greater","keyword":"10485760"}]}]}]' \
    --actions '[{"action":"ActSavetoFolder","parameters":["6"]}]'
Flags:
      --email string       用户的邮箱地址 (必填)
      --name string        规则名称 (必填)
      --enabled string     是否启用: true/false (必填)
      --conditions string  规则条件 JSON 数组 (可选)
      --actions string     规则动作 JSON 数组 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | bool | 创建是否成功 |
| `errorCode` | string | 错误码 |
| `errorMsg` | string | 错误消息 |
| `id` | string | 新建规则 ID |

#### 更新收信规则

更新已有的收信规则。**除 `--conditions` 外所有参数均为必填**。

> **建议工作流：** 先通过 `dws mail rule list` 获取当前规则的完整配置，再传入需要修改的字段值。
>
> `--conditions` 为空或不传表示命中所有邮件（无条件匹配）。`--actions` 格式同 create 命令。

```
Usage:
  dws mail rule update [flags]
Example:
  dws mail rule update --email user@company.com --id <ruleId> --name "新规则名" --enabled true \
    --actions '[{"action":"ActSavetoFolder","parameters":["6"]}]'
  dws mail rule update --email user@company.com --id <ruleId> --name "全量归档" --enabled false \
    --conditions '[{"object":"subject","or":[{"and":[{"operation":"include","keyword":"报告","ignoreCase":true}]}]}]' \
    --actions '[{"action":"ActSavetoFolder","parameters":["6"]}]'
Flags:
      --email string       用户的邮箱地址 (必填)
      --id string          规则 ID (必填)
      --name string        规则名称 (必填)
      --enabled string     是否启用: true/false (必填)
      --conditions string  规则条件 JSON 数组 (可选，为空表示命中所有邮件)
      --actions string     规则动作 JSON 数组 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | bool | 更新是否成功 |
| `errorCode` | string | 错误码 |
| `errorMsg` | string | 错误信息 |
| `result` | object | 更新结果 |

#### 删除收信规则

删除指定的收信规则。

```
Usage:
  dws mail rule delete [flags]
Example:
  dws mail rule delete --email user@company.com --id <ruleId>
Flags:
      --email string   用户的邮箱地址 (必填)
      --id string      规则 ID (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | bool | 删除是否成功 |
| `errorCode` | string | 错误码 |
| `errorMsg` | string | 错误信息 |
| `result` | object | 删除结果 |

#### 调整收信规则排序

调整指定收信规则的排序位置，向上(up)或向下(down)移动。

```
Usage:
  dws mail rule adjust [flags]
Example:
  dws mail rule adjust --email user@company.com --id <ruleId> --direction up
  dws mail rule adjust --email user@company.com --id <ruleId> --direction down
Flags:
      --email string      用户的邮箱地址 (必填)
      --id string         规则 ID (必填)
      --direction string  调整方向: up/down (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | bool | 调整是否成功 |
| `errorCode` | string | 错误码 |
| `errorMsg` | string | 错误消息 |
| `result` | object | 调整结果 |

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
用户说"创建邮件文件夹/新建邮箱文件夹/新建邮件目录" → `folder create`
用户说"在某个邮件文件夹下创建子文件夹" → 先 `folder list` 找到父文件夹 ID，再 `folder create --folder <folderId>`；禁止把父文件夹名称直接填给 `--folder`
用户说"删除邮件文件夹/删除邮箱文件夹/删除邮件目录" → 先确认要删除的文件夹 ID；如果用户只给名称，先 `folder list` 找到 `folders[].id`，再 `folder delete --id <folderId>`
用户说"重命名邮件文件夹/修改邮箱文件夹名称/更新邮件目录名称" → 先确认要更新的文件夹 ID；如果用户只给原名称，先 `folder list` 找到 `folders[].id`，再 `folder update --id <folderId> --name <新名称>`
用户说"查看邮件标签/列出邮箱标签/查看邮箱 label" → `tag list`
用户说"创建邮件标签/新建邮箱标签/新增 label" → `tag create`
用户说"在某个邮件标签下创建子标签" → 先 `tag list` 找到父标签 ID，再 `tag create --parent-id <tagId>`；禁止把父标签名称直接填给 `--parent-id`
用户说"删除邮件标签/删除邮箱标签/删除 label" → 先确认要删除的标签 ID；如果用户只给名称，先 `tag list` 找到 `tags[].id`，再 `tag delete --id <tagId>`
用户说"重命名邮件标签/修改邮箱标签名称/更新 label 名称" → 先确认要更新的标签 ID；如果用户只给原名称，先 `tag list` 找到 `tags[].id`，再 `tag update --id <tagId> --name <新名称>`
用户说"列出邮件会话/查看会话列表/查看某个文件夹里的邮件会话" → 先确认邮箱地址和文件夹 ID；如果只有文件夹名称，先 `folder list` 找到 `folders[].id`，再 `thread list --folder <folderId>`
用户说"标记会话已读/未读/给会话加标签/移除会话标签" → 用 `thread update`；如果是多条会话，用 `thread batch-update`；标签操作必须先有标签 ID
用户说"删除会话/把会话放入已删除/批量删除会话" → 单条用 `thread trash`，多条用 `thread batch-trash`；传入的是会话 ID，不是邮件 ID
用户说"批量标记邮件已读/未读/给邮件加标签/移除邮件标签" → `message batch-update --action markRead/markUnread/addTags/removeTags`（操作对象是邮件 ID；会话级操作用 `thread update/batch-update`）
用户说"邮件发出去了吗/查邮件发送状态/确认邮件是否发送成功/邮件投递结果" → 用发送类命令返回的 `internetMessageId`，调用 `message verify` 查询 `sendStatus`
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
- **严禁把文件夹名称当作 `folder create --folder` 的值**；`--folder` 只能填父文件夹 ID，父文件夹 ID 必须来自 `folder list` 的 `folders[].id`
- **严禁把文件夹名称当作 `folder delete/update --id` 的值**；`--id` 只能填要操作的文件夹 ID，文件夹 ID 必须来自 `folder list` 的 `folders[].id` 或 `folder create` 的 `result.folder.id`
- **严禁把标签名称当作 `tag create --parent-id` 的值**；`--parent-id` 只能填父标签 ID，父标签 ID 必须来自 `tag list` 的 `tags[].id`
- **严禁把标签名称当作 `tag delete/update --id` 的值**；`--id` 只能填要操作的标签 ID，标签 ID 必须来自 `tag list` 的 `tags[].id` 或 `tag create` 的 `result.tag.id`
- **严禁更新或删除系统标签**；`tag update/delete` 只适用于用户自定义标签
- **严禁把文件夹名称当作 `thread list --folder` 的值**；`--folder` 只能填文件夹 ID，文件夹 ID 必须来自 `folder list` 的 `folders[].id`
- **严禁把邮件 ID 当作 `thread get/update/trash/batch-update/batch-trash` 的会话 ID**；这些命令需要 conversationId，可来自 `thread list` 的 `conversations[].id` 或邮件结果中的 `conversationId`

## 核心工作流

```bash
# 1. 查看可用邮箱 — 提取邮箱地址
dws mail mailbox list --format json

# 1b. 创建顶层邮件文件夹
dws mail folder create --email user@company.com --name "项目资料" --format json

# 1c. 创建子文件夹 — 先通过 folder list 获取父文件夹 id，再传给 --folder
dws mail folder list --email user@company.com --format json
dws mail folder create --email user@company.com --name "子文件夹" --folder <folderId> --format json

# 1d. 更新或删除邮件文件夹 — 先通过 folder list 获取目标文件夹 id，再传给 --id
dws mail folder list --email user@company.com --format json
dws mail folder update --email user@company.com --id <folderId> --name "新文件夹名" --format json
dws mail folder delete --email user@company.com --id <folderId> --format json

# 1e. 创建顶层邮件标签
dws mail tag create --email user@company.com --name "项目资料" --format json

# 1f. 创建子标签 — 先通过 tag list 获取父标签 id，再传给 --parent-id
dws mail tag list --email user@company.com --format json
dws mail tag create --email user@company.com --name "子标签" --parent-id <tagId> --format json

# 1g. 更新或删除邮件标签 — 先通过 tag list 获取目标标签 id，再传给 --id
dws mail tag list --email user@company.com --format json
dws mail tag update --email user@company.com --id <tagId> --name "新标签名" --format json
dws mail tag delete --email user@company.com --id <tagId> --format json

# 1h. 列出文件夹中的邮件会话 — 先通过 folder list 获取文件夹 id，再传给 --folder
dws mail folder list --email user@company.com --format json
dws mail thread list --email user@company.com --folder <folderId> --limit 10 --format json

# 1i. 修改或删除邮件会话 — 先通过 thread list 获取 conversationId
dws mail thread update --email user@company.com --id <conversationId> --action markRead --format json
dws mail thread batch-update --email user@company.com --ids <conversationId1>,<conversationId2> --action markUnread --format json
dws mail thread trash --email user@company.com --id <conversationId> --yes --format json
dws mail thread batch-trash --email user@company.com --ids <conversationId1>,<conversationId2> --yes --format json

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
| `folder list` | `folders[].id` | folder create 的 --folder；folder delete/update 的 --id |
| `folder create` | `result.folder.id` | 后续创建子文件夹或移动邮件时作为 --folder；更新/删除该文件夹时作为 --id |
| `folder list` | `folders[].id` | thread list 的 --folder |
| `thread list` | `conversations[].id` | thread get/update/trash/batch-update/batch-trash 的 --id/--ids |
| `tag list` | `tags[].id` | message batch-update 的 --tags；thread update/batch-update 的 --tag-ids |
| `tag list` | `tags[].id` | tag create 的 --parent-id；tag delete/update 的 --id |
| `tag create` | `result.tag.id` | 后续创建子标签时作为 --parent-id；更新/删除该标签时作为 --id |
| `aisearch person` → `contact user get` / `contact user search` / `mail user search` | 用户邮箱 (orgAuthEmail / email) | message send 的 --to/--cc（三路并发，取先到结果） |
| `user search` | 用户邮箱 (email) | message send 的 --to/--cc |
| `message send` / `draft send` / `message reply` / `message reply-all` / `message forward` | `internetMessageId` | `message verify` 的 --internet-message-id |

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
- `thread list --folder` 的值必须是文件夹 ID，不是文件夹显示名称；不知道文件夹 ID 时，先调用 `folder list` 查 `folders[].id`
- `thread get/update/trash/batch-update/batch-trash` 使用的是会话 ID（conversationId），不是邮件 ID；会话 ID 可来自 `thread list` 的 `conversations[].id`，也可来自 `message search` 或 `message get` 返回的 `conversationId`
- `thread update` / `thread batch-update` 仅支持 `markRead`、`markUnread`、`addTags`、`removeTags`；标签操作必须传 `--tag-ids`
- `folder create --folder` 的值必须是父文件夹 ID，不是文件夹显示名称；不知道父文件夹 ID 时，先调用 `folder list` 查 `folders[].id`
- `folder delete/update --id` 的值必须是目标文件夹 ID，不是文件夹显示名称；不知道目标文件夹 ID 时，先调用 `folder list` 查 `folders[].id`
