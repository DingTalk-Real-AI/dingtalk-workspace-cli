# 邮件会话与消息动作命令

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

**常用标签 ID：**

| 标签 ID | 名称 | 图标 |
|---------|------|------|
| `1` | 跟进事项 | 小红旗 |
| `2` | 完成事项 | 绿色小勾 |
| `11` | 重要 | 星标 |

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
      --yes            跳过确认提示，直接执行 (可选)
```

> ⚠️ **危险操作**：此命令会将邮件会话移动到已删除文件夹。建议先通过 `thread get` 确认目标会话后再执行。

将指定邮件会话移动到已删除文件夹，不会永久删除邮件。`--id` 必须填写会话 ID，不是邮件 ID。默认需要用户确认，传入 `--yes` 可跳过确认。

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
      --yes            跳过确认提示，直接执行 (可选)
```

> ⚠️ **危险操作**：此命令会批量将邮件会话移动到已删除文件夹。建议先通过 `thread list` 确认目标会话后再执行。

将指定邮件会话批量移动到已删除文件夹，单次最多 100 个会话。不会永久删除邮件。`--ids` 必须填写会话 ID 列表，不是邮件 ID 列表。默认需要用户确认，传入 `--yes` 可跳过确认。

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
  dws mail message reply --from user@company.com --id <messageId> --subject "Re: 周报" --content "已收到，谢谢！"
Flags:
      --from string                     发件人邮箱 (必填)，别名: --sender
      --to string                       收件人列表（可选）
      --id string                       要回复的邮件 ID (必填)
      --subject string                  回复邮件标题（可选）
      --content string                  回复正文（可选），别名: --body
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
  dws mail message reply-all --from user@company.com --id <messageId> --subject "Re: 周报" --content "感谢大家的参与！"
Flags:
      --from string                     发件人邮箱 (必填)，别名: --sender
      --to string                       收件人列表（可选，包含发件人及所有原始收件人）
      --id string                       要回复的邮件 ID (必填)
      --subject string                  回复邮件标题（可选）
      --content string                  回复正文（可选），别名: --body
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
      --content string                  转发附言（可选），别名: --body
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

### [危险] 撤回已发送的邮件

```
Usage:
  dws mail sent-message recall [flags]
Example:
  dws mail sent-message recall --email user@company.com --id <mailId> --subject "邮件主题" --yes
Flags:
      --email string    发件人邮箱地址 (必填)
      --id string       要撤回的邮件 ID (必填)
      --subject string  邮件主题 (必填)
      --yes             跳过确认提示，直接执行 (可选)
```

> ⚠️ **危险操作**：此命令会撤回已发送的邮件。仅支持撤回同组织内未读邮件。

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | `string` | 撤回任务 ID（可用于 recall-detail 查询进度） |
| `success` | `boolean` | 接口调用是否成功 |
| `errorCode` | `string` | 错误码（仅失败时存在） |
| `errorMsg` | `string` | 错误信息（仅失败时存在） |

### 查询邮件撤回进度

```
Usage:
  dws mail sent-message recall-detail [flags]
Example:
  dws mail sent-message recall-detail --email user@company.com --id <recallTaskId>
Flags:
      --email string   用户的邮箱地址 (必填)
      --id string      撤回任务 ID (必填)，由 recall 命令返回
```

根据撤回任务 ID 查询邮件撤回的详细进度。撤回任务 ID 来源：`sent-message recall` 命令返回值中的 `id` 字段。

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | `boolean` | 调用是否成功 |
| `id` | `string` | 任务 ID |
| `status` | `string` | 任务状态（见下方枚举） |
| `errorCode` | `string` | 错误码（仅失败时存在） |
| `errorMsg` | `string` | 错误信息（仅失败时存在） |

**任务状态枚举：**

| 状态值 | 说明 |
|--------|------|
| `UNINITED` | 未初始化 |
| `SUBMITTED` | 已提交 |
| `RUNNING` | 执行中 |
| `FINISHED` | 已完成 |
| `CANCELED` | 已取消 |
| `FAILED` | 失败 |
