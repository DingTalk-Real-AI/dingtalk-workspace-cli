# 文档 JSONML 节点结构参考

> **权威定义**：合法节点类型、允许的子节点和属性约束以 `wukong/products/jsonml-schema-v2.json` 为准。本文为可读版摘要，若与 schema-v2.json 冲突以后者为准。

本文档定义钉钉文档 body JSONML 中所有节点类型的结构，供 agent 编辑文档时参考。
**写法范例**见 [doc-jsonml-cookbook.md](./doc-jsonml-cookbook.md)；本文聚焦字段定义、枚举与约束。

## 格式说明

JSONML 是文档内容树的序列化格式：

```
[tag, attrs?, ...children]
```

- `tag` — 字符串，节点类型标识
- `attrs` — 可选对象，节点属性（写入时**强烈建议**始终传 `{}` 而非省略）
- `children` — 子节点数组；可以是嵌套节点或（仅 inline 上下文中）字符串

文档 body 是一个以 `"root"` 为根的 JSONML 节点，`dws doc read --content-format jsonml` 返回此格式：

```json
["root", {"sectPr": {"pgSz": {"w": 11906, "h": 16838}}},
  ["p", {"uuid": "p1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "第一段"]]],
  ["p", {"uuid": "p2"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "第二段"]]]
]
```

- 第一个元素固定为 `"root"`
- 第二个元素为文档级属性对象（如 `sectPr` 页面设置），可选
- 后续元素为块级节点（每个 block 节点应带 `uuid`）

全量覆写（overwrite）时，CLI 接受三种 body 形态：

1. `["root", {sectPr}, ...blocks]` — 服务端 canonical 形式，`doc read` 输出
2. `[blocks, ...]` — 纯块数组（缺少 root，validator 会 warn 但不阻断）
3. 单个 `[tag, ...]` — 当作单 block，validator warn

## CLI 行为概览（validator + normalize）

写入端（`doc create/update`、`block insert/update`）默认会走 **normalize → validate** 两步：

| 行为 | 缺省 | `--fix-jsonml` | `--no-fix-jsonml` |
|------|------|----------------|-------------------|
| JSON 语法修复（括号/逗号补全） | ✗ | ✓（打印 `[FIX]`） | ✗ |
| 解包单 block 为 body | ✓ | ✓ | ✗ |
| **attrs 槽完全缺失**时补 `attrs` + `uuid` | ✓ | ✓ | ✗ |
| 裸字符串 → `span/text + span/leaf` 自动包裹 | ✓（打印 `[FIX]`） | ✓（打印 `[FIX]`） | ✗ |
| validator 阻断（HasErrors → 拒绝发送） | ✓ | ✓ | ✓ |
| validator 警告（warnings → 仅 stderr） | ✓ | ✓ | ✓ |

> **uuid 注入的边界**：仅当 attrs 槽**完全缺失**（如 `["p", "text"]`）才补 `attrs` + `uuid`。当生产者已显式给出 `attrs`（哪怕是 `{}` 或不含 uuid），normalize 一律不再补 uuid —— 视为生产者明确意图。这避免 `doc read → doc update` 回灌时污染原文档中未修改的节点（真实文档中常见 `["h1", {}, ...]` 形态）。

三态设计：缺省 = 结构修复 ON + JSON 语法修复 OFF；`--fix-jsonml` 全开（含 JSON repair，推荐 agent 调用）；`--no-fix-jsonml` 全关（用于排查原始错误）。校验始终执行，不可跳过。

报错格式（面向 agent）：

```
$[2][2]: paragraph child must be span wrapper, got raw string.
Suggestion: ["span",{"data-type":"text"},["span",{"data-type":"leaf"},"<your text>"]]
```

`$` 表示输入根，`[i]` 是数组下标，`.attrs.k` 是属性名。

## 文本节点（Text）

**Canonical 形式**（服务端 serialize 输出、`doc read --content-format jsonml` 返回的就是这个）：

```json
["span", {"data-type": "text"},
  ["span", {"data-type": "leaf", "bold": true}, "加粗文本"],
  ["span", {"data-type": "leaf"}, "普通文本"]
]
```

