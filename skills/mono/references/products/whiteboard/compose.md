# 白板构图：Shape、Connector、Frame 与样式

仅在构造 OpenNodes 时读取；通用目标、确认、调用预算和读回规则沿用根 Reference。
节点放入信封，以 `--source @whiteboard.json` 传入。

## 首次远端写入前检查

- 信封是 `overwrite=false`、`1.0` / `dml-v1`、非空 `nodes`。
- 每个节点（包括 connector）都有唯一稳定 ID；connector `nodeRef` 和 `parentId`
  只引用本次节点，后者是 Frame。
- connector 不含 `x/y/width/height/angle/parentId`：端点只用 point 或同一请求的
  `scope=request` 节点，节点端点不得引用 connector/hidden 节点或形成自环；straight
  禁止 waypoints，polyline 至少一个有限坐标点。
- 坐标有限、尺寸为正；catalog 值来自本页安全子集或一份精确协议依据。
- 所需节点、文本、连接、父子关系都在同一稳定 Payload 文件。
- 有边界的分区、泳道和卡片组直接使用 Frame，避免 group 坐标规范化后反复修改。

## 安全子集

geometry：`dml:rect` / `dml:roundRect` / `dml:diamond`；marker：`none` /
`arrow.open` / `arrow.filled`；icon：`task/task-done`；path：绝对 `M` + 显式绝对
`Q` 和正数 intrinsic 尺寸。原样复用这些值不读取 catalog；其他值只读对应协议
章节，不加载完整目录。

## Shape 与 Connector

connector 只能引用同一请求内的临时节点 ID：

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
        "style": {"fill": {"type": "solid", "color": "#DBEAFE"}},
        "text": {
          "blocks": [{
            "type": "paragraph",
            "horizontalAlign": "center",
            "runs": [{"text": "开始", "marks": {"bold": true}}]
          }],
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
        "style": {"fill": {"type": "solid", "color": "#A7F3D0"}},
        "text": {
          "blocks": [{
            "type": "paragraph",
            "horizontalAlign": "center",
            "runs": [{"text": "完成", "marks": {"bold": true}}]
          }],
          "verticalAlign": "center"
        }
      },
      {
        "id": "start-to-finish",
        "type": "connector",
        "start": {
          "type": "node",
          "nodeRef": {"scope": "request", "id": "start"},
          "anchor": {"mode": "fixed", "side": "right"}
        },
        "end": {
          "type": "node",
          "nodeRef": {"scope": "request", "id": "finish"},
          "anchor": {"mode": "fixed", "side": "left"},
          "marker": {"catalogId": "arrow.filled"}
        },
        "routing": "straight"
      }
    ]
  }
}
```

## 样式、Icon 与 Path 片段

对象彼此独立，只把本次需要的节点加入 `source.nodes`：

```json
[
  {
    "id": "styled-card",
    "type": "shape",
    "x": 80,
    "y": 80,
    "width": 300,
    "height": 140,
    "geometry": "dml:roundRect",
    "style": {
      "fill": {
        "type": "linearGradient",
        "angle": 35,
        "stops": [
          {"offset": 0, "color": "#3B82F6"},
          {"offset": 100, "color": "#10B981"}
        ]
      },
      "stroke": {
        "paint": {"type": "solid", "color": "#2563EB"},
        "width": 2
      },
      "effects": [{
        "type": "shadow",
        "offsetX": 5,
        "offsetY": 7,
        "blur": 18,
        "color": "#0F172A",
        "opacity": 0.22
      }]
    }
  },
  {
    "id": "done-icon",
    "type": "icon",
    "x": 410,
    "y": 110,
    "width": 64,
    "height": 64,
    "catalogId": "task/task-done"
  },
  {
    "id": "annotation",
    "type": "path",
    "x": 100,
    "y": 230,
    "width": 250,
    "height": 30,
    "path": {
      "data": "M0,18 Q60,2 125,16 Q190,30 250,10",
      "intrinsicWidth": 250,
      "intrinsicHeight": 30
    },
    "style": {
      "fill": {"type": "none"},
      "stroke": {
        "paint": {"type": "solid", "color": "#2563EB"},
        "width": 6,
        "lineCap": "round",
        "lineJoin": "round"
      }
    }
  }
]
```

## Frame 与子节点

Frame 页面直属，子节点坐标相对其左上角；connector 不放入 Frame，跨区域连接线
保持页面直属：

```json
[
  {
    "id": "stage",
    "type": "frame",
    "x": 60,
    "y": 320,
    "width": 720,
    "height": 300,
    "title": {
      "text": {
        "blocks": [{
          "type": "paragraph",
          "runs": [{"text": "流程阶段", "marks": {"bold": true}}]
        }]
      }
    }
  },
  {
    "id": "stage-item",
    "type": "shape",
    "parentId": "stage",
    "x": 60,
    "y": 80,
    "width": 180,
    "height": 72,
    "geometry": "dml:rect"
  }
]
```

`presentationOrder` 须唯一且非负；未知字段、跨请求引用或不支持值会拒绝整批。
