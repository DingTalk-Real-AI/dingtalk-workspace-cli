# JSONML Cookbook

> 本文档提供 `dws doc create/update --content-format jsonml` 和 `dws doc block insert/update --content-format jsonml` 的**完整可用范例**。
> 所有示例均基于真实文档 serialize 输出验证。节点结构详细定义见 [doc-jsonml-schema.md](./doc-jsonml-schema.md)。
> 合法节点类型和属性的权威参考为 `wukong/products/jsonml-schema-v2.json`。

## 决策型文档骨架范例（doc create 用）

以下是一个"方案对比汇报"的完整 JSONML 文件内容，展示摘要 callout + 彩色表格 + 状态高亮。
**可直接复制到 `/tmp/<name>.json` 后用 `dws doc create --name "..." --content-file /tmp/<name>.json --content-format jsonml` 创建。**

```json
["root", {},
  ["container", {"subType": "colorBlocks", "metadata": {"bgcolor": "#E8F5E9", "border": "left"}},
    ["p", {}, ["span", {"data-type": "text"},
      ["span", {"data-type": "leaf", "bold": true, "sz": 14, "szUnit": "pt"}, "✅ 推荐方案 A：上线快、依赖已有流程"]
    ]],
    ["p", {}, ["span", {"data-type": "text"},
      ["span", {"data-type": "leaf"}, "主要风险：权限配置需补 ｜ 决策时限：本周五前"]
    ]]
  ],
  ["h2", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "方案对比"]]],
  ["table", {"colsWidth": [120, 200, 200]},
    ["tr", {},
      ["tc", {"fill": "#F5F5F5"}, ["p", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf", "bold": true}, "维度"]]]],
      ["tc", {"fill": "#E8F5E9"}, ["p", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf", "bold": true}, "方案 A（推荐）"]]]],
      ["tc", {}, ["p", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf", "bold": true}, "方案 B"]]]]
    ],
    ["tr", {},
      ["tc", {}, ["p", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "上线周期"]]]],
      ["tc", {}, ["p", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf", "color": "#2E7D32"}, "1 周"]]]],
      ["tc", {}, ["p", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "3 周"]]]]
    ],
    ["tr", {},
      ["tc", {}, ["p", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "风险"]]]],
      ["tc", {}, ["p", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "低"]]]],
      ["tc", {}, ["p", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf", "color": "#C62828"}, "高：需新流程审批"]]]]
    ]
  ],
  ["h2", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "下一步"]]],
  ["p", {}, ["span", {"data-type": "text"},
    ["span", {"data-type": "leaf"}, "• "],
    ["span", {"data-type": "leaf", "highlight": "#FFF9C4"}, "待确认"],
    ["span", {"data-type": "leaf"}, " @负责人 完成权限配置"]
  ]]
]
```

**设计要点**：
- 根节点固定 `"root"`，不是 `"body"`
- 摘要用 `container`（callout），`metadata.bgcolor` 选浅绿表示"推荐结论"
- 表格用 `table → tr → tc`（无 th/td），表头底色用 `tc` 的 `"fill"` 属性
- 关键数据着色用 leaf 的 `"color"`（绿=好 / 红=风险）
- 状态标记用 leaf 的 `"highlight"`（黄=待确认、绿=完成、红=阻塞）
- uuid 可全部省略——CLI normalize 自动补充

## ⚠️ JSONML 结构严格约束（生成时必须遵守）

每个节点是一个 JSON 数组：`[tagName, attributes?, ...children]`

- **第一个元素**是字符串，表示标签名（如 `"p"`, `"h1"`, `"span"`, `"container"`）
- **第二个元素**（可选）是一个 JSON 对象，表示属性（如 `{"uuid": "abc"}`）。如果无属性，可以直接进入子节点
- **随后的元素**是子节点，可以是纯字符串（仅限 leaf span 内），也可以是另一个 JSONML 数组
- **所有 `[` 必须有对应 `]`，所有 `{` 必须有对应 `}`，数组元素之间用 `,` 分隔，最后一个元素后不加 `,`**