- **text 容器**：`["span", {"data-type": "text"}, ...leaves]` — 包裹所有 leaf
- **leaf 节点**：`["span", {"data-type": "leaf", ...marks}, "<文本>"]` — 实际承载文字与样式
- 一个 block 通常只有一个 text 容器，可以含多个 leaf（不同样式片段）
- text 容器与 `["a", ...]`、`["img", ...]`、`["tag", ...]` 等其他 inline 节点**并列**作为 block 的子节点

### 文本样式属性（Marks，写在 leaf 的 attrs 上）

所有 marks 均为 optional，按需组合。

| 属性 | 类型 | 格式/枚举 | 说明 |
|------|------|-----------|------|
| `bold` | `boolean` | `true` / `false` | 加粗。`false` 可反向取消继承 |
| `italic` | `boolean` | `true` | 斜体 |
| `strike` | `boolean` | `true` / `false` | 单删除线 |
| `dstrike` | `boolean` | `true` / `false` | 双删除线（独立于 strike） |
| `underline` | `object` | `{value, color?}` | 下划线。value: `"single"` \| `"dash"` \| `"wave"` \| `"double"` \| `"none"` |
| `color` | `string` | CSS 颜色 | 文字颜色，如 `"#ff0000"` |
| `highlight` | `string` | CSS 颜色 | 文字高亮背景色 |
| `shd` | `object` | `{val?, color?, fill?}` | OOXML 底纹（Word 导入保留） |
| `sz` | `number` | 数值 | 字号，配合 `szUnit` |
| `szUnit` | `string` | `"px"` \| `"pt"` | 字号单位，默认 `"px"` |
| `fonts` | `object` | `{ascii, hAnsi, cs, eastAsia}` | OOXML 四分区字体 |
| `vertAlign` | `string` | `"superscript"` \| `"subscript"` \| `"baseline"` | 上标/下标/基线 |
| `spacing` | `number` | 数值（pt） | 字间距 |

示例：

```json
["span", {"data-type": "text"},
  ["span", {"data-type": "leaf", "bold": true, "italic": true, "color": "#1a73e8"}, "加粗斜体蓝字"]
]
["span", {"data-type": "text"},
  ["span", {"data-type": "leaf", "underline": {"value": "single", "color": "#ff0000"}, "sz": 14, "szUnit": "px"}, "红色下划线"]
]
```

### 历史/兼容形式

| 写法 | validator | 服务端 | 建议 |
|------|-----------|--------|------|
| `["span", {"data-type":"text"}, ["span", {"data-type":"leaf"}, "x"]]` | ✓ canonical | ✓ | ✅ 新内容首选 |
| `["text", {marks}, "x"]` | ✓（`text` 在 inline 白名单中） | ✓（兼容） | ⚠️ 历史 inline tag。`doc read` 不会输出这种形式；如需复制粘贴回写、保持与现有内容一致，建议改写为 canonical |
| `"raw string"` 作为 block 子节点 | ✗ 报错 `段落子节点不能是裸字符串` | — | 不要直接写。默认 normalize 会自动包成 canonical（打印 `[FIX]`） |

> Marks 表的属性集对 canonical 的 leaf 和 legacy 的 text 都适用；差别仅在承载位置（leaf 的 attrs vs text 的 attrs）。

---

## 块级节点

所有 block 节点的 tag 白名单（validator `validBlockTags`）：
`p` / `h1` / `h2` / `h3` / `h4` / `h5` / `h6` / `hr` / `table` / `code` / `container` / `embed` / `onlineVideo` / `card` / `toc` / `refblock` / `cangjie-voidblock` / `cangjie-container`

未在白名单的 tag 会触发 `未知的块级 tag` 警告，并给出基于编辑距离 (Levenshtein ≤2) 的最接近建议（如 `"containr"` → `did you mean "container"?`）。

### paragraph（段落）

