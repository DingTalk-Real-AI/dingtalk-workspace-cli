# 文档 (doc) 命令参考

> 本文档与真实 CLI 严格对齐。历史版本里的 `doc upload` / `doc download` **不存在**（请改用跨产品的 drive 或 `get-doc-attachment-upload-info`）；`block insert`/`block update` 的 `--text`/`--heading`/`--level` 快捷参数**也不存在**，仅支持 `--element` JSON。

## 命令总览

### 顶级文档操作

| 子命令 | 用途 |
|-------|------|
| `search` | 搜索文档 |
| `list` | 遍历文件列表 |
| `info` | 获取文档元信息 |
| `read` | 读取文档内容（Markdown） |
| `create` | 创建文档 |
| `update` | 更新文档内容（追加 / 覆盖） |
| `delete` | 删除文档节点（⚠️ 危险） |
| `copy-document` | 复制文档到目标文件夹 |
| `move-document` | 移动文档到目标文件夹 |
| `rename-document` | 重命名文档 |
| `get-doc-attachment-upload-info` | 获取文档附件上传凭证 |

### 子树

| 子命令 | 用途 |
|-------|------|
| `folder create` | 创建文件夹 |
| `file create` | 创建文件（类型通用） |
| `block list` | 查询块元素 |
| `block insert` | 插入块元素（仅 JSON） |
| `block update` | 更新块元素（仅 JSON） |
| `block delete` | 删除块元素（⚠️ 危险） |
| `comment list` | 查询文档评论列表 |
| `comment create` | 创建全文评论 |
| `comment create-inline` | 创建划词评论 |
| `comment reply` | 回复评论 |

---

## search — 搜索文档

```
Usage:
  dws doc search [flags]
Example:
  dws doc search --query "会议纪要"
  dws doc search --extensions pdf,docx
  dws doc search --query "方案" --created-from 1700000000000 --created-to 1710000000000
  dws doc search --creator-uids uid1,uid2
  dws doc search --workspace-ids wsId1,wsId2
Flags:
      --query string              关键词 keyword（不传则返回最近访问）
      --extensions string         文件扩展名过滤，不含点号，逗号分隔（如 pdf,docx,png）。在线文档类型后缀：adoc=文字 / axls=表格 / appt=演示文稿 / awbd=白板 / adraw=画板 / amind=脑图 / able=多维表格 / aform=收集表
      --created-from string       创建时间起始（毫秒时间戳，含）createdTimeFrom
      --created-to string         创建时间截止 createdTimeTo
      --visited-from string       访问时间起始 visitedTimeFrom
      --visited-to string         访问时间截止 visitedTimeTo
      --creator-uids string       按创建者 userId 过滤，逗号分隔 creatorUserIds
      --editor-uids string        按编辑者 userId 过滤 editorUserIds
      --mentioned-uids string     按 @ 提及的 userId 过滤 mentionedUserIds
      --workspace-ids string      按知识库 ID 过滤（支持 URL），逗号分隔 workspaceIds
      --page-size string          每页数量（默认 10，最大 30）
      --page-token string         分页游标 pageToken
```

> 所有 flag 真实类型均为 string；时间戳传毫秒字符串。

---

## list — 遍历文件列表

```
Usage:
  dws doc list [flags]
Example:
  dws doc list
  dws doc list --folder <FOLDER_ID>
  dws doc list --workspace <WS_ID> --page-size 20
Flags:
      --folder string       文件夹 ID 或 URL
      --workspace string    知识库 ID
      --page-size string    每页数量（默认 50，最大 50）
      --page-token string   分页游标 pageToken
```

---

## info — 获取文档元信息

```
Usage:
  dws doc info [flags]
Example:
  dws doc info --node <DOC_ID>
  dws doc info --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"
Flags:
      --node string   文档 ID 或 URL (必填)
```

---

## read — 读取文档内容

```
Usage:
  dws doc read [flags]
Example:
  dws doc read --node <DOC_ID>
  dws doc read --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"
Flags:
      --node string   文档 ID 或 URL (必填)
```

---

## create — 创建文档

