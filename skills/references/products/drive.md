# 钉盘 (drive) 命令参考

钉盘 = DingTalk Drive，用于云端文件存储 / 上传 / 下载 / 目录管理。不是在线文档编辑；要编辑文档请用 [doc](./doc.md)。

## 命令总览

### 列出钉盘目录
```
Usage:
  dws drive list [flags]
Example:
  dws drive list
  dws drive list --parent-id <FOLDER_ID>
  dws drive list --parent-id <FOLDER_ID> --max 20 --order-by modifiedTime --order desc
Flags:
      --parent-id string     父目录 ID (不传则列根目录)
      --space-id string      钉盘空间 ID (一般无需指定)
      --max int              每页数量 (默认 20)
      --next-token string    分页游标 (从上次结果的 nextToken 获取)
      --order-by string      排序字段: name / createdTime / modifiedTime
      --order string         排序方向: asc / desc
      --thumbnail            是否返回缩略图链接
```

### 获取文件元信息
```
Usage:
  dws drive info [flags]
Example:
  dws drive info --file-id <FILE_ID>
Flags:
      --file-id string    文件或文件夹 ID (必填)
      --space-id string   钉盘空间 ID (一般无需指定)
```

### 创建文件夹
```
Usage:
  dws drive mkdir [flags]
Example:
  dws drive mkdir --name "项目资料"
  dws drive mkdir --name "子目录" --parent-id <PARENT_FOLDER_ID>
Flags:
      --name string        文件夹名称 (必填)
      --parent-id string   父目录 ID (不传则建在根目录)
      --space-id string    钉盘空间 ID (一般无需指定)
```

### 获取下载临时链接
```
Usage:
  dws drive download [flags]
Example:
  dws drive download --file-id <FILE_ID>
Flags:
      --file-id string    文件 ID (必填)
      --space-id string   钉盘空间 ID (一般无需指定)
```

### 获取上传凭证 (上传第一步)
```
Usage:
  dws drive upload-info [flags]
Example:
  dws drive upload-info --file-name "report.pdf" --file-size 102400
  dws drive upload-info --file-name "slides.pptx" --file-size 512000 --parent-id <FOLDER_ID>
Flags:
      --file-name string   文件名含后缀 (必填)
      --file-size int      文件大小，单位字节 (必填)
      --mime-type string   MIME 类型 (可选，服务端会自动推断)
      --parent-id string   父目录 ID (不传则上传到根目录)
      --space-id string    钉盘空间 ID (一般无需指定)
```

### 提交上传 (上传第三步)
```
Usage:
  dws drive commit [flags]
Example:
  dws drive commit --file-name "report.pdf" --file-size 102400 --upload-id <UPLOAD_ID>
Flags:
      --file-name string          文件名含后缀 (必填，须与 upload-info 一致)
      --file-size int             文件大小，单位字节 (必填，须与 upload-info 一致)
      --upload-id string          upload-info 返回的 uploadId (必填)
      --parent-id string          父目录 ID (必填时须与 upload-info 一致)
      --space-id string           钉盘空间 ID (一般无需指定)
      --conflict-handler string   同名冲突策略: AUTO_RENAME / OVERWRITE / RETURN_DENTRY_IF_EXIST (默认 AUTO_RENAME)
```

## 意图判断

用户说"钉盘有什么文件/列钉盘/看钉盘目录" → `list`
用户说"钉盘文件详情/文件信息" → `info` (需 fileId)
用户说"新建钉盘目录/钉盘里建文件夹" → `mkdir`
用户说"下载钉盘文件/把这个文件拿下来" → `download` 拿临时 URL，再由 Agent 自行发起 HTTP GET
用户说"上传文件到钉盘/把本地文件传钉盘":
- 三步走: `upload-info` → HTTP PUT 到预签名 URL → `commit`
- **没有**一条聚合命令可以完成；必须完整走完三步

关键区分:
- drive(钉盘云存储，面向文件二进制) vs doc(在线文档/知识库，面向富文本内容)
- 把图片/文件发到群里一般走 drive 上传拿链接 → chat 发送 Markdown 链接 (见 [chat.md](./chat.md) 的 `drive → chat` 工作流)