常见 LLM 生成错误（务必避免）：

| 错误类型 | 示例 | 后果 |
|---------|------|------|
| 缺少闭合 `]` | `["p", {}, ["span", ...]` | JSON 解析失败 |
| 多余逗号 | `["p", {},]` | JSON 解析失败 |
| 缺少逗号 | `["p", {} ["span"]]` | JSON 解析失败 |
| 引号不匹配 | `["p", {"uuid": "abc}]` | JSON 解析失败 |

## 文本节点格式（最重要）

钉钉文档的文本是**三层结构**，不是裸字符串：

```json
["p", {"uuid": "xxx"},
  ["span", {"data-type": "text"},
    ["span", {"data-type": "leaf"}, "文本内容"]
  ]
]
```

- **text 容器**：`["span", {"data-type": "text"}, ...leaves]` — 包裹所有文本 leaf
- **leaf 节点**：`["span", {"data-type": "leaf", ...格式属性}, "文字"]` — 实际文本，可带 bold/italic 等
- 一个 block 节点只有一个 text 容器，但可以有多个 leaf（不同格式的文字片段）

**简写**：无格式纯文本可以省略格式属性：
```json
["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "纯文本"]]
```

## 核心规则

1. **每个 block 节点应有 uuid**：`["tag", {"uuid": "唯一ID"}, ...children]`
   - insert 时可以自行生成任意唯一字符串，后端会自动分配正式 uuid
   - update 时 uuid **必须**与 `--block-id` 一致
   - normalize **仅在 attrs 槽完全缺失**（如 `["p", "text"]`）时才补 uuid；如果你写了 `["p", {}, ...]`，normalize 会尊重这个空 attrs 不再补 uuid（这一行为保证 `doc read → doc update` 不污染原文档）
2. **文本必须用 span + leaf 三层结构**，不要直接写裸字符串
   - ✅ `["p", {"uuid": "x"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "hello"]]]`
   - ❌ `["p", {"uuid": "x"}, "hello"]` — validator 会报错；默认模式下 CLI 会自动包成 ✅ 的形式并打印 `[FIX]` 行
   - ⚠️ `["p", {"uuid": "x"}, ["text", {}, "hello"]]` — `text` 是历史 inline tag，validator 不会报错，但建议改写为 ✅ 形式以与 `dws doc read --content-format jsonml` 的输出保持一致
3. **attrs 对象必须存在**（即使为空）：`["p", {}, ...]` 不能省略 `{}`

> **自动修复 vs 严格模式**：CLI 默认 normalize 会把 ❌ 的裸字符串自动包成 ✅；如要 1:1 透传原始 JSONML（例如复现服务端报错），用 `--no-fix-jsonml`，此时 validator 会以 `JSONPath + Suggestion` 形式逐条报错。如果输入来自 LLM 且可能有 JSON 语法错误（缺括号/逗号），用 `--fix-jsonml` 启用全部修复。

## 段落 (p)

```bash
# 纯文本段落
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["p", {"uuid": "new1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "这是一段普通文本"]]]'

# 带格式文本（多个 leaf）
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["p", {"uuid": "new2"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf", "bold": true}, "加粗"], ["span", {"data-type": "leaf"}, "普通"], ["span", {"data-type": "leaf", "italic": true}, "斜体"]]]'

# 带链接（link 是与 text 并列的子节点）
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["p", {"uuid": "new3"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "请访问"]], ["a", {"href": "https://example.com"}, "链接文字"]]'
```

**leaf 支持的格式属性**：
- `bold: true` — 加粗
- `italic: true` — 斜体
- `underline: true` — 下划线
- `strike: true` — 删除线
- `color: "#ff0000"` — 文字颜色
- `highlight: "#ffff00"` — 高亮背景色
- `sz: 14` / `szUnit: "pt"` — 字号

## 标题 (h1-h6)

