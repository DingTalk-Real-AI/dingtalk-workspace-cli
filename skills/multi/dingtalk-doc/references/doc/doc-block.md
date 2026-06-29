# doc block（块级精细编辑：list / insert / update / delete）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流
> 2. [`./style/doc-update-workflow.md`](./style/doc-update-workflow.md) — 改写流程（编辑形态优先级、JSONML validator 行为）
> 3. [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md) — JSONML 范例（含 callout / 分栏 / 表格 / 标题等节点的完整命令）
> 4. [`./format/doc-jsonml-schema.md`](./format/doc-jsonml-schema.md) — JSONML 节点结构字段定义
>
> **同任务常配合**：[`doc-update.md`](./doc-update.md)（整篇 overwrite / 末尾追加纯文本）/ [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md)（JSONML 复制范例）

> **改写已有文档优先 JSONML**：保真度最高、callout / 分栏 / 表格 / @人 / 附件 / 颜色 / 嵌套都能 1:1 round-trip；写入端有 validator 兜底。详见 [`./style/doc-update-workflow.md` §1.3 编辑形态优先级](./style/doc-update-workflow.md)。

---

## doc block list（查询块元素）

```
Usage:
  dws doc block list [flags]
Example:
  dws doc block list --node <DOC_ID>
  dws doc block list --node <DOC_ID> --start-index 0 --end-index 5
  dws doc block list --node <DOC_ID> --block-type heading
  dws doc block list --node <DOC_ID> --content-format jsonml
  dws doc block list --node <DOC_ID> --content-format jsonml --block-id <UUID>
Flags:
      --node string         文档 ID 或 URL (必填)
      --start-index int     起始位置 (从 0 开始)
      --end-index int       终止位置 (含)
      --block-type string   按块类型过滤
      --content-format string       输出格式: 默认为 element，可选 jsonml（返回 JSONML 节点数组）
      --block-id string     指定块 UUID（content-format=jsonml 时读取完整子树）
```

### content-format=jsonml 返回示例

每个 block 包含 `jsonml` 字段（JSON string，解析后为 JSONML 数组）：

```json
{
  "blocks": [
    {
      "blockId": "mpeurp5mj5o9xz4hnj",
      "blockType": "h2",
      "index": 0,
      "jsonml": "[\"h2\",{\"uuid\":\"mpeurp5mj5o9xz4hnj\"},...]"
    }
  ],
  "totalCount": 5
}
```

---

## doc block insert（插入块元素）

```
Usage:
  dws doc block insert [flags]
Example:
  dws doc block insert --node <DOC_ID> --text "这是一段文字"
  dws doc block insert --node <DOC_ID> --heading "二级标题" --level 2
  dws doc block insert --node <DOC_ID> --element '{"blockType":"paragraph","paragraph":{"text":"内容"}}'
  dws doc block insert --node <DOC_ID> --text "在此处之前插入" --ref-block <BLOCK_ID> --where before
  dws doc block insert --node <DOC_ID> --content-format jsonml --element '["p",{"uuid":"..."},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"新段落"]]]'

  # 插入引用块(blockquote)
  dws doc block insert --node <DOC_ID> --element '{"blockType":"blockquote","blockquote":{"text":"这是一段引用内容"}}'

  # 插入分栏块(columns)：2 栏，children 为每栏的内容
  dws doc block insert --node <DOC_ID> --element '{"blockType":"columns","columns":{"size":2},"children":[{"blockType":"paragraph","paragraph":{"text":"左栏内容"}},{"blockType":"paragraph","paragraph":{"text":"右栏内容"}}]}'

  # 插入表格(table)：2 行 3 列
  dws doc block insert --node <DOC_ID> --element '{"blockType":"table","table":{"rolSize":2,"colSize":3,"cells":[["姓名","部门","职位"],["张三","工程部","开发"]]}}'

  # 插入行内图片(inline image)：构造一个空 paragraph，在 children 中用 image elementType 指定图片 src
  dws doc block insert --node <DOC_ID> --element '{"blockType":"paragraph","paragraph":{},"children":[{"elementType":"image","properties":{"src":"https://example.com/photo.png"}}]}'

  # 插入附件块(attachment)：resourceId 为资源ID，type 为 MIME 类型，viewType 可选 preview/summary
  dws doc block insert --node <DOC_ID> --element '{"blockType":"attachment","attachment":{"resourceId":"12345-xxx-xxx-123-xxxxx","type":"application/pdf","name":"报告.pdf","viewType":"preview"}}'

  # 插入分割线：使用 doc update --mode append 以 Markdown 格式追加
  dws doc update --node <DOC_ID> --content "---" --mode append
Flags:
      --node string        文档 ID 或 URL (必填)
      --text string        快捷: 段落文本内容
      --heading string     快捷: 标题文本
      --level int          标题级别 1-6 (配合 --heading，默认 1)
      --element string     块元素 JSON (高级)；content-format=jsonml 时为 JSONML 数组字符串
      --content-format string      输入格式: 默认为 element，可选 jsonml
      --fix-jsonml              启用 JSON 语法修复（括号/逗号补全），推荐 agent 调用时使用
      --index int          参照位置索引 (从 0 开始)
      --where string       插入方向: before / after (默认 after)
      --ref-block string   参照块 ID (优先级高于 --index)
```

