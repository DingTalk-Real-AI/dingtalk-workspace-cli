# OpenNodes V1 — 支持矩阵、富文本和样式

> 本文件是 DWS OpenNodes V1 协议的拆分章节。按需读取入口见
> [协议索引](../open-nodes-v1.md)。

## 7. 节点类型

### 7.1 支持矩阵

| `type` | query | update | 主要字段 |
| --- | --- | --- | --- |
| `shape` | 支持 | 支持 | `geometry`、`text?`、`style?` |
| `text` | 支持 | 支持 | `text`、`style?` |
| `connector` | 支持 | 支持 | `start`、`end`、`routing`、`waypoints?`、`style?` |
| `stickyNote` | 支持 | 支持 | `text?`、`style?` |
| `frame` | 支持 | 支持 | `title?`、`style?`、`presentationOrder?`、`resizeMode?` |
| `group` | 支持 | 支持 | 子关系通过 `parentId` 表达 |
| `image` | 支持 | 只读 | 仅公共字段 |
| `vector` | 支持 | 支持 | `resource` |
| `icon` | 支持 | 支持 | `catalogId` |
| `path` | 支持 | 支持 | `path`、`style?` |
| `pdf` | 支持 | 只读 | 仅公共字段 |
| `media` | 支持 | 只读 | 仅公共字段 |
| `webLink` | 支持 | 只读 | 仅公共字段 |
| `table` | 支持 | 只读 | 仅公共字段 |
| `chart` | 支持 | 只读 | 仅公共字段 |
| `uml` | 支持 | 只读 | 仅公共字段 |
| `swimlane` | 支持 | 只读 | 仅公共字段 |
| `mind` | 支持 | 只读 | 仅公共字段 |
| `timer` | 支持 | 只读 | 仅公共字段 |
| `placeholder` | 支持 | 只读 | 仅公共字段 |
| `unknown` | 支持 | 只读 | 未识别或尚未定义独立语义的节点统一映射到此类型 |

表中标记为只读的类型仍会完整返回公共几何、层级和顺序信息，但 update 提交会以
`nodeTypeUnsupported` 失败。

`timer`、`table`、`webLink` 不支持 V1 update。query 仅返回这些节点的公共几何、
层级、顺序和诊断字段，不承诺完整业务字段。文字 run 中的 `link` 是富文本能力，
不属于 `webLink` 节点，V1 支持读写。

`webLink` 只保留只读查询；OpenNodes V1 不支持创建或重建该节点。

### 7.2 Text

query 和 update 均支持普通段落、无序列表、有序列表、多 block、多 run 和文字
链接：

```ts
interface OpenTextRun {
  text: string;
  marks?: {
    fontFamily?: string;
    fontSize?: number;
    bold?: boolean;
    italic?: boolean;
    underline?: boolean;
    strike?: boolean;
    color?: string;
    highlight?: string;
  };
  link?: { url: string };
}

interface OpenTextBlock {
  type: "paragraph" | "bulletList" | "orderedList";
  horizontalAlign?: "left" | "center" | "right";
  runs: OpenTextRun[];
}

interface OpenTextWrite {
  blocks: OpenTextBlock[];
  verticalAlign?: "top" | "center" | "bottom";
  padding?: number | [number, number];
}

interface OpenText extends OpenTextWrite {
  plainText: string;
  writeSupport: "readWrite" | "readOnly";
  unsupportedFeatures?: string[];
}
```

约束：

- `blocks.length >= 1`，每个 block 的 `type` 必须是 `paragraph`、
  `bulletList` 或 `orderedList`。
- 每个 block 都必须满足 `runs.length >= 1`。
- run 的 `text` 不能包含 `\r`、`\n`、U+2028 或 U+2029。换行和列表项使用
  独立 block 表达；每一段或每个列表项应写成一个 block，而不是把原始换行符
  放进单个 run。
- 每个 `bulletList` / `orderedList` block 表示一个列表项；服务端会把连续且同类的
  block 解释为同一个列表中的多个列表项。
- `link.url` 长度必须为 `1..2048`，不能包含控制字符、`<`、`>`。支持无 scheme
  的相对/裸链接，以及 `http`、`https`、`mailto`、`tel`、`dingtalk` scheme；
  `javascript:`、`data:` 等可执行或未知 scheme 会被拒绝。