## 核心工作流

```bash
# ── 工作流 1: 浏览钉盘 ──

# 1. 看根目录
dws drive list --format json

# 2. 进入子目录 (parentId 取自上一步的 dentryUuid)
dws drive list --parent-id <FOLDER_ID> --format json

# 3. 看单个文件的元信息
dws drive info --file-id <FILE_ID> --format json

# ── 工作流 2: 上传本地文件到钉盘 (三步不能省) ──

# Step 1: 拿上传凭证
dws drive upload-info --file-name "report.pdf" --file-size 102400 --parent-id <FOLDER_ID> --format json
# → 返回: resourceUrl (预签名 URL), headers, uploadId

# Step 2: Agent 自己发 HTTP PUT 把文件二进制推到 resourceUrl
#         请求头必须携带返回的 headers 全部键值对；期望 HTTP 200
curl -X PUT -T ./report.pdf -H "<header-from-step-1>: <value>" "<resourceUrl>"

# Step 3: 提交入库
dws drive commit --file-name "report.pdf" --file-size 102400 --upload-id <UPLOAD_ID> \
  --parent-id <FOLDER_ID> --format json
# → 返回: dentryUuid (= 后续 download/info 用的 fileId)

# ── 工作流 3: 下载钉盘文件到本地 (两步) ──

# Step 1: 拿临时下载 URL
dws drive download --file-id <FILE_ID> --format json
# → 返回: resourceUrl (带签名的临时下载 URL), expirationSeconds

# Step 2: Agent 自己发 HTTP GET 下载二进制
curl -o ./report.pdf "<resourceUrl>"

# ── 工作流 4: 建目录后批量上传 ──

# 1. 建目录 → 拿 folderId
dws drive mkdir --name "2026 Q1 归档" --format json

# 2. 再走 "工作流 2" 把每个文件上传到该目录
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list` | `dentryUuid` (folder 类) | 下次 list 的 --parent-id、upload-info / commit 的 --parent-id、mkdir 的 --parent-id |
| `list` | `dentryUuid` (file 类) | info / download 的 --file-id |
| `list` | `nextToken` | 下次 list 的 --next-token |
| `mkdir` | `dentryUuid` | 作为父目录传给 upload-info / commit 的 --parent-id |
| `upload-info` | `resourceUrl` + `headers` | Agent 自行执行 HTTP PUT 上传二进制 |
| `upload-info` | `uploadId` | commit 的 --upload-id |
| `commit` | `dentryUuid` | download / info / chat message 发送图片链接的 --file-id |
| `download` | `resourceUrl` | Agent 自行执行 HTTP GET 下载；也可作为 Markdown 图片/附件链接发送到 chat |

## 注意事项

- 上传是**三步**流程：`upload-info` → 客户端 HTTP PUT 到预签名 URL → `commit`。**没有**自动聚合命令，跳过任何一步都会失败
- Step 2 的 HTTP PUT 必须把 upload-info 返回的 `headers` 全部回传，`Content-Type` 通常要留空；只有 PUT 返回 200 才能调 `commit`
- 上传凭证 (`uploadId`) 有过期时间，拿到后尽快 commit；过期需重新调 `upload-info`
- `download` 只返回临时 URL（几分钟级别有效期），不会把文件落地；要真正下载到本地必须再发 HTTP GET
- `--parent-id` 在 upload-info / commit 中要保持一致，否则 commit 会报位置不匹配
- `--conflict-handler` 默认 `AUTO_RENAME` (自动重命名)；`OVERWRITE` 覆盖同名文件前必须和用户确认
- `--space-id` 绝大多数场景不需要传；默认会用用户主钉盘空间
- 文件名规则：头尾不能有空格；不能含 `*`、`"`、`<`、`>`、`|`、制表符；不能以 `.` 结尾

## 相关产品

- [doc](./doc.md) — 钉钉在线文档 / 知识库（文字、表格、脑图等富文本节点），和 drive 的裸文件存储不同
- [chat](./chat.md) — 结合 drive 发送图片 / 附件消息到群聊（Markdown 链接语法）
