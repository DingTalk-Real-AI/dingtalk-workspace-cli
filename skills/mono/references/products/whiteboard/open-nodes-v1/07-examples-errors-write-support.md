# OpenNodes V1 — Update 示例、回写规则、错误模型和 writeSupport

> 本文件是 DWS OpenNodes V1 协议的拆分章节。按需读取入口见
> [协议索引](../open-nodes-v1.md)。

## 8. Update 示例

### 8.1 Append 一个文本节点

```json
{
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "title",
        "type": "text",
        "x": 120,
        "y": 80,
        "width": 240,
        "height": 48,
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
          "padding": [2, 4]
        }
      }
    ]
  }
}
```

### 8.2 Append 两个形状和一条引用连接线

```json
{
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "left",
        "type": "shape",
        "x": 80,
        "y": 100,
        "width": 120,
        "height": 80,
        "geometry": "dml:roundRect"
      },
      {
        "id": "right",
        "type": "shape",
        "x": 360,
        "y": 100,
        "width": 120,
        "height": 80,
        "geometry": "dml:roundRect"
      },
      {
        "id": "line",
        "type": "connector",
        "start": {
          "type": "node",
          "nodeRef": {
            "scope": "request",
            "id": "left"
          },
          "anchor": {
            "mode": "fixed",
            "side": "right"
          }
        },
        "end": {
          "type": "node",
          "nodeRef": {
            "scope": "request",
            "id": "right"
          },
          "anchor": {
            "mode": "fixed",
            "side": "left"
          },
          "marker": {
            "catalogId": "arrow.filled"
          }
        },
        "routing": "straight"
      }
    ]
  }
}
```

### 8.3 Overwrite 整页

```json
{
  "overwrite": true,
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "replacement",
        "type": "text",
        "x": 120,
        "y": 80,
        "width": 240,
        "height": 48,
        "text": {
          "blocks": [
            {
              "type": "paragraph",
              "runs": [
                {
                  "text": "Replacement content"
                }
              ]
            }
          ]
        }
      }
    ]
  }
}
```

### 8.4 清空当前页面

```json
{
  "overwrite": true,
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": []
  }
}
```

## 9. Query 数据不能直接回写

query 是完整可读投影，update 是受约束的创建协议，两者不是对称 JSON：

| Query 字段/能力 | Update 处理方式 |
| --- | --- |
| 真实 `id` | 只能作为请求级临时 ID；不能引用既有 document 节点。 |
| `children` | 删除，通过子节点 `parentId` 重建。 |
| `absoluteBounds` | 删除；普通节点使用 `x/y/width/height`，connector 使用端点和路由字段。 |
| `locked`、`source`、`writeSupport`、`unsupportedFeatures` | 删除，均为 query-only。 |
| 文本 `plainText` | 删除，由服务端根据 paragraph 和 run 重新计算。 |
| 多 paragraph、列表、多 run、文字链接 | 可以保留；每个 block 必须是受支持类型，且 run 内不能包含原始换行符。 |
| 未知 list style、非法链接或未支持的 block/marks | 需要移除或降级为受支持的 block/run。 |
| theme paint | 保留 `token/lumMod/lumOff`，删除 query-only `resolvedColor`；token 必须能在当前白板主题中解析。 |
| image paint | 需要降级成受支持的 paint，或不更新该节点。 |
| 受支持的 linear/radial gradient、单个 shadow | 可以保留；gradient offset 使用 `0～100`，radial `custom` 不能回写。 |
| blur、unknown 或叠加 effects | 需要删除、降级成单个 shadow，或不更新该节点。 |
| connector `scope: "document"` | 不能回写；改为引用同一请求节点的 `scope: "request"`。 |
| connector `resolvedPoint`、`position`、`resolvedPath` | 删除，均由服务端重新计算。 |
| shape `adjustments` | V1 不支持写入。 |
| stickyNote `creator`、`tags` | V1 不支持写入。 |

即使 query 节点显示 `writeSupport = "readWrite"`，update 仍会对版本、目录、
字段和请求关系做完整校验。调用方不应跳过 update 错误处理。

## 10. 错误模型

### 10.1 顶层错误码