- **tag**: `"p"`
- **attrs**（全部 optional）:
  - `jc?: "left" | "center" | "right" | "both" | "distribute" | "justify"` — 对齐
  - `ind?` — 缩进（单位 **twips**，1 twip = 1/1440 英寸）
    - `left?: number`, `right?: number`, `firstLine?: number`, `firstLineChars?: number`, `hanging?: number`
  - `spacing?` — 行间距（单位 **twips**；`line` 字段：`lineRule=auto` 时 1/240 行，否则 twips）
    - `line?: number`, `before?: number`, `after?: number`
    - `lineRule?: "atLeast" | "auto" | "exact"`
  - `shd?: {val?, fill?, color?}` — 底纹
  - `quote?: boolean` — 引用标识（注：服务端也接受 `blockquote: true` 的别名）
  - `list?: object` — 列表标识（见下方 list 节点）
  - `refs?: string[]` — 脚注引用 ID（footnote 标识）
- **children**: 一个 text 容器 + 可选的 inline 节点（link/img/tag/mention 等）
- **示例**:

```json
["p", {"uuid": "p1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "普通段落"]]]
["p", {"uuid": "p2", "jc": "center"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "居中段落"]]]
["p", {"uuid": "p3", "ind": {"firstLine": 420}}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "首行缩进段落"]]]
```

### heading（标题）

- **tag**: `"h1"` | `"h2"` | `"h3"` | `"h4"` | `"h5"` | `"h6"`
- **attrs**: 同 paragraph
- **children**: 同 paragraph
- **示例**:

```json
["h1", {"uuid": "h1a"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "一级标题"]]]
["h3", {"uuid": "h3a", "jc": "center"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "居中三级标题"]]]
```

### blockquote（引用）

- **tag**: `"p"` 或 `"h1"`~`"h6"`（不是独立 tag，是属性装饰）
- **标识**: `attrs.quote: true`（也接受 `blockquote: true`）
- **示例**:

```json
["p", {"uuid": "q1", "quote": true}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "这是一段引用"]]]
["h2", {"uuid": "q2", "quote": true}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "引用标题"]]]
```

### list（列表）

- **tag**: `"p"`（列表项在 JSONML 层面是扁平的段落，通过 `attrs.list` 标识）
- **attrs.list**:
  - `listId: string` — **必传**。同一列表的项共享相同 ID。validator 报错：`必传字段缺失`
  - `level: number` — **必传**。缩进层级（0-based，≥0）。validator 报错：`必传字段缺失` / `必须 ≥0`
  - `isOrdered?: boolean` — 是否有序，默认 `false`
  - `isTaskList?: boolean` — 是否任务列表，默认 `false`
  - `isChecked?: boolean` — 任务是否完成（仅 isTaskList=true 时有意义）
  - `isCanceled?: boolean` — 任务是否取消
  - `start?: number` — 有序列表起始序号（≥1）。validator 报错：`必须 ≥1`
  - `listStyleType?: string` — 样式类型（31 种预设）
  - `hideSymbol?: boolean` — 隐藏列表符号
- **示例**:

```json
["p", {"uuid": "li1", "list": {"listId": "abc", "level": 0, "isOrdered": false}},
  ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "无序列表项"]]]
["p", {"uuid": "li2", "list": {"listId": "def", "level": 0, "isOrdered": true, "start": 1}},
  ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "有序列表项"]]]
["p", {"uuid": "li3", "list": {"listId": "ghi", "level": 1, "isOrdered": false}},
  ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "二级缩进"]]]
["p", {"uuid": "li4", "list": {"listId": "jkl", "level": 0, "isTaskList": true, "isChecked": false}},
  ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "待办事项"]]]
```

- **注意**: 列表是扁平结构，不是嵌套的 ol/ul/li

### hr（分割线）

- **tag**: `"hr"`
- **attrs**（全部 optional）:
  - `type?: TLineStyle` — 16 种枚举（`"single"` / `"dotted"` / `"dashed"` / `"double"` / `"wave"` 等），默认 `"single"`
  - `sz?: number` — 粗细（px），默认 `1`
  - `color?: string` — 颜色
- **children**: 构造时无需传子节点；服务端返回的真实文档中可能含内部配置数据子节点
- **示例**:

```json
["hr", {"uuid": "hr1"}]
["hr", {"uuid": "hr2", "type": "dashed", "color": "#ccc", "sz": 2}]
```

### table（表格）

- **tag**: `"table"`
- **attrs**:
  - `colsWidth: number[]` — 列宽（语义必传），单位 **px**。缺失时 validator 警告 `table 应提供 colsWidth`
  - `sr?: boolean` — `true` 表示这是分栏布局（columns），不是普通表格
  - `jc?: string` — 对齐（源码注释"目前无消费"）
- **children**: `["tr", ...]` 行节点
- **示例**:

```json
["table", {"uuid": "tb1", "colsWidth": [200, 200]},
  ["tr", {"uuid": "tr1"},
    ["tc", {"uuid": "tc1", "colSpan": 1, "rowSpan": 1},
      ["p", {"uuid": "tcp1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "单元格1"]]]],
    ["tc", {"uuid": "tc2", "colSpan": 1, "rowSpan": 1},
      ["p", {"uuid": "tcp2"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "单元格2"]]]]
  ]
]
```

#### tr（表格行）

- **tag**: `"tr"`
- **attrs**: `{h?: number, isTblHeader?: boolean}`

#### tc（表格单元格）

- **tag**: `"tc"`
- **attrs**:
  - `colSpan?: number` — 横跨列数，默认 `1`。validator 报错：`必须 ≥1`
  - `rowSpan?: number` — 横跨行数，默认 `1`。validator 报错：`必须 ≥1`
  - `fill?: string` — 填充色
  - `vAlign?: "top" | "middle" | "bottom"` — 垂直对齐，默认 `"middle"`。validator 报错枚举不符（注意 `"center"` 不是合法值，应用 `"middle"`）
  - `bdr?: object` — 单元格边框
- **children**: 任意块级节点数组；**至少包含一个 `p`**（即使空），否则单元格无法渲染光标

### code（代码块）

- **tag**: `"code"`
- **attrs**（全部 optional）:
  - `code?: string` — 代码内容，默认 `""`
  - `syntax?: string` — 语言标识，默认 `"plaintext"`
  - `theme?: string` — 主题枚举：`"default"` / `"light"` / `"dracula"` / `"github"` / `"cobalt"` / `"atomOneDark"` / `"oneLightPro"` / `"nightOwl"` / `"githubDark"` / `"realDracula"`。未知值 validator 警告 `未知主题`
  - `wrap?: boolean` — 自动换行，默认 `true`
  - `showLineNumber?: boolean` — 显示行号，默认 `true`
  - `title?: string` — 标题（≤1000 字符）
  - `fold?: boolean` — 是否折叠，默认 `false`
- **children**: 构造时无需传子节点（代码存在 `attrs.code` 中）
- **示例**:

```json
["code", {"uuid": "cd1", "syntax": "javascript", "code": "console.log('hello');"}]
["code", {"uuid": "cd2", "syntax": "python", "code": "print('hello')", "theme": "dracula"}]
```

### container（容器/高亮块）

- **tag**: `"container"`
- **attrs**:
  - `subType: string` — **必传**。callout 为 `"colorBlocks"`。缺失时 validator 报错 `container 必须包含 subType`
  - `metadata?: object` — 自定义元数据（callout 用 `{bgcolor, border}`）
- **children**: 任意块级节点
- **示例（callout）**:

```json
["container", {"uuid": "co1", "subType": "colorBlocks", "metadata": {"bgcolor": "#E8F2FE", "border": "#B3D4FC"}},
  ["p", {"uuid": "co1p1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "这是一段提示内容"]]]
]
```

### columns（分栏布局）

- **tag**: `"table"`（复用 table tag，通过 `sr: true` 区分）
- **attrs**:
  - `sr: true` — 固定标识
  - `colsWidth?: number[]` — 各列宽度
  - `spacing?: number` — 栏间距
- **children**: 单个 `["tr", ...]`，内含多个 `["tc", ...]`
- **示例**:

