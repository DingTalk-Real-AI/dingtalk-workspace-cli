# 文档 (doc) 命令参考

> **渐进式文档**：本文件为路由层（命令索引 + 场景索引 + 意图判断 + 工作流），各命令的详细参数、示例和踩坑说明在 [doc/](./doc/) 目录下按需加载。

## 文档地址 (URI)

| 资源 | URI 格式 |
|------|----------|
| 文档节点 | `https://alidocs.dingtalk.com/i/nodes/{dentryUuid}` |
| edit / preview 链接 | `https://alidocs.dingtalk.com/document/{edit\|preview}?...&dentryKey={key}` |

> **操作后请返回文档 URI**：每次执行 create / read / update 等操作后，从返回数据中提取 `docUrl` 直接返回；缺失时用 `doc info --node <ID>` 补查。

## 前置条件 — 执行操作前必读

**CRITICAL — 执行对应操作前，MUST 先用 Read 工具读取以下子文件：**

1. **解析 URL / 定位文档**（几乎所有命令都需要先拿 nodeId）
   → 必读 [`doc/doc-info.md`](./doc/doc-info.md)（URL/dentryKey 提取规则、ID 边界、contentType 路由、**获取 nodeId 三种方式 A/B/C**）

2. **创建或编辑文档内容**（`doc create` / `doc update` / `doc block insert|update`）
   → 必读 [`doc/style/doc-update-workflow.md`](./doc/style/doc-update-workflow.md)（**形态优先级硬规则：JSONML > element JSON > markdown**；markdown overwrite 会丢富结构）
   - 从零创建时加读 [`doc/style/doc-create-workflow.md`](./doc/style/doc-create-workflow.md)
   - **任何 `doc create` 都必须先读 [`doc/style/doc-style-guideline.md`](./doc/style/doc-style-guideline.md) §2.0 类型决策表 + §1 硬规则**（决定骨架 + 全局约束，不读就不知道用哪种骨架）
   - 涉及 callout / 分栏 / 富 block 精修时再加读 style-guideline §4-§7 + [`doc/format/doc-jsonml-cookbook.md`](./doc/format/doc-jsonml-cookbook.md)

**未读以上文件就改写已有文档会导致富结构丢失、参数错误或样式不达标。其他命令（阅读 / 评论 / 权限 / 附件 / 下载导出 / 文件操作）按需查下方 §命令索引表跳转对应子文件加载，不必提前加载。**

## 查询命令帮助

当你不确定某个命令的具体参数、格式或可选项时，**优先执行 `--help` 查询**，不要猜测参数名或凭记忆编造。

```bash
# 查看 doc 下所有子命令
dws doc --help

# 查看具体命令的完整参数说明
dws doc list --help
dws doc create --help
dws doc block insert --help

# 查看子命令组下的所有命令
dws doc block --help
dws doc permission --help
```

规则：

- 参数名不确定时 → 先 `--help`，再调用
- 报错 "unknown flag" 时 → `--help` 确认正确的 flag 名称
- 不确定某个功能是否存在时 → `dws doc --help` 查看命令列表

## 命令索引表

> 命令名 → 单文件，按需加载子文档。复杂任务请优先看下方 §场景索引。

### 检索 / 阅读 / 元信息

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `doc search` | 关键字搜索 / 最近访问 | — | [`doc/doc-search.md`](./doc/doc-search.md) |
| `doc list` | 文件夹遍历 | — | [`doc/doc-list.md`](./doc/doc-list.md) |
| `doc info` | 文档元信息（含 contentType / extension） | `--node` | [`doc/doc-info.md`](./doc/doc-info.md) |
| `doc read` | 读取正文（markdown 或 jsonml） | `--node` | [`doc/doc-read.md`](./doc/doc-read.md) |

### 创建 / 写入 / 块编辑

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `doc create` | 创建文字文档（adoc） | `--name` | [`doc/doc-create.md`](./doc/doc-create.md) |
| `doc update` | 整篇 / 段落级更新（markdown / jsonml） | `--node` `--mode` | [`doc/doc-update.md`](./doc/doc-update.md) |
| `doc block list/insert/update/delete` | 块级精细编辑（含 JSONML 节点操作） | `--node` (+ `--block-id`) | [`doc/doc-block.md`](./doc/doc-block.md) |

