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
| `dws whiteboard query --node ... --part-id ...` | 可查看白板 | 读取单页白板，不修改内容。 |
| `dws whiteboard update --node ... --part-id ... --source ... --yes` | 可编辑白板 | 追加节点或整页重建；所有更新都需先取得用户确认。 |

DWS 当前只支持文字文档中已有的单页内嵌白板。命令不接收 `pageId`，也不提供
创建页面、切换页面或按既有节点 ID 局部修改的能力。
