# DWS Agent Schema 统一方案

## 1. 目标

DWS 已采用静态端点和固定命令面。本方案在不恢复运行时服务发现的前提下，建立一套可发布、可审计、面向 Agent 的命令契约：

- `--help` 面向人，描述当前二进制中真实可执行的 Cobra 命令和 flag。
- `dws schema` 面向 Agent，提供稳定 canonical path、真实 CLI path、参数类型与约束、风险语义和渐进查询。
- Command Catalog 是构建产物，不是运行时 MCP `tools/list` 缓存。
- MCP/Wukong 描述只以脱敏、固定 revision 的版本化快照进入构建，不在启动或查询 Schema 时拉取。
- Skills 负责路由和工作流语义，不负责定义一个并不存在的命令或参数。

当前发布面为 21 个产品、504 个工具。

## 2. 总体架构

```text
Go/Cobra 命令与 flag
  + 强类型参数/约束注解
  + schema_command_surface.json       (审核过的公开命令面)
  + schema_mcp_metadata.json          (脱敏接口事实快照)
  + schema-hints/*.json               (显式 Agent 语义)
  + Skills/Markdown                    (路由、场景、工作流)
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
          CI / smoke / drift gate
```

运行时不执行 MCP `tools/list` 来生成命令或 Schema。

## 3. 数据分工

| 数据源 | 负责内容 | 不负责内容 |
|---|---|---|
| Go/Cobra | 可执行路径、flag、类型、默认值、本地兼容逻辑 | Agent 场景描述 |
| 强类型注解 | `required`、`required_when`、one-of、互斥、联动、格式、枚举、位置参数 | 新增命令 |
| 版本化 parameter binding JSON | 稳定 CLI flag 到 RPC property 的映射 | `required`、约束、命令发现 |
| `schema_command_surface.json` | 公开 canonical path、primary CLI path、alias、跨产品 source binding | endpoint、token |
| `schema_mcp_metadata.json` | 固定 revision 的 RPC 名、接口描述和脱敏 input schema | 运行时路由、风险推断 |
| `schema-hints/*.json` | 审核过的 summary、effect、risk、confirmation、idempotency、use/avoid、示例 | 参数契约 |
| Skills/Markdown | 产品路由、使用/禁用场景、前置条件、工作流、提示 | 命令存在性 |
| `schema_catalog.json` | 上述信息合并后的发布级 Command Catalog | 手工编辑源 |

`required` 是 CLI 执行契约，只能来自 Cobra required 标记、当前 Go helper 的强类型注解或审核过的参数 hint。MCP input schema 的 `required` 描述 RPC payload，helper 可能负责合成该字段，因此它只能作为接口事实，不能把一个可选 CLI flag 提升为全局必填。

### 3.1 接口事实

“IR/MCP 为接口事实”指 RPC 名称、参数 JSON Schema、接口描述等服务端事实。它们经过脱敏后固定到 `schema_mcp_metadata.json`，并保留 source revision/hash。

接口事实不能直接决定：

- 命令是否公开；
- CLI 路径和兼容 alias；
- 写入风险和确认策略；
- Agent 应在什么业务意图下选择它。

这些决策分别由公开 command surface、Cobra/注解和 Agent hints/Skills 管理。

## 4. Command Catalog

Catalog 是发布版本内的统一命令中间表示，位于 `internal/cli/schema_catalog.json`，包含：

- 稳定 `canonical_path`；
- `primary_cli_path`、alias、source product/RPC binding；
- 参数类型、required、条件必填和组合约束；
- Agent summary、use/avoid、effect、risk、confirmation、idempotency、示例；
- 来源 revision/hash 和生成 hash。

核心入口位于：

- `internal/cli/schema_catalog.go`：嵌入、校验、查询 Catalog；
- `internal/generator/cmd_schema_catalog`：从发布命令树生成 Catalog；
- `internal/helpers/catalog_fallback.go`：以 `-100` 优先级补齐固定契约中缺失的执行叶子。

真实 Go helper 优先于 fallback。fallback 只能补缺，不能覆盖现有命令；低优先级 fallback 也不能把显式隐藏/下线的命令重新显示。经过审核的隐藏兼容命令可以进入固定 Schema；已经下线的入口继续保留错误提示兼容，但必须从公开 command surface 和 Schema 中删除。

内嵌 Catalog 对真实 Go 命令只允许回填稳定 canonical identity，禁止回灌任何参数元数据。flag-to-RPC property binding 固定在版本化 `internal/cli/schema_parameter_bindings.json`，每次人工执行 `make generate-schema` 都先应用该输入；required 和 constraints 则从当前 Cobra 树及强类型注解重算。这样 generator 不依赖上一次 Catalog 自回放。完整旧契约只允许用于 `-100` 优先级的 fallback leaf。

## 5. Agent 元数据

每个公开工具必须具备：