```json
["table", {"uuid": "col1", "sr": true, "colsWidth": [300, 300]},
  ["tr", {"uuid": "colr"},
    ["tc", {"uuid": "colc1"},
      ["p", {"uuid": "colp1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "左栏"]]]],
    ["tc", {"uuid": "colc2"},
      ["p", {"uuid": "colp2"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "右栏"]]]]
  ]
]
```

### embed（嵌入文件）

- **tag**: `"embed"`（void 块）
- **attrs**:
  - `src?: string` — 文件来源 URL（语义必传）
  - `name?: string` — 文件名
  - `type?: string` — 文件类型（`"pdf"` / `"xlsx"` / `"html"` / `"file"` 等）
  - `size?: number` — 文件大小
  - `viewType?: string` — 视图类型（如 `"preview"`）
  - `previewSize?: {height: number}` — 预览高度（px）
- **children**: 无
- **示例**:

```json
["embed", {"uuid": "em1", "name": "report.pdf", "type": "pdf", "src": "https://example.com/report.pdf", "size": 2048, "viewType": "preview", "previewSize": {"height": 600}}]
```

### onlineVideo（在线视频）

- **tag**: `"onlineVideo"`（void 块）
- **attrs**（全部 optional，但 src 语义必传）:
  - `src?: string` — 视频地址
  - `type?: string` — 平台标识（`"bilibili"` / `"youku"` / `"mp4"` 等）
  - `poster?: string` — 封面图 URL
- **children**: 无
- **示例**:

```json
["onlineVideo", {"uuid": "ov1", "src": "https://example.com/video.mp4"}]
["onlineVideo", {"uuid": "ov2", "src": "https://player.bilibili.com/player.html?aid=1", "type": "bilibili", "poster": "https://example.com/poster.jpg"}]
```

### card（河图组件 / 富卡片）

- **tag**: `"card"`
- **attrs**:
  - `cardType: string` — 卡片类型标识（如 `"groupChatCard"`、`"vote"`）
  - `metadata: {id: string, ...}` — 组件元数据，必须包含 id
  - `height?: number`
- **children**: 服务端 serialize 通常返回带一个空的 span/leaf 占位子节点，写入时建议保留以避免反序列化差异
- **示例**:

```json
["card", {"uuid": "cd1", "cardType": "vote", "metadata": {"id": "card_abc123"}},
  ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, ""]]]
```

- **注意**: card 节点的重数据存储在独立 parts 层，body 中只存轻量引用

### toc（目录）

- **tag**: `"toc"`（void 块）
- **attrs**（全部 **required**，缺失逐个 validator 报错）:
  - `title: string` — 目录标题
  - `mode: "outline" | "column"` — 展示模式（其他值 validator 报错 `无效值`）
  - `styles: object` — 样式配置
    - `styles.global: {maxLevel: number, bgColor: string, css: object}`
    - `styles.title: {font: string, color: string, numbering: boolean, css: object}`
    - `styles.item: {symbol: "disc" | "none", css: object}`
  - `content: TocItem[]` — 目录条目数组；**写入时填 `[]` 即可**，服务端基于文档 heading 自动重建
    - 每项: `{uuid, anchorId: string|null, level, children: TocItem[]}`
- **children**: 无
- **示例**:

```json
["toc", {"uuid": "toc1", "title": "目录", "mode": "outline",
  "styles": {
    "global": {"maxLevel": 5, "bgColor": "#F0EBF7", "css": {}},
    "title": {"font": "", "color": "#000", "numbering": false, "css": {}},
    "item": {"symbol": "disc", "css": {}}
  },
  "content": []
}]
```

### refblock（引用块）

- **tag**: `"refblock"`
- **attrs**:
  - `docKey?: string` — 所属/被引文档标识（语义必传）
  - `refblockUUID?: string` — 引用块唯一标识（语义必传）
- **children**: 块级节点（降级显示快照；真实内容由服务端按 docKey/refblockUUID 拉取覆写）
- **示例**:

```json
["refblock", {"uuid": "rb1", "docKey": "doc123", "refblockUUID": "block456"},
  ["p", {"uuid": "rb1p1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "（引用预览内容，服务端会回填）"]]]
]
```

