# OpenNodes V1 — Query 请求、返回结构和节点公共字段

> 本文件是 DWS OpenNodes V1 协议的拆分章节。按需读取入口见
> [协议索引](../open-nodes-v1.md)。

## 4. Query 协议

### 4.1 请求

内嵌白板（显式 partId，保持原有调用）：

```bash
dws whiteboard query \
  --node <DOC_NODE_ID> \
  --part-id <WHITEBOARD_PART_ID> \
  --format json
```

独立白板（完全省略 partId）：

```bash
dws whiteboard query --node <WHITEBOARD_NODE_ID> --view summary --format json
dws whiteboard query --node <WHITEBOARD_NODE_ID> --view page \
  --page-id <PAGE_ID> --format json
dws whiteboard query --node <WHITEBOARD_NODE_ID> --view all --format json
```

内嵌分支不接收请求体、`--view` 或 `--page-id`；CLI 会把原接口返回的
`resultJson` JSON 字符串解析成对象。独立分支的 `view=page` 必须带 pageId，其他
view 禁止 pageId。`--part-id` 显式空值报错；服务错误不得跨类型重试。

### 4.2 返回结构

```ts
interface OpenNodesDocument {
  schemaVersion: "1.0";
  catalogVersion: "dml-v1";
  pages: OpenPage[];
}

interface OpenPage {
  id: string;
  nodes: OpenNode[];
}
```

内嵌分支固定返回一个页面，调用方仍应从返回值读取页面 `id`，但不能把它作为
pageId 传给内嵌调用。独立分支的 `view=page/all` 可返回页面内容，`view=summary`
仅用于取得白板元信息和当前 revision；应从实际响应读取页面 ID 和 revision。

母版节点不会作为独立页面返回，而会合并到引用它的页面 `nodes` 中，并带有：

```json
{
  "source": "master",
  "writeSupport": "readOnly",
  "unsupportedFeatures": ["node.source.master"]
}
```

### 4.3 节点公共字段

`type` 的完整公开枚举如下。前一组可由 update 创建，后一组仅供 query 返回：

```ts
type WritableOpenNodeType =
  | "shape"
  | "text"
  | "connector"
  | "stickyNote"
  | "frame"
  | "group"
  | "vector"
  | "icon"
  | "path";

type ReadOnlyOpenNodeType =
  | "image"
  | "pdf"
  | "media"
  | "webLink"
  | "table"
  | "chart"
  | "uml"
  | "swimlane"
  | "mind"
  | "timer"
  | "placeholder"
  | "unknown";

type OpenNodeType = WritableOpenNodeType | ReadOnlyOpenNodeType;
```

每个 query 节点都包含以下公共字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | `string` | 白板中真实、稳定的节点 ID。 |
| `type` | `OpenNodeType` | OpenNodes 公开语义类型，取值见上方枚举。 |
| `parentId` | `string?` | group/frame 父节点 ID。无父节点时省略。 |
| `children` | `string[]?` | group/frame 的直接子节点 ID，由服务端推导。 |
| `x`、`y` | `number` | 有父节点时相对父节点；否则相对页面。单位为 px。 |
| `width`、`height` | `number` | 节点包围盒尺寸，单位为 px。连接线允许其中一个为 `0`。 |
| `angle` | `number` | 归一化到 `[0, 360)` 的角度。 |
| `absoluteBounds` | `OpenBounds` | 页面坐标系中的绝对包围盒。 |
| `layer` | `background \| normal \| foreground` | 节点所在层。 |
| `zIndex` | `number` | 同一父节点、同一 layer 内的非负顺序，值越小越靠后。 |
| `hidden` | `boolean` | 节点是否隐藏。 |
| `locked` | `boolean` | 节点是否锁定。 |
| `source` | `page \| master` | 节点来自当前页面还是母版。 |
| `writeSupport` | `readWrite \| readOnly` | 当前节点能否由 V1 update 表达。 |
| `unsupportedFeatures` | `string[]?` | 只读原因；`readWrite` 节点省略。 |

公共结构和各节点分支定义如下；分支中的字段含义与写入限制见第 7 节：