```
Usage:
  dws doc create [flags]
Example:
  dws doc create --name "项目周报"
  dws doc create --name "Q1 总结" --markdown "# Q1 总结" --folder <FOLDER_ID>
  dws doc create --name "知识库文档" --workspace <WS_ID>
Flags:
      --name string        文档名称 (必填)
      --folder string      目标文件夹 ID 或 URL folderId
      --workspace string   目标知识库 ID workspaceId
      --markdown string    文档初始 Markdown 内容
```

---

## update — 更新文档内容

```
Usage:
  dws doc update [flags]
Example:
  dws doc update --node <DOC_ID> --markdown "# 追加内容" --mode append
  dws doc update --node <DOC_ID> --markdown "# 完整替换" --mode overwrite
Flags:
      --node string       文档 ID 或 URL (必填)
      --markdown string   Markdown 内容 (必填)
      --mode string       更新模式：overwrite=覆盖 / append=追加（默认 append）
```

---

## delete — 删除文档节点

> ⚠️ 危险操作：不可逆，执行前必须向用户确认，同意后才加 `--yes`。

```
Usage:
  dws doc delete [flags]
Example:
  dws doc delete --node <DOC_ID>
Flags:
      --node string   文档 ID 或 URL nodeId (必填)
```

---

## copy-document — 复制文档

```
Usage:
  dws doc copy-document [flags]
Example:
  dws doc copy-document --node-id <DOC_ID> --target-folder-id <FOLDER_ID>
  dws doc copy-document --node-id <DOC_ID> --target-folder-id <FOLDER_ID> --workspace-id <WS_ID>
Flags:
      --node-id string            源文档 nodeId (必填)
      --target-folder-id string   目标文件夹 ID (必填)
      --workspace-id string       目标知识库 ID（跨知识库复制时填）
```

---

## move-document — 移动文档

```
Usage:
  dws doc move-document [flags]
Example:
  dws doc move-document --node-id <DOC_ID> --target-folder-id <FOLDER_ID>
Flags:
      --node-id string            源文档 nodeId (必填)
      --target-folder-id string   目标文件夹 ID (必填)
      --workspace-id string       目标知识库 ID（跨知识库移动时填）
```

---

## rename-document — 重命名文档

```
Usage:
  dws doc rename-document [flags]
Example:
  dws doc rename-document --node-id <DOC_ID> --new-name "Q2 项目总结"
Flags:
      --node-id string   文档 nodeId (必填)
      --new-name string  新名称 (必填)
```

---

## get-doc-attachment-upload-info — 获取文档附件上传凭证

用于在文档内嵌入附件：先拿凭证 → HTTP PUT 上传 → 再用 `block insert` 附上附件块。

```
Usage:
  dws doc get-doc-attachment-upload-info [flags]
Example:
  dws doc get-doc-attachment-upload-info --node-id <DOC_ID> --file-name "截图.png" --file-size 102400 --mime-type "image/png"
Flags:
      --node-id string    目标文档 nodeId (必填)
      --file-name string  文件名 (必填)
      --file-size string  文件大小（字节） (必填)
      --mime-type string  MIME 类型，如 image/png
```

---

## folder create — 创建文件夹

```
Usage:
  dws doc folder create [flags]
Example:
  dws doc folder create --name "项目资料"
  dws doc folder create --name "子文件夹" --folder <PARENT_FOLDER_ID>
Flags:
      --name string        文件夹名称 (必填)
      --folder string      父文件夹 ID 或 URL folderId
      --workspace string   目标知识库 ID
```

---

## file create — 创建文件（通用类型）

```
Usage:
  dws doc file create [flags]
Example:
  dws doc file create --name "报告草稿" --type adoc
Flags:
      --name string        文件名称 (必填)
      --type string        文件类型，如 adoc / axls / appt / awbd / amind / able / aform
      --folder string      父文件夹 ID 或 URL
      --workspace string   目标知识库 ID
```

---

## block list — 查询块元素

```
Usage:
  dws doc block list [flags]
Example:
  dws doc block list --node <DOC_ID>
  dws doc block list --node <DOC_ID> --start-index 0 --end-index 5
  dws doc block list --node <DOC_ID> --block-type heading
Flags:
      --node string         文档 ID 或 URL (必填)
      --start-index string  起始位置（从 0 开始）
      --end-index string    终止位置（含）
      --block-type string   按块类型过滤 blockType
```