- **注意**: 主块 (host)：`docKey === refblockUUID`；副块 (copy)：`docKey !== refblockUUID`

---

## Inline 节点

所有 inline 节点 tag 白名单（validator `validInlineTags`）：
`text`（legacy） / `a` / `img` / `span` / `tag` / `inlineCode` / `br` / `cangjie-textinline` / `cangjie-voidinline`

block tag 也可出现在 inline 上下文（如 `img` 既是 block 又是 inline）。未在白名单的 tag 会触发警告并给出 Levenshtein 建议。

### a（链接）

- **tag**: `"a"`
- **attrs**（全部 optional，但 href 语义必传）:
  - `href?: string` — 链接地址（缺失时 validator 警告 `link 缺少 href`）
  - `cardInfo?: object` — 链接卡片信息
    - `cardInfo.displayType?: "link" | "card"` — 展示模式
    - `cardInfo.title?: string`, `cardInfo.desc?: string`, `cardInfo.imgURL?: string`
  - `metadata?: object` — 扩展业务信息
- **children**: 链接的展示文本（直接字符串，与 block 上下文不同）
- **示例**（作为 paragraph 的 inline 子节点）:

```json
["p", {"uuid": "p1"},
  ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "请访问"]],
  ["a", {"href": "https://example.com"}, "链接文字"]
]
```

### img（图片）

- **tag**: `"img"`（既可作 block 也可作 inline 子节点）
- **attrs**（全部 optional，但 src 语义必传）:
  - `src?: string` — 图片地址
  - `width?: number` — 宽度（px）
  - `height?: number` — 高度（px）
  - `rectClip?: {left?, right?, top?, bottom?}` — 裁剪比例（0-1）
  - `rotation?: number` — 旋转角度
  - `radius?: number` — 圆角（px）
  - `shadow?: string` — CSS shadow
  - `outline?: {width?, type?, color?}` — 边框
- **children**: 构造时无需传子节点；真实文档中可能含内部配置数据
- **示例**:

```json
["img", {"uuid": "img1", "src": "https://example.com/photo.png", "width": 400, "height": 300}]
```

- **注意**: 不存在 `layout` 属性，布局由 UI 层控制

### mention（@提及）

- **tag**: `"span"`
- **attrs**:
  - `data-type: "mention"` — **必传**，固定标识
  - `id?: string` — 被@用户 ID（语义必传）
  - `name?: string` — 被@用户名（语义必传）
  - `login?: string` — 登录名
  - `metadata?: object` — 扩展元数据
- **children**: 显示文本
- **示例**:

```json
["span", {"data-type": "mention", "id": "user123", "name": "张三"}, "@张三"]
```

### tag（通用标签节点）

- **tag**: `"tag"`
- **attrs**:
  - `tagType: string` — **必传**。子类型标识（如 `"formula"` / `"imTag"`）
  - 其他属性依 tagType 而异
- **示例**:

```json
["tag", {"tagType": "imTag", "text": "#标签名"}]
```

### formula（公式）— `tag` 节点的特化

- **tag**: `"tag"`
- **attrs**:
  - `tagType: "formula"` — **必传**，固定值
  - `metadata: {formula: string}` — **必传**。LaTeX 代码（空串表示空公式）。缺失或类型错时 validator 报错
- **children**: 无
- **示例**:

```json
["tag", {"tagType": "formula", "metadata": {"formula": "E=mc^2"}}]
```

### sticker（表情贴纸）

- **tag**: `"span"`
- **attrs**:
  - `data-type: "emoji"` — **必传**（注意：`"emoji"` 不是 `"sticker"`）
  - `code?: string` — 表情纯文本标识（如 `"[微笑]"`）
  - `newCode?: IEmoji` — 完整表情对象，5 种子类型：
    - `{type: "dingding", id, name, url}`
    - `{type: "unicode", value}`
    - `{type: "custom", url}`
    - `{type: "icon", id, color?}`
    - `{type: "svg", id, url, color?}`
- **children**: 无或空文本
- **示例**:

