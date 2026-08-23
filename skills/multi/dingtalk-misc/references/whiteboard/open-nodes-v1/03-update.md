# OpenNodes V1 — Update 信封、Append/Overwrite 和公共写入字段

> 本文件是 DWS OpenNodes V1 协议的拆分章节。按需读取入口见
> [协议索引](../open-nodes-v1.md)。

## 5. Update 协议

### 5.1 请求信封

```ts
interface OpenNodesUpdateRequest {
  overwrite?: boolean;
  source: {
    schemaVersion: "1.0";
    catalogVersion: "dml-v1";
    nodes: OpenNodeWrite[];
  };
}
```

字段含义：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `overwrite` | 否 | `false` 或省略为 append；`true` 为 overwrite。 |
| `source.schemaVersion` | 是 | 必须为 `1.0`。 |
| `source.catalogVersion` | 是 | 必须为 `dml-v1`。 |
| `source.nodes` | 是 | 本次创建的节点数组。append 至少一个；overwrite 允许空数组。 |

`source` 不能直接使用 query 返回的 `OpenNodesDocument`，也不接受 `pages`。
DWS 不接受 `pageId`；传入会在 CLI 本地校验阶段失败。

V1 没有“按真实节点 ID patch 既有节点”的语义。`source.nodes` 中的每一项都会
创建一个新节点，`id` 仅是本次请求内建立父子关系和连接线引用的临时 ID：
append 是新增节点，overwrite 是整页删除后重新创建。

### 5.2 Append 与 Overwrite

> **注意：** `overwrite: true` 是破坏性操作，会删除当前白板页面的全部自有
> 节点。调用前应先 query 并确认影响范围；`nodes: []` 会清空页面。不希望删除
> 既有内容时，应使用 append。

| 行为 | append | overwrite |
| --- | --- | --- |
| `overwrite` | `false` 或省略 | `true` |
| 空 `nodes` | 禁止 | 允许，用于清空当前页面 |
| 当前页面旧节点 | 全部保留 | 删除页面自有节点后创建新节点 |
| 母版节点 | 保留 | 保留 |
| 页面级设置 | 保留 | 保留 |
| 原子性 | 全部成功或全部回滚 | 全部成功或全部回滚 |

overwrite 会替换当前页面的全部自有节点，不要求旧节点本身可由 OpenNodes V1
写入。因此页面中存在 image、PDF、复杂文本等只读节点，不会单独阻止清空页面。

overwrite 会在删除前执行安全预检；以下情况会拒绝执行：

- 节点被锁定：`lockedNode`。
- 节点不允许被删除：`deleteForbidden`。
- 页面包含无法安全保留或清理的关联数据：`unknownMetadataReference`。
- 目标节点未能完整删除：`deleteFailed`。

与被删除节点绑定且有明确清理规则的关联数据会随节点清理；无法安全保留或清理的
关联数据会使 overwrite 失败。

overwrite 只替换当前页面节点，不会重建页面，也不会修改主题等页面级设置。
白板已有的主题会被保留；白板没有有效主题时也不会自动添加。调用方应使用
已配置有效主题的白板，或者显式提供不依赖主题的颜色。

### 5.3 成功结果

```ts
interface DWSWhiteboardUpdateResponse {
  success: true;
  nodeId: string;
  partId: string;
  resultJson: {
    mode: "append" | "overwrite";
    createdNodeIds: string[];
    idMap: Record<string, string>;
    deletedNodeCount: number;
    message: string;
  };
}
```

| 字段 | 说明 |
| --- | --- |
| `success` | `true` 表示本次 DWS 调用成功。 |
| `nodeId` | 输入的文档节点 ID。 |
| `partId` | 输入的白板标识。 |
| `resultJson.mode` | 实际执行的模式。 |
| `resultJson.createdNodeIds` | 按请求节点顺序返回真实节点 ID。 |
| `resultJson.idMap` | 请求中显式临时 ID 到真实节点 ID 的映射。 |
| `resultJson.deletedNodeCount` | append 恒为 `0`；overwrite 为删除的页面自有节点数。 |
| `resultJson.message` | 供人阅读的结果摘要，不应作为机器判断依据。 |

响应可能增加其他可选字段；Agent 不应依赖本节未声明的字段。

示例：

```json
{
  "success": true,
  "nodeId": "DOC_NODE_ID",
  "partId": "WHITEBOARD_PART_ID",
  "resultJson": {
    "mode": "append",
    "createdNodeIds": ["generated-title-id", "generated-body-id"],
    "idMap": {
      "title": "generated-title-id",
      "body": "generated-body-id"
    },
    "deletedNodeCount": 0,
    "message": "Created 2 Whiteboard nodes"
  }
}
```