```bash
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["h1", {"uuid": "new4"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "一级标题"]]]'

dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["h2", {"uuid": "new5"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "二级标题"]]]'

# 更新已有标题
dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --content-format jsonml \
  --element '["h2", {"uuid": "<BLOCK_ID>"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "修改后的标题"]]]'
```

## 列表 (list)

列表在 JSONML 中是 **带 `list` 属性的 `p` 节点**，不是独立 tag。

```bash
# 无序列表项
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["p", {"uuid": "li1", "list": {"listId": "mylist1", "level": 0, "isOrdered": false}}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "无序列表第一项"]]]'

# 有序列表项
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["p", {"uuid": "li2", "list": {"listId": "mylist2", "level": 0, "isOrdered": true, "start": 1}}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "有序列表第一项"]]]'

# 缩进子项（level: 1）
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["p", {"uuid": "li3", "list": {"listId": "mylist2", "level": 1, "isOrdered": true}}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "子列表项"]]]'

# 待办列表（checkbox）
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["p", {"uuid": "li4", "list": {"listId": "todo1", "level": 0, "isOrdered": false, "isTaskList": true, "isChecked": false}}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "待办事项"]]]'
```

## 引用 (blockquote)

引用是 **带 `quote` 属性的 `p` 节点**。

```bash
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["p", {"uuid": "q1", "quote": true}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "这是一段引用文字"]]]'
```

## 高亮块 / Callout (container)

```bash
# 蓝色高亮块
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["container", {"uuid": "co1", "subType": "colorBlocks", "metadata": {"bgcolor": "#E8F2FE", "border": "#B3D4FC"}}, ["p", {"uuid": "co1p1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "这是一段提示内容"]]]]'

# 黄色警告块（多段落）
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["container", {"uuid": "co2", "subType": "colorBlocks", "metadata": {"bgcolor": "#FFF2CC", "border": "#FFE599"}}, ["p", {"uuid": "co2p1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "⚠️ 注意事项"]]], ["p", {"uuid": "co2p2"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "请仔细阅读以下内容"]]]]'
```

**常用颜色预设**：

| 含义 | bgcolor | border |
|------|---------|--------|
| 信息（蓝） | `#E8F2FE` | `#B3D4FC` |
| 成功（绿） | `#E6F7E6` | `#B7EB8F` |
| 警告（黄） | `#FFF2CC` | `#FFE599` |
| 危险（红） | `#FFF1F0` | `#FFA39E` |
| 紫色 | `#F3E8FF` | `#D3ADF7` |

## 代码块 (code)

代码内容存在 attrs.code 中，不需要 text/leaf 子节点。

```bash
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["code", {"uuid": "cd1", "syntax": "javascript", "code": "function hello() {\n  return \"world\";\n}"}]'

dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["code", {"uuid": "cd2", "syntax": "python", "code": "print(\"hello\")", "showLineNumber": true, "theme": "dracula"}]'
```

## 分割线 (hr)

```bash
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["hr", {"uuid": "hr1"}]'
```

## 表格 (table)

```bash
# 2行2列表格
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["table", {"uuid": "tb1", "colsWidth": [200, 200]}, ["tr", {"uuid": "tr1"}, ["tc", {"uuid": "tc1", "colSpan": 1, "rowSpan": 1}, ["p", {"uuid": "tcp1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "标题A"]]]], ["tc", {"uuid": "tc2", "colSpan": 1, "rowSpan": 1}, ["p", {"uuid": "tcp2"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "标题B"]]]]], ["tr", {"uuid": "tr2"}, ["tc", {"uuid": "tc3", "colSpan": 1, "rowSpan": 1}, ["p", {"uuid": "tcp3"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "数据1"]]]], ["tc", {"uuid": "tc4", "colSpan": 1, "rowSpan": 1}, ["p", {"uuid": "tcp4"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "数据2"]]]]]]]'
```

> 表格较复杂时建议写入文件后用 `--element "$(cat table.json)"` 传入。

## 图片 (img)

> `img` 是 inline 元素，必须包裹在 `p` 段落中才能作为 block 插入。