---

## block insert — 插入块元素

> ⚠️ 真实 CLI 只接受 `--element` JSON，**没有** `--text`/`--heading`/`--level` 快捷参数。如需插入段落/标题，自行构造 element JSON。

```
Usage:
  dws doc block insert [flags]
Example:
  dws doc block insert --node <DOC_ID> --element '{"blockType":"paragraph","paragraph":{"text":"内容"}}'
  dws doc block insert --node <DOC_ID> --element '{"blockType":"heading","heading":{"text":"二级标题","level":2}}'
  dws doc block insert --node <DOC_ID> --element '{"blockType":"paragraph","paragraph":{"text":"在此处之前插入"}}' --ref-block <BLOCK_ID> --where before
Flags:
      --node string        文档 ID 或 URL (必填)
      --element string     块元素 JSON (必填)
      --index string       参照位置索引（从 0 开始）
      --where string       插入方向：before / after (默认 after)
      --ref-block string   参照块 ID referenceBlockId（优先级高于 --index）
```

---

## block update — 更新块元素

> 同 `block insert`：只接受 `--element` JSON。

```
Usage:
  dws doc block update [flags]
Example:
  dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --element '{"blockType":"paragraph","paragraph":{"text":"新内容"}}'
  dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --element '{"blockType":"heading","heading":{"text":"新标题","level":1}}'
Flags:
      --node string      文档 ID 或 URL (必填)
      --block-id string  目标块 ID (必填)
      --element string   块元素 JSON (必填)
```

---

## block delete — 删除块元素

> ⚠️ 不可逆：执行前必须向用户确认，同意后才加 `--yes`（`--yes` 是全局 flag）。

```
Usage:
  dws doc block delete [flags]
Example:
  dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID> --yes
Flags:
      --node string      文档 ID 或 URL (必填)
      --block-id string  目标块 ID (必填)
```

---

## comment list — 查询文档评论列表

```
Usage:
  dws doc comment list [flags]
Example:
  dws doc comment list --node <DOC_ID>
  dws doc comment list --node <DOC_ID> --type inline --resolve-status unresolved
  dws doc comment list --node <DOC_ID> --page-size 20 --next-token <TOKEN>
Flags:
      --node string             目标文档 URL 或 ID (必填)
      --page-size string        每页数量（默认 50，最大 50）
      --next-token string       分页游标
      --type string             评论类型：global=全文 / inline=划词 commentType
      --resolve-status string   解决状态：resolved / unresolved resolveStatus
```

---

## comment create — 创建全文评论

```
Usage:
  dws doc comment create [flags]
Example:
  dws doc comment create --node <DOC_ID> --content "这里需要修改"
  dws doc comment create --node <DOC_ID> --content "请 review" --mention uid1,uid2
Flags:
      --node string      目标文档 URL 或 ID (必填)
      --content string   评论文字内容（纯文本） (必填)
      --mention string   被 @ 的 userId 列表，逗号分隔 mentionedUserIds
```

---

## comment create-inline — 创建划词评论

```
Usage:
  dws doc comment create-inline [flags]
Example:
  dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --selected-text "关键结论" --start 0 --end 4 --content "这里需要依据"
Flags:
      --node string            目标文档 URL 或 ID (必填)
      --block-id string        划词所在的 blockId (必填)
      --selected-text string   被划选的文字 selectedText (必填)
      --start string           划选起始偏移 start (必填)
      --end string             划选结束偏移 end (必填)
      --content string         评论文字内容 (必填)
      --mention string         被 @ 的 userId 列表，逗号分隔
```

---

## comment reply — 回复评论

```
Usage:
  dws doc comment reply [flags]
Example:
  dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "同意"
  dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "👍" --emoji "THUMBS_UP"
  dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "请确认" --mention uid1,uid2
Flags:
      --node string         目标文档 URL 或 ID (必填)
      --comment-key string  被回复评论的 replyCommentKey（格式：13 位毫秒时间戳 + 32 位 UUID，从 list/create 获取） (必填)
      --content string      回复的文字内容（表情回复时填表情名称） (必填)
      --emoji string        表情名称（传该 flag 时作为表情贴图回复；**string 类型，非 bool**）
      --mention string      被 @ 的 userId 列表，逗号分隔
```