```ts
interface OpenPoint {
  x: number;
  y: number;
}

interface OpenBounds {
  x: number;
  y: number;
  width: number;
  height: number;
  angle: number;
}

interface OpenNodeBase {
  id: string;
  type: OpenNodeType;
  parentId?: string;
  children?: string[];
  x: number;
  y: number;
  width: number;
  height: number;
  angle: number;
  absoluteBounds: OpenBounds;
  layer: "background" | "normal" | "foreground";
  zIndex: number;
  hidden: boolean;
  locked: boolean;
  source: "page" | "master";
  writeSupport: "readWrite" | "readOnly";
  unsupportedFeatures?: string[];
}

interface OpenShapeNode extends OpenNodeBase {
  type: "shape";
  geometry: `dml:${string}`;
  adjustments?: Record<string, number>;
  text?: OpenText;
  style?: OpenNodeStyle;
}

interface OpenTextNode extends OpenNodeBase {
  type: "text";
  text: OpenText;
  style?: OpenNodeStyle;
}

interface OpenConnectorNode extends OpenNodeBase {
  type: "connector";
  start: OpenConnectorEndpoint;
  end: OpenConnectorEndpoint;
  routing: OpenConnectorRouting;
  waypoints?: OpenPoint[];
  style?: OpenNodeStyle;
  resolvedPath: OpenResolvedConnectorPath;
}

interface OpenStickyNoteNode extends OpenNodeBase {
  type: "stickyNote";
  text?: OpenText;
  style?: OpenNodeStyle;
  creator?: {
    displayName?: string;
    hasAvatar?: boolean;
  };
  tags?: Array<{
    id: string;
    text: string;
    background: OpenPaint;
  }>;
}

interface OpenFrameNode extends OpenNodeBase {
  type: "frame";
  title?: {
    text: OpenText;
    box: { width: number; height: number };
  };
  style?: OpenNodeStyle;
  presentationOrder?: number;
  resizeMode: "free" | "fixedAspectRatio";
}

interface OpenGroupNode extends OpenNodeBase {
  type: "group";
  children: string[];
}

interface OpenVectorNode extends OpenNodeBase {
  type: "vector";
  resource: OpenVectorResource;
}

interface OpenIconNode extends OpenNodeBase {
  type: "icon";
  catalogId: string;
}

interface OpenPathNode extends OpenNodeBase {
  type: "path";
  path: OpenPathData;
  style?: OpenNodeStyle;
}

interface OpenReadOnlyNode extends OpenNodeBase {
  type: ReadOnlyOpenNodeType;
  writeSupport: "readOnly";
  unsupportedFeatures: string[];
}

type OpenNode =
  | OpenShapeNode
  | OpenTextNode
  | OpenConnectorNode
  | OpenStickyNoteNode
  | OpenFrameNode
  | OpenGroupNode
  | OpenVectorNode
  | OpenIconNode
  | OpenPathNode
  | OpenReadOnlyNode;
```

query 中 `icon.catalogId` 使用 `string`，是为了让无法映射到当前目录的既有图标仍可
被诊断；update 只能传入 7.10 节 `OpenIconCatalogId` 中列出的值。

### 4.4 Query 示例

```json
{
  "schemaVersion": "1.0",
  "catalogVersion": "dml-v1",
  "pages": [
    {
      "id": "page",
      "nodes": [
        {
          "id": "real-node-id",
          "type": "text",
          "x": 120,
          "y": 80,
          "width": 240,
          "height": 48,
          "angle": 0,
          "absoluteBounds": {
            "x": 120,
            "y": 80,
            "width": 240,
            "height": 48,
            "angle": 0
          },
          "layer": "normal",
          "zIndex": 0,
          "hidden": false,
          "locked": false,
          "source": "page",
          "writeSupport": "readWrite",
          "text": {
            "blocks": [
              {
                "type": "paragraph",
                "horizontalAlign": "left",
                "runs": [
                  {
                    "text": "Hello OpenNodes",
                    "marks": {
                      "fontSize": 16,
                      "color": "#223344"
                    }
                  }
                ]
              }
            ],
            "verticalAlign": "center",
            "padding": [2, 4],
            "plainText": "Hello OpenNodes",
            "writeSupport": "readWrite"
          }
        }
      ]
    }
  ]
}
```
