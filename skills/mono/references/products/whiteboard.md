# 钉钉文档内嵌白板

`dws whiteboard` 读取和更新已存在于在线文档中的单页白板。创建白板卡片使用
`dws doc whiteboard insert`；删除卡片使用已有的 `dws doc block delete`。

OpenNodes V1 的完整字段、节点类型、目录枚举和错误语义按需读取
[协议索引](./whiteboard/open-nodes-v1.md)；不要根据本页概要猜测节点字段或
`geometry`、`catalogId` 等枚举值。渐变卡片、Frame 分支、SVG/Vector 等完整
工作流见 [常用 Recipes](./whiteboard/recipes.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "whiteboard +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws whiteboard <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service whiteboard --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws whiteboard +query` | read | 严格读取已有文档白板的 OpenNodes 快照 |
| `dws whiteboard +update` | high-risk-write | 确认后更新白板并按同一稳定目标精确读回 |
<!-- VISIBLE_SHORTCUTS_END -->

## 标准流程

1. 从用户输入或真实文档 JSONML 取得 `nodeId` 和 card `metadata.id`（partId）。
2. `dws whiteboard +query --node <DOC_ID> --part-id <PART_ID> --format json` 保存并严格校验当前内容。
3. 生成 OpenNodes V1 文件；不能把 query 响应直接回写。
4. 向用户展示写入范围并取得确认。
5. `dws whiteboard +update --node <DOC_ID> --part-id <PART_ID> --source <FILE> --format json`；该 shortcut 会验证终态 receipt，并按同一白板执行独立读回。
6. 仅在需要人工审阅完整节点细节时，再执行一次 `+query`。

执行层只能在已取得本次写入确认后添加 `--yes`；可复制的存储示例不得预置该参数。

更新文件：

```json
{
  "overwrite": false,
  "source": {
    "schemaVersion": "1.0",
    "catalogVersion": "dml-v1",
    "nodes": [
      {
        "id": "n1",
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

- append (`overwrite=false`) 至少包含一个节点。
- overwrite (`true`) 整页重建并允许空数组；必须先备份当前 query 结果。
- 所有 update 都要求用户确认和 `--yes`。
- 当前只支持单页，不支持 `pageId`，也不支持使用真实节点 ID 做局部更新。
- `--jq` / `--fields` 不适用于白板命令。

## 创建白板

```bash
dws doc whiteboard insert --node <DOC_ID> --yes --format json
```

返回的 `whiteboardId` 是 partId，`blockId` 是文档块 ID。删除卡片走：

```bash
dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID> --yes --format json
```

## Vector / SVG

先上传绑定到同一 nodeId 的媒体资源：

```bash
dws doc media upload --node <DOC_ID> --file ./icon.svg \
  --mime-type image/svg+xml --yes --format json
```

只使用稳定输出 `resourceId` / `resourceUrl`，不要使用临时 uploadUrl、本地路径或
跨 nodeId 资源。
