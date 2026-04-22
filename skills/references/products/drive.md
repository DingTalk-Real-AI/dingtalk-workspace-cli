# 钉盘 (drive) 命令参考

## 命令总览

### 获取文件/文件夹列表

```
Usage:
  dws drive list [flags]
Example:
  dws drive list --max 20
  dws drive list --max 20 --parent-id <dentryUuid> --order-by name --order asc
Flags:
      --max int             每页返回数量 (默认 20，最大 100)，别名: --limit
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

### 获取文件下载链接

```
Usage:
  dws drive download [flags]
Example:
  dws drive download --file-id <dentryUuid>
Flags:
      --file-id string    文件 ID (dentryUuid) (必填)
      --space-id string   文件所属空间 ID (可选)
```

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

### 上传本地文件到钉盘

> **⚠️ 上传文件必须使用 `dws drive upload` 命令，禁止使用 `upload-info` + `curl` + `commit` 三步流程。**

```
Usage:
  dws drive upload [flags]
Example:
  dws drive upload --file ./report.pdf
  dws drive upload --file ./slides.pptx --file-name "Q1汇报.pptx"
  dws drive upload --file ./data.xlsx --parent-id <dentryUuid>
Flags:
      --file string        本地文件路径 (必填)
      --file-name string   文件显示名称 (默认使用文件名)
      --space-id string    目标空间 ID，不传则使用「我的文件」 (可选)
      --mime-type string   文件 MIME 类型，不传则自动推断 (可选)
      --parent-id string   父节点 ID (dentryUuid)，不传则上传到空间根目录 (可选)
```

`upload` 命令内部自动完成三步流程（获取凭证 → OSS PUT → 提交入库），无需手动分步操作。

## 意图判断

用户说"我的文件/钉盘/网盘/云盘" → `list`
用户说"文件详情/文件信息" → `info`
用户说"下载文件" → `download`
用户说"新建文件夹/创建目录" → `mkdir`
用户说"上传文件/传文件到钉盘" → `upload`（必须使用此命令，自动完成三步流程）

关键区分: drive(钉盘文件管理) vs doc(文档内容读写)

**drive upload vs doc upload**: 用户提到"钉盘/网盘/我的文件"→ `drive upload`；提到"知识库/文档空间/workspace"→ `doc upload`；未明确目标时默认 `drive upload`

## 核心工作流

```bash
# 1. 浏览「我的文件」根目录
dws drive list --max 20 --format json

# 2. 进入子目录 — 提取 dentryUuid 作为 parent-id
dws drive list --max 20 --parent-id <dentryUuid> --format json

# 3. 查看文件元数据
dws drive info --file-id <dentryUuid> --format json

# 4. 获取下载链接
dws drive download --file-id <dentryUuid> --format json

# 5. 创建文件夹
dws drive mkdir --name "项目资料" --format json

# 6. 上传文件（必须使用 upload 命令，禁止手动分步操作）
dws drive upload --file ./报告.pdf --format json
dws drive upload --file ./报告.pdf --parent-id <dentryUuid> --format json
```

## 上下文传递表


| 操作            | 从返回中提取                       | 用于                                                       |
| ------------- | ---------------------------- | -------------------------------------------------------- |
| `list`        | `dentryUuid`                 | info / download / mkdir / list 的 --file-id 或 --parent-id |
| `list`        | `spaceId`                    | info / download / mkdir / commit 的 --space-id            |
| `mkdir`       | `dentryUuid`                 | list 的 --parent-id                                       |


## 注意事项

- 不传 `--space-id` 时默认使用「我的文件」空间
- 不传 `--parent-id` 时默认操作空间根目录
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

