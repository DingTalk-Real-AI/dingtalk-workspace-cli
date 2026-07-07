# 钉盘 (drive) 命令参考

钉盘 = DingTalk Drive，用于云端文件存储 / 上传 / 下载 / 目录管理。不是在线文档编辑；要编辑文档请用 [doc](./doc.md)。

## 查询命令帮助

当你不确定某个命令的具体参数、格式或可选项时，**优先执行 `--help` 查询**，不要猜测参数名或凭记忆编造。

```bash
# 查看 drive 下所有子命令
dws drive --help

# 查看具体命令的完整参数说明
dws drive list --help
dws drive upload --help
dws drive download --help
```

规则：
- 参数名不确定时 → 先 `--help`，再调用
- 报错 "unknown flag" 时 → `--help` 确认正确的 flag 名称
- 不确定某个功能是否存在时 → `dws drive --help` 查看命令列表

## 命令总览

### 列出钉盘空间
```
Usage:
  dws drive list-spaces [flags]
Example:
  dws drive list-spaces --format json
  dws drive list-spaces --space-type orgSpace --max 50 --format json
Flags:
      --max int             每页数量 (默认 20)
      --next-token string   分页游标 (从上次结果的 nextToken 获取)
      --space-type string   空间类型过滤
```

> 适用场景：复制/移动文件到「我的文件」或团队空间根目录时，先取 `rootFolderId`；或者枚举用户可访问的团队空间。
> 注意：由企业服务发现 envelope 注册；个别企业 MCP gateway 可能未开通，调用前可用 `dws drive list-spaces --help` / `--dry-run` 验证。

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

### 获取钉盘空间列表

```
Usage:
  dws drive list-spaces [flags]
Example:
  dws drive list-spaces
  dws drive list-spaces --space-type mySpace
  dws drive list-spaces --space-type orgSpace --max 20 --next-token <TOKEN>
Flags:
      --space-type string   空间类型: orgSpace=企业空间(默认), mySpace=我的文件 (可选)
      --max int             每页返回数量 (默认 20)，仅 spaceType 为 orgSpace 时有效
      --next-token string   分页游标，仅企业空间支持分页 (可选)
```

spaceType 筛选规则：
- `orgSpace`（默认/不传）：返回企业空间列表，支持 `nextToken` 分页
- `mySpace`：返回用户的"我的文件"个人空间（单个，不支持分页）

返回字段说明：
- `spaceId` — 空间 ID，用于 `list`/`info`/`upload` 等命令的 `--space-id`
- `spaceName` — 空间名称（如"全员文件夹"、"我的文件"）
- `rootFolderId` — 空间根目录的 dentryUuid，可作为 `doc copy/move` 的 `--folder` 参数
- `spaceType` — 空间类型（如 `orgSpace`）
- `nextToken` — 若不为空，表示还有更多空间可查询（仅企业空间）

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

### 文件内容获取路由规则

> 当用户请求"分析/查看/读取某个钉盘文件内容"时，**必须先调用 `dws drive info` 获取文件元数据**，再根据返回的 `extension` 字段选择对应链路。
> 注意：若检测到钉钉文档类型（adoc/axls/amind/adraw），会自动跟进调用 `doc info` 返回更准确的文档信息。

| extension | 文件类型 | 操作 | 命令 |
|-----------|---------|------|------|
| adoc | 在线文档 | 在线获取 Markdown 内容 | `dws doc read --node <fileId>` |
| axls | 在线表格 | 在线读取表格数据 | `dws sheet range read` |
| able | 多维表格 | 在线查询记录 | `dws aitable base get` → `dws aitable record query` |
| 其他（pdf/docx/txt/png 等） | 普通文件 | **不支持在线分析**，需用户主动下载后本地查看 | `dws drive download --file-id <fileId> --output <path>` |

### 删除文件/文件夹到回收站

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws drive delete [flags]
Example:
  dws drive delete --file-id <dentryUuid> --format json    # 查询 fileId: dws drive list
Flags:
      --file-id string    文件/文件夹 ID (dentryUuid)，即 drive list 返回的 fileId (必填)
```

注意：`--file-id` 使用的是 `drive list` 返回结果中的 `fileId` 字段（即 `dentryUuid`），**不是** `dentryId` 字段。

### 下载钉盘文件到本地
```
Usage:
  dws drive download [flags]