> **content-format=jsonml 完整范例**：参见 [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md)，包含所有节点类型的正确 insert/update 命令示例。

---

## doc block update（更新块元素）

```
Usage:
  dws doc block update [flags]
Example:
  dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --text "新内容"
  dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --element '{"blockType":"heading","heading":{"text":"新标题","level":1}}'
  dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --content-format jsonml --element '["h1",{"uuid":"<BLOCK_ID>"},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"新标题"]]]'
Flags:
      --node string        文档 ID 或 URL (必填)
      --block-id string    目标块 ID (必填)
      --text string        快捷: 段落文本内容
      --heading string     快捷: 标题文本
      --level int          标题级别 1-6 (配合 --heading，默认 1)
      --element string     块元素 JSON (高级)；content-format=jsonml 时为 JSONML 数组字符串
      --content-format string      输入格式: 默认为 element，可选 jsonml
      --fix-jsonml              启用 JSON 语法修复（括号/逗号补全），推荐 agent 调用时使用
```

> 使用 `--content-format jsonml` 时，`element` 中的 `uuid` **必须**等于 `--block-id`，否则报错。

---

## doc block delete（删除块元素）

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

---

## JSONML 格式的块操作（首选路径）

使用 `--content-format jsonml` 可以直接以 JSONML 节点格式进行块操作，覆盖所有节点类型（不限于 block element 定义的有限类型）。

```bash
# 列出顶层 JSONML 节点
dws doc block list --node DOC_ID --content-format jsonml

# 读取指定 uuid 的完整子树
dws doc block list --node DOC_ID --content-format jsonml --block-id UUID

# 插入 JSONML 节点（同级定位）
dws doc block insert --node DOC_ID --content-format jsonml \
  --element '["p", {}, ["span", {"data-type":"text"}, ["span", {"data-type":"leaf"}, "新段落"]]]' \
  --ref-block UUID --where after

# 插入 JSONML 节点（容器内定位）
dws doc block insert --node DOC_ID --content-format jsonml \
  --element '["p", {}, ["span", {"data-type":"text"}, ["span", {"data-type":"leaf"}, "新段落"]]]' \
  --parent-block UUID --index 2

# 整体替换（update 时 uuid 必须等于 --block-id）
dws doc block update --node DOC_ID --block-id UUID --content-format jsonml \
  --element '["p", {"uuid": "UUID"}, ["span", {"data-type":"text"}, ["span", {"data-type":"leaf"}, "修改后内容"]]]'

# 删除（无需 format 区分）
dws doc block delete --node DOC_ID --block-id UUID
```

> 关于校验：CLI 不做结构修复，裸字符串、缺 uuid 等错误会被 validator 直接抦下。如需同时启用 JSON 语法修复（修复 LLM 遗漏的括号/逗号），用 `--fix-jsonml`。文本结构定义见 [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md)。

## 关键说明

- **块类型**：paragraph、heading、blockquote、callout、columns、orderedList、unorderedList、table、sheet、attachment、slot。
- **快捷 vs --element**：`block insert` 优先使用 `--text` 或 `--heading` 快捷方式；复杂块类型（table、callout、columns 等）使用 `--element` JSON 或 `--content-format jsonml`。
- **简单内容追加**：建议用 [`./doc-update.md`](./doc-update.md) `--mode append`，不必走 block insert。
- **JSONML validator**（写入端默认行为）：
  - 裸字符串、缺 uuid 等结构错误会被 validator 抦下并返回带 path 的错误（如 `$[2][2]: paragraph child must be span wrapper, got raw string.`）。
  - `--fix-jsonml` 开启 JSON 语法修复，推荐 agent 调用。
