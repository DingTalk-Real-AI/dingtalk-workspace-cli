# doc 文件操作（upload / download / copy / move / rename / delete + folder create）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流

> **弃用提示（文件管理命令正在迁移到 drive / wiki）**：本文所列 `doc` 文件管理命令虽仍能跑，但执行时会打印弃用警告，请优先改用 `drive` / `wiki` 对应命令：
> - `doc download` → **`dws drive download`**（下载已有文件；在线文档导出 docx 仍走 `doc export`）
> - `doc folder create` → **`dws drive mkdir`**（个人空间/钉盘）或 **`dws wiki node create --workspace <id> --type folder`**（知识库内）
> - `doc upload` / `doc copy` / `doc move` / `doc rename` / `doc delete` 也在同一迁移方向（→ `drive`），命令用法以 `drive` 为准。

---

## doc upload（上传文件到钉钉文档/知识库）

```
Usage:
  dws doc upload [flags]
Example:
  dws doc upload --file ./report.pdf
  dws doc upload --file ./slides.pptx --name "Q1汇报.pptx" --folder <DOC_FOLDER_NODE_ID>
  dws doc upload --file ./data.xlsx --workspace <WS_ID> --convert
Flags:
      --file string        本地文件路径 (必填)
      --name string        文件显示名称 (默认使用文件名)
      --folder string      目标文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id 这类纯数字 ID
      --workspace string   目标知识库 ID
      --convert            是否转换为钉钉在线文档
```

### 关键说明

- `upload` 是三步自动完成的流程（获取凭证 → OSS 上传 → 提交入库），无需手动分步操作。
- 支持上传任意类型文件（PDF、Office、图片等）到钉钉文档空间或知识库。
- `--convert` 可将 Office 文件转换为钉钉在线文档。
- **`doc upload` vs `drive upload`**：用户提到「知识库 / 文档空间 / workspace」→ `doc upload`；提到「钉盘 / 网盘 / 我的文件」→ `drive upload`；未明确目标时默认 `drive upload`。
- 与 [`./doc-media.md`](./doc-media.md) `media insert` 的区别：`upload` 上传到文档空间作为**独立文件**；`media insert` 作为**附件块插入到文档正文中**。

---

## doc download（下载文件到本地）

> **路由前置判断**：用户说「下载 / 导出」时**必须**先用 [`./doc-info.md`](./doc-info.md) `info --node <ID> --format json` 查 `contentType`：
>
> - `contentType` 为 `ALIDOC`（在线文档）→ 走 [`./doc-export.md`](./doc-export.md) `export`
> - `contentType` 为 `DOCUMENT`/`IMAGE`/`VIDEO` 等（已有文件）→ 走本命令 `download`

```
Usage:
  dws doc download [flags]
Example:
  dws doc download --node <NODE_ID>
  dws doc download --node <NODE_ID> --output ./report.pdf
  dws doc download --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --output ~/downloads/
Flags:
      --node string     文件节点 ID 或 URL (必填)
      --output string   本地保存路径 (文件路径或目录，必填)
```

### 关键说明

- `download` 是两步自动完成的流程（获取下载链接 → HTTP GET 下载），支持自动推断文件名；`--output` 可指定文件路径或目录。
- **严禁将「导出文档」直接路由到 `download`**——`download` 只能下载已有文件（原样下载），`export` 是将在线文档格式转换后导出为 docx。

---

## doc copy（复制文档/文件）

```
Usage:
  dws doc copy [flags]
Example:
  dws doc copy --node <DOC_ID> --folder <TARGET_DOC_FOLDER_NODE_ID>
  dws doc copy --node <DOC_ID> --workspace <TARGET_WS_ID>
  dws doc copy --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --folder <DOC_FOLDER_NODE_ID>
Flags:
      --node string        文档/文件 ID 或 URL (必填)
      --folder string      目标文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id 这类纯数字 ID
      --workspace string   目标知识库 ID 或 URL (不传 --folder 时复制到该知识库根目录)
```

### 关键说明

- 需要对源文档有「阅读」权限，且对目标文件夹有「编辑」权限。
- `copy` 是异步任务，若任务未完成则不会返回新文档 ID；如需获取可稍后通过 [`./doc-list.md`](./doc-list.md) 查询目标文件夹。

---

## doc move（移动文档/文件）

```
Usage:
  dws doc move [flags]
Example:
  dws doc move --node <DOC_ID> --folder <TARGET_DOC_FOLDER_NODE_ID>
  dws doc move --node <DOC_ID> --workspace <TARGET_WS_ID>
  dws doc move --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --folder <DOC_FOLDER_NODE_ID>
Flags:
      --node string        文档/文件 ID 或 URL (必填)
      --folder string      目标文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id 这类纯数字 ID
      --workspace string   目标知识库 ID 或 URL (不传 --folder 时移动到该知识库根目录)
```