| 错误码 | 含义 |
| --- | --- |
| `invalidRequest.whiteboard.schemaInvalid` | JSON、字段或节点 schema 不合法。 |
| `invalidRequest.whiteboard.catalogVersionUnsupported` | catalogVersion 不受支持。 |
| `invalidRequest.whiteboard.validationFailed` | 节点间引用、父子关系或路径关系不合法。 |
| `invalidRequest.whiteboard.emptySource` | append 的 nodes 为空。 |
| `invalidRequest.whiteboard.overwriteUnsafe` | overwrite 安全预检失败。 |

### 10.2 DWS 错误输出

远端校验失败时，DWS 以统一 CLI 错误结构返回：

```json
{
  "error": {
    "category": "api",
    "reason": "business_error",
    "server_key": "whiteboard",
    "server_error_code": "invalidRequest.whiteboard.validationFailed",
    "message": "Whiteboard request graph is invalid",
    "trace_id": "TRACE_ID"
  }
}
```

部分服务错误会使用更宽泛的 `invalidRequest.inputArgs.invalid`。Agent 应结合
`server_error_code` 和 `message` 修正输入；需要排障时保留 `trace_id`。JSON 或
信封级错误可能由 CLI 本地返回，不一定包含 `server_key` 和 `trace_id`。

校验类错误不可通过原样重试恢复。常见原因包括：

- Schema：缺少字段、未知字段、类型或枚举值错误、提交 query-only 字段、
  节点类型不支持。
- 请求关系：临时 ID 重复、引用不存在、父子关系非法、连接线目标不支持、
  路径退化或主题 token 不存在。
- Overwrite 预检：锁定节点、禁止删除、关联数据无法安全处理或删除失败。

任一阶段失败都不会保留部分更新。

## 11. writeSupport 的含义

`writeSupport` 表示当前 query 节点是否能由 V1 update 无损表达，不代表用户
权限，也不代表 overwrite 是否允许移除该既有节点。

以下 `unsupportedFeatures` 均为对外返回的诊断枚举值：

- `node.source.master`、`node.locked`、`node.role`、`node.placeholder`、
  `node.extras`、`node.ability`。
- `node.type.image`、`node.type.pdf`、`node.type.media`、
  `node.type.webLink`、`node.type.table`、`node.type.chart`、
  `node.type.uml`、`node.type.swimlane`、`node.type.mind`、
  `node.type.timer`、`node.type.placeholder`、`node.type.unknown`。
- `text.list.unsupported`、`text.link.unsupported`、`text.block.unsupported`、
  `text.lineBreak.unsupported`、`text.marks.unsupported`、
  `text.color.unsupported`、`text.highlight.unsupported`。
- `style.fill.color`、`style.fill.opacity`、`style.fill.theme.token`、
  `style.fill.theme.unresolved`、`style.fill.theme.modifier`、
  `style.fill.theme.opacity`、
  `style.fill.gradient.angle`、`style.fill.gradient.offset`、
  `style.fill.gradient.color`、`style.fill.gradient.opacity`、
  `style.fill.gradient.position`、`style.fill.image`。
- `style.stroke.color`、`style.stroke.opacity`、`style.stroke.theme.token`、
  `style.stroke.theme.unresolved`、
  `style.stroke.theme.modifier`、`style.stroke.theme.opacity`、
  `style.stroke.gradient.angle`、`style.stroke.gradient.offset`、
  `style.stroke.gradient.color`、`style.stroke.gradient.opacity`、
  `style.stroke.gradient.position`、`style.stroke.image`。
- `style.effects`。
- `shape.adjustments`、`stickyNote.creator`、`stickyNote.tags`。
- `vector.resource.external`、`vector.resource.embedded`、
  `vector.resource.unresolved`、`vector.fill`、`vector.stroke`、
  `vector.opacity`、`vector.effect`、`vector.adjustments`。
- `icon.catalogId`、`icon.opacity`、`icon.effect`、`icon.text`、
  `icon.adjustments`。
- `path.commands`、`path.data.size`、`path.commands.limit`、`path.text`、
  `path.fillRule`、`path.adjustments`。
- `connector.parent`、`connector.marker.unsupported`、
  `connector.target.unexposed`、`connector.target.unsupported`、
  `connector.anchor.unresolved`、`connector.anchor.custom`、
  `connector.selfLoop`。
- `group.angle`、`group.children.minimum`、`group.children.hidden`、
  `group.child.readOnly`。
- `frame.angle`、`frame.child.frame`、`frame.child.readOnly`。

调用方应把 `unsupportedFeatures` 当作诊断信息，不应把当前枚举穷举写死为
业务逻辑。真正可写与否以 `writeSupport` 和 update 校验结果为准。
