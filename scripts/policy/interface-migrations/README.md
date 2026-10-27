# Interface migration ledgers

这些清单是供真人审阅的精确迁移计划，不是作者自报的 breaking-change 豁免。
审批数据与比较器必须来自已合入的 base；candidate 只提供待审批声明或消费回执。

| 文件 | 迁移范围 |
| --- | --- |
| `approved-flag-migrations-v1.json` | flag 改名、同名 flag 提升为必填 |
| `approved-command-migrations-v1.json` | 命令迁移、flag 拆为命令、Schema availability 收紧 |
| `approved-schema-parameter-migrations-v1.json` | 同一 Schema string 参数的精确 enum 变更 |

流程：**机制及 pending 审批 → consumed 业务消费 → 保留回执 → GA 追平后可清理**。
独立 Schema 参数清单的首次机制 MR 可以同时登记 pending，但业务参数必须仍为 before；
旧 base checker 完整执行、不读取新清单，也不批准本次产品变化。新 checker 仅以空审批清单
补查声明格式和三份 before 契约，须与机制一起交真人审阅；详见
[完整治理规范](../../../docs/cli-interface-flag-migrations.md)。

## Schema enum 声明格式

以下仅是格式示例，不表示任何工具已获批准。审批 MR 必须使用真实的 product/tool/parameter、
canonical CLI 路径及历史枚举；只新增 pending，不能同时改业务参数。

```json
{
  "version": 1,
  "migrations": [
    {
      "kind": "schema_enum_change",
      "command": "dws example configure",
      "schema": {
        "product_id": "example",
        "tool_id": "example.configure",
        "parameter": "mode"
      },
      "before": {"enum": ["legacy", "local"]},
      "after": {"enum": ["cloud", "local"]},
      "state": "pending",
      "reason": "Explain the intentional change, old-client impact and migration plan."
    }
  ]
}
```

- enum 必须显式写数组；`[]` 表示旧 Schema 无枚举限制，不是匹配任意 before 的通配符。
- 值按字符串升序排列，禁止空值、首尾空白及重复；after 必须非空且与 before 不同。
- 标识和路径必须精确，禁止通配匹配；同一工具/参数或同一命令/参数不能有多条迁移。
- 未知字段、字段大小写漂移、重复 JSON 键、null、错误类型和尾随内容一律拒绝。
- base、GA 和 candidate 中参数都必须存在、类型为 string、路径正确。审批时三者保持
  before；消费时只有 base 已持有的 pending 能授权 candidate 的精确 after。
- 除 state 的 pending → consumed 外，不得改写原记录。只规范化历史参数的 enum 副本，
  不改原始历史快照，也不按错误字符串删除 finding；目标参数的其他字段漂移仍失败。
- 参数归属、权限、服务端兼容、CLI 旧值是否继续可执行、发布顺序和迁移影响，须在 MR
  的 reason/说明及证据中明确供真人判断；ledger 不把有意破坏变更自动证明为兼容。