### 关键说明

- 需要对源文档有「管理」权限，且对目标文件夹有「编辑」权限；移动后原位置的文档将不再存在。

---

## doc rename（重命名文档/文件）

```
Usage:
  dws doc rename [flags]
Example:
  dws doc rename --node <DOC_ID> --name "新名称"
  dws doc rename --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --name "项目周报 v2"
Flags:
      --node string   文档/文件 ID 或 URL (必填)
      --name string   新名称 (必填)
```

### 关键说明

- 需要对文档有「编辑」权限。
- 只要意图是修改文档在列表和链接中展示的名称，统一路由到本命令；**不要**走 `drive`、`doc update` 或重新 `doc create`。
- 只有用户明确说「正文里的标题/章节标题/段落标题/H1 标题」时，才走 [`./doc-block.md`](./doc-block.md) `block update`。

---

## doc delete（删除到回收站）

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws doc delete [flags]
Example:
  dws doc delete --node <DOC_ID> --format json    # 查询 nodeId: dws drive search --query "..." 或 dws drive list
Flags:
      --node string   文档/文件 ID 或 URL (必填)
```

权限要求: 对文档有「管理」权限。

正确流程：1.向用户展示「即将删除「文档名」到回收站」 → 2.等用户确认 → 3.执行 `dws doc delete --node <ID> --yes`。

---

## doc folder create（创建文件夹）

```
Usage:
  dws doc folder create [flags]
Example:
  dws doc folder create --name "项目资料"
  dws doc folder create --name "子文件夹" --folder <PARENT_DOC_FOLDER_NODE_ID>
Flags:
      --name string        文件夹名称 (必填)
      --folder string      父文档文件夹 nodeId 或 alidocs 文件夹 URL；不要传 drive dentryId/parent-id 这类纯数字 ID
      --workspace string   目标知识库 ID
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `upload` | `nodeId` / URL | 上传后文件的访问链接；后续 `--node` 入参 |
| `download` | 本地文件路径 | 下载后的文件保存位置 |
| `copy` | `nodeId` / URL（异步，不保证返回） | 异步任务，未完成时不返回新文档 ID；可用 [`./doc-list.md`](./doc-list.md) 在目标文件夹查询 |
| `folder create` | `nodeId` | [`./doc-create.md`](./doc-create.md) / `list` / `upload` / `copy` / `move` 的 `--folder` |

## 常用模板

```bash
# ── upload ──
# 上传到「我的文档」根目录
dws doc upload --file ./report.pdf

# 上传到指定文件夹
dws doc upload --file ./slides.pptx --name "Q1汇报.pptx" --folder <DOC_FOLDER_NODE_ID>

# 上传到知识库并转换为在线文档
dws doc upload --file ./data.xlsx --workspace <WS_ID> --convert

# ── download（仅限非 ALIDOC 文件）──
# 自动推断文件名，下到目录
dws doc download --node <NODE_ID> --output ~/downloads/

# 指定文件路径
dws doc download --node <NODE_ID> --output ./report.pdf

# ── copy ──
# 复制到指定文件夹（异步）
dws doc copy --node <DOC_ID_OR_URL> --folder <TARGET_DOC_FOLDER_NODE_ID> --format json

# 复制到目标知识库根目录
dws doc copy --node <DOC_ID_OR_URL> --workspace <TARGET_WS_ID> --format json

# ── move ──
# 移动到指定文件夹
dws doc move --node <DOC_ID_OR_URL> --folder <TARGET_DOC_FOLDER_NODE_ID> --format json

# 移动到知识库根目录
dws doc move --node <DOC_ID_OR_URL> --workspace <TARGET_WS_ID> --format json

# ── rename ──
dws doc rename --node <DOC_ID_OR_URL> --name "新名称" --format json

# ── delete（用户确认后才加 --yes）──
dws doc delete --node <DOC_ID_OR_URL> --yes --format json

# ── folder create ──
# 在「我的文档」根目录创建
dws doc folder create --name "项目资料" --format json

# 在指定父文件夹下创建
dws doc folder create --name "子文件夹" --folder <PARENT_DOC_FOLDER_NODE_ID> --format json

# 在知识库下创建
dws doc folder create --name "项目资料" --workspace <WS_ID> --format json
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令族）
- [`./doc-info.md`](./doc-info.md)（前置：判断 contentType 决定 download / export）
- [`./doc-export.md`](./doc-export.md)（在线文档格式转换导出 docx）
- [`./doc-media.md`](./doc-media.md)（文件作为附件块插入文档正文，与 upload 区分）
- [`./doc-search.md`](./doc-search.md) / [`./doc-list.md`](./doc-list.md)（拿 nodeId 的入口）