### 附件 / 评论 / 权限 / 导出

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `doc media insert/download` | 附件 / 图片插入与下载 | `--node` `--file` 或 `--resource-id` | [`doc/doc-media.md`](./doc/doc-media.md) |
| `doc comment list/create/reply/create-inline` | 文档评论与划词评论 | `--node` (+ ...) | [`doc/doc-comment.md`](./doc/doc-comment.md) |
| `doc permission add/update/list` | 节点级授权 | `--node` `--user` `--role` | [`doc/doc-permission.md`](./doc/doc-permission.md) |
| `doc export` / `doc export get` | 在线文档导出 docx | `--node` `--output` | [`doc/doc-export.md`](./doc/doc-export.md) |

### 文件操作

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `doc upload` | 上传文件到文档空间/知识库 | `--file` | [`doc/doc-file-ops.md`](./doc/doc-file-ops.md) |
| `doc download` | 下载已有文件（非 ALIDOC） | `--node` `--output` | [`doc/doc-file-ops.md`](./doc/doc-file-ops.md) |
| `doc copy` / `doc move` / `doc rename` / `doc delete` | 复制/移动/重命名/删除 | `--node` (+ ...) | [`doc/doc-file-ops.md`](./doc/doc-file-ops.md) |
| `doc folder create` | 创建文件夹 | `--name` | [`doc/doc-file-ops.md`](./doc/doc-file-ops.md) |

### 排版规范 / JSONML 参考

| 资源 | 用途 | 详见 |
|------|------|------|
| 创建工作流 | 标题、位置、骨架、回读校验 | [`doc/style/doc-create-workflow.md`](./doc/style/doc-create-workflow.md) |
| 改写工作流 | 编辑形态优先级、分片 append、回读验收 | [`doc/style/doc-update-workflow.md`](./doc/style/doc-update-workflow.md) |
| 排版规范 | 文档类型 / 骨架 / 元素边界 / 颜色语义 | [`doc/style/doc-style-guideline.md`](./doc/style/doc-style-guideline.md) |
| JSONML 范例 | 所有节点类型的可复制命令 | [`doc/format/doc-jsonml-cookbook.md`](./doc/format/doc-jsonml-cookbook.md) |
| JSONML 节点结构 | 字段定义 + JSON Schema | [`doc/format/doc-jsonml-schema.md`](./doc/format/doc-jsonml-schema.md) |

## 场景索引

> 任务驱动的入口：知道任务但不确定要读哪些命令文件时，按本表**一次性 Read 文件组**，不要逐个跳。

| 任务场景 | 一次性读取 | 主命令 |
|---------|-----------|--------|
| 定位 nodeId / URL 解析 / 目录遍历 | [`doc-search.md`](./doc/doc-search.md) + [`doc-list.md`](./doc/doc-list.md) + [`doc-info.md`](./doc/doc-info.md) | search |
| 阅读已有文档 | [`doc-info.md`](./doc/doc-info.md) + [`doc-read.md`](./doc/doc-read.md) | read |
| 创建新文档 | [`doc-create.md`](./doc/doc-create.md) + [`doc-update.md`](./doc/doc-update.md)（写入管道）+ [`style/doc-create-workflow.md`](./doc/style/doc-create-workflow.md) + [`style/doc-style-guideline.md`](./doc/style/doc-style-guideline.md) | create |
| 创建文档且包含图片/截图/图文并茂 | [`doc-create.md`](./doc/doc-create.md) + [`doc-media.md`](./doc/doc-media.md) + [`style/doc-create-workflow.md`](./doc/style/doc-create-workflow.md) + [`style/doc-style-guideline.md`](./doc/style/doc-style-guideline.md) | create → media insert |
| 局部改写 / 段落替换（保真） | [`doc-read.md`](./doc/doc-read.md) + [`doc-update.md`](./doc/doc-update.md) + [`doc-block.md`](./doc/doc-block.md) + [`format/doc-jsonml-cookbook.md`](./doc/format/doc-jsonml-cookbook.md) + [`style/doc-update-workflow.md`](./doc/style/doc-update-workflow.md) | block update |
| 整篇 overwrite（可选并发检查） | [`doc-read.md`](./doc/doc-read.md) + [`doc-update.md`](./doc/doc-update.md) + [`format/doc-jsonml-schema.md`](./doc/format/doc-jsonml-schema.md) + [`style/doc-update-workflow.md`](./doc/style/doc-update-workflow.md) | update |
| 插入富 block（callout / 分栏 / 表格） | [`doc-block.md`](./doc/doc-block.md) + [`format/doc-jsonml-cookbook.md`](./doc/format/doc-jsonml-cookbook.md) + [`style/doc-style-guideline.md`](./doc/style/doc-style-guideline.md) | block insert |
| 上传图片 / 附件 | [`doc-media.md`](./doc/doc-media.md) | media insert |
| 评论 / 划词评论（含 @人） | [`doc-comment.md`](./doc/doc-comment.md)（+ `dws contact user search` 取 mention 用 userId） | comment create |
| 文档分享 / 节点级权限 | [`doc-permission.md`](./doc/doc-permission.md) | permission add |
| 导出 PDF / DOCX | [`doc-info.md`](./doc/doc-info.md) + [`doc-export.md`](./doc/doc-export.md) | export |
| 文件下载 / 上传 / 移动 / 重命名 / 复制 | [`doc-file-ops.md`](./doc/doc-file-ops.md) + [`doc-info.md`](./doc/doc-info.md)（download 前判 contentType） | upload / download / move / copy / rename |