```json
["span", {"data-type": "emoji", "code": "[微笑]", "newCode": {"type": "unicode", "value": "😊"}}]
```

### inlineCode（行内代码）

- **tag**: `"inlineCode"`
- **attrs**: `{bgColor?: string}` 或 `{}`
- **children**: 文本（inline 子节点）
- **示例**:

```json
["inlineCode", {}, "const x = 1"]
```

### br（换行符）

- **tag**: `"br"`（void inline）
- **attrs**: `{}`
- **children**: 空文本占位
- **示例**:

```json
["br", {}, ""]
```

### refer（行内引用）

- **tag**: `"span"`
- **attrs**:
  - `data-type: "refer"` — 固定标识
  - 其他业务属性（自由 key-value）
- **示例**:

```json
["span", {"data-type": "refer", "docId": "xxx", "blockId": "yyy"}, "引用内容"]
```

---

## Cangjie 系列（扩展块）

### attachment（附件）

- **tag**: `"cangjie-voidblock"` 或 `"cangjie-voidinline"`
- **attrs**:
  - `subType: "attachment"` — 固定标识
  - `data.viewType: "preview" | "abstractCard"` — 视图类型
  - `data.fileData: {name?, src?, size?, fileType?, category: "media" | "file"}` — 文件数据
  - `data.previewSize?: {width?, height?}` — 预览尺寸
- **children**: 无
- **示例**:

```json
["cangjie-voidblock", {"subType": "attachment", "data": {"viewType": "abstractCard", "fileData": {"name": "report.pdf", "src": "https://...", "size": 2048, "category": "file"}}}]
```

- **注意**: 反序列化时匹配 `"embed"` tag 并转换为 attachment

### footnote（脚注）

- **tag**: 宿主节点 tag（`"p"` / `"h1"`~`"h6"`），不是独立节点
- **标识**: `attrs.refs: string[]`（脚注引用 ID 数组）
- **示例**:

```json
["p", {"uuid": "fn1", "refs": ["footnote-id-1"]},
  ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "正文文本"]]]
```

- **注意**: footnote 将 `refs` 属性注入到宿主 block 节点中，类似 blockquote 的属性装饰模式

### calendar（日程）

- **tag**: `"cangjie-voidinline"` 或 `"cangjie-voidblock"`
- **attrs**:
  - `subType: "calendar"` — 固定标识
  - `data.viewType: "inlineCalendar" | "blockCalendar"` — 形态
  - `data.calendarId: string` — 日程 ID
  - `data.subject: string` — 日程名称
  - `data.detailUrl: string` — 日程详情链接
- **示例**:

```json
["cangjie-voidinline", {"subType": "calendar", "data": {"viewType": "inlineCalendar", "calendarId": "cal-001", "subject": "周会", "detailUrl": "https://..."}}]
```

### label（标签）

- **tag**: `"cangjie-textinline"`
- **attrs**:
  - `subType: "label"` — 固定标识
  - `data.bgColor?: string` — 标签背景色
  - `data.color?: string` — 标签文字颜色
  - `data.labelType?: "normal" | "note" | "spoiler"` — 标签模式
- **children**: 文本内容
- **示例**:

```json
["cangjie-textinline", {"subType": "label", "data": {"bgColor": "#FFE8CC", "color": "#D46B08", "labelType": "normal"}}, "重要"]
```

### templateButton（模板按钮）

- **tag**: `"cangjie-container"`
- **attrs**:
  - `subType: "templateButton"` — 固定标识
  - `metadata.direction: "top" | "bottom"` — 按钮方向
  - `metadata.isOnce: boolean` — 是否一次性
  - `metadata.title: string` — 按钮标题
- **children**: 块级节点数组
- **示例**:

```json
["cangjie-container", {"subType": "templateButton", "metadata": {"direction": "bottom", "isOnce": false, "title": "添加待办"}},
  ["p", {"uuid": "tb1p1", "list": {"level": 0, "isChecked": false, "isOrdered": false, "isTaskList": true, "listId": "abc"}},
    ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, ""]]]
]
```

