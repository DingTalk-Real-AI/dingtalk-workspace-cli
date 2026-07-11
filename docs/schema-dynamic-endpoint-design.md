# DWS Agent Schema 统一方案

## 1. 核心定义

DWS Schema 是**构建期从当前 Go/Cobra 真实命令树生成的版本化 Agent 执行契约**。它描述当前二进制真实可执行的 CLI surface，并在此基础上合并 Agent 选择语义、安全语义、接口事实和示例。

当前定义遵循一条原则：**schema 描述真实 CLI，而不是 schema 反向制造 CLI**。

因此：

- `--help` 面向人，展示当前二进制真实可执行的 Cobra 命令和 flag。
- `dws schema` 面向 Agent/程序，提供稳定 JSON 契约，包括 canonical path、CLI path、参数类型、required、约束、风险语义、接口事实和示例。
- Command Catalog 是构建期产物，经 `go:embed` 进入二进制；运行时只读，不动态发现。
- schema 查询不调用 MCP `tools/list`，不访问网络，不读取用户本地 discovery cache。
- Skill 和 hints 只补充 Agent 语义，不能定义一个真实 Cobra 树里不存在的命令或参数。
- runtime catalog fallback 已废弃，不再用旧 catalog 反向补 Cobra leaf。

当前生成结果为 **20 个产品 / 537 个工具**。

## 2. 生命周期

```text
开发者修改 Go/Cobra 命令、flag、help、参数注解或 schema hint
        |
        v
make generate-schema-command-surface
        |
        v
internal/cli/schema_command_surface.json
        |
        v
make generate-schema-agent-metadata
        |
        v
internal/cli/schema_agent_metadata/*.json
internal/cli/schema_agent_metadata_audit.json
        |
        v
make generate-schema-catalog
        |
        v
internal/cli/schema_catalog.json
        |
        v
go:embed 进入二进制
        |
        v
运行时 dws schema 只读内嵌 catalog
```

## 3. 架构

```text
Go/Cobra 真实命令树
  + 强类型参数/约束注解
  + schema_parameter_bindings.json      (稳定 flag -> RPC property 映射)
  + schema_mcp_metadata.json            (固定 revision 的脱敏接口事实)
  + schema-hints/*.json                 (显式 Agent 语义)
  + Skills/Markdown                      (路由、场景、工作流)
                 |
                 | 构建期生成
                 v
        schema_agent_metadata/*.json
                 +
          schema_catalog.json
                 |
       +---------+----------+
       |                    |
    --help              dws schema
   人类用法              Agent 契约
       |                    |
       +---------+----------+
                 |
          CI / drift / catalog gate
```

## 4. 数据分工

| 数据源 | 负责内容 | 不负责内容 |
|---|---|---|
| Go/Cobra | 真实可执行路径、flag、类型、默认值、本地兼容逻辑、help 文本 | Agent 场景描述 |
| 强类型注解 | `required`、`required_when`、one-of、互斥、联动、格式、枚举、位置参数 | 新增命令 |
| `schema_parameter_bindings.json` | 稳定 CLI flag 到 RPC property 的映射 | 命令发现、risk 推断 |
| `schema_command_surface.json` | 当前公开 canonical path、primary CLI path、alias、source binding | endpoint、token |
| `schema_mcp_metadata.json` | 固定 revision 的 RPC 名、接口描述、脱敏 input schema | 运行时路由、风险推断 |
| `schema-hints/*.json` | 审核过的 summary、use/avoid、effect、risk、confirmation、idempotency、examples、interface mode | 参数事实 |
| Skills/Markdown | 产品路由、使用场景、禁用场景、工作流、操作建议 | 命令存在性 |
| `schema_catalog.json` | 上述信息合并后的发布级 Command Catalog | 手工编辑源 |

`required` 是 CLI 执行契约，只能来自 Cobra required 标记、当前 Go helper 的强类型注解或审核过的参数 hint。MCP input schema 的 `required` 描述 RPC payload，helper 可能负责合成该字段，因此它只能作为接口事实，不能把一个可选 CLI flag 提升为全局必填。

## 5. 生成步骤

### 5.1 生成命令面

```bash
make generate-schema-command-surface
```

该目标直接基于 `app.NewRootCommand()` 的真实 Cobra 树构建 command surface，不再通过 `dws schema --all` 读取旧 embedded catalog。

### 5.2 生成 Agent 元数据

```bash
make generate-schema-agent-metadata
```

输入包括：

- `skills/mono/SKILL.md`
- `skills/mono/references/products/**/*.md`
- `skills/mono/schema-hints/*.json`
- `internal/cli/schema_mcp_metadata.json`
- `internal/cli/schema_command_surface.json`

