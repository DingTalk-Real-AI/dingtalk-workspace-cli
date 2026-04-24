# 文档 (doc) 命令参考

## 命令总览

### 搜索文档
```
Usage:
  dws doc search [flags]
Example:
  dws doc search --query "会议纪要"
  dws doc search
  dws doc search --extensions pdf,docx
  dws doc search --query "方案" --created-from 1700000000000 --created-to 1710000000000
  dws doc search --creator-uids uid1,uid2
  dws doc search --workspace-ids wsId1,wsId2
Flags:
      --query string              搜索关键词 (不传则返回最近访问)
      --extensions strings        按文件扩展名过滤，不含点号，逗号分隔 (如 pdf,docx,png)。支持的在线文档类型后缀名: adoc=文字, axls=表格, appt=演示文稿, awbd=白板, adraw=画板, amind=脑图, able=多维表格, aform=收集表
      --created-from int          创建时间起始 (毫秒时间戳，含)
      --created-to int            创建时间截止 (毫秒时间戳，含)
      --visited-from int          访问时间起始 (毫秒时间戳，含)
      --visited-to int            访问时间截止 (毫秒时间戳，含)
      --creator-uids strings      按创建者用户 ID 过滤，逗号分隔
      --editor-uids strings       按编辑者用户 ID 过滤，逗号分隔
      --mentioned-uids strings    按 @提及的用户 ID 过滤，逗号分隔
      --workspace-ids strings     按知识库 ID 过滤，支持知识库 URL，逗号分隔
      --page-size int             每页数量 (默认 10，最大 30)
      --page-token string         分页游标 (从上次结果的 nextPageToken 获取)
```

### 遍历文件列表
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
      --page-size int       每页数量 (默认 50，最大 50)
      --page-token string   分页游标 (从上次结果的 nextPageToken 获取)
```

### 获取文档元信息
```
Usage:
  dws doc info [flags]
Example:
  dws doc info --node <DOC_ID>
  dws doc info --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"
Flags:
      --node string   文档 ID 或 URL (必填)
```

### 读取文档内容
```
Usage:
  dws doc read [flags]
Example:
  dws doc read --node <DOC_ID>
  dws doc read --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"
Flags:
      --node string   文档 ID 或 URL (必填)
```

### 创建文档
```
Usage:
  dws doc create [flags]
Example:
  dws doc create --name "项目周报"
  dws doc create --name "Q1 总结" --markdown "# Q1 总结" --folder <FOLDER_ID>
  dws doc create --name "知识库文档" --workspace <WS_ID>
Flags:
      --name string        文档名称 (必填)
      --folder string      目标文件夹 ID 或 URL
      --workspace string   目标知识库 ID
      --markdown string    文档初始 Markdown 内容
```

### 创建其他类型文件 (表格/脑图/白板/多维表/画板)
```
Usage:
  dws doc file create [flags]
Example:
  dws doc file create --name "项目周报" --type adoc
  dws doc file create --name "数据统计" --type axls --folder <FOLDER_ID>
  dws doc file create --name "思维导图" --type amind --workspace <WS_ID>
  dws doc file create --name "子文件夹" --type folder
Flags:
      --name string        文件名称 (必填)
      --type string        文件类型 (必填): adoc=文档, axls=表格, appt=演示, adraw=白板, amind=脑图, able=多维表, folder=文件夹
      --folder string      目标文件夹 ID 或 URL
      --workspace string   目标知识库 ID 或 URL
```

### 更新文档内容
```
Usage:
  dws doc update [flags]
Example:
  dws doc update --node <DOC_ID> --markdown "# 追加内容" --mode append
  dws doc update --node <DOC_ID> --markdown "# 完整替换" --mode overwrite
Flags:
      --node string       文档 ID 或 URL (必填)
      --markdown string   Markdown 内容 (必填)
      --mode string       更新模式: overwrite=覆盖, append=追加 (默认 append)
```

### 上传文件到钉钉文档或钉钉知识库
```
Usage:
  dws doc upload [flags]
Example:
  dws doc upload --file ./report.pdf
  dws doc upload --file ./slides.pptx --name "Q1汇报.pptx" --folder <FOLDER_ID>
  dws doc upload --file ./data.xlsx --workspace <WS_ID> --convert
Flags:
      --file string        本地文件路径 (必填)
      --name string        文件显示名称 (默认使用文件名)
      --folder string      目标文件夹 ID 或 URL
      --workspace string   目标知识库 ID
      --convert            是否转换为钉钉在线文档
