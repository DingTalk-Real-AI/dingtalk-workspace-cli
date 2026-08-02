# doc update（更新文档内容）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流
> 2. [`./style/doc-update-workflow.md`](./style/doc-update-workflow.md) — 改写流程（编辑形态优先级、分片 append、回读验收）
> 3. [`./style/doc-style-guideline.md`](./style/doc-style-guideline.md) — 排版规范
> 4. [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md) — 仅当使用 `--content-format jsonml` 时必读
>
> **同任务常配合**：[`doc-read.md`](./doc-read.md)（改写前必读，jsonml 模式拿当前结构；担心被并发覆盖时再取 revision）/ [`doc-block.md`](./doc-block.md)（单 block 改写优先；本命令更适合追加 / 整篇 overwrite）

## 命令格式

```
Usage:
  dws doc update [flags]
Example:
  dws doc update --node <DOC_ID> --content "# 追加内容" --mode append
  dws doc update --node <DOC_ID> --content "# 完整替换" --mode overwrite
  dws doc update --node <DOC_ID> --content-file ./part1.md --mode append
  dws doc update --node <DOC_ID> --content "# 插入到第3个block前" --mode append --index 2
  dws doc update --node <DOC_ID> --content-file ./body.json --content-format jsonml --mode overwrite
  cat part2.md | dws doc update --node <DOC_ID> --content - --mode append
Flags:
      --node string           文档 ID 或 URL (必填)
      --content string        文档内容（短文本字面量）；传 - 表示从 stdin 读取
      --content-file string   从文件读取文档内容（UTF-8）。推荐长/多行/表格内容使用
      --mode string           更新模式: overwrite=覆盖, append=追加 (必填)
      --content-format string         内容格式: 默认为 markdown，可选 jsonml
      --revision int          文档版本号（仅 --content-format jsonml 时生效，可选）；传入后服务端做并发检查，版本不一致时返回 VersionConflict。不传则直接覆盖，不做并发检查
      --fix-jsonml              启用 JSON 语法修复（括号/逗号补全），推荐 agent 调用时使用
      --index int             插入位置（从 0 开始），仅在 mode=append 时生效。指定将内容插入到文档第几个 block 之前。不传时追加到末尾。block 的 index 可通过 doc block list 获取。插入成功后，该位置及之后所有 block 的 index 会依次 +1
```

## 关键说明

- `--mode` 必填，无默认值。`overwrite` **清空原内容后重写**，谨慎使用；`append` 更安全。
- 整篇 overwrite 大文档前**必须**先向用户提示风险并等待确认（详见 [`./style/doc-update-workflow.md` §4.5](./style/doc-update-workflow.md)）。
- `--content` 中的换行必须是**真实换行符**（Unicode `U+000A`），不是字面量 `\n`；多行/表格/长文本优先 `--content-file` 或 `--content -`。
- **写入后必须回读**——返回 `success=true` 不等于内容真的写入完整（详见 [`./style/doc-update-workflow.md` §6](./style/doc-update-workflow.md)）。

## JSONML 格式写入

使用 `--content-format jsonml` 可以 JSONML 结构直接写入文档，实现无损读写。当前仅支持 `--mode overwrite`。

**输入格式**：JSON 对象，包含 `jsonml` 字段（文本必须用 `span/data-type=text + span/data-type=leaf` 包裹，详见 [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md)）：

```json
{"jsonml": ["root", {"sectPr": {}},
  ["p", {"uuid": "p1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "hello"]]],
  ["p", {"uuid": "p2"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "world"]]]
]}
```

> CLI 不做结构修复——裸字符串、缺 uuid 等错误会被 validator 直接抦下。body 必须以 `["root", ...]` 为根节点，缺少会报错。如果输入来自 LLM 生成且可能有 JSON 语法错误（缺括号/逗号），加 `--fix-jsonml` 启用 JSON 语法修复。

**典型流程**（无损读改写）：

1. `dws doc read --node <DOC_ID> --content-format jsonml --output ./doc.json` — 获取文档 JSONML 结构
2. 修改 `doc.json` 中的 jsonml 数组内容
3. `dws doc update --node <DOC_ID> --content-file ./doc.json --content-format jsonml --mode overwrite` — 写回

