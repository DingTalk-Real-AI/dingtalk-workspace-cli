# 钉盘 (drive) 命令参考

> ✅ **CLI 对齐状态（开源 dws v1.0.30）**：本文档已对齐开源 dws 实际暴露的 6 个子命令：`commit / download / info / list / mkdir / upload-info`。上传文件必须走两步：(1) `drive upload-info` 拿到 upload URL → (2) HTTP PUT 文件 → (3) `drive commit` 注册。单步 `drive upload` 命令在开源 v1.0.30 不存在；钉盘 space 枚举 (`list-spaces`) 也未暴露。


## 查询命令帮助

当你不确定某个命令的具体参数、格式或可选项时，**优先执行 `--help` 查询**，不要猜测参数名或凭记忆编造。

```bash
# 查看 drive 下所有子命令
dws drive --help

# 查看具体命令的完整参数说明
dws drive list --help
dws drive upload-info --help
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
  dws drive list --max 20
  dws drive list --max 20 --parent-id <dentryUuid> --order-by name --order asc
Flags:
      --max int             每页返回数量 (默认 20，最大 50)，别名: --limit
      --next-token string   分页游标，首次不传 (可选)
      --order string        排序方向: asc|desc，默认 desc (可选)
      --order-by string     排序字段: createTime|modifyTime|name (可选)
      --parent-id string    父节点 ID (dentryUuid)，不传则列出空间根目录 (可选)
      --space-id string     空间 ID，不传则使用「我的文件」对应 spaceId (可选)
      --thumbnail           是否返回缩略图信息 (可选)
```

### 获取文件元数据信息

```
Usage:
  dws drive info [flags]
Example:
  dws drive info --file-id <dentryUuid>
Flags:
      --file-id string    节点 ID (dentryUuid) (必填)
      --space-id string   节点所属空间 ID (可选)
```

### 下载文件到本地

下载流程一步到位：获取下载 URL → HTTP GET 下载文件二进制内容到本地。

```
Usage:
  dws drive download [flags]
Example:
  dws drive download --file-id <dentryUuid> --output ./report.pdf
  dws drive download --file-id <dentryUuid> --output ~/downloads/
Flags:
      --file-id string    文件 ID (dentryUuid) (必填)
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
  dws drive mkdir --name "子目录" --parent-id <dentryUuid>
Flags:
      --name string        文件夹名称，最长 50 字符 (必填)
      --parent-id string   父节点 ID (dentryUuid)，不传则在空间根目录下创建 (可选)
      --space-id string    目标空间 ID，不传则使用「我的文件」 (可选)
```

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

**drive vs doc upload**: 用户提到"钉盘/网盘/我的文件"→ 用 `drive upload-info` + `drive commit` 两步流程；提到"知识库/文档空间/workspace"→ `doc upload`

**钉盘文件复制/移动**: drive 本身没有 copy/move 命令，需使用 `dws doc copy`/`dws doc move` 实现（详见下方工作流）

## 核心工作流

```bash
# 1. 浏览「我的文件」根目录
dws drive list --max 20 --format json

# 2. 进入子目录 — 提取 dentryUuid 作为 parent-id
dws drive list --max 20 --parent-id <dentryUuid> --format json

# 3. 查看文件元数据
dws drive info --file-id <dentryUuid> --format json

# 4. 下载文件到本地
dws drive download --file-id <dentryUuid> --output /tmp/ --format json

# 5. 创建文件夹
dws drive mkdir --name "项目资料" --format json

# 6. 上传文件（必须使用 upload 命令，禁止手动分步操作）

# 7. 删除文件/文件夹到回收站（危险操作：必须先向用户确认，用户同意后才加 --yes 执行）
# 正确流程：1.向用户展示"即将删除「文件名」到回收站" → 2.等用户确认 → 3.执行下面命令
dws drive delete --file-id <dentryUuid> --yes --format json
```

## 复制/移动钉盘文件

钉盘本身没有 copy/move 命令，需使用 `dws doc copy`/`dws doc move` 实现。