- 相邻且 URL 相同的 linked run 会呈现为同一个链接，同时保留各 run 自己的 marks。
- `fontSize > 0`，padding 各项必须大于等于 `0`。
- `plainText`、文本级 `writeSupport` 和 `unsupportedFeatures` 是 query-only。
- text 节点必须提供 `text`；shape 和 stickyNote 的 `text` 可省略。
- 受支持的 paragraph、列表、链接、多 run 都保持
  `writeSupport = "readWrite"`；未知 list style、非法链接、未支持的 block 或
  run marks 会令文本和所属节点变为 `readOnly`。

下面的文本会显示为两个段落，第一段由两个不同样式的 run 组成：

```json
{
  "blocks": [
    {
      "type": "paragraph",
      "horizontalAlign": "left",
      "runs": [
        {
          "text": "OpenNodes ",
          "marks": { "fontSize": 18, "bold": true, "color": "#2563EB" }
        },
        {
          "text": "rich text",
          "marks": { "fontSize": 18, "italic": true, "color": "#0F172A" }
        }
      ]
    },
    {
      "type": "paragraph",
      "horizontalAlign": "right",
      "runs": [
        {
          "text": "第二段",
          "marks": { "fontSize": 16, "underline": true, "color": "#047857" }
        }
      ]
    }
  ],
  "verticalAlign": "center",
  "padding": [4, 8]
}
```

同一 `OpenTextWrite` 结构适用于独立 text 节点、shape 文本、stickyNote 文本和
frame title。

下面三个 block 会生成两个无序列表项，其中第二项包含文字链接：

```json
{
  "blocks": [
    {
      "type": "bulletList",
      "runs": [{ "text": "准备输入数据" }]
    },
    {
      "type": "bulletList",
      "runs": [
        {
          "text": "查看钉钉文档",
          "marks": { "underline": true },
          "link": { "url": "https://alidocs.dingtalk.com" }
        }
      ]
    },
    {
      "type": "orderedList",
      "runs": [{ "text": "执行生成" }]
    }
  ]
}
```

颜色接受以下 CSS 形式：3/4/6/8 位十六进制、颜色名，以及
`rgb()`、`rgba()`、`hsl()`、`hsla()`、`oklch()`、`lab()`、`lch()`、
`color()`。字符串最长 128 字符，不能包含控制字符、`<`、`>` 或 `;`。
这里只接受可独立解析的字面量；依赖外部样式上下文的 `var()`、`calc()`、
`color-mix()` 等动态表达式不属于 V1。颜色名必须是标准 CSS named color，
任意字母串不会被当成颜色。

### 7.3 Style

query 可表达：

- `none`、`solid`、`theme`、线性渐变、径向渐变和图片 paint。
- shadow、blur 和 unknown effect。

V1 update 支持 `none`、`solid`、`theme`、线性渐变、九方向径向渐变，以及
单个自定义 shadow。主题明暗参数和渐变色标位置使用 `0～100` 的百分比，
不会归一化成 `0～1`：

```ts
type OpenRadialGradientPosition =
  | "topLeft"
  | "topCenter"
  | "topRight"
  | "centerLeft"
  | "center"
  | "centerRight"
  | "bottomLeft"
  | "bottomCenter"
  | "bottomRight";

interface OpenColorStop {
  offset: number;
  color: string;
  opacity?: number;
}

interface OpenImagePaint {
  type: "image";
  resource: {
    kind: "managed" | "external" | "embedded" | "unresolved";
    resourceId?: string;
  };
  intrinsicWidth: number;
  intrinsicHeight: number;
}

type OpenPaint =
  | { type: "none" }
  | { type: "solid"; color: string; opacity?: number }
  | {
      type: "theme";
      token: string;
      lumMod?: number;
      lumOff?: number;
      resolvedColor?: string;
    }
  | {
      type: "linearGradient";
      angle: number;
      stops: OpenColorStop[];
    }
  | {
      type: "radialGradient";
      position: OpenRadialGradientPosition | "custom";
      stops: OpenColorStop[];
    }
  | OpenImagePaint;

type OpenEffect =
  | {
      type: "shadow";
      offsetX: number;
      offsetY: number;
      blur: number;
      color: string;
      opacity: number;
    }
  | { type: "blur"; blur: number }
  | { type: "unknown" };

interface OpenNodeStyle {
  opacity?: number;
  fill?: OpenPaint;
  stroke?: {
    paint: OpenPaint;
    width?: number;
    dash?: number[];
    lineCap?: "butt" | "round" | "square";
    lineJoin?: "miter" | "round" | "bevel";
  };
  effects?: OpenEffect[];
  writeSupport: "readWrite" | "readOnly";
  unsupportedFeatures?: string[];
}

interface OpenColorStopWrite {
  offset: number; // [0, 100]，百分比
  color: string;
  opacity?: number; // [0, 1]
}

type OpenPaintWrite =
  | { type: "none" }
  | { type: "solid"; color: string; opacity?: number }
  | {
      type: "theme";
      token: string;
      lumMod?: number; // [0, 100]，默认 100
      lumOff?: number; // [0, 100]，默认 0
    }
  | {
      type: "linearGradient";
      angle: number;
      stops: OpenColorStopWrite[];
    }
  | {
      type: "radialGradient";
      position: OpenRadialGradientPosition;
      stops: OpenColorStopWrite[];
    };

interface OpenShadowEffectWrite {
  type: "shadow";
  offsetX: number;
  offsetY: number;
  blur: number;
  color: string;
  opacity: number;
}

interface OpenNodeStyleWrite {
  opacity?: number;
  fill?: OpenPaintWrite;
  stroke?: {
    paint: OpenPaintWrite;
    width?: number;
    dash?: number[];
    lineCap?: "butt" | "round" | "square";
    lineJoin?: "miter" | "round" | "bevel";
  };
  effects?: OpenShadowEffectWrite[];
}
```

