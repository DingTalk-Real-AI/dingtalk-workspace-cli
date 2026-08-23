# OpenNodes V1 — Vector、Icon 和 Path

> 本文件是 DWS OpenNodes V1 协议的拆分章节。按需读取入口见
> [协议索引](../open-nodes-v1.md)。

### 7.9 Vector（已上传 SVG/矢量资源）

`vector` 表示已经通过 `dws doc media upload` 获得稳定引用的 SVG/矢量图片。
OpenNodes 只接收上传结果中的资源引用，不接收本地路径或原始 SVG/XML 内容。

上传时必须使用与后续白板更新相同的文档 `nodeId`：

```bash
dws doc media upload \
  --node <DOC_NODE_ID> \
  --file ./icon.svg \
  --mime-type image/svg+xml \
  --format json
```

将上传结果的 `resourceId` 和 `resourceUrl` 分别写入
`resource.resourceId` 和 `resource.url`。

Update 必须提供完整的托管资源信息：

```ts
interface OpenManagedVectorResourceWrite {
  kind: "managed";
  resourceId: string;
  url: string;
}
```

完整节点结构见第 6 节 `OpenVectorNodeWrite`。

`resource` 字段规则：

| 字段 | 必填 | 规则 |
| --- | --- | --- |
| `kind` | 是 | 当前只允许固定值 `managed`，表示资源已经通过 DWS 上传。 |
| `resourceId` | 是 | 资源稳定 ID，长度 1～256，只允许字母、数字、`.`、`_`、`:`、`-`。 |
| `url` | 是 | 已上传资源地址，最长 4096；必须包含且只能包含一个同值的 `resourceId` 查询参数。 |

`url` 必须直接使用 `dws doc media upload` 返回的 `resourceUrl`，不得自行拼装
或修改。

以下内容会被拒绝：

- 原始 SVG/XML、`data:`、`blob:`、`http:` URL。
- `//host/path` 协议相对地址，以及自行构造或修改的其他相对地址。
- 含空白、控制字符、反斜杠、fragment 或用户凭证的 URL。
- URL 缺少 `resourceId`、重复出现 `resourceId`，或者 URL 中 ID 与显式
  `resource.resourceId` 不一致。
- `resource` 缺失、字段不完整、`kind` 不是 `managed`，或包含未知字段。

完整 Append 示例：

```json
{
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "uploaded-svg-1",
        "type": "vector",
        "x": 100,
        "y": 80,
        "width": 240,
        "height": 180,
        "angle": 0,
        "resource": {
          "kind": "managed",
          "resourceId": "0c1c94e1-f9af-4228-b32f-42bbd1555253",
          "url": "https://resources.example.com/assets/opaque-path?resourceId=0c1c94e1-f9af-4228-b32f-42bbd1555253"
        }
      }
    ]
  },
  "overwrite": false
}
```

示例中的 `resources.example.com` 是占位域名，实际调用必须使用
`dws doc media upload` 返回的 `resourceUrl`。

`resourceId` 同时显式出现并包含在 URL 中，用于校验资源身份与地址是否一致。
两者必须来自同一次 `dws doc media upload` 结果。

Query 的 `resource` 可能是：

```ts
type OpenVectorResource =
  | { kind: "managed"; resourceId: string; url: string }
  | { kind: "external" | "embedded" | "unresolved" };
```

- 能识别为托管资源的引用返回完整 `managed` 信息。
- HTTP(S) 外链但不满足托管资源契约时返回 `external`。
- `data:`/`blob:` 返回 `embedded`。
- 其他缺失或无法识别的地址返回 `unresolved`。
- 非 `managed` 资源会令节点只读，并分别产生
  `vector.resource.external`、`vector.resource.embedded` 或
  `vector.resource.unresolved`。

V1 vector update 不接受 `style`。既有节点包含 V1 无法表达的 fill、stroke、
opacity、effect 或 adjustments 时，query 仍可读取资源和几何，但节点会标为只读。
不影响资源内容的兼容性装饰不会单独令节点变为只读。

DWS 会校验资源引用。上传与 `whiteboard update` 必须使用同一个文档
`nodeId`；不要跨文档复用资源，也不要使用临时 `uploadUrl`。

### 7.10 Icon（内置图标）

`icon` 表示内置图标。OpenNodes 使用版本化的 `catalogId` 作为稳定标识，
允许值见下列类型定义和附录 B。

Update 结构：

```ts
type OpenIconCatalogId =
  | `emoji/${
      | "happy"
      | "smile"
      | "laugh"
      | "fighting"
      | "like"
      | "ok"
      | "please"
      | "face-plam"
      | "tears-of-joy"
      | "cry"
      | "question"
      | "face-with-sweat"
      | "bloody-nose"
      | "doggy"}`
  | `tools/${
      | "pad"
      | "blue-note"
      | "yellow-notes"
      | "chart"
      | "chart-2"
      | "pencil"
      | "pen"
      | "bag"
      | "rocket"
      | "fire"
      | "gold"
      | "light"
      | "pin"
      | "red-flag"
      | "tea"
      | "island"
      | "ball"
      | "lucky-fish"
      | "coffee"
      | "milky-tea"
      | "pan"}`
  | `priority/priority-${1 | 2 | 3 | 4 | 5 | 6 | 7}`
  | `task/${
      | "task-start"
      | "task-oct"
      | "task-3oct"
      | "task-half"
      | "task-5oct"
      | "task-7oct"
      | "task-done"}`;
```