> ⚠️ `--emoji` 在真实 CLI 是 `string`（不是 bool），传表情代号字符串而不是空值。

---

## URL 识别与 DOC_ID 提取

当用户输入包含钉钉文档 URL 时，**必须先识别并提取 DOC_ID**，再判断意图。

### 支持的 URL 格式

| 格式 | 示例 | DOC_ID 提取方式 |
|------|------|----------------|
| `alidocs.dingtalk.com/i/nodes/{id}` | `https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA` | 取 URL 路径最后一段：`9E05BDRVQePjzLkZt2p2vE7kV63zgkYA` |
| `alidocs.dingtalk.com/i/nodes/{id}?queryParams` | `https://alidocs.dingtalk.com/i/nodes/abc123?doc_type=wiki_doc` | 忽略 query 参数，取路径最后一段：`abc123` |

### 提取规则

1. 匹配 URL 中 `alidocs.dingtalk.com` 域名
2. 取 URL path 的最后一段作为 DOC_ID（去掉 query string 和 fragment）
3. 提取出的 DOC_ID 可直接用于所有 `--node` 参数，也可将完整 URL 传给 `--node`（CLI 会自动解析）

---

## 意图判断

- "找文档/搜文档/最近文档" → 搜索 `search`；浏览 `list`
- "看文档/读内容/文档内容" → `read`（需文档 ID 或 URL）；元信息 `info`
- "写文档/创建文档" → `create`；追加 `update --mode append`；覆盖 `update --mode overwrite`
- "删文档/删除这篇" → `delete`（⚠️ 敏感，需用户确认）
- "复制文档/拷贝" → `copy-document`
- "移动文档/换目录" → `move-document`
- "重命名文档/改名" → `rename-document`
- "建文件夹/新建目录" → `folder create`
- "建文件/新建多维表格/脑图/电子表格" → `file create --type`
- "编辑块/改段落/插入标题/删除块" → `block list/insert/update/delete`
- "文档里插个附件/上传附件到文档" → `get-doc-attachment-upload-info` → HTTP PUT → `block insert` 附件块
- "看评论/回评论/@某人评论" → `comment list/create/create-inline/reply`
- 用户直接粘贴文档 URL → 默认 `read`；明显是文件夹则 `list`

关键区分：doc（文档编辑/阅读）vs aitable（数据表格操作）vs drive（钉盘文件管理）。

---

## 核心工作流

```bash
# ── 工作流 1: 浏览并阅读文档 ──
dws doc list --format json
dws doc list --folder <FOLDER_ID> --format json
dws doc info --node <DOC_ID> --format json
dws doc read --node <DOC_ID> --format json

# ── 工作流 2: 创建文档并写入内容 ──
dws doc folder create --name "项目资料" --format json             # 拿 nodeId
dws doc create --name "项目周报" --folder <FOLDER_ID> --format json
dws doc update --node <DOC_ID> --markdown "# 本周总结\n\n- 完成了 A" --mode append --format json

# ── 工作流 3: 一步创建带内容的文档 ──
dws doc create --name "会议纪要" --markdown "# 会议纪要\n\n## 议题\n1. ..." --format json

# ── 工作流 4: 上传附件到文档里 ──
dws doc get-doc-attachment-upload-info --node-id <DOC_ID> --file-name "截图.png" --file-size <bytes> --mime-type "image/png" --format json
#   → 返回 upload URL 和 mediaId
curl -X PUT -T "截图.png" "<upload URL>"
dws doc block insert --node <DOC_ID> --element '{"blockType":"attachment","attachment":{"mediaId":"<mediaId>","fileName":"截图.png"}}' --format json

# ── 工作流 5: 块级精细编辑 ──
dws doc block list --node <DOC_ID> --format json   # 拿 blockId
dws doc block insert --node <DOC_ID> --element '{"blockType":"paragraph","paragraph":{"text":"新增内容"}}'
dws doc block insert --node <DOC_ID> --element '{"blockType":"heading","heading":{"text":"新章节","level":2}}' --ref-block <BLOCK_ID> --where before
dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --element '{"blockType":"paragraph","paragraph":{"text":"修改后的内容"}}'
dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID> --yes

# ── 工作流 6: 文档评论 ──
dws doc comment list --node <DOC_ID> --format json
dws doc comment create --node <DOC_ID> --content "这里需要补充数据来源" --format json
dws doc comment create --node <DOC_ID> --content "请确认" --mention <userId1>,<userId2> --format json
dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --selected-text "结论" --start 0 --end 2 --content "依据？" --format json
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已修改" --format json
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "比心" --emoji "THUMBS_UP" --format json

# ── 工作流 7: 整理文档（复制/移动/重命名/删除）──
dws doc copy-document --node-id <DOC_ID> --target-folder-id <FOLDER_ID> --format json
dws doc move-document --node-id <DOC_ID> --target-folder-id <FOLDER_ID> --format json
dws doc rename-document --node-id <DOC_ID> --new-name "Q2 项目总结" --format json
dws doc delete --node <DOC_ID> --yes --format json   # ⚠️ 需用户确认
```

