# 钉盘 (drive) 命令参考

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

### 获取文件/文件夹列表

```
Usage:
  dws drive list [flags]
Example:
  dws drive list --limit 20
  dws drive list --limit 20 --folder <dentryUuid> --order-by name --order asc
Flags:
      --limit int           每页返回数量，默认 20，最大 100 (可选)
      --cursor string       分页游标，首次不传 (可选)
      --order string        排序方向: asc|desc，默认 desc (可选)
      --order-by string     排序字段: createTime|modifyTime|name (可选)
      --folder string       父节点 ID (dentryUuid)，不传则列出空间根目录 (可选)
      --space-id string     空间 ID，不传则使用「我的文件」对应 spaceId (可选)
      --thumbnail           是否返回缩略图信息 (可选)
```

### 获取钉盘空间列表

```
Usage:
  dws drive list-spaces [flags]
Example:
  dws drive list-spaces
  dws drive list-spaces --space-type mySpace
  dws drive list-spaces --space-type orgSpace --limit 20 --cursor <TOKEN>
Flags:
      --space-type string   空间类型: orgSpace=企业空间(默认), mySpace=我的文件 (可选)
      --limit int           每页返回数量 (默认 20，最大 50)，仅 spaceType 为 orgSpace 时有效
      --cursor string   分页游标，仅企业空间支持分页 (可选)
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

### 获取文件元数据信息

```
Usage:
  dws drive info [flags]
Example:
  dws drive info --node <dentryUuid>
Flags:
      --node string    节点 ID (dentryUuid) (必填)
      --space-id string   节点所属空间 ID (可选)
```

### 文件内容获取路由规则

> 当用户请求"分析/查看/读取某个钉盘文件内容"时，**必须先调用 `dws drive info` 获取文件元数据**，再根据返回的 `extension` 字段选择对应链路。
> 注意：若检测到钉钉文档类型（adoc/axls/amind/adraw），会自动跟进调用 `doc info` 返回更准确的文档信息。

| extension | 文件类型 | 操作 | 命令 |
|-----------|---------|------|------|
| adoc | 在线文档 | 在线获取 Markdown 内容 | `dws doc read --node <fileId>` |
| axls | 在线表格 | 在线读取表格数据 | `dws sheet list` → `dws sheet range read` |
| able | 多维表格 | 在线查询记录 | `dws aitable table list` → `dws aitable record query` |
| 其他（pdf/docx/txt/png 等） | 普通文件 | **不支持在线分析**，需用户主动下载后本地查看 | `dws drive download` |

### 下载文件到本地

下载流程一步到位：获取下载 URL → HTTP GET 下载文件二进制内容到本地。

```
Usage:
  dws drive download [flags]
Example:
  dws drive download --node <dentryUuid> --output ./report.pdf
  dws drive download --node <dentryUuid> --output ~/downloads/
Flags:
      --node string    文件 ID (dentryUuid) (必填)
      --output string     本地保存路径 (必填)，可以是文件路径或目录；如果指定目录，文件名从下载 URL 中自动推断
      --space-id string   文件所属空间 ID (可选)
```

> **注意**：`--output` 是必填参数，不传会报错。

### 创建文件夹

```
Usage:
  dws drive mkdir [flags]
Example:
  dws drive mkdir --name "项目资料"
  dws drive mkdir --name "子目录" --folder <dentryUuid>
Flags:
      --name string        文件夹名称，最长 50 字符 (必填)
      --folder string   父节点 ID (dentryUuid)，不传则在空间根目录下创建 (可选)
      --space-id string    目标空间 ID，不传则使用「我的文件」 (可选)
```

### 上传本地文件到钉盘

> **注意：** 上传文件必须使用 `dws drive upload` 命令，禁止使用 `upload-info` + `curl` + `commit` 三步流程。

```
Usage:
  dws drive upload [flags]
Example:
  dws drive upload --file ./report.pdf
  dws drive upload --file ./slides.pptx --file-name "Q1汇报.pptx"
  dws drive upload --file ./data.xlsx --folder <dentryUuid>
Flags:
      --file string        本地文件路径 (必填)
      --file-name string   文件显示名称 (默认使用文件名)
      --space-id string    目标空间 ID，不传则使用「我的文件」 (可选)
      --mime-type string   文件 MIME 类型，不传则自动推断 (可选)
      --folder string   父节点 ID (dentryUuid)，不传则上传到空间根目录 (可选)