> 默认不传 `--revision`，服务端直接覆盖，不做并发检查。担心多 agent 同时改时，按下方 §并发安全模式 加 `--revision`。

**节点结构参考**：[`./format/doc-jsonml-schema.md`](./format/doc-jsonml-schema.md)

### 并发安全模式（担心被并发覆盖时使用）

如果你担心在编辑期间别人也在改这个文档，可以传 `--revision` 触发服务端并发检查：

1. `dws doc read --node <DOC_ID> --content-format jsonml --output /tmp/doc.json` — 返回的 JSON 里有一个 `revision` 字段（比如 `42`）。
2. 编辑 `/tmp/doc.json` 里的 `jsonml` 字段。
3. `dws doc update --node <DOC_ID> --content-file /tmp/doc.json --content-format jsonml --mode overwrite --revision 42` — 如果文档在期间被改过，服务端返回 `VersionConflict`，此时重新执行第 1 步即可。

不带 `--revision` 时，服务端不做并发检查，直接覆盖。普通单 agent 编辑场景下默认不传即可。

## 内容写入管道（create / update 共用）

> **关键原则**：CLI 内置自动分片。超长内容（>30000 字符）自动按 markdown 结构切分后逐片写入，对调用方透明。写入完成后由调用方自行决定是否回读确认。

### 输入方式选择

| 场景 | 推荐方式 | 说明 |
|------|---------|------|
| 短文本（<2KB，无换行/表格/特殊字符） | `--content "..."` | 字面量传入，最简单 |
| 长文本（≥2KB）、含换行、含表格 | `--content-file ./file.md` | **必须**用文件路径，避免 shell escape 和截断 |
| 含特殊字符（`"`、`\`、`$`、`` ` ``） | `--content-file ./file.md` | 字面量传入会被 shell 转义破坏 |
| 管道/heredoc 输入 | `--content -` 或 `cat file \| dws doc ...` | 从 stdin 读取 |

### 自动分片行为

当内容超过 30000 字符时，CLI 自动执行：

1. **create**: 先创建空文档拿 `nodeId`，再按 markdown 标题边界切分后逐片 append
2. **update (overwrite)**: 第一片用 overwrite，后续片用 append
3. **update (append)**: 所有片段用 append

分片策略按优先级：H1 标题 → H2 标题 → H3 标题 → 空行（段落边界）→ 硬切（保留表格/代码块完整性）

如果某片写入超时，自动将分片大小减半重试（最小 5000 字符，低于此值报错）。

### 输出格式

写入成功后输出 JSON（混合 `[INFO]` 进度行）：

```json
{"success": true, "nodeId": "xxx", "chunksWritten": 3}
```

| 字段 | 说明 |
|------|------|
| `nodeId` | 文档节点 ID，可用于后续读取或追加 |
| `chunksWritten` | 实际写入的分片数（1 = 单次写入） |

### 内容完整性验证（必读）

CLI **不会**自动执行回读验证。**你必须在文档写入完成后主动回读确认**：

1. 使用 `dws doc read --node <nodeId>` 读取写入后的文档内容
2. 检查关键段落是否完整、顺序是否正确
3. 如发现内容缺失或异常，使用 `dws doc update --mode append` 补写缺失部分

> **何时回读**：每次 create/update 操作完成后都应回读。如果是连续多次编辑同一文档，可以在全部编辑完成后统一回读一次。

### 进度输出示例

```
[INFO] 内容较长 (45000 字符)，自动分片写入...
[INFO] 已创建空文档 (nodeId=abc123)，开始分片写入...
[INFO] 写入分片 (1/3)，15000 字符...
[INFO] 写入分片 (2/3)，15000 字符...
[INFO] 写入分片 (3/3)，15000 字符...
[INFO] 全部 3 个分片写入完成
{"success": true, "nodeId": "abc123", "chunksWritten": 3}
```

### CONTENT_TRUNCATED 错误

当分片写入持续超时且减半到最小阈值仍失败时，返回 `CONTENT_TRUNCATED` 错误码。应对策略：