```

### 下载文件到本地
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

### 创建文件夹
```
Usage:
  dws doc folder create [flags]
Example:
  dws doc folder create --name "项目资料"
  dws doc folder create --name "子文件夹" --folder <PARENT_FOLDER_ID>
Flags:
      --name string        文件夹名称 (必填)
      --folder string      父文件夹 ID 或 URL
      --workspace string   目标知识库 ID
```

### 复制文档/文件
```
Usage:
  dws doc copy [flags]
Example:
  dws doc copy --node <DOC_ID> --folder <TARGET_FOLDER_ID>
  dws doc copy --node <DOC_ID> --workspace <TARGET_WS_ID>
  dws doc copy --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --folder <FOLDER_ID>
Flags:
      --node string        源文档/文件 ID 或 URL (必填)
      --folder string      目标文件夹 ID 或 URL
      --workspace string   目标知识库 ID 或 URL (不传 --folder 时复制到该知识库根目录)
```

### 移动文档/文件
```
Usage:
  dws doc move [flags]
Example:
  dws doc move --node <DOC_ID> --folder <TARGET_FOLDER_ID>
  dws doc move --node <DOC_ID> --workspace <TARGET_WS_ID>
Flags:
      --node string        源文档/文件 ID 或 URL (必填)
      --folder string      目标文件夹 ID 或 URL
      --workspace string   目标知识库 ID 或 URL (不传 --folder 时移动到该知识库根目录)
```

### 重命名文档/文件
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

### 查询块元素
```
Usage:
  dws doc block list [flags]
Example:
  dws doc block list --node <DOC_ID>
  dws doc block list --node <DOC_ID> --start-index 0 --end-index 5
  dws doc block list --node <DOC_ID> --block-type heading
Flags:
      --node string         文档 ID 或 URL (必填)
      --start-index int     起始位置 (从 0 开始)
      --end-index int       终止位置 (含)
      --block-type string   按块类型过滤
```

### 插入块元素
```
Usage:
  dws doc block insert [flags]
Example:
  dws doc block insert --node <DOC_ID> --text "这是一段文字"
  dws doc block insert --node <DOC_ID> --heading "二级标题" --level 2
  dws doc block insert --node <DOC_ID> --element '{"blockType":"paragraph","paragraph":{"text":"内容"}}'
  dws doc block insert --node <DOC_ID> --text "在此处之前插入" --ref-block <BLOCK_ID> --where before
Flags:
      --node string        文档 ID 或 URL (必填)
      --text string        快捷: 段落文本内容
      --heading string     快捷: 标题文本
      --level int          标题级别 1-6 (配合 --heading，默认 1)
      --element string     块元素 JSON (高级)
      --index int          参照位置索引 (从 0 开始)
      --where string       插入方向: before / after (默认 after)
      --ref-block string   参照块 ID (优先级高于 --index)
```

### 更新块元素
```
Usage:
  dws doc block update [flags]
Example:
  dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --text "新内容"
  dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --element '{"blockType":"heading","heading":{"text":"新标题","level":1}}'
Flags:
      --node string        文档 ID 或 URL (必填)
      --block-id string    目标块 ID (必填)
      --text string        快捷: 段落文本内容
      --heading string     快捷: 标题文本
      --level int          标题级别 1-6 (配合 --heading，默认 1)
      --element string     块元素 JSON (高级)
```

### 删除块元素

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws doc block delete [flags]
Example:
  dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID> --yes
Flags:
      --node string        文档 ID 或 URL (必填)
      --block-id string    目标块 ID (必填)
```

### 查询文档评论列表
```
Usage:
  dws doc comment list [flags]
Example:
  dws doc comment list --node <DOC_ID>
  dws doc comment list --node <DOC_ID> --type inline --resolve-status unresolved
  dws doc comment list --node <DOC_ID> --page-size 20 --next-token <TOKEN>
Flags:
      --node string            目标文档的标识，支持传入 URL 或 ID (必填)
      --page-size int          每页返回的评论数量，默认 50，最大 50
      --next-token string      分页游标，从上一次请求的返回结果中获取 (首次请求不传)
      --type string            按评论类型过滤: global (全文评论) / inline (划词评论)
      --resolve-status string  按解决状态过滤: resolved (已解决) / unresolved (未解决)
```