```bash
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["p", {"uuid": "p-img1"}, ["img", {"uuid": "img1", "src": "https://example.com/photo.png", "width": 400, "height": 300}], ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, ""]]]'
```

## 分栏布局 (columns)

分栏复用 table tag，通过 `sr: true` 区分。

```bash
# 两栏布局
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["table", {"uuid": "col1", "sr": true, "colsWidth": [300, 300]}, ["tr", {"uuid": "coltr"}, ["tc", {"uuid": "coltc1"}, ["p", {"uuid": "colp1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "左栏内容"]]]], ["tc", {"uuid": "coltc2"}, ["p", {"uuid": "colp2"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "右栏内容"]]]]]]'
```

## 嵌入块 (embed)

通用文件/iframe 嵌入。`embed` 是 void 块，仅含 attrs，无子节点。

```bash
# 嵌入文件预览
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["embed", {"uuid": "em1", "name": "design.pdf", "type": "pdf", "src": "https://example.com/design.pdf", "size": 524288, "viewType": "preview", "previewSize": {"height": 600}}]'
```

**关键 attrs**：
- `src`（**必填**）— 资源 URL
- `type` — `pdf` / `xlsx` / `html` 等
- `name` — 展示名
- `previewSize.height` — 预览高度（px）

## 在线视频 (onlineVideo)

外链视频（B 站 / 优酷 / 自定义 mp4 等）。

```bash
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["onlineVideo", {"uuid": "ov1", "src": "https://player.bilibili.com/player.html?aid=12345", "type": "bilibili", "poster": "https://example.com/poster.jpg"}]'
```

**关键 attrs**：
- `src`（**必填**）— 视频播放页或 mp4 URL
- `type` — 平台标识（`bilibili` / `youku` / `mp4` 等）
- `poster` — 封面图 URL

## 卡片 (card)

群名片 / 应用卡片等富交互卡片。`cardType` 决定渲染形态，`metadata` 内容随类型变化。

```bash
# 群名片
dws doc block insert --node <DOC_ID> --content-format jsonml \
  --element '["card", {"uuid": "cd1", "cardType": "groupChatCard", "metadata": {"id": "63953109506", "name": "测试组", "inviteUrl": "https://qr.dingtalk.com/...", "expires": 1810865989}}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, ""]]]'
```

> 注意：服务端 serialize 出来的 card 通常带一个空的 span/leaf 占位子节点，建议保留以避免反序列化差异。

## 目录 (toc)

目录块 attrs 上必带 4 个字段：`title` / `mode` / `styles` / `content`。如果不知道怎么填，**最简方式**是先在 web 端插入一个 toc，然后 `block list --content-format jsonml` 把现成结构拿下来改。

```bash
dws doc block insert --node <DOC_ID> --content-format jsonml --element '
["toc", {
  "uuid": "toc1",
  "title": "目录",
  "mode": "outline",
  "styles": {
    "global": {"maxLevel": 5, "bgColor": "#F0EBF7", "css": {}},
    "title": {"font": "DingTalk JinBuTi", "color": "#6940A5", "numbering": true, "css": {"fontWeight": "normal"}},
    "item": {"symbol": "disc", "css": {}}
  },
  "content": []
}]'
```

**关键 attrs**：
- `mode` — `outline`（大纲）/ `column`（分栏）
- `styles.global.maxLevel` — 最大显示层级
- `content` — 目录条目数组；**留空数组即可**，服务端会基于文档 heading 自动重建

## 引用块 (refblock)

引用另一文档的内容片段。`refblock` 像容器一样包子节点。

```bash
dws doc block insert --node <DOC_ID> --content-format jsonml --element '
["refblock", {"uuid": "rb1", "docKey": "OTHER_DOC_NODE_ID", "refblockUUID": "BLOCK_UUID_IN_OTHER_DOC"},
  ["p", {"uuid": "rb1p1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "（引用预览内容，服务端会回填）"]]]
]'
```

