# 钉钉文档内嵌白板

`dws whiteboard` 只操作已经存在于钉钉在线文档中的单页内嵌白板。创建白板卡片使用
`dws doc whiteboard insert`；普通文档块仍使用 `dws doc block`。

OpenNodes V1 的完整字段、节点类型、目录枚举和错误语义按需读取
[协议索引](./whiteboard/open-nodes-v1.md)；不要根据本页概要猜测节点字段或
`geometry`、`catalogId` 等枚举值。渐变卡片、Frame 分支、SVG/Vector 等完整
工作流见 [常用 Recipes](./whiteboard/recipes.md)。

## 定位白板

每次操作都需要真实的文档 `nodeId` 和白板 `partId`。缺少 `partId` 时先读取文档
JSONML，查找 `cardType=hetu` 且 `metadata.id` 非空的 card；`uuid` 是 blockId，
不能当作 partId。多个候选时必须让用户选择，不能取第一个。

```bash
dws doc read --node <DOC_ID> --content-format jsonml --scope tags --tags card --format json
```

## 读取

```bash
dws whiteboard query --node <DOC_ID> --part-id <PART_ID> --format json
```

CLI 会把服务端 `resultJson` 字符串解析为结构化 JSON。白板命令不支持全局
`--jq` 或 `--fields`。

## 更新

更新文件使用 OpenNodes V1 信封：

```json
{
  "overwrite": false,
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "title",
        "type": "text",
        "x": 40,
        "y": 40,
        "width": 240,
        "height": 48,
        "text": {
          "blocks": [
            {
              "type": "paragraph",
              "runs": [{"text": "方案"}]
            }
          ]
        }
      }
    ]
  }
}
```

- `overwrite=false`：追加，`nodes` 至少一个对象。
- `overwrite=true`：整页重建，允许空数组；执行前必须先 query 保存当前内容。
- 所有更新都是远端写入，必须先获得用户确认，再加 `--yes`。
- Query 返回不能直接作为 update 输入；真实节点 ID 不能用于局部修改。

```bash
dws whiteboard update --node <DOC_ID> --part-id <PART_ID> \
  --source ./whiteboard.json --yes --format json
```

常用节点类型包括 `text`、`shape`、`frame`、`group`、`connector`、`vector`。
节点可用请求内临时 `id` 建立 `parentId` 或 connector 引用；服务端负责完整字段、
层级和枚举校验，未知字段会使整次更新失败。

## Vector / SVG 资源

本地 SVG 不能直接写入 OpenNodes。先上传为绑定到同一文档 nodeId 的资源：

```bash
dws doc media upload --node <DOC_ID> --file ./icon.svg \
  --mime-type image/svg+xml --yes --format json
```

将返回的 `resourceId` 和 `resourceUrl` 分别映射为 Vector resource 的
`resourceId` 与 `url`。禁止使用临时 uploadUrl、跨 nodeId 复用或传本地路径。

## 创建和删除白板卡片

```bash
dws doc whiteboard insert --node <DOC_ID> --yes --format json
dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID> --yes --format json
```

insert 返回 `blockId` 和 `whiteboardId`。前者用于块删除，后者就是后续 whiteboard
命令的 partId；两者不可混用。插入成功但回查暂未取到 partId 时，命令会返回
`whiteboardId: null` 并提示稍后按 blockId 回查。
