# 钉钉文档富结构创建流程

本页只处理用户明确要求 Markdown 无法表达的富结构，例如颜色、callout、分栏或必须保留的复杂块结构。普通纪要、周报、月报、方案、SOP、长文本和普通表格默认使用 Markdown，不读取本页。

执行入口固定为 `dws doc +create`。参数、constraint 和 confirmation 以已加载的 CommandMeta 或精确 leaf Schema 为准；本页不另建参数权威，也不继续加载 style、cookbook 或 schema reference。

## 1. 锁定用户约束

按用户原话形成内存 checklist，不额外创建 RFC、设计规范或配色文件：

- 文档标题和目标位置；
- 必须出现的标题、段落、表格、图片和顺序；
- 用户明确要求的颜色、callout、分栏等富结构；
- 禁止补写或推断的内容；
- 完成后必须验证的短语、数量和结构。

用户明确格式要求高于默认样式。`--name` 是文档名称；用户明确要求正文包含一级标题时必须保留 H1，不能以“避免重复标题”为由删除。

## 2. 选择最小格式

| 条件 | 格式 |
|---|---|
| 普通正文、标题、有文本列表、引用、代码块、普通表格 | Markdown |
| 必须保留无文本列表项/空列表占位 | JSONML；禁止用 Markdown 裸 `-`、`*` 或 `1.` |
| 用户提供 `.md` 或明确要求 Markdown | Markdown |
| 明确要求颜色、callout、分栏，且 Markdown 无法表达 | JSONML |
| 仅要求“美观、清晰、专业”，没有具体富结构 | Markdown，使用清楚的标题层级和留白 |

不要因为文档是中文、篇幅长、属于周报/方案/复盘，或出现“先、然后”等顺序词就切到 JSONML。不要主动增加图片、配色、分栏或未要求的业务内容。

## 3. 准备正文

- 已有或临时正文写到 cwd 内相对文件并通过 `@相对路径` 传入；单次文本可用 stdin。
- 禁止绝对路径、`..`、临时目录和把用户正文作为 `printf` 格式字符串。
- 保留真实换行、百分号、反斜杠、代码和用户给定的标点。
- JSONML 顶层必须是单个非空 root 元素，不得用元素数组二次包裹。
- 空列表项必须保留 `p.attrs.list.listId/level/isOrdered` 和空 leaf，例如 `["p",{"uuid":"li-empty","list":{"listId":"list-empty","level":0,"isOrdered":false}},["span",{"data-type":"text"},["span",{"data-type":"leaf"},""]]]`。
- 只生成完成用户 checklist 所需的最小结构；不先做 Markdown 脚手架再固定精修一遍。

JSONML 校验失败时，根据 validator 的具体错误修正一次。仍失败则简化非必要富结构；如果简化会违反用户明确要求，停止并报告具体未满足项，禁止无界重试或悄悄降级。

## 4. 一次创建

普通 Markdown：

```bash
dws doc +create --name "<标题>" --content @./drafts/body.md --doc-format markdown --format json
```

明确富结构：

```bash
dws doc +create --name "<标题>" --content @./drafts/body.json --doc-format jsonml --format json
```

优先消费已加载的 CommandMeta。只有目标 leaf 的参数、constraint 或 confirmation 缺失时查询一次精确 Schema。`not_required` 直接执行且不加 `--yes`；`user_required` 按两阶段确认协议执行，首次创建请求不能代替独立明确同意。

`+create` 内置写入和有界回读。成功后使用回执中的真实 `nodeId`、revision 和验证状态；不要固定再读一次全文。

## 5. 媒体和局部精修

用户提供本地图片或附件时，创建正文后使用 `+media-insert`，保存真实 `insertedBlockId/resourceId`。只有用户指定插入位置且当前回执不足以确定目标 block 时，才用 `+fetch --detail with-ids` 定点取 ID。

普通正文创建完成后不要主动进入精修。只有仍有用户明确要求且 Markdown 无法表达的结构，才使用 `+update`；执行规则以更新 workflow 为准。

## 6. 交付验证

1. 回执为 `success` 且 `complete=true`；部分成功或未知状态按回执恢复，不重放创建。
2. 返回真实 `nodeId`；需要后续步骤时只使用真实稳定 ID。
3. 用户要求的短语、标题、数量和顺序均存在；不得只验证“接口成功”。
4. 媒体步骤验证真实 `insertedBlockId/resourceId`。
5. 没有渲染或截图证据时，只报告结构验证，不能宣称视觉排版完全正常。
