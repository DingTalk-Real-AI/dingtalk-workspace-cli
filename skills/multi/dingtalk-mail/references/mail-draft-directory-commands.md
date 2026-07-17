# 草稿、用户、模板与联系人命令

### 创建草稿
```
Usage:
  dws mail draft create [flags]
Example:
  dws mail draft create --from user@company.com --to colleague@company.com \
    --subject "草稿标题" --content "草稿正文"
  dws mail draft create --from user@company.com --subject "草稿标题"
  dws mail draft create --from user@company.com --subject "带附件草稿" \
    --content "见附件" --attachment ./report.pdf
  dws mail draft create --from user@company.com --subject "带图片草稿" \
    --content "图表：[inline:chart.png]" --inline-attachment ./chart.png
Flags:
      --from string                     发件人邮箱 (必填)，别名: --sender
      --subject string                  邮件标题 (必填)
      --to string                       收件人列表（可选，有确定收件人时才传）
      --cc string                       抄送人列表（可选，有确定抄送人时才传）
      --content string                  邮件正文（可选，有正文内容时才传），别名: --body
      --attachment stringArray          附件文件路径，可多次指定 (可选)
      --inline-attachment stringArray   内联图片路径，可多次指定，cid 自动生成 (可选)
```

> **注意：** `--to`、`--cc`、`--content` 均为可选参数，**仅在用户明确提供对应信息时才传入**。若用户未指定收件人，不要传 `--to ""`（空字符串）。

**附件说明：**

指定 `--attachment` 或 `--inline-attachment` 时，CLI 自动完成草稿创建和附件上传，**草稿保留在草稿箱，不会发送**。内联图片用法同 `message send`（`--content` 中使用 `[inline:文件名]` 占位符）。

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `messageId` | `string` | 新建草稿的邮件 ID |

### 更新草稿
```
Usage:
  dws mail draft update [flags]
Example:
  dws mail draft update --from user@company.com --id <messageId> --subject "新标题" --content "新正文"
  dws mail draft update --from user@company.com --id <messageId> --content "见附件" --attachment ./report.pdf
  dws mail draft update --from user@company.com --id <messageId> \
    --content "图表：[inline:chart.png]" --inline-attachment ./chart.png
Flags:
      --from string                     发件人邮箱 (必填)，别名: --sender
      --id string                       草稿邮件 ID (必填)
      --to string                       收件人列表（可选）
      --cc string                       抄送人列表（可选）
      --subject string                  邮件标题（可选）
      --content string                  邮件正文（可选），别名: --body
      --attachment stringArray          附件文件路径，可多次指定 (可选)
      --inline-attachment stringArray   内联图片路径，可多次指定，cid 自动生成 (可选)
```

**附件说明：**

指定 `--attachment` 或 `--inline-attachment` 时，CLI 自动完成草稿更新和附件上传，**草稿保留在草稿箱，不会发送**。内联图片用法同 `message send`（`--content` 中使用 `[inline:文件名]` 占位符）。

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
  dws mail user search --email user@company.com --keyword "alice" --limit 10
  dws mail user search --email user@company.com --keyword "alice" --cursor <nextCursor>
  dws mail user search --email user@company.com --employee-no "E123456"
Flags:
      --email string        搜索目标邮箱地址 (可选)
      --keyword string      搜索关键词（未提供 --employee-no 时为必填）
      --employee-no string  按工号搜索用户；提供此参数时 keyword 不再必填
      --cursor string       分页游标，取自响应中的 nextCursor 字段（可选）
      --limit string        每页返回数量（可选），别名: --size
```

> **重要区别（三个容易混淆的命令）：**
> - `mail user search` — 搜索**企业通讯录用户**（按姓名/关键词或工号找人），用于获取某人的邮箱地址。需要企业邮箱权限。
> - `mail contact list` — 列举**个人联系人**（用户自己保存/创建的联系人列表），不需要关键词，返回自己的联系人。
> - `mail message search` — 搜索**邮件内容**（按 KQL 语法搜邮件，如主题、发件人、日期等）
>
> 不要混淆：查找"某人的邮箱地址"用 `user search`；查看"自己保存的联系人"用 `contact list`；查找"某封邮件"用 `message search`。
>
> 仅企业邮箱（非 `@dingtalk.com` 个人邮箱）可使用 `user search`；使用个人邮箱调用将因无权限而报错。
>
> `--keyword` 与 `--employee-no` 至少需要提供一个；当提供 `--employee-no` 时，`--keyword` 不再是必填字段。

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
      --content string    模板邮件正文 (必填)，别名: --body
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
      --limit string    每页返回数量 (必填)，别名: --size
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
      --content string    模板邮件正文 (可选)，别名: --body
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
      --limit string    每页返回数量 (必填)，别名: --size
```

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
