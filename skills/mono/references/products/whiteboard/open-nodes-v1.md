# OpenNodes V1（DWS 白板协议索引）

本目录承载 `dws whiteboard query/update` 使用的 OpenNodes V1 协议。协议按
调用阶段拆分，Agent 只加载当前任务所需章节，避免一次性读取全部内容。

## DWS 使用规则

- 只通过 `dws whiteboard query/update` 读写白板。
- 每次调用必须提供承载白板的文档 `--node` 和目标白板 `--part-id`。
- DWS 当前只支持单页白板：命令没有 `--page-id`，update 文件禁止包含
  `pageId`。
- `query` 不接收请求体；CLI 会把返回的 `resultJson` 解析成对象。
- 白板命令不支持使用全局 `--jq` 或 `--fields` 过滤输出；传入任一参数都会报错，
  Agent 直接读取 CLI 返回的结构化 JSON。
- `update --source` 使用 `overwrite + source` 信封；append 和 overwrite 都是
  远端写入，获得用户确认后必须通过 `--yes` 显式确认。
- 原子 `whiteboard update` 本地预检 JSON、信封、版本、`nodes` 对象数组和 append 非空。
  首选的 `whiteboard +update` 还在调用服务前校验节点 ID 唯一且 type 非空，以及 connector 的
  端点类型/有限坐标、同请求 `nodeRef` 引用及目标可用性、anchor、marker、routing 和
  waypoints 组合，拒绝自环及已识别的 query-only/推导字段。本地校验失败不会调用服务。
  其余节点字段、枚举、层级和业务约束仍由白板服务完整校验；远端提交后的连接中断或
  读回失败不代表未写入，必须保留回执和稳定 ID，不能盲目重发。
- 原子命令返回 `success`、`nodeId`、`partId`、`resultJson` 和可选的 `resultSummary`。
  `+query/+update` 使用统一结果信封；`+update` 的 `data.receipt` 区分真实终态回执与
  `dryRun=true/executed=false` 预览，真实成功经独立读回验证且不重复返回完整快照。

## 按任务读取

| 当前任务 | 必读章节 |
|---|---|
| 理解版本、兼容和命令语义 | [01-overview](open-nodes-v1/01-overview.md) |
| 读取或解释 query 结果 | [02-query](open-nodes-v1/02-query.md) |
| 构造 append、overwrite 或清空请求 | [03-update](open-nodes-v1/03-update.md) |
| 写富文本、列表、链接、主题色、渐变或阴影 | [04-text-style](open-nodes-v1/04-text-style.md) |
| 写 shape、便签、frame、group 或 connector | [05-shape-frame-group-connector](open-nodes-v1/05-shape-frame-group-connector.md) |
| 写已上传 Vector、内置 Icon 或自由 Path | [06-vector-icon-path](open-nodes-v1/06-vector-icon-path.md) |
| 处理错误、query 转写或判断 writeSupport | [07-examples-errors-write-support](open-nodes-v1/07-examples-errors-write-support.md) |
| 选择合法 geometry 或 icon catalogId | [08-catalogs](open-nodes-v1/08-catalogs.md) |

## 强制读取规则

- [构图 Reference](compose.md) 已评审并内嵌
  `dml:rect` / `dml:roundRect` / `dml:diamond`、三种 connector marker、
  `task/task-done` 和一条合法 Path 模板；原样复用这些值时不重复读取目录章节。
- 使用安全子集之外的 `shape.geometry` 前必须读取
  [08-catalogs](open-nodes-v1/08-catalogs.md)，不得猜测 geometry。
- 使用安全子集之外的 `icon.catalogId` 前必须读取
  [06-vector-icon-path](open-nodes-v1/06-vector-icon-path.md) 和
  [08-catalogs](open-nodes-v1/08-catalogs.md)。
- 原样复用 `compose.md` 的安全 Path 无需额外读取；其他 Path 必须读取
  [06-vector-icon-path](open-nodes-v1/06-vector-icon-path.md)，不得把它当作通用 SVG Path。
- Query 结果不能直接作为 update source；转换前必须读取
  [03-update](open-nodes-v1/03-update.md) 和
  [07-examples-errors-write-support](open-nodes-v1/07-examples-errors-write-support.md)。