> **注意：字段选择**：`drive list` 返回中有 `dentryId`（数字格式）和 `fileId`（UUID 格式）两个字段，**必须使用 `fileId`（UUID 格式，如 `ZgpG2NdyVXYOR2D5UGDok65MJMwvDqPk`）**作为 `--node` 和 `--folder` 的参数值。**禁止使用 `dentryId`（数字格式，如 `220335325118`），传入数字格式会导致命令失败。**

> **注意**：钉盘场域下，仅支持将文件复制/移动到文件夹下，不支持文档下嵌套文档。

### 目标位置参数规则

| 目标位置 | 参数传递方式 | 前置步骤 |
|---------|-----------|---------|
| 未指定目标（默认） | `--folder <rootFolderId>` | rootFolderId 需在钉盘 UI 中拿；开源 v1.0.30 暂无 `list-spaces` 命令枚举空间 |
| 知识库空间根目录 | `--workspace <workspaceId>` | 无需额外步骤，直接传入 workspaceId |
| 钉盘 space 根目录 | `--folder <rootFolderId>` | rootFolderId 来自钉盘 UI 或 `dws drive list --space-id <id>` 返回的父目录 |
| 钉盘 space 下的子文件夹 | `--folder <fileId>` | 先 `dws drive list --space-id <spaceId>` 逐层浏览，获取目标文件夹的 `fileId`（dentryUuid 格式） |

### 工作流示例

```bash
# ── 场景 默认: 用户未指定目标位置 → 复制/移动到「我的文件」根目录 ──
# 1. 获取源文件 dentryUuid
dws drive list --space-id <SPACE_ID> --format json
# 2. 获取「我的文件」个人空间的 rootFolderId
# dws drive list-spaces 在开源 v1.0.30 未暴露；用 dws drive list 替代
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
# 若目标在更深层级，继续用 --parent-id 逐层浏览
dws drive list --space-id <TARGET_SPACE_ID> --parent-id <父文件夹dentryUuid> --format json
# 3. 用目标文件夹的 fileId 作为 --folder
dws doc copy --node <源文件dentryUuid> --folder <目标文件夹fileId> --format json
```

## 上下文传递表


| 操作            | 从返回中提取                       | 用于                                                       |
| ------------- | ---------------------------- | -------------------------------------------------------- |
| `list`        | **`fileId`**（UUID 格式，注意：不是 `dentryId`） | info / download / mkdir / delete / list 的 --file-id 或 --parent-id；`doc copy/move` 的 --node 或 --folder |
| `list`        | `spaceId`                    | info / download / mkdir / commit 的 --space-id            |
| `list-spaces` | `rootFolderId`               | `doc copy/move` 的 --folder（复制/移动到钉盘 space 根目录时） |
| `list-spaces` | `spaceId`                    | list / info / download / mkdir / upload 的 --space-id     |
| `mkdir`       | `fileId`（UUID 格式）            | list 的 --parent-id                                       |

> **重要**：`drive list` 返回结果中同时包含 `dentryId` 和 `fileId` 两个字段。所有需要传 `--file-id` 的命令（info / download / delete）必须使用 `fileId`（即 dentryUuid），**不要使用** `dentryId`。


## 注意事项

- 不传 `--space-id` 时默认使用「我的文件」空间
- 不传 `--parent-id` 时默认操作空间根目录
- `--parent-id` 只能使用父文件夹的 `dentryUuid`。不要把 `drive info` 返回的数字型 `dentryId` 当作父目录；`dentryId` 只用于 `chat message send --dentry-id`
- `--order-by` 支持: `createTime`、`modifyTime`、`name`
- **上传文件必须使用 `dws drive upload-info` + `dws drive commit` 两步流程**（开源 v1.0.30 没有 `drive upload` 单步封装）
- `--file-name` 必须包含扩展名（如 `report.pdf`）

## 自动化脚本


| 脚本                                                     | 场景          | 用法                                    |
| ------------------------------------------------------ | ----------- | ------------------------------------- |
| [drive_tree_list.py](../scripts/drive_tree_list.py) | 递归列出钉盘目录树结构 | `python drive_tree_list.py --depth 2` |


## 相关产品

- `dingtalk-doc` (`references/doc.md`) — 文档内容读写/知识库空间，不是文件存储
- `dingtalk-chat` (`references/chat.md`) — 上传文件到 drive 后可通过 Markdown 语法发送图片/文件消息
