# 文档 (doc) 命令参考

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
  dws doc info --node "https://alidocs.dingtalk.com/document/edit?dentryKey=<DENTRY_KEY>"
  dws doc info --node "https://alidocs.dingtalk.com/document/preview?dentryKey=<DENTRY_KEY>"
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
  dws doc read --node "https://alidocs.dingtalk.com/document/edit?dentryKey=<DENTRY_KEY>"
  dws doc read --node "https://alidocs.dingtalk.com/document/preview?dentryKey=<DENTRY_KEY>"
Flags:
      --node string   文档 ID 或 URL (必填)
```

### 创建文档
```
Usage:
  dws doc create [flags]
Example:
  dws doc create --name "项目周报"
  dws doc create --name "Q1 总结" --content "# Q1 总结" --folder <FOLDER_ID>
  dws doc create --name "知识库文档" --workspace <WS_ID>
Flags:
      --name string           文档名称 (必填)
      --folder string         父文件夹 nodeId / URL
      --workspace string      Workspace ID
      --content string        初始 Markdown 内容
      --content-file string   从文件读取初始 Markdown
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
  dws doc update --node <DOC_ID> --content "# 追加内容" --mode append
  dws doc update --node <DOC_ID> --content "# 完整替换" --mode overwrite
Flags:
      --node string          文档 nodeId / URL
      --content string       Markdown 内容
      --content-file string  从文件读取 Markdown
      --mode string          更新模式: append / overwrite
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

### 文件内容获取路由规则

> 当用户请求"分析/查看/读取某个文件内容"时，**必须先调用 `dws doc info` 获取文件元数据**，再根据返回的 `contentType` 和 `extension` 字段选择对应链路：

| contentType | extension | 操作 | 命令 |
|-------------|-----------|------|------|
| ALIDOC | adoc | 在线获取 Markdown 内容 | `dws doc read --node <ID>` |
| ALIDOC | axls | 在线读取表格数据 | `dws sheet list --node <ID>` → `dws sheet range read --node <ID>` |
| ALIDOC | able | 在线查询多维表格记录 | `dws aitable table get --base-id <BASE_ID>` → `dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID>` |
| 非 ALIDOC | — | **不支持在线分析** | 告知用户需下载到本地后查看 |

**关键规则**：非 ALIDOC 类型文件（PDF/Word/图片/视频等）不支持在线分析，用户可以选择下载后本地查看。

### 格式保留度声明（adoc ↔ markdown lossy projection）

`dws doc read --format json` 返回的 `markdown` 字段是钉钉 adoc 文档的**有损投影**。源 adoc 中以下富格式属性在 markdown 这一层**没有对应表达，会被丢弃**：

- 行高（row height）
- 单元格背景色（cell background color）
- 字体大小（font size）
- 字体颜色的**渐变形态**（`radial-gradient` / `linear-gradient` — 文本会以 `<span style="color: radial-gradient(...)">` 字面保留，但 CSS `color` 不接受渐变，浏览器无法渲染，**字符串保留但语义已死**）
- 块级缩进 / 表格列宽
- 合并单元格状态
- checkbox 视觉样式（颜色、形状）
- 任何 SVG / 嵌入式画板的渲染细节

**硬指引（任何 contentType=ALIDOC 文档都适用）**：