1. 检查网络和后端服务状态
2. 已写入的部分内容可通过 `dws doc read --node <NODE_ID>` 查看
3. 从断点处手动用 `dws doc update --mode append` 继续追加

## 长 Markdown 写入

**核心规则**：含多行、表格、`\n` 或长度 >2KB 的 Markdown **必须**通过 `--content-file` 或 `--content -`（stdin）传入，禁止直接作为 `--content` 命令行字符串——shell escape 会破坏换行和表格，且命令行长度受限。

`dws doc create` 和 `dws doc update` 支持两种内容来源（`--content-file` 优先于 `--content`）：

| 形式 | 说明 |
|------|------|
| `--content "..."` | 字面量（仅推荐短文本 <2KB 且无换行/表格） |
| `--content -` | 从 stdin 读取（可配合 heredoc/pipe） |
| `--content-file path` | 从文件读取（UTF-8），推荐 |

### 短/中等长度（< 200KB）— 单步写入

```bash
# 1. 把内容写入 UTF-8 文本文件：
#    Linux/Mac: /tmp/<name>.md；Windows: %TEMP%\<name>.md
# 2. 一步写入：
dws doc update --node <DOC_ID> --content-file <tmp> --mode overwrite --content-format markdown
```

### 超长（> 200KB 兜底）— 分片追加

```bash
# 1. 按 markdown 标题或段落边界切成 ≤200KB 的片段（不要切断表格）
# 2. 逐个追加：
dws doc update --node <nodeId> --content-file <part> --mode append --content-format markdown
```

> **注意**：分块 append 存在静默失败风险（部分片段返回 success 但实际未写入），执行前**必须**向用户发出截断风险提示并等待确认。完整规范见 [`../../best_practices/04-document.md` «分块 append 截断风险提示»](../04-document.md)。

### stdin 变体

```bash
# pipe
cat report.md | dws doc update --node <DOC_ID> --content - --mode append --content-format markdown

# heredoc（真实换行，含表格）
dws doc update --node <DOC_ID> --mode append --content - --content-format markdown <<'EOF'
## 追加段落

| 列1 | 列2 |
|---|---|
| a | b |
EOF
```

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `success` + `chunksWritten` | 判断是否需要回读补救（`chunksWritten > 1` 时重点查章节顺序） |
| 错误码 `CONTENT_TRUNCATED` | 触发 [`./doc-read.md`](./doc-read.md) 查断点 + 再次 `update --mode append` |

## 常用模板

```bash
# overwrite 整段（用户已确认）
dws doc update --node <DOC_ID> --content-file /tmp/<name>.md --mode overwrite --content-format markdown

# append 末尾追加
dws doc update --node <DOC_ID> --content-file /tmp/<name>-append.md --mode append --content-format markdown

# append 到指定 block 前（index 通过 doc block list 获取）
dws doc update --node <DOC_ID> --content-file /tmp/<name>.md --mode append --index 2 --content-format markdown

# JSONML 整篇无损 overwrite（默认不做并发检查；并发敏感时加 --revision <N>）
dws doc update --node <DOC_ID> --content-file /tmp/<name>.json --content-format jsonml --mode overwrite

# 短文本字面量（<2KB 无换行）
dws doc update --node <DOC_ID> --content "## 简短追加" --mode append --content-format markdown

# stdin（pipe）
cat report.md | dws doc update --node <DOC_ID> --content - --mode append --content-format markdown

# stdin（heredoc，含表格）
dws doc update --node <DOC_ID> --mode append --content - --content-format markdown <<'EOF'
## 追加段落

| 列1 | 列2 |
|---|---|
| a | b |
EOF
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令）
- [`./doc-read.md`](./doc-read.md)（改写前必读；jsonml 模式拿当前结构，担心并发时再取 revision）
- [`./doc-block.md`](./doc-block.md)（单 block 改写更精准）
- [`./style/doc-update-workflow.md`](./style/doc-update-workflow.md)（编辑形态优先级、分片 append 风险、回读验收）
- [`./style/doc-style-guideline.md`](./style/doc-style-guideline.md)（排版规范）
- [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md) / [`./format/doc-jsonml-schema.md`](./format/doc-jsonml-schema.md)（JSONML 范例 / 节点结构）