## 意图判断

用户说"找文档/搜文档/最近文档":

- 搜索 → `search`
- 浏览 → `list`

用户说"看文档/读内容/文档内容":

- 读取 → `read`（需文档 ID 或 URL）
- 元信息 → `info`

用户说"写文档/创建文档":

- 新建文字文档（adoc）→ `doc create`
- 追加内容 → `update --mode append`
- 覆盖替换 → `update --mode overwrite`
- 指定父目录时，只有明确的文档文件夹 `nodeId` / `alidocs` 文件夹 URL 才能放入 `--folder`；上一步若只返回了纯数字 `dentryId`、`spaceId` 或 drive `parent-id`，不要把它传给 doc 的 `--folder`

> 严禁把「创建表格」路由到 `doc create`：
>
> - 用户说"创建表格/新建表格/建个电子表格/在线表格" → 走 [`dws sheet create`](./sheet.md#创建钉钉表格文档)（axls 在线电子表格）
> - 用户说"创建多维表格/新建 AI 表格/建 base/数据库表" → 走 [`dws aitable base create`](./aitable.md#创建-ai-表格)（able 多维表格）

用户说"建文件夹/新建目录":

- 创建 → `folder create`

用户说"上传文件/传文件/上传到文档/上传到知识库":

- 上传 → `upload`（需本地文件路径）
- 上传并转换 → `upload --convert`

用户说"下载/导出/下载到本地/导出文档/导出为Word/导出为docx/把文档导出来":

- **必须先判断目标文件类型**，再决定走 `export` 还是 `download`：
  - 在线文档 (alidocs/adoc) → **`export`**（格式转换后导出为 docx）
  - 已有文件（PDF、图片、附件、视频等非在线文档） → **`download`**（直接下载原始文件）
- 判断方法：
  1. 如果用户明确说了"导出文档"、"导出为Word/docx" → 直接走 `export`
  2. 如果用户明确说了"下载PDF/图片/附件" → 直接走 `download`
  3. 不确定时，先用 `info --node <ID>` 查询节点信息，根据返回的 `contentType` 字段判断：
     - `contentType` 为 `ALIDOC` → 走 `export`
     - `contentType` 为 `DOCUMENT`/`IMAGE`/`VIDEO` 等 → 走 `download`

> **严禁将"导出文档"直接路由到 `download`**。`download` 只能下载已有文件（原样下载），`export` 是将在线文档格式转换后导出为 docx，两者完全不同。

用户说"复制文档/拷贝文件/复制到":

- 复制 → `copy`（需文档 ID 或 URL + 目标位置）

用户说"移动文档/搬到/移到/转移文件":

- 移动 → `move`（需文档 ID 或 URL + 目标位置）

用户说"重命名/rename/改名/改文档名/修改文档名称/修改文档标题/把这个文档叫做...":

- 重命名 → `doc rename`（需文档 ID 或 URL + 新名称）
- 只要意图是修改文档在列表和链接中展示的名称，统一路由到 `dws doc rename --node <DOC_ID_OR_URL> --name "新名称"`；不要走 `drive`、`doc update` 或重新 `doc create`。
- 只有用户明确说"正文里的标题/章节标题/段落标题/H1 标题"时，才走 `block update`。

用户说"删除文档/删掉这个文件/移到回收站/丢掉这篇文档":

- 删除节点 → `delete`（危险操作，需确认；需文档 ID 或 URL）

用户说"插入附件/上传附件到文档/往文档里加文件/加附件":

- 插入附件 → `media insert`（需文档 ID 或 URL + 本地文件路径）

用户说"插入图片/加图片/放张图/嵌入图片/往文档里插图":

- 插入图片 → `media insert`（需文档 ID 或 URL + 本地图片文件路径）
- 注意：图片也是通过 `media insert` 作为附件块插入文档，不是通过 `block insert`

用户说"下载附件/获取附件/取出文档里的附件":

- 下载附件 → `media download`（需文档 ID 或 URL + 资源 ID）

用户说"编辑块/改段落/插入标题/删除块":

- 查看结构 → `block list`
- 插入 → `block insert`
- 修改 → `block update`
- 删除 → `block delete`

用户说"给某人开权限/分享给某人/授权某文档/把这篇文档给 xxx 看":

- 新增权限 → `permission add`（需 `--node` + `--user` + `--role`）
- 修改权限 → `permission update`
- 查看谁有权限 → `permission list`

> **关键区分**：
>
> - "把**某篇文档**授权给某人" → `doc permission add`（节点级，包括「我的文档」下的文档都支持）
> - "把**某个知识库**整体授权给某人" → `wiki member add`（容器级，但**「我的文档」个人空间不支持**）

> 补充：如果用户直接粘贴的是原始 `alidocs` URL，先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe；只有 probe 确认是 `adoc` / `file` / `folder` 后，才继续按下列意图执行。

**用户直接粘贴文档 URL（无其他指令）**:

- 默认 → `read`（读取文档内容）
- 如 URL 明显是文件夹 → `list`（列出文件夹内容）

**用户粘贴 URL + 附加指令**:

- "帮我看看这个文档" → `read`
- "这个文档的信息" → `info`
- "往这个文档追加内容" → `update --mode append`
- "把这个文档标题改成 X" / "这个文档改名为 X" → `rename`
- "把正文里的一级标题/章节标题改成 X" → `block update`

关键区分: doc(文档编辑/阅读) vs aitable(数据表格操作) vs drive(钉盘文件管理)

## 核心工作流

> 步骤性指引。"读哪些文件"组合参见上方 §场景索引；命令详细参数参见对应子文件。

```bash
# ── 工作流 1: 浏览并阅读文档 ──
dws doc list --format json                                    # 1. 浏览根目录
dws doc list --folder <DOC_FOLDER_NODE_ID> --format json      # 2. 进入子目录
dws doc info --node <DOC_ID> --format json                    # 3. 元信息（含 contentType）
dws doc read --node <DOC_ID> --format json                    # 4. 读 markdown 正文

# ── 工作流 2: 创建文档（含分片自动写入）──
dws doc folder create --name "项目资料" --format json          # 1. (可选) 文件夹
dws doc create --name "项目周报" --content-file /tmp/x.md \
  --folder <DOC_FOLDER_NODE_ID> --format json                 # 2. 创建 + 写入
dws doc read --node <DOC_ID> --format json                    # 3. 回读校验（必须）

# ── 工作流 3: 局部改写（保真，首选 JSONML）──
dws doc block list --node <DOC_ID> --content-format jsonml             # 1. 取 uuid
dws doc block list --node <DOC_ID> --content-format jsonml --block-id <UUID>   # 2. 读子树
dws doc block update --node <DOC_ID> --block-id <UUID> \
  --content-format jsonml --element '[...]'                            # 3. 写回（uuid 必须 == --block-id）
dws doc read --node <DOC_ID> --content-format jsonml                   # 4. 回读

# ── 工作流 4: 整篇 overwrite（仅在新骨架重写时）──
dws doc read --node <DOC_ID> --content-format jsonml --output /tmp/doc.json   # 1. 读出当前 JSONML
# 修改 /tmp/doc.json 中的 jsonml 数组
dws doc update --node <DOC_ID> --content-file /tmp/doc.json \
  --content-format jsonml --mode overwrite                             # 2. 写回（默认不做并发检查）
dws doc read --node <DOC_ID> --content-format jsonml                   # 3. 回读
# 担心被并发覆盖时，可加 --revision <N> 触发并发检查（详见 doc-update.md）

# ── 工作流 5: 上传 vs 插入附件 ──
dws doc upload --file ./report.pdf --folder <ID>               # 上传作为独立文件
dws doc media insert --node <DOC_ID> --file ./report.pdf       # 上传并作为附件块插入正文

# ── 工作流 6: 下载 vs 导出（先 info 判 contentType）──
dws doc info --node <NODE_ID> --format json                    # 必须先查 contentType
dws doc export --node <NODE_ID> --output ~/downloads/          # contentType=ALIDOC 走 export
dws doc download --node <NODE_ID> --output ~/downloads/        # contentType≠ALIDOC 走 download

# ── 工作流 7: 评论 + 划词（@人 需 userId）──
dws contact user search --query "张三" --format json            # 取 userId
dws doc comment create --node <DOC_ID> --content "请确认" --mention <uid> --format json
dws doc block list --node <DOC_ID> --format json                # 划词需先取 blockId / paragraph.text
dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> \
  --start 0 --end 10 --content "建议调整" --selected-text "原文" --format json

# ── 工作流 8: 权限授予（节点级）──
dws contact user search --query "张三" --format json            # 取 userId
dws doc permission add --node <DOC_ID> --user <uid1>,<uid2> --role EDITOR --format json
dws doc permission list --node <DOC_ID> --format json          # 校验

# ── 工作流 9: 文件操作（复制/移动/重命名/删除）──
# 第一步：获取 nodeId（三种方式按场景选一，命中即停，不要冗余调用）
#   方式 A（优先）：用户直接提供 URL / nodeId → 直接传 --node，跳过 search/list
#   方式 B：按关键字找：dws doc search --query "项目周报" --format json
#   方式 C：按文件夹遍历：dws doc list --folder <DOC_FOLDER_NODE_ID> --format json
# 第二步：执行（--node 支持 ID 或完整 URL；--folder 支持文件夹 nodeId 或 alidocs 文件夹 URL）
dws doc copy   --node <DOC_ID_OR_URL> --folder <TARGET_FOLDER> --format json   # 异步任务
dws doc move   --node <DOC_ID_OR_URL> --folder <TARGET_FOLDER> --format json
dws doc rename --node <DOC_ID_OR_URL> --name "新名称" --format json
# 删除是危险操作：必须先向用户展示「即将删除「文档名」到回收站」 → 用户确认 → 才加 --yes
dws doc delete --node <DOC_ID_OR_URL> --yes --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list` | `nodes[].nodeId` | read / info / update / copy / move / rename / block 操作的 --node |
| `list` | folder 类型的 `nodeId` | list 的 --folder, create / copy / move 的 --folder |
| `search` | 文档 `nodeId` / URL / `createTime` / `creatorUid` | read / info / update / copy / move / rename 的 --node；创建时间与创建者信息 |
| `create` | `nodeId` | update / block 操作的 --node |
| `folder create` | `nodeId` | create / list / upload / copy / move 的 --folder |
| `block list` | `blockId` | block insert 的 --ref-block, block update/delete 的 --block-id |
| `read --content-format jsonml` | `revision` | update --content-format jsonml 的 --revision（可选，并发检查时使用） |
| `upload` | `nodeId` / URL | 上传后文件的访问链接 |
| `download` | 本地文件路径 | 下载后的文件保存位置 |
| `copy` | `nodeId` / URL (异步，不保证返回) | 复制是异步任务，若任务未完成则不会返回新文档 ID；如需获取可稍后通过 list 查询目标文件夹 |
| `media insert` | `resourceId` | 附件已插入文档，可通过 block list 查看附件块 |
| `media download` | 附件下载链接 `downloadUrl` | 下载文档中的附件资源 |
| `block list` | attachment 块的 `resourceId` | media download 的 --resource-id |
| `comment list` | `commentList[].commentKey` | comment reply 的 --comment-key |
| `comment create` | `commentKey` | comment reply 的 --comment-key |
| `comment create-inline` | `commentKey` | comment reply 的 --comment-key |
| `block list` | `blocks[].element.id` | comment create-inline 的 --block-id |
| `block list` | `blocks[].element.paragraph.text` | 计算 create-inline 的 --start / --end 偏移量 |
| `contact user search` | `userId` | comment create/reply/create-inline 的 --mention；permission add/update 的 --user |

## 相关产品

- [wiki](./wiki.md) — 知识库空间级管理（创建/查询/列出/搜索知识库），doc 中的文档存储在 wiki 知识库中
- [aitable](./aitable.md) — 结构化数据表格（行列/字段/记录），不是富文本文档
- [drive](./drive.md) — 钉盘文件存储/上传/下载，不是文档内容编辑
- [report](./report.md) — 钉钉日志系统（日报/周报模版），不是在线文档
