# 白板托管资源 Vector

仅在白板需要 SVG/Vector 资源时读取本页。资源必须绑定到承载白板的同一
`nodeId`；本地文件、临时 upload URL、独立文件节点 URL 和其他文档的资源都不能
直接写入 OpenNodes。

## 上传与映射

```bash
dws doc media upload --node <DOC_NODE_ID> --file ./whiteboard-asset.svg \
  --mime-type image/svg+xml --format json
```

上传只准备资源，不插入文档正文。从同一次成功回执取稳定 `resourceId` 和
`resourceUrl`，不要重新搜索或改写 URL；`resourceId` 原样映射为
`resource.resourceId`，`resourceUrl` 原样映射为 `resource.url`。

## Vector 更新文件

```json
{
  "overwrite": false,
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "managed-vector",
        "type": "vector",
        "x": 80,
        "y": 80,
        "width": 160,
        "height": 160,
        "resource": {
          "kind": "managed",
          "resourceId": "<RESOURCE_ID>",
          "url": "<RESOURCE_URL>"
        }
      }
    ]
  }
}
```

按根 Reference 的 update 命令提交 `@vector.json`。上传和 update 必须使用同一个
`DOC_NODE_ID`。成功结果中的读回 Vector 必须保留同一 `resourceId`；
`verified=false`、资源身份变化或缺少读回都不能报告成功。

需要 Icon/Path 或非托管 Vector 时改读
[06-vector-icon-path.md](./open-nodes-v1/06-vector-icon-path.md)，不要同时加载构图
Reference。