**关键 attrs**：
- `docKey` — 被引用文档的 nodeId
- `refblockUUID` — 被引用块的 uuid

> 引用块的子节点是「快照」，真实内容由服务端按 docKey/refblockUUID 拉取覆写。

## 表格单元格嵌套块 (tableCell with nested blocks)

`tc` 的子节点是**块级节点**（不仅是 `p`）。可以塞多个段落、列表、代码块、甚至嵌套表格。

```bash
# 单元格内含多段落 + 代码块
dws doc block insert --node <DOC_ID> --content-format jsonml --element '
["table", {"uuid": "tbn1", "colsWidth": [400]},
  ["tr", {"uuid": "tbn1r1"},
    ["tc", {"uuid": "tbn1c1", "colSpan": 1, "rowSpan": 1},
      ["p", {"uuid": "tbn1p1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "标题段落"]]],
      ["p", {"uuid": "tbn1p2", "list": {"listId": "tbn1list", "level": 0, "isOrdered": false}}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "列表项 1"]]],
      ["p", {"uuid": "tbn1p3", "list": {"listId": "tbn1list", "level": 0, "isOrdered": false}}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "列表项 2"]]],
      ["code", {"uuid": "tbn1code", "syntax": "bash", "code": "echo hello"}]
    ]
  ]
]'
```

**规则**：
- `tc` 子节点 **必须是块节点数组**，不能直接放 `span` 或裸字符串
- 至少包含一个 `["p", {...}, ...]`（即使空也要），否则单元格无法渲染光标
- 单元格可嵌套 `table`，但嵌套时务必保证内层每个 `tc` 也满足上述规则

## Update 操作注意事项

1. **uuid 必须与 --block-id 一致**
2. Update 是**整块替换**，不是 patch — 需提供完整节点结构
3. 推荐流程：先 `block list --content-format jsonml --block-id <ID>` 获取当前结构，修改后写回

```bash
# 典型 update 流程
# 1. 获取当前结构
dws doc block list --node <DOC_ID> --content-format jsonml --block-id <BLOCK_ID>

# 2. 修改后写回（uuid 不变）
dws doc block update --node <DOC_ID> --block-id <BLOCK_ID> --content-format jsonml \
  --element '["p", {"uuid": "<BLOCK_ID>"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "修改后的内容"]]]'
```

## 常见错误

| 错误写法 | 问题 | 正确写法 |
|---------|------|---------|
| `["p", {}, "文字"]` | 裸字符串。validator 会报 `段落子节点不能是裸字符串`；默认 normalize 会自动包成右侧形式（打印 `[FIX]` 行） | `["p", {}, ["span", {"data-type":"text"}, ["span", {"data-type":"leaf"}, "文字"]]]` |
| `["p", {}, ["text", {}, "文字"]]` | `text` 是历史 inline tag，validator 不报错但服务端实际渲染的 canonical 形式是 span/leaf；为与 `doc read` 输出一致，建议改写 | 同上，用 span + data-type |
| `["callout", {}, ...]` | 不存在 callout tag | `["container", {"subType": "colorBlocks", ...}, ...]` |
| `["list", {}, ...]` | 不存在 list tag | `["p", {"list": {...}}, ...]` |
| `["blockquote", {}, ...]` | 不存在 blockquote tag | `["p", {"quote": true}, ...]` |
| `["ul", {}, ["li", ...]]` | 不存在 ul/li tag | 多个 `["p", {"list": {...}}, ...]` |

## 快捷模板

为方便使用，以下是最常用节点的最小完整模板：

```
纯文本段落:  ["p", {"uuid":"U"}, ["span", {"data-type":"text"}, ["span", {"data-type":"leaf"}, "TEXT"]]]
标题:        ["h2", {"uuid":"U"}, ["span", {"data-type":"text"}, ["span", {"data-type":"leaf"}, "TITLE"]]]
代码块:      ["code", {"uuid":"U", "syntax":"LANG", "code":"CODE"}]
分割线:      ["hr", {"uuid":"U"}]
```