### textSlot（文本插槽）

- **tag**: `"cangjie-textinline"`
- **attrs**:
  - `subType: "textSlot"` — 固定标识
  - `data.slotInfo.style.color: string` — 文字颜色（支持渐变色）
- **children**: 文本内容
- **示例**:

```json
["cangjie-textinline", {"subType": "textSlot", "data": {"slotInfo": {"style": {"color": "#1890ff"}}}}, "插槽文本"]
```

---

## 完整文档示例

```json
["root", {"sectPr": {"pgSz": {"w": 11906, "h": 16838}}},
  ["h1", {"uuid": "h1"},
    ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "文档标题"]]],

  ["p", {"uuid": "p1"},
    ["span", {"data-type": "text"},
      ["span", {"data-type": "leaf"}, "这是一段普通文本，包含"],
      ["span", {"data-type": "leaf", "bold": true}, "加粗"],
      ["span", {"data-type": "leaf"}, "和"]],
    ["a", {"href": "https://example.com"}, "链接"],
    ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "。"]]],

  ["h2", {"uuid": "h2"},
    ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "列表示例"]]],
  ["p", {"uuid": "li1", "list": {"listId": "l1", "level": 0, "isOrdered": true, "start": 1}},
    ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "第一项"]]],
  ["p", {"uuid": "li2", "list": {"listId": "l1", "level": 0, "isOrdered": true, "start": 2}},
    ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "第二项"]]],
  ["p", {"uuid": "li3", "list": {"listId": "l1", "level": 1, "isOrdered": false}},
    ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "子项"]]],

  ["hr", {"uuid": "hr1"}],

  ["code", {"uuid": "cd1", "syntax": "javascript", "code": "function hello() {\n  return 'world';\n}"}],

  ["container", {"uuid": "co1", "subType": "colorBlocks", "metadata": {"bgcolor": "#E8F2FE"}},
    ["p", {"uuid": "co1p1"},
      ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "这是一个提示块"]]]],

  ["table", {"uuid": "tb1", "colsWidth": [200, 200]},
    ["tr", {"uuid": "tr1"},
      ["tc", {"uuid": "tc1", "colSpan": 1, "rowSpan": 1},
        ["p", {"uuid": "tcp1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "A"]]]],
      ["tc", {"uuid": "tc2", "colSpan": 1, "rowSpan": 1},
        ["p", {"uuid": "tcp2"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "B"]]]]
    ]
  ]
]
```

## 设计要点

1. **Canonical 文本是 span/leaf**：每段文字 = `["span", {"data-type":"text"}, ["span", {"data-type":"leaf", ...marks}, "..."]]`。legacy `["text", {marks}, "..."]` 仍被接受但不建议新写。
2. **裸字符串作 block 子节点违法**：validator 报错；默认 normalize 自动包成 canonical 并打印 `[FIX]`；可用 `--no-fix-jsonml` 关闭。
3. **每个 block 必带 `uuid`**：手写 JSONML 时建议每个 block 自带 `uuid`（base32 alphanumeric，dws CLI 用 `dws` 前缀）。写入端 normalize **仅在 `attrs` 槽完全缺失（如 `["p", "text"]`）** 时自动补一个；只要 `attrs` 槽存在（即使是 `{}`），normalize 不再补 uuid —— 这保证 `doc read → doc update` 回灌不会污染原文档节点。
4. **扁平列表**: 列表不嵌套，通过 `listId` + `level` 表达层级。
5. **属性装饰**: blockquote / list / footnote 不是独立 tag，是 paragraph 的属性。
6. **Void 节点**: hr / code / img / card / toc / embed / onlineVideo 在构造时不需要传子节点；服务端返回的真实文档中这些节点**可能包含内部配置数据子节点**，解析时应兼容。
7. **columns = table + sr:true**: 分栏复用表格结构。
8. **card 轻引用**: body 中只存 cardType + metadata.id，重数据在 parts 层。
9. **root 节点**: 服务端返回的完整 body 以 `["root", {sectPr...}, ...blocks]` 包裹，写入时可直接使用此格式或纯块列表格式。
