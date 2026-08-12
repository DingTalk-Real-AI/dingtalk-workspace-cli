# 钉钉白板常用 Recipes

以下各 Recipe 的 JSON 都写入本地文件，再通过 `dws whiteboard update --source <FILE.json>`
传给白板。执行任何远端写入前，必须先向用户展示影响并取得明确确认；确认后
才可添加 `--yes`：

```bash
dws whiteboard update \
  --node <DOC_NODE_ID> \
  --part-id <WHITEBOARD_PART_ID> \
  --source <FILE.json> \
  --yes \
  --format json
```

每次更新后都要再次执行 `dws whiteboard query ... --format json` 回读验证。

## 1. 追加两个流程节点和一条箭头

```json
{
  "overwrite": false,
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "start",
        "type": "shape",
        "x": 80,
        "y": 100,
        "width": 160,
        "height": 72,
        "geometry": "dml:roundRect",
        "text": {
          "blocks": [
            {
              "type": "paragraph",
              "horizontalAlign": "center",
              "runs": [{ "text": "读取需求", "marks": { "bold": true } }]
            }
          ],
          "verticalAlign": "center"
        }
      },
      {
        "id": "finish",
        "type": "shape",
        "x": 360,
        "y": 100,
        "width": 160,
        "height": 72,
        "geometry": "dml:roundRect",
        "text": {
          "blocks": [
            {
              "type": "paragraph",
              "horizontalAlign": "center",
              "runs": [{ "text": "输出结果", "marks": { "bold": true } }]
            }
          ],
          "verticalAlign": "center"
        }
      },
      {
        "type": "connector",
        "start": {
          "type": "node",
          "nodeRef": { "scope": "request", "id": "start" },
          "anchor": { "mode": "fixed", "side": "right" }
        },
        "end": {
          "type": "node",
          "nodeRef": { "scope": "request", "id": "finish" },
          "anchor": { "mode": "fixed", "side": "left" },
          "marker": { "catalogId": "arrow.filled" }
        },
        "routing": "straight"
      }
    ]
  }
}
```

## 2. 追加带渐变和阴影的卡片

```json
{
  "overwrite": false,
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "styled-card",
        "type": "shape",
        "x": 80,
        "y": 260,
        "width": 260,
        "height": 120,
        "geometry": "dml:roundRect",
        "style": {
          "fill": {
            "type": "linearGradient",
            "angle": 35,
            "stops": [
              { "offset": 0, "color": "#DBEAFE" },
              { "offset": 100, "color": "#A7F3D0" }
            ]
          },
          "stroke": {
            "paint": { "type": "solid", "color": "#2563EB" },
            "width": 2
          },
          "effects": [
            {
              "type": "shadow",
              "offsetX": 5,
              "offsetY": 7,
              "blur": 18,
              "color": "#0F172A",
              "opacity": 0.22
            }
          ]
        },
        "text": {
          "blocks": [
            {
              "type": "paragraph",
              "horizontalAlign": "center",
              "runs": [
                {
                  "text": "复杂样式",
                  "marks": { "fontSize": 20, "bold": true, "color": "#0F172A" }
                }
              ]
            }
          ],
          "verticalAlign": "center"
        }
      }
    ]
  }
}
```

## 3. Frame 中放置分支流程

先创建 frame，再让子节点通过 `parentId` 引用 frame 的临时 ID。子节点坐标相对
frame 左上角：

```json
{
  "overwrite": false,
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "pipeline",
        "type": "frame",
        "x": 60,
        "y": 440,
        "width": 720,
        "height": 300,
        "title": {
          "text": {
            "blocks": [
              {
                "type": "paragraph",
                "runs": [{ "text": "生成流水线", "marks": { "bold": true } }]
              }
            ]
          }
        }
      },
      {
        "id": "branch-a",
        "type": "shape",
        "parentId": "pipeline",
        "x": 60,
        "y": 80,
        "width": 180,
        "height": 72,
        "geometry": "dml:rect"
      },
      {
        "id": "branch-b",
        "type": "shape",
        "parentId": "pipeline",
        "x": 420,
        "y": 80,
        "width": 180,
        "height": 72,
        "geometry": "dml:rect"
      }
    ]
  }
}
```

## 4. 上传 SVG 并追加 Vector

Vector 固定使用“上传 → 字段映射 → update → query”流程，并且所有命令使用同一个
`DOC_NODE_ID`。

先上传 SVG。该命令只准备资源，不会插入文档正文：

```bash
dws doc media upload \
  --node <DOC_NODE_ID> \
  --file ./icon.svg \
  --mime-type image/svg+xml \
  --yes \
  --format json
```

从成功输出取 `resourceId` 和 `resourceUrl`：

```json
{
  "nodeId": "<DOC_NODE_ID>",
  "resourceId": "resource-stable-id",
  "resourceUrl": "https://resource.example/resource-stable-id?resourceId=resource-stable-id",
  "fileName": "icon.svg",
  "mimeType": "image/svg+xml",
  "size": 1024
}
```

写入 `whiteboard-vector.json`，其中 `resourceId` 原样映射到
`resource.resourceId`，`resourceUrl` 映射到 `resource.url`：

```json
{
  "overwrite": false,
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "uploaded-vector",
        "type": "vector",
        "x": 80,
        "y": 80,
        "width": 160,
        "height": 160,
        "resource": {
          "kind": "managed",
          "resourceId": "resource-stable-id",
          "url": "https://resource.example/resource-stable-id?resourceId=resource-stable-id"
        }
      }
    ]
  }
}
```

执行更新：

```bash
dws whiteboard update \
  --node <DOC_NODE_ID> \
  --part-id <WHITEBOARD_PART_ID> \
  --source ./whiteboard-vector.json \
  --yes \
  --format json
```

最后独立回读，不以 update 的成功响应替代验证：

```bash
dws whiteboard query \
  --node <DOC_NODE_ID> \
  --part-id <WHITEBOARD_PART_ID> \
  --format json
```

禁止跨 nodeId 复用资源，也不要把本地 SVG 路径、独立文件节点 URL 或临时
`uploadUrl` 写入 `resource.url`。

## 5. 整页替换或清空

把文件设为 `overwrite: true` 后，命令必须加 `--yes`：

```bash
dws whiteboard update \
  --node <DOC_NODE_ID> \
  --part-id <WHITEBOARD_PART_ID> \
  --source ./overwrite.json \
  --yes \
  --format json
```

清空整页：

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

仅当用户明确要求清空时使用。执行前必须 query、展示影响摘要并获得确认。
