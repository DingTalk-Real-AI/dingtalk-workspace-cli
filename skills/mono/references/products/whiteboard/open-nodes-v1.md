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
- CLI 只预检 JSON、信封、版本和 `nodes` 数组等外层结构；节点字段、枚举、层级、
  引用和业务约束由白板服务完整校验。任一层失败都不会保留部分更新。
- DWS 返回以 `success`、`nodeId`、`partId`、`resultJson` 和可选的
  `resultSummary` 为准。

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

- 使用 `shape.geometry` 前必须读取 [08-catalogs](open-nodes-v1/08-catalogs.md)，
  不得猜测 geometry。
- 使用 `icon.catalogId` 前必须读取 [06-vector-icon-path](open-nodes-v1/06-vector-icon-path.md)
  和 [08-catalogs](open-nodes-v1/08-catalogs.md)。
- 使用 `path` 前必须读取 [06-vector-icon-path](open-nodes-v1/06-vector-icon-path.md)，
  不得把它当作通用 SVG Path。
- Query 结果不能直接作为 update source；转换前必须读取
  [03-update](open-nodes-v1/03-update.md) 和
  [07-examples-errors-write-support](open-nodes-v1/07-examples-errors-write-support.md)。