约束：

- opacity 范围为 `[0, 1]`。
- solid paint 的 `color` 与 `opacity` 在 query 后仍保持独立字段，不会合并为
  动态 CSS 表达式。
- theme 的 `token` 必须能在当前白板主题中解析；
  `lumMod`、`lumOff` 范围均为 `[0, 100]`。token 不存在时在
  Request graph 阶段返回 `themeTokenNotFound`，不会写出部分节点。
  token 长度为 `1～64`，首尾不能有空白，也不能包含空白、控制字符、
  `<`、`>` 或 `;`。
- query 的 theme paint 还会返回按当前白板主题计算出的 query-only
  `resolvedColor`。调用方写入时只传 `token/lumMod/lumOff`，不传
  `resolvedColor`。
- 渐变必须至少有一个 stop；`offset` 范围为 `[0, 100]`，单位是百分比。
- 线性渐变 `angle` 范围为 `[0, 360]`。
- 径向渐变只接受上述九宫格位置。query 遇到九宫格以外的位置时返回
  `position: "custom"` 并将该节点标为只读。
- `effects` 最多包含一个 `shadow`；`offsetX/offsetY` 必须是有限数，
  `blur >= 0`，shadow opacity 范围为 `[0, 1]`。
- stroke width 和 dash 各项必须大于等于 `0`。
- 一旦提供 stroke，`stroke.paint` 必填。
- 样式级 `writeSupport` 和 `unsupportedFeatures` 是 query-only。
- 能在当前白板主题中解析且明暗参数合法的 theme paint 可写。无法解析的主题
  token、越界的主题明暗参数、image paint、blur、unknown effect、叠加 effect
  或自定义径向位置会令节点只读；受支持的 theme、渐变和单个 shadow 本身不会
  令节点只读。

主题色示例：

```json
{
  "fill": {
    "type": "theme",
    "token": "ac3",
    "lumMod": 20,
    "lumOff": 80
  },
  "stroke": {
    "paint": {
      "type": "theme",
      "token": "sk1",
      "lumMod": 80,
      "lumOff": 20
    },
    "width": 2
  }
}
```

示例中的 `ac3`、`sk1` 只是主题 token 示例，不是所有白板都可用的全局枚举。
调用方只能使用已确认可由当前白板主题解析的 token；无法确认时应改用
`solid` 颜色。

DWS 不提供新增、修改或切换主题的命令。当前白板没有有效主题时，应改用
`solid`。

示例：

```json
{
  "fill": {
    "type": "linearGradient",
    "angle": 35,
    "stops": [
      { "offset": 0, "color": "#1677ff", "opacity": 0.4 },
      { "offset": 100, "color": "#69b1ff" }
    ]
  },
  "effects": [
    {
      "type": "shadow",
      "offsetX": 8,
      "offsetY": 8,
      "blur": 19,
      "color": "rgba(93,190,172,1)",
      "opacity": 0.5
    }
  ]
}
```

未传 `style` 时使用节点类型的默认样式。调用方如需稳定的视觉结果，应显式传入
`fill` 和 `stroke`。
