# DWS OpenNodes V1 协议说明

> 本文件是 DWS OpenNodes V1 协议的拆分章节。按需读取入口见
> [协议索引](../open-nodes-v1.md)。

> 协议版本：`schemaVersion = "1.0"`，`catalogVersion = "dml-v1"`。

## 1. 协议用途

OpenNodes 是 DWS 白板命令使用的语义节点协议，提供两类能力：

- `dws whiteboard query`：返回稳定、可理解的页面和节点数据。
- `dws whiteboard update`：接收受约束的节点描述，以 `append` 或
  `overwrite` 模式修改白板。

调用方只应依赖本文声明的语义字段和行为：

- `query` 不修改白板。
- `update` 全部成功或全部回滚，不返回中间状态。
- 未声明的存储字段、类型名称和处理过程不属于协议承诺。

OpenNodes V1 支持的节点类型、字段和读写范围见第 7 节。

DWS 负责身份认证和权限校验。Vector 资源准备使用 `dws doc media upload`，
具体流程见白板命令参考。

## 2. 版本与兼容原则

| 字段 | 当前值 | 作用 |
| --- | --- | --- |
| `schemaVersion` | `1.0` | 控制文档结构、节点字段和字段语义。 |
| `catalogVersion` | `dml-v1` | 控制允许写入的 DML 几何、连接线标记和内置 icon 目录。 |

V1 采用严格校验：

- 必填字段缺失会失败。
- 未声明字段会失败，不会被静默忽略。
- query-only 字段出现在 update 中会以 `readOnlyField` 失败。
- 不支持的节点类型、目录值或引用范围会失败。
- `null` 不代表“使用默认值”；除非字段类型明确允许，否则会失败。

本文列出的请求枚举值都是协议字面量，调用方必须按文档中的大小写和拼写原样传入，
不能自行转换或猜测。响应中未来可能增加可选字段，调用方应忽略不认识的响应字段。

调用方必须原样携带当前版本值。新增不兼容结构时应升级
`schemaVersion`；修改 DML、marker 或 icon 目录时应评估并升级
`catalogVersion`。

## 3. DWS 命令一览

| 命令 | 所需权限 | 效果 |
| --- | --- | --- |
| `dws whiteboard query --node ... [--part-id ...] [--view ...]` | 可查看白板 | 有 partId 读取内嵌单页；无 partId 读取独立白板。 |
| `dws whiteboard update --node ... [--part-id ...] --source ... --yes` | 可编辑白板 | 有 partId 更新内嵌白板；无 partId 按 revision 更新独立白板。 |
| `dws whiteboard create-with-content --name ... --source 'JSON' --request-id ...`（长内容也可用 `--source ./whiteboard.json`） | 可创建白板 | source 是 OpenNodes V1 JSON String 或保存相同 JSON 的文件路径；`nodes: []` 表示空白创建，nodes 不可缺失或为 null；不接受 checkpoint。 |

分流只由 `--part-id` 是否显式提供决定：非空值走内嵌接口，完全省略走独立接口；
显式空值和纯空白会在本地失败。内嵌分支保持原有单页参数和输出，不接收 pageId。
独立分支支持 summary/page/all 视图；写入使用 expectedRevision 与稳定 requestId 做
并发和幂等保护，overwrite 明确指定 pageId。服务错误不会触发另一分支。