完整节点结构见第 6 节 `OpenIconNodeWrite`。

完整 Append 示例：

```json
{
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "pencil-icon",
        "type": "icon",
        "x": 100,
        "y": 80,
        "width": 48,
        "height": 48,
        "catalogId": "tools/pencil"
      }
    ]
  },
  "overwrite": false
}
```

规则：

- `catalogId` 必填，格式为 `<group>/<name>`，并且必须精确命中附录 B 的
  `dml-v1` allowlist；大小写、连字符和历史拼写都不能自行修正。
- 当前目录有 `emoji`、`tools`、`priority`、`task` 四组，共 49 项。
- update 只接受 `catalogId`，不接受 `group`、`name`、`style` 或 `resource`。
- 内置 icon 不需要调用方上传资源，也不需要 `resourceId`。
- 任意已上传 SVG 或自定义图标应使用 `vector`，并按 7.9 节提供完整
  `resource` 信息，不能伪造一个 icon `catalogId`。
- icon 可以作为 group/frame 子节点；V1 connector 的 node 端点当前仍不接受
  icon 作为目标。

Query 返回相同的 `catalogId`。既有 icon 无法映射到当前目录时，节点仍会以
`type: "icon"` 返回以便诊断，但 `writeSupport` 为 `readOnly`，原因包含
`icon.catalogId`。非默认 opacity、filter/effect、text 或 adjustments 同样会令节点
只读。

### 7.11 Path（自由画笔）

`path` 表示自由画笔轨迹。OpenNodes 保留两组互相独立的尺寸：

- 节点公共 `width` / `height` 是画布上的实际渲染尺寸，缩放节点时会变化。
- `path.intrinsicWidth` / `path.intrinsicHeight` 是 SVG path 自身的坐标空间尺寸，
  表示 `path.data` 使用的内部坐标空间。

```ts
interface OpenPathData {
  data: string;
  intrinsicWidth: number;
  intrinsicHeight: number;
}

type OpenPathDataWrite = OpenPathData;
```

完整 update 节点结构见第 6 节 `OpenPathNodeWrite`。query 和 update 的 `path`
字段结构相同，但 update 仍必须满足下方命令子集和大小限制。

```json
{
  "id": "freehand-stroke",
  "type": "path",
  "x": 120,
  "y": 100,
  "width": 500,
  "height": 150,
  "path": {
    "data": "M0,75 Q50,0 100,75 Q150,150 200,75 Q250,0 300,75 Q350,150 400,75 Q450,0 500,75",
    "intrinsicWidth": 500,
    "intrinsicHeight": 150
  },
  "style": {
    "fill": { "type": "none" },
    "stroke": {
      "paint": { "type": "solid", "color": "#7C3AED" },
      "width": 10,
      "lineCap": "round",
      "lineJoin": "round"
    }
  }
}
```

下面是一个仍然只使用 V1 命令子集、但包含 12 段二次贝塞尔曲线的蝴蝶轮廓。
它显式回到起点，因此不需要使用尚未支持的 `Z`：

```json
{
  "id": "complex-butterfly-path",
  "type": "path",
  "x": 1500,
  "y": 1215,
  "width": 500,
  "height": 420,
  "path": {
    "data": "M250,180 Q210,105 145,70 Q55,25 35,110 Q10,195 125,220 Q35,275 80,355 Q125,415 210,315 Q235,285 250,250 Q265,285 290,315 Q375,415 420,355 Q465,275 375,220 Q490,195 465,110 Q445,25 355,70 Q290,105 250,180",
    "intrinsicWidth": 500,
    "intrinsicHeight": 420
  },
  "style": {
    "opacity": 0.96,
    "fill": {
      "type": "solid",
      "color": "#EDE9FE",
      "opacity": 0.72
    },
    "stroke": {
      "paint": {
        "type": "solid",
        "color": "#6D28D9"
      },
      "width": 8,
      "lineCap": "round",
      "lineJoin": "round"
    }
  }
}
```

V1 path 写入约束如下：

- `data` 必须是一个绝对 `M`，后跟至少一个显式写出的绝对 `Q`；不接受相对命令，
  也不接受 `L`、`C`、`A`、`Z` 等通用 SVG 命令。
- 所有命令参数必须完整且为有限数值；单节点 `data` 最长 1 MiB，最多 50,000 个
  命令。
- `intrinsicWidth` 和 `intrinsicHeight` 必须为有限正数；它们不要求等于节点的
  `width` 和 `height`。
- 未传 `style` 时，默认使用透明填充、`#222222` 描边、宽度 5，以及 round
  line cap/join。需要稳定视觉结果时仍应显式传入 `style`。
- path 可以作为 group/frame 的子节点，也可以作为同一 update 请求中 connector
  的 node 端点。

query 会把其他能够解析的 SVG path 保留为 `type: "path"`，但标记为
`readOnly`，原因包含 `path.commands`；超出上述 V1 限制时使用
`path.data.size` 或 `path.commands.limit`。既有 path 带 text、adjustments、非
`nonzero` fillRule 或 V1 无法表达的样式时也会只读。仅包含不影响画笔几何的
兼容性信息时，不会因此变为只读。theme stroke 遵循通用 style 契约：query 会
保留 token 和明暗参数；只要 token 能在当前白板主题中解析，就可以按 theme
paint 回写。