- **图片插入**：插入图片走 [`./doc-media.md`](./doc-media.md) `media insert`（作为附件块），不走 block insert。
- **分割线**：用 [`./doc-update.md`](./doc-update.md) `--content "---" --mode append`，不走 block insert。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `blocks[].blockId` | `block insert` 的 `--ref-block`、`block update/delete` 的 `--block-id` |
| `blocks[].element.id` | [`./doc-comment.md`](./doc-comment.md) `comment create-inline` 的 `--block-id` |
| `blocks[].element.paragraph.text` | 计算 [`./doc-comment.md`](./doc-comment.md) `comment create-inline` 的 `--start` / `--end` 偏移量 |
| attachment 块的 `resourceId` | [`./doc-media.md`](./doc-media.md) `media download` 的 `--resource-id` |

## 常用模板

```bash
# ── element JSON 形态（次选，老接口）──

# 文本段落
dws doc block insert --node <DOC_ID> --text "这是段落文字" --content-format element

# 标题（level 1-6）
dws doc block insert --node <DOC_ID> --heading "二级标题" --level 2 --content-format element

# 引用块
dws doc block insert --node <DOC_ID> --content-format element \
  --element '{"blockType":"blockquote","blockquote":{"text":"引用内容"}}'

# 分栏（2 栏）
dws doc block insert --node <DOC_ID> --content-format element \
  --element '{"blockType":"columns","columns":{"size":2},"children":[{"blockType":"paragraph","paragraph":{"text":"左栏"}},{"blockType":"paragraph","paragraph":{"text":"右栏"}}]}'

# 表格（2x3）
dws doc block insert --node <DOC_ID> --content-format element \
  --element '{"blockType":"table","table":{"rolSize":2,"colSize":3,"cells":[["姓名","部门","职位"],["张三","工程部","开发"]]}}'

# Callout（element JSON 次选；首选 JSONML container[subType=colorBlocks]）
dws doc block insert --node <DOC_ID> --content-format element \
  --element '{"blockType":"callout","callout":{"emoji":"⚠️","bgColor":"#FDE2E0","content":[{"text":"高风险操作，先备份"}]}}'

# 在指定 block 之前插入
dws doc block insert --node <DOC_ID> --text "在此之前" --ref-block <BLOCK_ID> --where before --content-format element

# 修改某块文本
dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --text "修改后" --content-format element

# 修改标题
dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --content-format element \
  --element '{"blockType":"heading","heading":{"text":"新标题","level":2}}'

# 删除（用户确认后）
dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID> --yes

# ── JSONML 形态（首选）──

# 列出 + 取 uuid
dws doc block list --node <DOC_ID> --content-format jsonml

# 读单个 block 完整子树
dws doc block list --node <DOC_ID> --content-format jsonml --block-id <UUID>

# 插入段落（同级定位）
dws doc block insert --node <DOC_ID> --content-format jsonml --ref-block <UUID> --where after \
  --element '["p",{},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"新段落"]]]'

# 插入 callout（colorBlocks）
dws doc block insert --node <DOC_ID> --content-format jsonml --ref-block <UUID> --where after \
  --element '["container",{"uuid":"co1","subType":"colorBlocks","metadata":{"bgcolor":"#FDE2E0","border":"#F5C2C7"}},["p",{"uuid":"co1p1"},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"高风险操作，先备份"]]]]'

# 插入分栏（columns / column）
dws doc block insert --node <DOC_ID> --content-format jsonml --ref-block <UUID> --where after \
  --element '["container",{"uuid":"cols1","subType":"columns","metadata":{"size":"2"}},["container",{"uuid":"cols1c1","subType":"column"},["p",{"uuid":"cols1c1p"},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"左栏"]]]],["container",{"uuid":"cols1c2","subType":"column"},["p",{"uuid":"cols1c2p"},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"右栏"]]]]]'

# 整体替换（update 的 uuid 必须等于 --block-id）
dws doc block update --node <DOC_ID> --block-id <UUID> --content-format jsonml \
  --element '["p",{"uuid":"<UUID>"},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"修改后内容"]]]'

# 容器内定位插入（指定 parent + index）
dws doc block insert --node <DOC_ID> --content-format jsonml --parent-block <UUID> --index 2 \
  --element '["p",{},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"在容器第 3 项位置插入"]]]'
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令族）
- [`./doc-update.md`](./doc-update.md)（整篇改写 / 纯文本追加）
- [`./style/doc-update-workflow.md`](./style/doc-update-workflow.md)（编辑形态优先级、validator 行为）
- [`./format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md)（所有节点的可复制 JSONML 范例）
- [`./format/doc-jsonml-schema.md`](./format/doc-jsonml-schema.md)（JSONML 节点结构字段定义）
- [`./style/doc-style-guideline.md`](./style/doc-style-guideline.md)（callout 颜色 / 元素边界规范）