### 创建文档评论
```
Usage:
  dws doc comment create [flags]
Example:
  dws doc comment create --node <DOC_ID> --content "这里需要修改"
  dws doc comment create --node <DOC_ID> --content "请review" --mention uid1,uid2
Flags:
      --node string      目标文档的标识，支持传入 URL 或 ID (必填)
      --content string   评论的文字内容，纯文本 (必填)
      --mention string   被 @ 的用户 uid 列表，逗号分隔
```

### 创建划词评论 (内联评论)
```
Usage:
  dws doc comment create-inline [flags]
Example:
  dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 10 --content "这里需要修改"
  dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 5 --end 20 --content "建议调整" --selected-text "被选中的原文"
  dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 10 --content "请review" --mention uid1,uid2
Flags:
      --node string            目标文档的标识，支持传入 URL 或 ID (必填)
      --block-id string        评论锚定所在的块 ID (必填，可通过 dws doc block list 获取)
      --start int              选中文本在块内的起始字符偏移量，从 0 开始 (必填)
      --end int                选中文本在块内的结束字符偏移量，必须大于 start (必填)
      --content string         评论的文字内容，纯文本 (必填)
      --selected-text string   选中文本内容，填写后评论列表会展示「引用原文：xxx」
      --mention string         被 @ 的用户 uid 列表，逗号分隔
```

### 回复文档评论
```
Usage:
  dws doc comment reply [flags]
Example:
  dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "同意"
  dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "比心" --emoji
  dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "请确认" --mention uid1,uid2
Flags:
      --node string         目标文档的标识，支持传入 URL 或 ID (必填)
      --content string      回复的文字内容，表情回复时填写表情名称 (必填)
      --comment-key string  被回复评论的 commentKey，格式: {13位毫秒时间戳}{32位UUID}，可从 list/create 结果获取 (必填)
      --emoji               设为 true 时作为表情贴图回复 (默认 false)
      --mention string      被 @ 的用户 uid 列表，逗号分隔
```

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

### 处理流程

```
用户输入含 alidocs.dingtalk.com URL
  → 提取 DOC_ID（URL 路径最后一段）
  → 结合用户意图选择命令（默认 read）
  → 将 DOC_ID 传给 --node 参数
```

## 意图判断

用户说"找文档/搜文档/最近文档":
- 搜索 → `search`
- 浏览 → `list`

用户说"看文档/读内容/文档内容":
- 读取 → `read` (需文档 ID 或 URL)
- 元信息 → `info`

用户说"写文档/创建文档":
- 新建纯文档 (adoc) → `create`
- 追加内容 → `update --mode append`
- 覆盖替换 → `update --mode overwrite`

用户说"新建表格/脑图/白板/多维表/演示文稿":
- 用 `file create --type` 指定类型 (axls/amind/adraw/able/appt/adraw)

用户说"建文件夹/新建目录":
- 创建 → `folder create` 或 `file create --type folder`

用户说"复制文档/拷贝一份":
- 复制 → `copy` (需源 --node 和目标 --folder/--workspace)

用户说"移动文档/换个目录":
- 移动 → `move` (需源 --node 和目标 --folder/--workspace)

用户说"改文档名字/重命名":
- 改名 → `rename` (需 --node 和新 --name)

用户说"上传文件/传文件/上传到文档/上传到知识库":
- 上传 → `upload`（需本地文件路径）
- 上传并转换 → `upload --convert`

用户说"下载文件/导出文件/下载到本地":
- 下载 → `download`（需文件节点 ID 或 URL）

用户说"编辑块/改段落/插入标题/删除块":
- 查看结构 → `block list`
- 插入 → `block insert`
- 修改 → `block update`
- 删除 → `block delete`

**用户直接粘贴文档 URL（无其他指令）**:
- 默认 → `read`（读取文档内容）
- 如 URL 明显是文件夹 → `list`（列出文件夹内容）

**用户粘贴 URL + 附加指令**:
- "帮我看看这个文档" → `read`
- "这个文档的信息" → `info`
- "往这个文档追加内容" → `update --mode append`
- "编辑这个文档的标题" → `block update`

关键区分: doc(文档编辑/阅读) vs aitable(数据表格操作) vs drive(钉盘文件管理)

## 核心工作流

