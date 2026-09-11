# 钉钉白板（独立与文档内嵌）

本页是 Whiteboard 的默认入口。未提供 `--part-id` 时，`whiteboard query/update`
默认操作独立 `.adraw` 白板；显式提供非空 `--part-id` 时操作文档内嵌白板。
接口失败后不得自动切换类型。Doc 只负责普通文件生命周期以及插入、定位、删除文档内
白板卡片容器。已知命令直接执行；仅真实 `unknown command` / `unknown flag` 时读一次
leaf Help，契约不确定时读一次 compact leaf Schema。

根据本次操作只读取以下一份操作 Reference，不预加载完整协议或多个示例集：

- Shape、Connector、Frame、样式、Icon 或 Path：
  [compose.md](./whiteboard/compose.md)
- 已上传或本地 SVG/Vector 资源：[vector.md](./whiteboard/vector.md)
- 整页替换或清空：[replace.md](./whiteboard/replace.md)

读取已有白板或普通定位不需要额外 Reference。每个普通任务最多读取一份操作
Reference；只有操作页仍缺少具体字段时，才读取一份精确协议章节。

## 执行契约

- 同一 profile，目标须有真实身份：独立白板使用 `nodeId`；内嵌白板使用承载文档
  `nodeId + whiteboardId/partId`。零/多目标、身份不明或 profile 不一致时停止，
  不能取第一个候选。
- `--part-id` 完全未提供时默认独立白板；显式提供空值或纯空白会报错，不能借此
  切换类型。权限、网络、Feature Switch、revision 冲突等失败均不得跨接口回退。
- Runtime 确认后执行层才添加 `--yes`；存储示例不得预置确认。
- 成功须同时满足终态 receipt、请求节点映射和同板读回；`verified=false`、partial、
  commit-unknown 不得报成功或盲重试。
- `+update` 内部仍使用完整同板 query 校验：内嵌分支调用
  `read_whiteboard_content`，独立分支调用 `get_whiteboard_detail(view=page)`；成功结果只返回稳定目标、mode、验证节点
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
- `get_whiteboard_detail` 的 `resultJson` 是 JSON 字符串，解析后的对象就是 OpenNodes
  `source`。原子 `whiteboard query --view all/page` 保留已解码的 `resultJson` 并提供
  等价的顶层 `source`；严格 `+query` 使用统一结果 envelope，路径是 `data.source`。页面 ID 必须从
  `source.pages[].id` 取得，不能因为读取错了 envelope 层级就猜测或硬编码 pageId。
- 投影只减少重复上下文，不删除业务信息：query/export/audit 需要完整快照时原样
  交付；写入 `source` 保留在 Payload 文件中，不得因丢字段而重发远端请求。

## 稳定 ID

| ID | 来源 | 用途 |
|---|---|---|
| `nodeId` | 独立白板创建结果 / 文档解析 | 独立白板自身 ID，或内嵌白板的承载文档 ID |
| `blockId` | `doc whiteboard insert` | 文档块定位、排序和删除 |
| `whiteboardId` | `doc whiteboard insert` | 白板命令的 `--part-id` |
| 请求节点 `id` | 本地 OpenNodes 文件 | 同一请求的 parent/connector 引用 |
| 真实节点 ID | `+query` / `+update` 读回 | 读回身份；不能直接做局部 update |

insert 返回 `whiteboardId` 后直接使用；若为 null，只 fetch 一次并按本次 `blockId`
定位，仍未落库则报 pending，禁止重复插入。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。按本 skill/recipe 路由，命中时 Shortcut 优先于原子命令。参数只查 `dws schema --cli-path "whiteboard +<shortcut>" --compact --jq '{cli_path,parameters,constraints,confirmation}' -f json`；仅需且已发布 `result` 时查 `--jq '{cli_path,outcomes:.result.outcomes,pagination}'`，字段级再查 `data_schema`；缺失不以 Help/样例推断。Schema 不可用才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 禁 Help：错误 suggestion → 已加载 Skill/reference 明确入口；均无则报漂移。禁全 Catalog/root/parent/product Help；仅映射、接口或 provenance 审计省略 `--compact`。现有路由和 reference 均无法定位低频能力时，才用 `dws shortcut list --service whiteboard --format json` 发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws whiteboard +query` | read | 严格读取文档内嵌或独立白板的 OpenNodes 快照 |
| `dws whiteboard +update` | high-risk-write | 确认后更新文档内嵌或独立白板并精确读回 |
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

# 独立白板（无 part-id，默认独立）
dws whiteboard +query --node <WHITEBOARD_NODE_ID> --view all --format json
dws whiteboard +update --node <WHITEBOARD_NODE_ID> \
  --expected-revision <REVISION> --request-id <STABLE_REQUEST_ID> \
  --source @whiteboard.json --format json

# 使用 OpenNodes V1 初始内容创建独立白板
dws whiteboard create-with-content --name "<白板名称>" \
  --source ./whiteboard.json --request-id <STABLE_REQUEST_ID> --format json

# 导出独立白板；--output 是目录，文件名自动使用白板名称
dws whiteboard export --node <WHITEBOARD_NODE_ID> \
  --export-format png --output ./exports --format json

# 轮询中断后用已有 jobId 恢复查询和下载
dws whiteboard export-get --job-id <JOB_ID> \
  --export-format png --output ./exports --format json
```

导出下载仅接受 HTTPS/443 公网地址，每次重定向及实际连接都会校验目标地址；最大文件大小为 512 MiB，超限下载会清理临时文件。

导出支持 `--dry-run`，只预览请求，不轮询或写文件。`--output` 必须是目录；同名文件已存在时不会覆盖，请改用其他目录或先处理已有文件。下载内容会在临时文件中检查 PNG/PDF 文件头，通过后再落到最终路径；无效下载不会占用目标文件名。恢复 PDF 任务时须传 `--export-format pdf`；任务后续出错时会保留 jobId，并给出包含格式及目录的恢复命令（POSIX shell）。

`create-with-content --source` 本质是 OpenNodes JSON String 参数。较短内容可直接传
`'{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[...]}'`；内容较长时可传
本地文件路径。文件既可直接保存 OpenNodes，也可使用 `{"source":{...}}` 包装结构。
创建时 `source.nodes` 允许 `[]`，表示创建空白独立白板；字段仍必填，不接受 null。
这不改变更新规则：append 仍要求至少一个节点。普通空白创建仍优先使用文档创建能力。
CLI 校验后只向 `create_whiteboard` 发送 `source` JSON 字符串，不接受或暴露 checkpoint。

`--source` 接受 JSON、`@relative-file.json` 或 stdin；本地文件必须加 `@`，裸路径
会被当作 JSON。白板 shortcut 不支持 `--jq` / `--fields`。

## 坐标读回与稳定结构

坐标读回采用统一数值语义：整数、浮点数和 JSON Number 等价，并允许最多 0.5 像素
的服务端规范化偏差。超过容差返回 `readback_field_mismatch`，不能声明成功：

1. **已提交**：已有成功回执或已读到真实节点时，停止重提并只读对账。
   append 会创建新节点，不会修正已有节点；改成 frame 再 append 仍会重复创建。
   Runtime 保留 `error.details` 中的 `nodeId/partId或pageId/mode`、`commitState=committed`、
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