## 6. Update 公共节点字段

V1 可写节点公共字段如下：

```ts
interface OpenNodeWriteBase {
  id?: string;
  layer?: "background" | "normal" | "foreground";
  zIndex?: number;
  hidden?: boolean;
}

interface OpenChildNodeWriteBase extends OpenNodeWriteBase {
  parentId?: string;
}

interface OpenSizedNodeWriteBase extends OpenChildNodeWriteBase {
  x: number;
  y: number;
  width: number;
  height: number;
  angle?: number;
}

interface OpenShapeNodeWrite extends OpenSizedNodeWriteBase {
  type: "shape";
  geometry: `dml:${string}`;
  text?: OpenTextWrite;
  style?: OpenNodeStyleWrite;
}

interface OpenTextNodeWrite extends OpenSizedNodeWriteBase {
  type: "text";
  text: OpenTextWrite;
  style?: OpenNodeStyleWrite;
}

interface OpenConnectorNodeWrite extends OpenNodeWriteBase {
  type: "connector";
  start: OpenConnectorEndpointWrite;
  end: OpenConnectorEndpointWrite;
  routing: OpenConnectorRouting;
  waypoints?: OpenPoint[];
  style?: OpenNodeStyleWrite;
}

interface OpenStickyNoteNodeWrite extends OpenSizedNodeWriteBase {
  type: "stickyNote";
  text?: OpenTextWrite;
  style?: OpenNodeStyleWrite;
}

interface OpenFrameNodeWrite extends OpenNodeWriteBase {
  type: "frame";
  x: number;
  y: number;
  width: number;
  height: number;
  angle?: 0;
  title?: {
    text: OpenTextWrite;
    box?: { width: number; height: number };
  };
  style?: OpenNodeStyleWrite;
  presentationOrder?: number;
  resizeMode?: "free" | "fixedAspectRatio";
}

interface OpenGroupNodeWrite extends OpenChildNodeWriteBase {
  id: string;
  type: "group";
  x: number;
  y: number;
}

interface OpenVectorNodeWrite extends OpenSizedNodeWriteBase {
  type: "vector";
  resource: OpenManagedVectorResourceWrite;
}

interface OpenIconNodeWrite extends OpenSizedNodeWriteBase {
  type: "icon";
  catalogId: OpenIconCatalogId;
}

interface OpenPathNodeWrite extends OpenSizedNodeWriteBase {
  type: "path";
  path: OpenPathDataWrite;
  style?: OpenNodeStyleWrite;
}

type OpenNodeWrite =
  | OpenShapeNodeWrite
  | OpenTextNodeWrite
  | OpenConnectorNodeWrite
  | OpenStickyNoteNodeWrite
  | OpenFrameNodeWrite
  | OpenGroupNodeWrite
  | OpenVectorNodeWrite
  | OpenIconNodeWrite
  | OpenPathNodeWrite;
```

上面的联合类型是 update 的字段白名单。各辅助结构和完整枚举值在第 7 节定义；
未出现在对应分支中的字段不能发送。

| 字段 | 规则 |
| --- | --- |
| `id` | 可选的请求级临时 ID；非空、区分大小写、在请求内唯一。被引用时必须提供。 |
| `type` | 必填，必须是 V1 可写类型。 |
| `parentId` | 可选，引用同一请求中 group/frame 的临时 ID。 |
| `x`、`y` | 除 connector 外必填；有父节点时为父节点相对坐标。 |
| `width`、`height` | shape/text/stickyNote/frame/vector/icon/path 必填且大于 `0`；group/connector 只读。 |
| `angle` | 可选，默认 `0`；frame 只允许 `0`；group/connector 只读。 |
| `layer` | 可选；frame 默认 `background`，其他节点默认 `normal`。 |
| `zIndex` | 可选的非负整数；相同值时按请求顺序稳定排序。 |
| `hidden` | 可选布尔值，默认 `false`。 |

以下 query 字段禁止写回：

`children`、`absoluteBounds`、`locked`、`source`、`writeSupport`、
`unsupportedFeatures`。

关系规则：

- `parentId` 只能引用同一请求中的 group 或 frame。
- frame 和 connector 必须是页面直属节点，不能带 `parentId`。
- group 可以嵌套，也可以放在 frame 中。
- `children` 始终由各子节点的 `parentId` 推导。
- 父子关系不能成环。
- group 必须有临时 `id`、至少两个直接子节点，且不能全部隐藏。