```bash
# ── 工作流 1: 浏览并阅读文档 ──

# 1. 浏览我的文档根目录
dws doc list --format json

# 2. 浏览子文件夹
dws doc list --folder <FOLDER_ID> --format json

# 3. 获取文档元信息 (标题、类型、权限)
dws doc info --node <DOC_ID> --format json

# 4. 读取文档内容 (Markdown 格式)
dws doc read --node <DOC_ID> --format json

# ── 工作流 2: 创建文档并写入内容 ──

# 1. (可选) 创建文件夹 — 提取 nodeId
dws doc folder create --name "项目资料" --format json

# 2. 创建文档 — 提取 nodeId
dws doc create --name "项目周报" --folder <FOLDER_ID> --format json

# 3. 写入内容 (追加模式)
dws doc update --node <DOC_ID> --markdown "# 本周总结\n\n- 完成了 A\n- 推进了 B" --mode append --format json

# ── 工作流 3: 一步创建带内容的文档 ──

dws doc create --name "会议纪要" --markdown "# 会议纪要\n\n## 议题\n\n1. ..." --format json

# ── 工作流 4: 上传本地文件到钉钉文档/知识库 ──

# 1. 上传到"我的文档"根目录
dws doc upload --file ./report.pdf

# 2. 上传到指定文件夹
dws doc upload --file ./slides.pptx --name "Q1汇报.pptx" --folder <FOLDER_ID>

# 3. 上传到知识库并转换为在线文档
dws doc upload --file ./data.xlsx --workspace <WS_ID> --convert

# ── 工作流 5: 下载文件到本地 ──

# 1. 下载到当前目录 (自动推断文件名)
dws doc download --node <NODE_ID>

# 2. 下载到指定路径
dws doc download --node <NODE_ID> --output ./report.pdf

# 3. 下载到指定目录 (自动推断文件名)
dws doc download --node <NODE_ID> --output ~/downloads/

# ── 工作流 6: 块级精细编辑 ──

# 1. 查看文档块结构 — 获取 blockId
dws doc block list --node <DOC_ID> --format json

# 2. 在文档末尾插入段落
dws doc block insert --node <DOC_ID> --text "新增内容"

# 3. 在指定块之前插入标题
dws doc block insert --node <DOC_ID> --heading "新章节" --level 2 --ref-block <BLOCK_ID> --where before

# 4. 更新某个块的内容
dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --text "修改后的内容"

# 5. 删除块
dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID> --yes

# ── 工作流 7: 文档评论管理 ──

# 1. 查看文档的所有评论
dws doc comment list --node <DOC_ID> --format json

# 2. 在文档上创建全文评论
dws doc comment create --node <DOC_ID> --content "这里需要补充数据来源" --format json

# 3. 创建评论并 @ 相关人
#    先搜索用户: dws contact user search --query "张三" --format json → 提取 userId
#    再将 userId 传入 --mention
dws doc comment create --node <DOC_ID> --content "请确认这部分内容" --mention <userId1>,<userId2> --format json

# 4. 对某段文字创建划词评论（需先 block list 拿 blockId 和字符偏移）
dws doc block list --node <DOC_ID> --format json
dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 12 \
  --content "这里的数据要复核" --selected-text "被选中的原文片段" --format json

# 5. 回复某条评论（commentKey 从 list 或 create 返回中获取）
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已修改" --format json

# 6. 用表情回复评论
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "比心" --emoji --format json

# ── 工作流 8: 创建非文档类型文件 ──

# 创建表格 / 脑图 / 白板 / 多维表 / 演示文稿
dws doc file create --name "销售数据" --type axls --folder <FOLDER_ID> --format json
dws doc file create --name "需求脑图" --type amind --workspace <WS_ID> --format json
dws doc file create --name "Q1 立项会" --type appt --format json

# ── 工作流 9: 整理文档结构 (复制 / 移动 / 重命名) ──

# 1. 把模板复制到目标目录作为新工作件
dws doc copy --node <TEMPLATE_DOC_ID> --folder <TARGET_FOLDER_ID> --format json

# 2. 把文档从个人空间挪到团队知识库
dws doc move --node <DOC_ID> --workspace <WS_ID> --format json

# 3. 重命名
dws doc rename --node <DOC_ID> --name "项目周报 v2" --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list` | `nodes[].nodeId` | read / info / update / block 操作的 --node |
| `list` | folder 类型的 `nodeId` | list 的 --folder, create 的 --folder |
| `search` | 文档 `nodeId` / URL / `createTime` / `creatorUid` | read / info / update 的 --node；创建时间与创建者信息 |
| `create` | `nodeId` | update / block 操作的 --node |
| `folder create` | `nodeId` | create / list / upload 的 --folder |
| `block list` | `blockId` | block insert 的 --ref-block, block update/delete 的 --block-id |
| `upload` | `nodeId` / URL | 上传后文件的访问链接 |
| `download` | 本地文件路径 | 下载后的文件保存位置 |
| `comment list` | `commentList[].commentKey` | comment reply 的 --comment-key |
| `comment create` / `comment create-inline` | `commentKey` | comment reply 的 --comment-key |
| `block list` | `blockId` + 文本内容 | comment create-inline 的 --block-id 及 --start/--end 计算 |
| `contact user search` | `userId` | comment create / create-inline / reply 的 --mention |
| `file create` | `nodeId` | 后续 read / update / block 操作的 --node（仅 adoc 支持 read/update，axls/amind 等类型用各自产品的命令） |
| `copy` / `move` | 新 `nodeId`（copy）或原 nodeId（move） | 后续 read / info 等的 --node |