```

`upload` 命令内部自动完成三步流程（获取凭证 → OSS PUT → 提交入库），无需手动分步操作。

### 删除文件/文件夹到回收站

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws drive delete [flags]
Example:
  dws drive delete --node <dentryUuid> --format json    # 查询 fileId: dws drive list
Flags:
      --node string    文件/文件夹 ID (dentryUuid)，即 drive list 返回的 fileId (必填)
```

注意：`--node` 使用的是 `drive list` 返回结果中的 `fileId` 字段（即 `dentryUuid`），**不是** `dentryId` 字段。

## 意图判断

用户说"我的文件/钉盘/网盘/云盘" → `list`
用户说"钉盘空间/团队文件/有哪些空间/空间列表/团队文件列表" → `list-spaces`
用户说"文件详情/文件信息" → `info`
用户说"下载文件" → `download`
用户说"新建文件夹/创建目录" → `mkdir`
用户说"上传文件/传文件到钉盘" → `upload`（必须使用此命令，自动完成三步流程）
用户说"复制文件/移动文件/搬到/移到" → 使用 `dws doc copy`/`dws doc move`（详见下方「复制/移动钉盘文件」工作流）
用户说"删除文件/删除文件夹/移到回收站" → `delete`（危险操作，需确认）

关键区分: drive(钉盘文件管理) vs doc(文档内容读写)

**drive upload vs doc upload**: 用户提到"钉盘/网盘/我的文件"→ `drive upload`；提到"知识库/文档空间/workspace"→ `doc upload`；未明确目标时默认 `drive upload`

**钉盘文件复制/移动**: drive 本身没有 copy/move 命令，需使用 `dws doc copy`/`dws doc move` 实现（详见下方工作流）

## 核心工作流

```bash
# 1. 浏览「我的文件」根目录
dws drive list --limit 20 --format json

# 2. 进入子目录 — 提取 dentryUuid 作为 folder
dws drive list --limit 20 --folder <dentryUuid> --format json

# 3. 查看文件元数据
dws drive info --node <dentryUuid> --format json

# 4. 下载文件到本地
dws drive download --node <dentryUuid> --output /tmp/ --format json

# 5. 创建文件夹
dws drive mkdir --name "项目资料" --format json

# 6. 上传文件（必须使用 upload 命令，禁止手动分步操作）
dws drive upload --file ./报告.pdf --format json
dws drive upload --file ./报告.pdf --folder <dentryUuid> --format json

# 7. 删除文件/文件夹到回收站（危险操作：必须先向用户确认，用户同意后才加 --yes 执行）
# 正确流程：1.向用户展示"即将删除「文件名」到回收站" → 2.等用户确认 → 3.执行下面命令
dws drive delete --node <dentryUuid> --yes --format json
```

## 复制/移动钉盘文件

钉盘本身没有 copy/move 命令，需使用 `dws doc copy`/`dws doc move` 实现。

> **注意：字段选择**：`drive list` 返回中有 `dentryId`（数字格式）和 `fileId`（UUID 格式）两个字段，**必须使用 `fileId`（UUID 格式，如 `ZgpG2NdyVXYOR2D5UGDok65MJMwvDqPk`）**作为 `--node` 和 `--folder` 的参数值。**禁止使用 `dentryId`（数字格式，如 `220335325118`），传入数字格式会导致命令失败。**

> **注意**：钉盘场域下，仅支持将文件复制/移动到文件夹下，不支持文档下嵌套文档。

### 目标位置参数规则

| 目标位置 | 参数传递方式 | 前置步骤 |
|---------|-----------|---------|
| 未指定目标（默认） | `--folder <rootFolderId>` | 先 `dws drive list-spaces --space-type mySpace` 获取「我的文件」的 `rootFolderId` |
| 知识库空间根目录 | `--workspace <workspaceId>` | 无需额外步骤，直接传入 workspaceId |
| 钉盘 space 根目录 | `--folder <rootFolderId>` | 先 `dws drive list-spaces` 获取目标 space 的 `rootFolderId` |
| 钉盘 space 下的子文件夹 | `--folder <fileId>` | 先 `dws drive list --space-id <spaceId>` 逐层浏览，获取目标文件夹的 `fileId`（dentryUuid 格式） |