---

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list` | `nodes[].nodeId`（folder 类型） | 下一层 `list --folder`、`create --folder`、`file create --folder` |
| `list` / `search` | 文档 `nodeId` / URL | `read` / `info` / `update` / `block *` 的 `--node` |
| `create` / `folder create` / `file create` | `nodeId` | 进一步 `update` / `block *` / `list --folder` |
| `block list` | `blockId` | `block insert --ref-block` / `block update` / `block delete --block-id` |
| `get-doc-attachment-upload-info` | upload URL、`mediaId` | HTTP PUT；随后 `block insert` 附件块 |
| `comment list` | `commentList[].commentKey` | `comment reply --comment-key` |
| `comment create` / `create-inline` | `commentKey` | 同上 |
| `contact user search` | `userId` | `comment create/reply --mention` |

---

## nodeId 双格式说明

所有 `--node`（和 `--node-id`）参数同时支持两种格式，系统自动识别：
- **文档 ID**：字母数字字符串，如 `9E05BDRVQePjzLkZt2p2vE7kV63zgkYA`
- **文档 URL**：`https://alidocs.dingtalk.com/i/nodes/{dentryUuid}`

两种方式等价：
```bash
dws doc read --node 9E05BDRVQePjzLkZt2p2vE7kV63zgkYA
dws doc read --node "https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA"
```

`--folder` 参数同样支持文件夹 URL 或 ID。

---

## 注意事项

- `update --mode overwrite` 会**清空原内容后重写**，⚠️ 谨慎使用；默认 `--mode append`（追加）更安全
- `read` 返回 Markdown 格式的文档内容，仅限有"下载"权限的文档
- `create` 不传 `--folder` 和 `--workspace` 时，默认创建在"我的文档"根目录
- **`block insert` / `block update` 只接受 `--element` JSON**，历史文档写的 `--text`/`--heading`/`--level` 快捷参数不存在于真实 CLI
- `block list/insert/update/delete` 是块级精细编辑；简单内容追加建议用 `update --mode append`
- `markdown` 参数中的换行必须使用**真实换行符**（Unicode `U+000A`），不是字面量 `\n`。否则所有内容渲染在同一行
- 常见块类型：paragraph / heading / blockquote / callout / columns / orderedList / unorderedList / table / sheet / attachment / slot
- **`doc upload` / `doc download` 不存在**：
  - 文档内附件：用 `doc get-doc-attachment-upload-info` + HTTP PUT + `block insert attachment`
  - 钉盘文件：用 `drive` 产品的 upload/download 命令
- 所有数值/日期 flag 真实类型均为 `string`（auto-generated stub），按字符串传入
- `--yes` 是全局 flag（对所有 agent 模式命令生效），subcommand `--help` 里不显示但可用
- 敏感/危险（需用户确认后才加 `--yes`）：`delete`、`block delete`

---

## 相关产品

- [aitable](./aitable.md) — 结构化数据表格（行列/字段/记录），不是富文本文档
- [drive](./drive.md) — 钉盘文件存储/上传/下载（对应原文档里 `doc upload`/`doc download` 的能力）
- [report](./report.md) — 钉钉日志系统（日报/周报模版），不是在线文档