Example:
  dws drive download --file-id <FILE_ID> --output ./report.pdf
Flags:
      --file-id string    文件 ID (必填)
      --output string     本地保存路径 (文件路径或目录，必填)
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

### 一键上传 (三步合成)
```
Usage:
  dws drive upload [flags]
Example:
  dws drive upload --file ./report.pdf
  dws drive upload --file ./slides.pptx --file-name "Q1汇报.pptx"
  dws drive upload --file ./data.xlsx --folder <dentryUuid>
Flags:
      --file string        本地文件路径 (必填)
      --file-name string   文件显示名称 (默认使用本地文件名)
      --folder string      父节点 ID (dentryUuid)，不传则上传到空间根目录
      --space-id string    目标空间 ID，不传则使用「我的文件」
      --mime-type string   文件 MIME 类型，不传则自动推断
```

> 客户端胶水命令：内部自动完成 `upload-info` → HTTP PUT 到 OSS → `commit_upload` 三步。
> 适合大多数本地文件上传场景，无需 Agent 自己发 HTTP PUT。
> 注意：`--folder` 接受 `dentryUuid`（UUID 格式），不要传纯数字 `dentryId`。

### 删除文件/文件夹到回收站

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws drive delete [flags]
Example:
  dws drive delete --file-id <dentryUuid> --yes --format json
Flags:
      --file-id string   文件或文件夹 nodeId（dentryUuid 格式，必填）

Global Flags:
      --yes   跳过二次确认 (危险操作，建议先与用户确认)
```

> 由企业服务发现 envelope 注册（路由到 doc 服务的 `delete_document` tool）；不同企业 MCP gateway 可能不暴露，调用前可用 `dws drive delete --help` 验证。
> 删除是软删除（进回收站），但仍需用户明确确认；不要在自动化脚本里默认带 `--yes`。

## 意图判断

用户说"钉盘有什么空间/团队文件/列空间" → `list-spaces`
用户说"钉盘有什么文件/列钉盘/看钉盘目录" → `list`
用户说"钉盘文件详情/文件信息" → `info` (需 fileId)
用户说"新建钉盘目录/钉盘里建文件夹" → `mkdir`
用户说"下载钉盘文件/把这个文件拿下来" → `download` 指定 `--output` 保存到本地
用户说"上传文件到钉盘/把本地文件传钉盘":
- **首选 `upload`**：一条命令自动完成三步（适合本地文件直接上传）
- 也可手动三步：`upload-info` → HTTP PUT 到预签名 URL → `commit`（适合自定义流式上传，或不想在客户端做 HTTP PUT 的场景）
用户说"删除钉盘文件/移到回收站" → `delete`（不可逆，必须确认）

关键区分:
- drive(钉盘云存储，面向文件二进制) vs doc(在线文档/知识库，面向富文本内容)
- 把图片/文件发到群里一般走 drive 上传拿链接 → chat 发送 Markdown 链接 (见 [chat.md](./chat.md) 的 `drive → chat` 工作流)

**drive upload vs doc upload**: 用户提到"钉盘/网盘/我的文件"→ drive 三步式；提到"知识库/文档空间/workspace"→ `doc upload`；未明确目标时默认走 drive

**钉盘文件复制/移动**: drive 本身没有 copy/move 命令，需使用 `dws doc copy` / `dws doc move` 实现（详见下方「复制/移动钉盘文件」工作流）

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

# ── 工作流 3: 下载钉盘文件到本地 ──
dws drive download --file-id <FILE_ID> --output ./report.pdf --format json

# ── 工作流 4: 建目录后批量上传 ──

# 1. 建目录 → 拿 folderId
dws drive mkdir --name "2026 Q1 归档" --format json

# 2. 再走 "工作流 2" 把每个文件上传到该目录
```

## 复制/移动钉盘文件

钉盘本身没有 copy/move 命令，需使用 `dws doc copy` / `dws doc move` 实现跨场景搬运。

> **关键陷阱：字段选择**
> `drive list` 返回中有 `dentryId`（数字格式，如 `220335325118`）和 `fileId`（UUID 格式，如 `ZgpG2NdyVXYOR2D5UGDok65MJMwvDqPk`）两个字段。
> **必须使用 `fileId`（UUID 格式）**作为 `--node` 和 `--folder` 的参数值。
> **禁止使用 `dentryId`（数字格式）**，传入数字格式会导致命令失败。