输出包括：

- `internal/cli/schema_agent_metadata/index.json`
- `internal/cli/schema_agent_metadata/<product>.json`
- `internal/cli/schema_agent_metadata_audit.json`

### 5.3 生成最终 Catalog

```bash
make generate-schema-catalog
```

输出：

- `internal/cli/schema_catalog.json`

运行时 `dws schema` 只读该内嵌 Catalog。

## 6. Agent 元数据要求

每个公开工具必须具备：

- `agent_summary`
- `use_when`
- `avoid_when`
- `examples`
- `effect`: `read | write | destructive`
- `risk`: `low | medium | high`
- `confirmation`: `not_required | user_required`
- `idempotency`: `idempotent | non_idempotent | unknown`
- `interface_mode`: `mcp | composite | local | unavailable`
- `availability`: `available | unavailable`

安全规则：

- 所有 `destructive` 工具必须是 `risk: high` 且 `confirmation: user_required`。
- 所有 `risk: high` 工具必须要求用户确认。
- 示例不得包含 `--yes`，Agent 必须先向用户确认，再显式传 `--yes` 执行。

当前生成覆盖：

- 产品：20/20
- 工具：537/537
- `agent_summary`：537/537
- `use_when`：537/537
- `avoid_when`：537/537
- `examples`：537/537
- `effect/risk/confirmation`：537/537
- `interface_mode/availability`：537/537

`unmatched_skill_tools` 是 Skill 文档里的旧路径、分组或复合表达，不代表 schema 命令缺失；它作为 audit 诊断项保留，不阻断 catalog 生成。

## 7. `--help` 与 `dws schema`

两者共享同一真实 CLI surface，但用途不同：

| 能力 | `--help` | `dws schema` |
|---|---|---|
| 面向对象 | 人 | Agent/程序 |
| 输出形式 | 文本 | 稳定 JSON |
| 路径 | CLI path | canonical path + CLI path + alias |
| 参数 | flag 用法 | 类型、required、默认值、枚举、格式、组合约束 |
| 风险语义 | 可在描述中提示 | effect/risk/confirmation/idempotency |
| 查询方式 | 单命令展开 | 产品 -> 分组 -> 工具渐进展开，或 `--all` |

业务级 persistent flags（如 `calendar --event`、`calendar --calendar-id`）属于 CLI 参数契约，应纳入 schema/help 一致性检查；基础设施级全局 flags（如 `--debug`、`--format`、`--yes`）不属于单工具参数。

## 8. 无运行时发现与无 fallback

以下行为被明确禁止：

- `dws schema` 运行时调用 MCP `tools/list`。
- `dws schema` 运行时访问网络生成命令面。
- 根据旧 `schema_catalog.json` 在运行时反向创建 Cobra 命令。
- 使用 `helpers.NewCatalogFallbackCommands` 补齐不存在的 leaf。
- 让 schema 暴露真实 Cobra 树里不存在的命令或参数。

如果命令、flag 或 help 发生变化，必须重新生成 schema 并提交生成物。

## 9. CI / 门禁

CI 需要验证：

- `schema_command_surface.json` 与 `schema_catalog.json` 的 canonical paths 完全一致。
- `schema_catalog.json`、`schema_agent_metadata/index.json` 数量和 surface hash 一致。
- 所有公开工具具备完整 Agent 元数据字段。
- `interface_mode == "mcp"` 当且仅当 `interface_ref != null`。
- `schema_parameter_bindings.json` 的 source hash 等于当前 catalog source hash，且每条 binding 都能映射到当前工具参数。
- 示例以工具 primary CLI path 开头，且不包含 `--yes`。
- destructive/high-risk 安全规则成立。
- 生成物连续生成无漂移。

推荐本地验证命令：

```bash
make generate-schema-command-surface
make generate-schema-agent-metadata
make generate-schema-catalog
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-schema-catalog.sh
go test ./internal/cli ./internal/app ./internal/generator/... -count=1
```

## 10. 后续治理

当前 schema 已收敛到真实 Cobra 命令树事实源。后续重点：

1. 修正式 schema/help 双向比对脚本，使其正确区分业务 persistent flags 与基础设施 global flags；
2. 清理 Skill 文档中的旧路径和复合表达，降低 `unmatched_skill_tools`；
3. 将更多 RunE 手工校验迁移为强类型注解，避免参数语义只存在于执行代码；
4. 为 one-of、互斥、`required_when` 增加分支 dry-run smoke；
5. 新增或修改 CLI 命令时，同时更新 help、schema、Skill 引用和 dry-run 契约测试。
