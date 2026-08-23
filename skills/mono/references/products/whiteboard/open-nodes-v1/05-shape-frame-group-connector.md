# OpenNodes V1 — Shape、Text、Sticky note、Frame、Group 和 Connector

> 本文件是 DWS OpenNodes V1 协议的拆分章节。按需读取入口见
> [协议索引](../open-nodes-v1.md)。

### 7.4 Shape

shape 必须提供 `geometry`，格式为 `dml:<name>`：

```json
{
  "id": "shape-1",
  "type": "shape",
  "x": 100,
  "y": 80,
  "width": 160,
  "height": 100,
  "geometry": "dml:roundRect",
  "style": {
    "fill": {
      "type": "solid",
      "color": "#DCEEFF"
    },
    "stroke": {
      "paint": {
        "type": "solid",
        "color": "#225588"
      },
      "width": 2
    }
  }
}
```

query 可能返回 `adjustments`，但 V1 update 不支持写入；带 adjustments 的
shape 会标为只读。`dml-v1` 的完整 geometry 目录见附录 A。

### 7.5 Text node 与 Sticky note

text node 使用公共几何、必填 `text` 和可选 `style`。

stickyNote 使用公共几何、可选 `text` 和可选 `style`。省略 `text` 时创建空
便签。query 还可能返回：

- `creator`：创建者展示信息。
- `tags`：标签 ID、文本和背景 paint。

这两个字段是 query-only；存在 creator 或 tags 的 stickyNote 会被标为只读。

### 7.6 Frame

frame 的 update 结构见第 6 节 `OpenFrameNodeWrite`。

约束：

- frame 必须是页面直属节点，不能带 `parentId`。
- angle 只允许 `0`。
- `presentationOrder` 是非负整数，并且不能和已有或本次创建的 frame 冲突。
- frame 的子节点通过子节点 `parentId` 引用 frame 临时 ID。
- frame 不能包含 frame 或 connector。
- frame 默认 layer 为 `background`。

### 7.7 Group

group 的写入字段只有公共字段中的 `id`、`parentId?`、`x`、`y`、`layer?`、
`zIndex?` 和 `hidden?`。其 width、height、angle 和 children 都由服务端根据
子节点推导。

group 必须：

- 提供临时 `id`。
- 至少包含两个直接子节点。
- 至少有一个直接子节点可见。

group 的任一子节点为只读时，query 会把 group 一并标为只读。

### 7.8 Connector

connector 的几何由端点和路由推导，因此 update 不能提供 `x`、`y`、
`width`、`height` 或 `angle`。

query 的连接线结构如下：

```ts
type OpenConnectorRouting =
  | "straight"
  | "polyline"
  | "curve"
  | "orthogonal";

interface OpenConnectorMarker {
  catalogId: string;
}

type OpenConnectorAnchor =
  | {
      mode: "fixed";
      side: "top" | "right" | "bottom" | "left";
      position: OpenPoint;
    }
  | {
      mode: "fixed";
      side: "custom";
      position: OpenPoint;
    };

type OpenConnectorEndpoint =
  | {
      type: "point";
      point: OpenPoint;
      marker: OpenConnectorMarker;
    }
  | {
      type: "node";
      nodeRef: { scope: "document"; id: string };
      anchor: OpenConnectorAnchor;
      resolvedPoint: OpenPoint;
      marker: OpenConnectorMarker;
    };

interface OpenBezierSegment {
  start: OpenPoint;
  control1: OpenPoint;
  control2: OpenPoint;
  end: OpenPoint;
}

type OpenResolvedConnectorPath =
  | { type: "polyline"; points: OpenPoint[] }
  | { type: "bezier"; segments: OpenBezierSegment[] };
```

update 端点有两种形式：

```ts
type OpenConnectorEndpointWrite =
  | {
      type: "point";
      point: { x: number; y: number };
      marker?: { catalogId: "none" | "arrow.open" | "arrow.filled" };
    }
  | {
      type: "node";
      nodeRef: { scope: "request"; id: string };
      anchor?:
        | { mode: "auto" }
        | {
            mode: "fixed";
            side: "top" | "right" | "bottom" | "left";
          };
      marker?: { catalogId: "none" | "arrow.open" | "arrow.filled" };
    };
```

query 的 marker `catalogId` 使用 `string`，以便返回无法映射的既有 marker；
update 只接受上面列出的 `"none"`、`"arrow.open"`、`"arrow.filled"`。

路由规则：

| `routing` | `waypoints` |
| --- | --- |
| `straight` | 禁止提供，包括空数组。 |
| `polyline` | 必须至少提供一个。 |
| `curve` | 可选。 |
| `orthogonal` | 可选；显式点路径的相邻线段必须水平或垂直。 |

其他约束：

- 所有 point 和 waypoint 都使用页面绝对坐标。
- node 端点只能引用同一请求中的 shape、text、stickyNote、frame、group 或 path。
- node 端点不能引用隐藏节点、connector 或 query 中既有节点。
- update 引用范围固定为 `scope: "request"`；query 返回的节点引用范围为
  `scope: "document"`，不能直接回写。
- 同一连接线的两端不能引用同一个节点。
- 零长度或无效路径会被拒绝。
- marker 省略时默认为 `none`。
- anchor 省略时按 `auto` 处理。

query 额外返回服务端解析后的：

- node 端点 `resolvedPoint`。
- fixed anchor 的归一化 `position`。
- `resolvedPath`，类型为 polyline points 或 cubic bezier segments。
- 由真实路径推导的 `absoluteBounds`。

这些解析字段都是 query-only。
