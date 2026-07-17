# 邮件文件夹、附件与标签命令

### 列举邮件文件夹
```
Usage:
  dws mail folder list [flags]
Example:
  dws mail folder list --email user@company.com
  dws mail folder list --email user@company.com --folder <folderId>
Flags:
      --email string      邮件所属邮箱地址 (必填)
      --folder string     父文件夹唯一标识，不传则返回顶层文件夹 (可选)，别名: --folder-id
```

不传 `--folder` 返回顶层文件夹列表；传入则返回该文件夹的子文件夹列表。

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