- 当用户要求"按模板生成同形态变体 / 参照这个生成 / 复刻 / 仿照这个文档"时，**禁止**走 doc read → create_file → doc create 链路——这条链路会做两次 lossy projection（adoc → markdown 丢一次，markdown → adoc 重建再丢一次），最终交付物只有文本内容，富格式全部失真。
- 正确路径：`dws doc copy`（adoc 层面整文档保形复制）+ `dws doc rename` + `dws doc block list / dws doc block update`（在副本上做局部修改）。详见 [best_practices/04-document.md](../best_practices/04-document.md#template-based-generation) 的 `template-based-generation` recipe。
- 当用户提到 `行高 / 颜色 / 字号 / 表格样式 / 周末高亮` 等富格式诉求时，agent 必须在动手前显式声明能力边界："这些属性 markdown 无法表达，需走 copy + block update 路径"，避免静默走破坏链。

### 内容写入管道（create / update 共用）

> **关键原则**：CLI 内置自动分片。超长内容（>10000 字符）自动按 markdown 结构切分后逐片写入，对调用方透明。写入完成后由调用方自行决定是否回读确认。

#### 输入方式选择

| 场景 | 推荐方式 | 说明 |
|------|---------|------|
| 短文本（<2KB，无换行/表格/特殊字符） | `--content "..."` | 字面量传入，最简单 |
| 长文本（≥2KB）、含换行、含表格 | `--content-file ./file.md` | **必须**用文件路径，避免 shell escape 和截断 |
| 含特殊字符（`"`、`\`、`$`、`` ` ``） | `--content-file ./file.md` | 字面量传入会被 shell 转义破坏 |
| 管道/heredoc 输入 | `--content -` 或 `cat file \| dws doc ...` | 从 stdin 读取 |

#### 自动分片行为

当内容超过 10000 字符时，CLI 自动执行：
1. **create**: 先创建空文档拿 `nodeId`，再按 markdown 标题边界切分后逐片 append
2. **update (overwrite)**: 第一片用 overwrite，后续片用 append
3. **update (append)**: 所有片段用 append

分片策略按优先级：H1 标题 → H2 标题 → H3 标题 → 空行（段落边界）→ 硬切（保留表格/代码块完整性）

如果某片写入超时，自动将分片大小减半重试（最小 5000 字符，低于此值报错）。

#### 输出格式

写入成功后输出 JSON（混合 `[INFO]` 进度行）：

```json
{"success": true, "nodeId": "xxx", "chunksWritten": 3}
```

| 字段 | 说明 |
|------|------|
| `nodeId` | 文档节点 ID，可用于后续读取或追加 |
| `chunksWritten` | 实际写入的分片数（1 = 单次写入） |

#### 内容完整性验证（必读）

CLI **不会**自动执行回读验证。**Agent 必须在文档写入完成后主动回读确认**——这是硬约束，不是建议：

1. 使用 `dws doc read --node <nodeId>` 读取写入后的文档内容
2. 检查关键段落是否完整、顺序是否正确
3. 如发现内容缺失或异常，使用 `dws doc update --mode append` 补写缺失部分

> **何时回读**：每次 create / update 操作完成后**必须**回读。
> - 单次写入（≤10000 字符）：写完立即回读一次
> - 分片写入（>10000 字符）：所有分片写完后回读一次全文，校验关键标题与段落完整性
> - 破坏性 overwrite（`--mode overwrite --yes`）：**必须**回读，确认 overwrite 未被后端静默降级为 append（详见 [best_practices/04-document.md "doc update 回读校验规范"](../best_practices/04-document.md#doc-update-回读校验规范)）
> - 连续多次编辑同一文档：可在全部编辑完成后统一回读一次
>
> **禁止**在未回读的情况下直接向用户报告"已完成 / 更新成功"。Agent 自陈"完成"必须基于回读结果，而非工具调用的 `success=true` 字段。

#### 进度输出示例

```
[INFO] 内容较长 (25000 字符)，自动分片写入...
[INFO] 已创建空文档 (nodeId=abc123)，开始分片写入...
[INFO] 写入分片 (1/3)，10000 字符...
[INFO] 写入分片 (2/3)，10000 字符...
[INFO] 写入分片 (3/3)，5000 字符...
[INFO] 全部 3 个分片写入完成
{"success": true, "nodeId": "abc123", "chunksWritten": 3}
```

#### CONTENT_TRUNCATED 错误

当分片写入持续超时且减半到最小阈值仍失败时，返回 `CONTENT_TRUNCATED` 错误码。应对策略：
1. 检查网络和后端服务状态
2. 已写入的部分内容可通过 `dws doc read --node <NODE_ID>` 查看
3. 从断点处手动用 `dws doc update --mode append` 继续追加

### 删除文档/文件到回收站

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws doc delete [flags]
Example:
  dws doc delete --node <DOC_ID> --format json    # 查询 nodeId: dws doc search --query "..." 或 dws doc list
Flags:
      --node string   文档/文件 ID 或 URL (必填)
```

权限要求: 对文档有"管理"权限。

### 下载文档附件
```
Usage:
  dws doc media download [flags]
Example:
  dws doc media download --node <DOC_ID> --resource-id <RESOURCE_ID>
  dws doc media download --node "https://alidocs.dingtalk.com/i/nodes/xxx" --resource-id <RESOURCE_ID>
Flags:
      --node string          目标文档的标识，支持传入 URL 或 ID (必填)
      --resource-id string   附件资源 ID，可通过 dws doc block list 获取 (必填)
```

### 上传附件并插入文档
```
Usage:
  dws doc media insert [flags]
Example:
  dws doc media insert --node <DOC_ID> --file ./report.pdf
  dws doc media insert --node <DOC_ID> --file ./data.bin --name "数据文件.dat" --mime-type application/octet-stream
  dws doc media insert --node <DOC_ID> --file ./image.png --ref-block <BLOCK_ID> --where before
Flags:
      --node string        目标文档的标识，支持传入 URL 或 ID (必填)
      --file string        本地文件路径 (必填)
      --name string        附件显示名称 (默认使用文件名)
      --mime-type string   文件 MIME 类型 (默认根据扩展名推断)
      --index int          插入位置索引
      --where string       相对位置: before / after (配合 --ref-block)
      --ref-block string   参考块 ID (配合 --where)
```

### 添加文档权限（节点级授权）
```
Usage:
  dws doc permission add [flags]
Example:
  dws doc permission add --node <DOC_ID> --users uid1 --role READER
  dws doc permission add --node <DOC_ID> --users uid1,uid2 --role EDITOR
  dws doc permission add --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --users uid1 --role MANAGER
Flags:
      --node string        目标文档/文件夹的 ID 或 URL (必填)
      --users strings      被授权的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string        授予的角色 (必填，大小写敏感，必须全大写): MANAGER (管理者) / EDITOR (可编辑) / DOWNLOADER (可下载) / READER (可阅读)
      --workspace string   所属知识库 ID (选填，仅用于辅助构造返回的 docUrl，业务实际依赖 nodeId)
```

> **重要约束**：
> - 仅支持 USER 类型授权。
> - 角色枚举严格大写：MANAGER / EDITOR / DOWNLOADER / READER（OWNER 不可通过此接口添加）。
> - 操作者需在该节点具备「可编辑（EDITOR）」及以上角色（OWNER / MANAGER / EDITOR）。
> - 授权对象是文档节点本身，不需要也不应该用 `wiki member add`（那个是知识库容器级授权）。

### 修改文档权限（节点级）
```
Usage:
  dws doc permission update [flags]
Example:
  dws doc permission update --node <DOC_ID> --users uid1 --role EDITOR
  dws doc permission update --node <DOC_ID> --users uid1,uid2 --role READER
Flags:
      --node string        目标文档/文件夹的 ID 或 URL (必填)
      --users strings      目标用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string        新角色 (必填，大小写敏感，必须全大写): MANAGER / EDITOR / DOWNLOADER / READER
      --workspace string   所属知识库 ID (选填)
```

### 列出文档权限（节点级）
```
Usage:
  dws doc permission list [flags]
Example:
  dws doc permission list --node <DOC_ID>
  dws doc permission list --node <DOC_ID> --limit 50
  dws doc permission list --node <DOC_ID> --filter-role EDITOR
Flags:
      --node string          目标文档/文件夹的 ID 或 URL (必填)
      --workspace string     所属知识库 ID (选填)
      --limit int             返回数量上限，最大 200 (默认 50)
      --filter-role string   按角色过滤: MANAGER / EDITOR / DOWNLOADER / READER (选填)
```

> 接口不支持游标分页，使用 `--limit` 一次性拉取。

### 导出在线文档为 docx（一体化命令）
```
Usage:
  dws doc export [flags]
Example:
  dws doc export --node "https://alidocs.dingtalk.com/i/nodes/xxx" --output ./exported.docx
  dws doc export --node <DOC_ID> --output ~/downloads/
Flags:
      --node string           要导出的文档标识，支持文档 URL 或 dentryUuid (必填)
      --output string         本地保存路径，文件路径或目录 (必填)
      --export-format string  导出格式，当前仅支持 docx (默认)
```

CLI 内部自动完成：提交导出任务 → 渐进式退避轮询（最多约 5 分钟）→ 成功后自动下载文件。
**只需一条命令，无需手动轮询。**

### 查询导出任务结果（手动兜底）
```
Usage:
  dws doc export get [flags]
Example:
  dws doc export get --job-id <JOB_ID>
Flags:
      --job-id string   导出任务 ID (必填)
```

仅在 `dws doc export` 超时或中断后，用于手动查询任务状态。通常不需要调用。


## URL 识别与 DOC_ID 提取

当用户输入包含钉钉文档 URL 时，**必须先识别并提取 DOC_ID**，再判断意图。

补充：如果这是用户直接提供的原始 `alidocs` URL，必须先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe 一次确认真实类型，再判断是否继续走 `doc`。

### 支持的 URL 格式

| 格式 | 示例 | DOC_ID 提取方式 |
|------|------|----------------|
| `alidocs.dingtalk.com/i/nodes/{id}` | `https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA` | 取 URL 路径最后一段：`9E05BDRVQePjzLkZt2p2vE7kV63zgkYA` |
| `alidocs.dingtalk.com/i/nodes/{id}?queryParams` | `https://alidocs.dingtalk.com/i/nodes/abc123?doc_type=wiki_doc` | 忽略 query 参数，取路径最后一段：`abc123` |
| `alidocs.dingtalk.com/document/{edit\|preview}?...&dentryKey={key}` | `https://alidocs.dingtalk.com/document/edit?dentryKey=wo1g3x54FzVEJ5yE` | **不要提取 `dentryKey` 单独使用**，必须将完整 URL 原样传给 `--node` |

### 提取规则

1. 匹配 URL 中 `alidocs.dingtalk.com` 域名
2. 路径为 `/i/nodes/{id}` 时，取 URL path 的最后一段作为 DOC_ID（去掉 query string 和 fragment）
3. 路径为 `/document/edit` 或 `/document/preview` 且 query 含 `dentryKey` 时，**禁止**提取 `dentryKey` 当 DOC_ID；将整段 URL 原样传给 `--node`，CLI 会自动解析（追踪参数如 `utm_source`、`chInfo` 也不必清理）
4. 提取出的 DOC_ID 可直接用于所有 `--node` 参数，也可将完整 URL 传给 `--node`（CLI 会自动解析）

### ID 边界与参数映射

- `nodeId` 是 `doc` 命令的统一节点标识；文档、文件夹、文件都通过 `nodeId` 或完整 `alidocs` URL 传给 `--node` / `--folder`。
- `dentryUuid` 是 `alidocs` URL `/i/nodes/{dentryUuid}` 的最后一段，在 `doc` 场景中等价于可传入 CLI 的 `nodeId`；不要把它改写成数字 ID。
- `dentryId` 通常是纯数字，**不是** `doc` 的 `nodeId`，也不是 `doc --folder` 的目标文件夹 ID；不要把数字 `dentryId` 当作 `--node`、`--folder`，也不要当作隐藏兼容 alias（如 `--parent-id` / `--parent-folder-id`）的取值。
- 目标父文件夹的 canonical flag 是 `--folder <folderNodeId或folderUrl>`，目标知识库使用 `--workspace <workspaceId或workspaceUrl>`。CLI 可能接受 `--parent-id` / `--parent-folder-id` 作为隐藏兼容 alias，但 skill 正文和示例应统一教 `--folder`，不要把 drive/chat 链路里的数字 parentId 搬到 doc 命令。
- 如果上下文只有数字 `dentryId`，但用户要读、改、移动、复制、重命名文档，先通过 `doc search` / `doc list` / 用户提供的 `alidocs` URL 获取 `nodeId` / `dentryUuid`，不要用数字 `dentryId` 重试为父目录参数。

### 处理流程

```
用户输入含 alidocs.dingtalk.com URL
  → 若是用户直接提供的原始 URL，先按链接规范做 probe
  → 提取 DOC_ID（URL 路径最后一段）
  → 结合用户意图选择命令（doc 默认 read，folder 默认 list，file 默认 download）
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
- 新建文字文档（adoc）→ `doc create`
- 追加内容 → `update --mode append`
- 整篇覆盖替换 → 先 `update --mode overwrite --dry-run` 预览，用户确认后再 `update --mode overwrite --yes`
- 指定父目录时，只有明确的文档文件夹 `nodeId` / `alidocs` 文件夹 URL 才能放入 `--folder`；上一步若只返回了纯数字 `dentryId`、`spaceId` 或 drive `parent-id`，不要把它传给 doc 的 `--folder`

> 严禁把「创建表格」路由到 `doc create`：
> - 用户说"创建表格/新建表格/建个电子表格/在线表格" → 走 [`dws sheet create`](./sheet.md)（axls 在线电子表格）
> - 用户说"创建多维表格/新建 AI 表格/建 base/数据库表" → 走 [`dws aitable base create`](./aitable.md)（able 多维表格）

用户说"新建脑图/白板/多维表/演示文稿":
- 用 `file create --type` 指定类型 (axls/amind/adraw/able/appt/adraw)

用户说"建文件夹/新建目录":
- 创建 → `folder create` 或 `file create --type folder`

用户说"上传文件/传文件/上传到文档/上传到知识库":
- 上传 → `upload`（需本地文件路径）
- 上传并转换 → `upload --convert`

用户说"下载/导出/下载到本地/导出文档/导出为Word/导出为docx/把文档导出来":
- **必须先判断目标文件类型**，再决定走 `export` 还是 `download`：
  - 在线文档（alidocs/adoc）→ **`export`**（格式转换后导出为 docx）
  - 已有文件（PDF、图片、附件、视频等非在线文档）→ **`download`**（直接下载原始文件）
- 判断方法：
  1. 用户明确说"导出文档"/"导出为 Word/docx" → 直接走 `export`
  2. 用户明确说"下载 PDF/图片/附件" → 直接走 `download`
  3. 不确定时，先用 `info --node <ID>` 查询节点信息，根据 `contentType` 判断：
     - `contentType` = `ALIDOC` → 走 `export`
     - `contentType` = `DOCUMENT`/`IMAGE`/`VIDEO` 等 → 走 `download`

> **严禁将"导出文档"直接路由到 `download`**。`download` 只能下载已有文件（原样下载），`export` 是将在线文档格式转换后导出为 docx，两者完全不同。

用户说"复制文档/拷贝一份/复制到":
- 复制 → `copy` (需源 --node 和目标 --folder/--workspace)

用户说"移动文档/搬到/移到/转移文件":
- 移动 → `move` (需源 --node 和目标 --folder/--workspace)

用户说"重命名/rename/改名/改文档名/修改文档名称/修改文档标题/把这个文档叫做...":
- 重命名 → `doc rename`（需文档 ID 或 URL + 新名称）
- 只要意图是修改文档在列表和链接中展示的名称，统一路由到 `dws doc rename --node <DOC_ID_OR_URL> --name "新名称"`；不要走 `drive`、`doc update` 或重新 `doc create`
- 只有用户明确说"正文里的标题/章节标题/段落标题/H1 标题"时，才走 `block update`

用户说"删除文档/删掉这个文件/移到回收站/丢掉这篇文档":
- 删除节点 → `delete`（危险操作，需 `--yes` 确认；需文档 ID 或 URL）

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
dws doc list --folder <DOC_FOLDER_NODE_ID> --format json

# 3. 获取文档元信息 (标题、类型、权限)
dws doc info --node <DOC_ID> --format json

# 4. 读取文档内容 (Markdown 格式)
dws doc read --node <DOC_ID> --format json

# ── 工作流 2: 创建文档并写入内容 ──

# 1. (可选) 创建文件夹 — 提取 nodeId
dws doc folder create --name "项目资料" --format json

# 2. 创建文档 — 提取 nodeId
dws doc create --name "项目周报" --folder <DOC_FOLDER_NODE_ID> --format json

# 3. 写入内容 (追加模式)
dws doc update --node <DOC_ID> --content "# 本周总结\n\n- 完成了 A\n- 推进了 B" --mode append --format json

# ── 工作流 3: 一步创建带内容的文档 ──

dws doc create --name "会议纪要" --content "# 会议纪要\n\n## 议题\n\n1. ..." --format json

# ── 工作流 4: 上传本地文件到钉钉文档/知识库 ──

# 1. 上传到"我的文档"根目录
dws doc upload --file ./report.pdf

# 2. 上传到指定文件夹
dws doc upload --file ./slides.pptx --name "Q1汇报.pptx" --folder <DOC_FOLDER_NODE_ID>

# 3. 上传到知识库并转换为在线文档
dws doc upload --file ./data.xlsx --workspace <WS_ID> --convert

# ── 工作流 5: 下载/导出文件到本地 ──
#  必须先用 info 判断文件类型，再决定用 download 还是 export：
#   - contentType 为 ALIDOC（在线文档）→ 用 export
#   - contentType 为 DOCUMENT/IMAGE/VIDEO 等（已有文件）→ 用 download

# 步骤 1: 查询文件类型
dws doc info --node <NODE_ID> --format json
# 根据返回的 contentType 字段判断：

# 如果是已有文件 (非 ALIDOC)，用 download：
dws doc download --node <NODE_ID> --output ~/downloads/

# 如果是在线文档 (ALIDOC)，用 export：
dws doc export --node <NODE_ID> --output ~/downloads/

# ── 工作流 6: 上传附件并插入文档 ──

# media insert 自动完成三步流程:
#   1. 获取附件上传凭证 (get_doc_attachment_upload_info)
#   2. HTTP PUT 上传文件到 OSS
#   3. 插入附件块到文档 (insert_document_block)

# 1. 基本用法: 插入本地文件到文档
dws doc media insert --node <DOC_ID> --file ./report.pdf

# 2. 指定附件显示名称
dws doc media insert --node <DOC_ID> --file ./data.xlsx --name "Q1数据报表.xlsx"

# 3. 指定 MIME 类型 (文件扩展名无法推断时)
dws doc media insert --node <DOC_ID> --file ./data.bin --name "导出数据.dat" --mime-type application/octet-stream

# 4. 在指定块之前插入附件
dws doc media insert --node <DOC_ID> --file ./image.png --ref-block <BLOCK_ID> --where before

# 5. 完整流程: 创建文档 → 写入内容 → 插入附件
dws doc create --name "项目报告" --content "# 项目报告\n\n以下为相关附件：" --format json
# 提取 nodeId 后:
dws doc media insert --node <DOC_ID> --file ./design.pdf
dws doc media insert --node <DOC_ID> --file ./timeline.xlsx --name "项目时间线.xlsx"

# ── 工作流 7: 块级精细编辑 ──

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

# ── 工作流 8: 复制/移动/重命名文档 ──

# 获取 nodeId 的三种方式（按场景选择，无需全部执行）:
#   方式 A: 用户直接提供文档 URL — 直接传给 --node，无需额外查询
#   方式 B: 搜索文档 — 从返回中提取 nodeId
dws doc search --query "项目周报" --format json
#   方式 C: 浏览文件夹 — 从返回中提取 nodeId
dws doc list --folder <DOC_FOLDER_NODE_ID> --format json
# 注意: 这里的 <DOC_FOLDER_NODE_ID> 是文件夹 nodeId/dentryUuid 或文件夹 URL，不是数字 dentryId；示例统一使用 canonical --folder。

# 复制文档到指定文件夹（--node 支持 ID 或 URL）
dws doc copy --node <DOC_ID_OR_URL> --folder <TARGET_DOC_FOLDER_NODE_ID> --format json

# 复制到目标知识库根目录（不传 --folder 时）
dws doc copy --node <DOC_ID_OR_URL> --workspace <TARGET_WS_ID> --format json

# 移动文档到指定文件夹
dws doc move --node <DOC_ID_OR_URL> --folder <TARGET_DOC_FOLDER_NODE_ID> --format json

# 移动到目标知识库根目录（不传 --folder 时）
dws doc move --node <DOC_ID_OR_URL> --workspace <TARGET_WS_ID> --format json

# 重命名文档
dws doc rename --node <DOC_ID_OR_URL> --name "新名称" --format json

# 删除文档到回收站（危险操作：必须先向用户确认，用户同意后才加 --yes 执行）
# 正确流程：1.向用户展示"即将删除「文档名」到回收站" → 2.等用户确认 → 3.执行下面命令
dws doc delete --node <DOC_ID_OR_URL> --yes --format json

# ── 工作流 9: 文档评论管理 ──

# 1. 查看文档的所有评论
dws doc comment list --node <DOC_ID> --format json

# 2. 在文档上创建评论
dws doc comment create --node <DOC_ID> --content "这里需要补充数据来源" --format json

# 3. 创建评论并 @ 相关人
#    先搜索用户: dws contact user search --query "张三" --format json → 提取 userId
#    再将 userId 传入 --mention
dws doc comment create --node <DOC_ID> --content "请确认这部分内容" --mention <userId1>,<userId2> --format json

# 4. 回复某条评论（commentKey 从 list 或 create 返回中获取）
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已修改" --format json

# 5. 用表情回复评论
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "比心" --emoji --format json

# 6. 创建划词评论（针对文档中某段选中文本）
#    先获取块列表: dws doc block list --node <DOC_ID> --format json → 提取 blockId 和文本内容
#    确定选中文本在块内的起始偏移量 (start) 和结束偏移量 (end)
dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 10 --content "这里需要修改" --format json

# 7. 创建划词评论并附带引用原文 + @ 相关人
dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 5 --end 20 --content "请确认这部分" --selected-text "被选中的原文内容" --mention <userId1>,<userId2> --format json

# ── 工作流 10: 导出在线文档为 docx ──

# 一条命令自动完成（提交→轮询→下载），无需手动编排
dws doc export --node <DOC_ID_OR_URL> --output ./exported.docx

# 如果导出命令超时或中断，可用 export get 手动查询任务状态：
# dws doc export get --job-id <JOB_ID>
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

## nodeId 多格式说明

所有 `--node` 参数同时支持以下格式，系统自动识别：
- **文档 ID**: 字母数字字符串，如 `9E05BDRVQePjzLkZt2p2vE7kV63zgkYA`
- **文档 URL**: `https://alidocs.dingtalk.com/i/nodes/{dentryUuid}`，如 `https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA`
- **文档链接（edit/preview）**: `https://alidocs.dingtalk.com/document/{edit|preview}?...&dentryKey={key}`（必须传入完整 URL，不要提取其中的 query 参数单独使用）

以下命令效果相同：
```bash
dws doc read --node 9E05BDRVQePjzLkZt2p2vE7kV63zgkYA
dws doc read --node "https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA"
dws doc read --node "https://alidocs.dingtalk.com/document/edit?dentryKey=wo1g3x54FzVEJ5yE"
dws doc read --node "https://alidocs.dingtalk.com/document/preview?cid=74993670680&type=d&docKey=Pd6l2Z7V8ZWydl7M&dentryKey=rBGBr2r1HmwanAGW"
```

> **注意**：`document/edit` 和 `document/preview` 格式 URL 中的 `dentryKey` 参数值不是合法的独立 nodeId，禁止提取后单独使用，必须传入完整 URL。URL 中可能包含 `utm_source`、`chInfo` 等追踪参数，无需手动去除，直接传入完整 URL 即可。

`--folder` 参数同样支持文件夹 URL 或 ID。

## 长 Markdown 写入

**核心规则**：含多行、表格、`\n` 或长度 >2KB 的 Markdown **必须**通过 `--content-file` 或 `--content -`（stdin）传入，禁止直接作为 `--content` 命令行字符串——shell escape 会破坏换行和表格，且命令行长度受限。

`dws doc create` 和 `dws doc update` 支持两种内容来源（`--content-file` 优先于 `--content`）：

| 形式 | 说明 |
|------|------|
| `--content "..."` | 字面量（仅推荐短文本 <2KB 且无换行/表格） |
| `--content -` | 从 stdin 读取（可配合 heredoc/pipe） |
| `--content-file path` | 从文件读取（UTF-8），推荐 |

### 短/中等长度（< 200KB）— 单步创建

```bash
# 1. 把内容写入 UTF-8 文本文件：
#    Linux/Mac: /tmp/<name>.md；Windows: %TEMP%\<name>.md
# 2. 一步创建+写入：
dws doc create --name "<文档名>" --content-file <tmp> [--folder <ID>] [--workspace <ID>]
```

### 超长（> 200KB 兜底）— 创建空文档 + 分片追加

```bash
# 1. 创建空文档拿 nodeId
dws doc create --name "<文档名>" [--folder/--workspace]  # → nodeId

# 2. 按 markdown 标题或段落边界切成 ≤200KB 的片段（不要切断表格）
# 3. 逐个追加：
dws doc update --node <nodeId> --content-file <part> --mode append
```

### stdin 变体

```bash
# pipe
cat report.md | dws doc update --node <DOC_ID> --content - --mode append

# heredoc（真实换行，含表格）
dws doc update --node <DOC_ID> --mode append --content - <<'EOF'
## 追加段落

| 列1 | 列2 |
|---|---|
| a | b |
EOF
```

## 注意事项

- `export` 是一体化命令，一条命令自动完成提交→轮询→下载，**无需手动编排轮询**。CLI 内部使用渐进式退避轮询（最多约 5 分钟）
- `export` 超时或中断后，CLI 会输出 `jobId`，可用 `dws doc export get --job-id <jobId>` 手动查询任务状态
- `export` 当前仅支持钉钉在线文档 (alidocs) 导出为 `docx`；当前开源 CLI 未暴露在线表格导出命令，在线表格数据读取走 `dws sheet range read --node <ID>`
- `update --mode overwrite` 会**清空原内容后重写**，⚠️ 谨慎使用；必须先 `--dry-run` 预览并在用户确认后加 `--yes`；默认 `--mode append`（追加）更安全
- `read` 返回 Markdown 格式的文档内容，仅限有"下载"权限的文档
- `read` 返回的内容中，文档里的附件会以 OSS 临时下载链接形式给出（如 `https://alidocs2.oss-cn-zhangjiakou.aliyuncs.com/res/.../att/<resourceId>.ext?Expires=...`），该链接会过期。链接过期后，可从 URL 路径中提取 `<resourceId>`（即 `/att/` 后、扩展名前的 UUID 部分），然后使用 `media download --node <DOC_ID> --resource-id <resourceId>` 重新获取下载链接
- `create` 不传 `--folder` 和 `--workspace` 时，默认创建在"我的文档"根目录
- `create` 只能建"文档"（adoc）；要建表格/脑图/白板/多维表/演示/文件夹，用 `file create --type`
- `block list/insert/update/delete` 是块级精细编辑，适合结构化修改；简单内容追加建议用 `update --mode append`
- `block insert` 优先使用 `--text` 或 `--heading` 快捷方式；复杂块类型 (table, callout 等) 使用 `--element` JSON
- `--content` 参数中的换行必须使用**真实换行符**（即实际的换行字符，Unicode `U+000A`），而不是字面量字符串 `\n`（反斜杠加字母 n）。在通过程序或大模型构造此参数时，请确保字符串在发送前已正确反转义。如果传入的是两个字符的字面量 `\n`，所有内容将渲染在同一行，导致标题、段落和表格格式全部错乱。**含多行/表格/长文本时优先用 `--content-file path.md` 或 `--content -`（stdin），不经过 shell escape，换行和表格都保持原样**（详见下方「长 Markdown 写入」）。
- 块类型包括: paragraph, heading, blockquote, callout, columns, orderedList, unorderedList, table, sheet, attachment, slot
- 关键区分: doc(文档内容级操作) vs wiki(知识库空间级管理) vs aitable(数据表格操作) vs drive(钉盘文件管理)
- wiki 是知识库容器，doc 是知识库中的文档内容；需要 `workspaceId` 时，先用 `dws wiki space list/search` 获取，再传给 doc 的 `--workspace` 参数
- `doc upload vs drive upload`：用户提到"知识库/文档空间/workspace" → `doc upload`；提到"钉盘/网盘/我的文件" → `drive upload`；未明确目标时默认 `drive upload`
- `upload` 支持上传任意类型文件 (PDF、Office、图片等) 到钉钉文档空间或知识库；`--convert` 可将 Office 文件转换为钉钉在线文档
- `upload` 是三步自动完成的流程 (获取凭证 → OSS 上传 → 提交入库)，无需手动分步操作
- `download` 是两步自动完成的流程 (获取下载链接 → HTTP GET 下载)，支持自动推断文件名；`--output` 可指定文件路径或目录
- `media insert` 是三步自动完成的流程 (获取附件上传凭证 → OSS 上传 → 插入附件块到文档)，无需手动分步操作
- `media insert` 的 `--mime-type` 可选，不指定时根据文件扩展名自动推断；支持常见文件类型 (PDF、Office、图片、视频、压缩包等)
- `media insert` 与 `upload` 的区别：`upload` 将文件上传到文档空间/知识库作为独立文件；`media insert` 将文件作为附件块插入到文档正文中
- `media download` 用于获取文档正文中附件的临时下载链接，`--resource-id` 可通过 `block list` 返回的 attachment 块获取
- `copy` 需要对源节点有"阅读"权限，且对目标文件夹有"编辑"权限
- `move` 需要对源节点有"管理"权限，且对目标文件夹有"编辑"权限；移动后原位置的文档将不再存在
- `rename` 需要对文档有"编辑"权限
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
