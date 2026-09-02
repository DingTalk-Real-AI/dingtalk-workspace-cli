# 钉钉文档内嵌白板

本页是 Whiteboard 的默认入口。白板/画布中的 OpenNodes 图形、文本、分组、连接线、
Vector 和布局由 Whiteboard 负责；Doc 只负责创建文档以及插入、定位、删除白板卡片
容器。已知命令直接执行；仅真实 `unknown command` / `unknown flag` 时读一次 leaf
Help，契约不确定时读一次 compact leaf Schema。

根据本次操作只读取以下一份操作 Reference，不预加载完整协议或多个示例集：

- Shape、Connector、Frame、样式、Icon 或 Path：
  [compose.md](./whiteboard/compose.md)
- 已上传或本地 SVG/Vector 资源：[vector.md](./whiteboard/vector.md)
- 整页替换或清空：[replace.md](./whiteboard/replace.md)

读取已有白板或普通定位不需要额外 Reference。每个普通任务最多读取一份操作
Reference；只有操作页仍缺少具体字段时，才读取一份精确协议章节。

## 执行契约

- 同一 profile，目标须有真实 `nodeId` 和 `whiteboardId/partId`；零/多目标、身份
  不明或 profile 不一致时停止，不能取第一个候选。
- Runtime 确认后执行层才添加 `--yes`；存储示例不得预置确认。
- 成功须同时满足终态 receipt、请求节点映射和同板读回；`verified=false`、partial、
  commit-unknown 不得报成功或盲重试。
- `+update` 内部仍使用完整同板 query 校验；成功结果只返回稳定目标、mode、验证节点
  数、summary 和精简 receipt，不返回 `source.pages[].nodes` 完整快照。用户明确要求
  更新后完整快照时，再执行一次 `+query`；否则不得为补输出重复查询。
- append 和 overwrite 的 `verified=true` 均包含独立读回证据，不再追加 query；
  额外 query 仅用于用户要求完整快照，或未验证/提交状态不明时的有界只读对账。

## 调用与上下文预算

- 每板建立 `{blockId, whiteboardId, payloadFile}`，禁止重复 fetch、insert 或搜索。
- 每阶段最多一次 update；提交前校验错误只修相关字段一次。相同 Payload、
  `--verbose` 或 commit-unknown 不重放；commit-unknown 按同一稳定目标 query 对账。
- 先用单次响应完整校验，再无损投影；ID、mode、verified、节点摘要和链接只是最小
  集合，用户要求及后续操作依赖的几何、样式、文本、关系必须保留。
- 投影只减少重复上下文，不删除业务信息：query/export/audit 需要完整快照时原样
  交付；写入 `source` 保留在 Payload 文件中，不得因丢字段而重发远端请求。

## 稳定 ID

| ID | 来源 | 用途 |
|---|---|---|
| `nodeId` | `doc +create` / 文档解析 | 文档和白板命令的 `--node` |
| `blockId` | `doc whiteboard insert` | 文档块定位、排序和删除 |
| `whiteboardId` | `doc whiteboard insert` | 白板命令的 `--part-id` |
| 请求节点 `id` | 本地 OpenNodes 文件 | 同一请求的 parent/connector 引用 |
| 真实节点 ID | `+query` / `+update` 读回 | 读回身份；不能直接做局部 update |

insert 返回 `whiteboardId` 后直接使用；若为 null，只 fetch 一次并按本次 `blockId`
定位，仍未落库则报 pending，禁止重复插入。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "whiteboard +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws whiteboard <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service whiteboard --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws whiteboard +query` | read | 严格读取已有文档白板的 OpenNodes 快照 |
| `dws whiteboard +update` | high-risk-write | 确认后更新白板并按同一稳定目标精确读回 |
<!-- VISIBLE_SHORTCUTS_END -->

## 定位与调用

```bash
# 新建文档和白板卡片
dws doc +create --name "<文档标题>" --format json
dws doc whiteboard insert --node <DOC_ID> --format json

# 在指定块后插入
dws doc +fetch --node <DOC_ID> --detail with-ids --format json
dws doc whiteboard insert --node <DOC_ID> \
  --ref-block <BLOCK_ID> --where after --format json

# 读取与更新
dws whiteboard +query --node <DOC_ID> --part-id <PART_ID> --format json
dws whiteboard +update --node <DOC_ID> --part-id <PART_ID> \
  --source @whiteboard.json --format json
```

`--source` 接受 JSON、`@relative-file.json` 或 stdin；本地文件必须加 `@`，裸路径
会被当作 JSON。白板 shortcut 不支持 `--jq` / `--fields`。

## 坐标读回与稳定结构

坐标读回采用统一数值语义：整数、浮点数和 JSON Number 等价，并允许最多 0.5 像素
的服务端规范化偏差。超过容差返回 `readback_field_mismatch`，不能声明成功：

1. **已提交**：已有成功回执或已读到真实节点时，停止重提并只读对账。
   append 会创建新节点，不会修正已有节点；改成 frame 再 append 仍会重复创建。
   Runtime 保留 `error.details` 中的 `nodeId/partId/mode`、`commitState=committed`、
   `verified=false` 和 `receipt.createdNodeIds/idMap/deletedNodeCount`，标记
   `execution_started=true` 且不可重试。保留原始 Payload 和当前差异；如需核实，
   最多再 query 同一白板一次，按回执真实 ID 对账，不循环轮询。
2. **提交状态不明**：连接中断、缺少回执或暂未读到节点，都不能证明未提交；
   停止写入，按同一稳定目标只读对账，仍不明确则报告阻塞，不重放。
3. **明确未提交**：仅本地预检未发请求，或服务端明确保证未提交/无副作用，才可
   修正相关字段后至多重试一次；`retryable=false` 的服务错误仍须先解决阻塞条件。

已提交但未验证不能报告完整成功，也不得自动 overwrite、删除节点或重新 append。
布局修复属于新操作，须先对账并另行确认范围和授权，不能作为校验失败的自动重试。
有边界的分区、泳道和卡片组在提交前优先使用 frame 和容器内相对坐标；frame 是布局
设计选择，不是已落库节点的修复手段。临时逻辑组合才使用 group。

## 精确协议补充

| 仅当操作 Reference 未覆盖时 | 读取 |
|---|---|
| 富文本、列表、链接、主题色、罕见样式 | [04-text-style.md](./whiteboard/open-nodes-v1/04-text-style.md) |
| 罕见 shape/frame/group/connector 约束 | [05-shape-frame-group-connector.md](./whiteboard/open-nodes-v1/05-shape-frame-group-connector.md) |
| 非托管 Vector、Icon、Path | [06-vector-icon-path.md](./whiteboard/open-nodes-v1/06-vector-icon-path.md) |
| query 快照转换和错误语义 | [07-examples-errors-write-support.md](./whiteboard/open-nodes-v1/07-examples-errors-write-support.md) |
| 新 geometry / icon catalog 值 | [08-catalogs.md](./whiteboard/open-nodes-v1/08-catalogs.md) |