- `agent_summary`；
- `effect`: `read | write | destructive`；
- `risk`: `low | medium | high`；
- `confirmation`: `not_required | user_required`；
- `idempotency`: `idempotent | non_idempotent | unknown`。

可选增强字段包括 `use_when`、`avoid_when`、`prerequisites`、`tips`、`workflow_refs` 和 `examples`。

合并优先级为：

1. 显式审核的 JSON hint；
2. Skill 中可约束解析的语义；
3. 固定 revision 的 Wukong/MCP 描述；
4. 命令标题和动作动词推断。

低优先级来源只能填空，不能覆盖显式风险规则。MCP 派生 summary 标记为 `reviewed:false`；所有 `destructive` 工具强制提升为 `risk:high` 和 `confirmation:user_required`，Agent 不能自行确认。

当前生成覆盖：

- 产品元数据：21/21；
- 工具元数据与 summary：504/504；
- effect/safety 字段：504/504；
- MCP 接口 summary 应用：271；
- intents：1166；
- examples：746；
- reviewed one-of/互斥/联动约束：21；
- destructive safety：48/48 通过。

`unmatched_skill_tools` 是 Skill 文档中的旧路径或复合表达未映射到公开工具，不代表命令缺失。它保留在 audit 中作为文档治理债务。

## 6. `--help` 与 `schema`

两者共享真实 CLI path 和 flag，但用途不同：

| 能力 | `--help` | `dws schema` |
|---|---|---|
| 面向对象 | 人 | Agent/程序 |
| 输出稳定性 | 文本，不作为机器契约 | JSON 稳定契约 |
| 路径 | CLI path | canonical + CLI path + alias |
| 参数 | flag 用法 | 类型、required、枚举、格式、组合约束 |
| 风险语义 | 可选提示 | effect/risk/confirmation/idempotency |
| 查询方式 | 单命令展开 | 产品 -> 分组 -> 工具渐进展开，或 `--all` |

`schema list` 作为兼容入口等价于根概览；新脚本应直接使用 `dws schema`。完整审计使用 `dws schema --all`。

## 7. 无运行时服务发现

以下路径已明确禁止启动期或 Schema 查询期 `tools/list`：

- `dws schema` 只读内嵌 Catalog；内嵌数据损坏时只允许本地 Cobra 回退；
- HTTP plugin 只从 manifest 注册 endpoint 和鉴权；
- stdio plugin 只注册 descriptor 和未启动 client；
- stdio 进程在真实执行时才 `Start + Initialize + tools/call`；
- Schema 生成不依赖用户登录、网络、个人 token 或本地 MCP cache。

`internal/transport` 仍保留 MCP 协议的 `ListTools` primitive 和协议测试，但 `internal/app`、`internal/cli` 不调用它生成命令面。

## 8. 构建与发布

修改 Skill、hint 或接口快照后执行：

```bash
make generate-schema
```

它按顺序生成 Agent metadata 和最终 Catalog。单独调试可使用：

```bash
make generate-schema-agent-metadata
make generate-schema-catalog
```

公开命令面不是普通生成物。只有确认新增、删除或重命名公开命令时才执行并评审：

```bash
make generate-schema-command-surface
```

外部 MCP/Wukong 元数据刷新必须满足：固定 commit/revision、脱敏、仅匹配现有公开 command surface、生成 audit；不得携带 endpoint、Authorization、token、secret 或用户数据。

## 9. 质量门禁

`make policy` 执行以下检查：

- 公开 surface、Agent index、MCP snapshot、Catalog 数量一致；
- Catalog canonical paths 与审核 surface 完全一致；
- 504 个工具均具备 summary/effect/safety；
- destructive 操作必须为 high risk 且要求用户确认；
- reviewed 组合约束数量固定，避免生成时静默丢失；
- Catalog 不含 endpoint 或凭据字段；
- `internal/app`、`internal/cli` 不调用 `ListTools`；
- 504 条定义都能映射到真实 Cobra 路径，参数都能映射到真实 flag；
- 隐藏子树不污染 `--help`，但固定 Schema 能正确遍历；
- metadata 与 Catalog 连续生成无漂移。

全量 smoke 还应遍历每个工具及 one-of 的每个分支执行 `--dry-run`，不能只测试每组的第一条参数组合。

## 10. 后续治理

当前已达到“稳定、完整、可生成、无运行时发现”的基础线，但 Agent 语义仍需持续评审：

1. 将 `reviewed:false` 的 MCP/标题回退 summary 按产品逐步转为显式 hint；
2. 清理 audit 中的旧 Skill 路径和复合表达；
3. 为 one-of、互斥、`required_when` 增加各分支 smoke；
4. 将更多 RunE 手工校验迁移为强类型注解，避免参数语义只存在于执行代码；
5. 变更 command surface 时同时更新 help、Schema、Skill 引用和 dry-run 契约测试。