> **能力限制**：钉盘场域下，仅支持将文件复制/移动到文件夹下，**不支持文档下嵌套文档**。

### 目标位置参数规则

| 目标位置 | 参数传递方式 | 前置步骤 |
|---------|-----------|---------|
| 未指定目标（默认） | `--folder <rootFolderId>` | 先 `dws drive list-spaces --space-type mySpace` 获取「我的文件」的 `rootFolderId` |
| 知识库空间根目录 | `--workspace <workspaceId>` | 无需额外步骤，直接传入 workspaceId |
| 钉盘 space 根目录 | `--folder <rootFolderId>` | 先 `dws drive list-spaces` 获取目标 space 的 `rootFolderId` |
| 钉盘 space 下的子文件夹 | `--folder <fileId>` | 先 `dws drive list --space-id <spaceId>` 逐层浏览，获取目标文件夹的 `fileId`（dentryUuid 格式） |

### 工作流示例

```bash
# ── 场景默认: 用户未指定目标位置 → 复制/移动到「我的文件」根目录 ──
dws drive list --space-id <SPACE_ID> --format json                       # 获取源文件 dentryUuid
dws drive list-spaces --space-type mySpace --format json                 # 获取「我的文件」rootFolderId
dws doc copy --node <源文件dentryUuid> --folder <我的文件rootFolderId> --format json

# ── 场景 A: 复制钉盘文件到知识库空间根目录 ──
dws drive list --space-id <SPACE_ID> --format json
dws doc copy --node <源文件dentryUuid> --workspace <TARGET_WS_ID> --format json

# ── 场景 B: 移动钉盘文件到另一个钉盘 space 根目录 ──
dws drive list --space-id <SOURCE_SPACE_ID> --format json
dws drive list-spaces --format json
dws doc move --node <源文件dentryUuid> --folder <目标space的rootFolderId> --format json
# 注意：移动到其他 space 时只传 --folder，不传 --workspace

# ── 场景 C: 复制钉盘文件到钉盘 space 下的子文件夹 ──
dws drive list --space-id <SOURCE_SPACE_ID> --format json
dws drive list --space-id <TARGET_SPACE_ID> --format json
dws drive list --space-id <TARGET_SPACE_ID> --parent-id <父文件夹dentryUuid> --format json  # 目标深层级时逐层浏览
dws doc copy --node <源文件dentryUuid> --folder <目标文件夹fileId> --format json
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
| `download` | `--output` 指定的路径 | 本地文件落地位置 |

## 注意事项

- 上传两条路径：**首选** `upload` 一条命令搞定（内部走三步）；如果需要自己控制 HTTP PUT，可走 `upload-info` → 客户端 HTTP PUT → `commit` 三步
- 用三步流程时，Step 2 的 HTTP PUT 必须把 upload-info 返回的 `headers` 全部回传，`Content-Type` 通常要留空；只有 PUT 返回 200 才能调 `commit`
- 上传凭证 (`uploadId`) 有过期时间，拿到后尽快 commit；过期需重新调 `upload-info`
- `download` 需要指定 `--output`，CLI 会把文件保存到本地路径或目录
- `--parent-id` 在 upload-info / commit 中要保持一致，否则 commit 会报位置不匹配
- `--conflict-handler` 默认 `AUTO_RENAME` (自动重命名)；`OVERWRITE` 覆盖同名文件前必须和用户确认
- `--space-id` 绝大多数场景不需要传；默认会用用户主钉盘空间
- 文件名规则：头尾不能有空格；不能含 `*`、`"`、`<`、`>`、`|`、制表符；不能以 `.` 结尾

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [drive_tree_list.py](../../scripts/drive_tree_list.py) | 递归列出钉盘目录树结构 | `python drive_tree_list.py --depth 2` |

## 相关产品

- [doc](./doc.md) — 钉钉在线文档 / 知识库（文字、表格、脑图等富文本节点），和 drive 的裸文件存储不同
- [chat](./chat.md) — 结合 drive 发送图片 / 附件消息到群聊（Markdown 链接语法）