### 工作流示例

```bash
# ── 场景 默认: 用户未指定目标位置 → 复制/移动到「我的文件」根目录 ──
# 1. 获取源文件 dentryUuid
dws drive list --space-id <SPACE_ID> --format json
# 2. 获取「我的文件」个人空间的 rootFolderId
dws drive list-spaces --space-type mySpace --format json
# 3. 用「我的文件」的 rootFolderId 作为 --folder
dws doc copy --node <源文件dentryUuid> --folder <我的文件rootFolderId> --format json

# ── 场景 A: 复制钉盘文件到知识库空间根目录 ──
# 1. 获取源文件 dentryUuid
dws drive list --space-id <SPACE_ID> --format json
# 2. 直接传 workspaceId 即可
dws doc copy --node <源文件dentryUuid> --workspace <TARGET_WS_ID> --format json

# ── 场景 B: 移动钉盘文件到另一个钉盘 space 根目录 ──
# 1. 获取源文件 dentryUuid
dws drive list --space-id <SOURCE_SPACE_ID> --format json
# 2. 获取目标 space 的 rootFolderId
dws drive list-spaces --format json
# 3. 用 rootFolderId 作为 --folder（不需要传 --workspace）
dws doc move --node <源文件dentryUuid> --folder <目标space的rootFolderId> --format json

# ── 场景 C: 复制钉盘文件到钉盘 space 下的子文件夹 ──
# 1. 获取源文件 dentryUuid
dws drive list --space-id <SOURCE_SPACE_ID> --format json
# 2. 浏览目标 space 找到目标文件夹的 fileId（dentryUuid 格式）
dws drive list --space-id <TARGET_SPACE_ID> --format json
# 若目标在更深层级，继续用 --folder 逐层浏览
dws drive list --space-id <TARGET_SPACE_ID> --folder <父文件夹dentryUuid> --format json
# 3. 用目标文件夹的 fileId 作为 --folder
dws doc copy --node <源文件dentryUuid> --folder <目标文件夹fileId> --format json
```

## 上下文传递表


| 操作            | 从返回中提取                       | 用于                                                       |
| ------------- | ---------------------------- | -------------------------------------------------------- |
| `list`        | **`fileId`**（UUID 格式，注意：不是 `dentryId`） | info / download / mkdir / delete / list 的 --node 或 --folder；`doc copy/move` 的 --node 或 --folder |
| `list`        | `spaceId`                    | info / download / mkdir / commit 的 --space-id            |
| `list-spaces` | `rootFolderId`               | `doc copy/move` 的 --folder（复制/移动到钉盘 space 根目录时） |
| `list-spaces` | `spaceId`                    | list / info / download / mkdir / upload 的 --space-id     |
| `mkdir`       | `fileId`（UUID 格式）            | list 的 --folder                                          |

> **重要**：`drive list` 返回结果中同时包含 `dentryId` 和 `fileId` 两个字段。所有需要传 `--node` 的命令（info / download / delete）必须使用 `fileId`（即 dentryUuid），**不要使用** `dentryId`。


## 注意事项

- 不传 `--space-id` 时默认使用「我的文件」空间
- 不传 `--folder` 时默认操作空间根目录
- `--folder` 只能使用父文件夹的 `dentryUuid`。不要把 `drive info` 返回的数字型 `dentryId` 当作父目录；`dentryId` 只用于 `chat message send --dentry-id`
- `--order-by` 支持: `createTime`、`modifyTime`、`name`
- **上传文件必须使用 `dws drive upload` 命令**，禁止使用 `upload-info` + `curl` + `commit` 三步手动流程
- `--file-name` 必须包含扩展名（如 `report.pdf`）

## 自动化脚本


| 脚本                                                     | 场景          | 用法                                    |
| ------------------------------------------------------ | ----------- | ------------------------------------- |
| [drive_tree_list.py](../../scripts/drive_tree_list.py) | 递归列出钉盘目录树结构 | `python drive_tree_list.py --depth 2` |


## 相关产品

- [doc](./doc.md) — 文档内容读写/知识库空间，不是文件存储
- [chat](./chat.md) — 上传文件到 drive 后可通过 Markdown 语法发送图片/文件消息