## nodeId 双格式说明

所有 `--node` 参数同时支持两种格式，系统自动识别：
- **文档 ID**: 字母数字字符串，如 `9E05BDRVQePjzLkZt2p2vE7kV63zgkYA`
- **文档 URL**: `https://alidocs.dingtalk.com/i/nodes/{dentryUuid}`，如 `https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA`

两种方式等价，以下命令效果相同：
```bash
dws doc read --node 9E05BDRVQePjzLkZt2p2vE7kV63zgkYA
dws doc read --node "https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA"
```

`--folder` 参数同样支持文件夹 URL 或 ID。

## 注意事项

- `update --mode overwrite` 会**清空原内容后重写**，⚠️ 谨慎使用；默认 `--mode append` (追加) 更安全
- `read` 返回 Markdown 格式的文档内容，仅限有"下载"权限的文档
- `create` 不传 `--folder` 和 `--workspace` 时，默认创建在"我的文档"根目录
- `block list/insert/update/delete` 是块级精细编辑，适合结构化修改；简单内容追加建议用 `update --mode append`
- `block insert` 优先使用 `--text` 或 `--heading` 快捷方式；复杂块类型 (table, callout 等) 使用 `--element` JSON
- `markdown` 参数中的换行必须使用**真实换行符**（即实际的换行字符，Unicode `U+000A`），而不是字面量字符串 `\n`（反斜杠加字母 n）。在通过程序或大模型构造此参数时，请确保字符串在发送前已正确反转义。如果传入的是两个字符的字面量 `\n`，所有内容将渲染在同一行，导致标题、段落和表格格式全部错乱。
- 块类型包括: paragraph, heading, blockquote, callout, columns, orderedList, unorderedList, table, sheet, attachment, slot
- 关键区分: doc(文档编辑/阅读) vs aitable(数据表格操作) vs drive(钉盘文件管理)
- `upload` 支持上传任意类型文件 (PDF、Office、图片等) 到钉钉文档空间或知识库；`--convert` 可将 Office 文件转换为钉钉在线文档
- `upload` 是三步自动完成的流程 (获取凭证 → OSS 上传 → 提交入库)，无需手动分步操作
- `download` 是两步自动完成的流程 (获取下载链接 → HTTP GET 下载)，支持自动推断文件名；`--output` 可指定文件路径或目录
- `create` 只能建"文档"（adoc）；要建表格/脑图/白板/多维表/演示/文件夹，用 `file create --type`
- `copy` 需要对源节点有"阅读"权限、对目标目录有"编辑"权限；`move` 需要对源节点有"管理"权限
- `copy` / `move` 不传 `--folder` 时，`--workspace` 表示放到知识库根目录；两者都不传则回落到"我的文档"
- `comment create` 是全文评论；`comment create-inline` 是划词评论，必须先 `block list` 拿到 `blockId` 并确定 `--start` / `--end` 偏移（按块内纯文本字符算，从 0 开始）

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [doc_create_and_write.py](../../scripts/doc_create_and_write.py) | 创建文档并写入 Markdown 内容 | `python doc_create_and_write.py --name "周报" --content "# 本周总结"` |

## 相关产品

- [aitable](./aitable.md) — 结构化数据表格（行列/字段/记录），不是富文本文档
- [drive](./drive.md) — 钉盘文件存储/上传/下载，不是文档内容编辑
- [report](./report.md) — 钉钉日志系统（日报/周报模版），不是在线文档
