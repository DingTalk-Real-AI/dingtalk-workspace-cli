# DWS Agent Schema 统一方案

> 状态：P0/P1 已落地并验证；P2 Agent 语义补齐进行中
>
> 基线：`feat/schema-gws-flat`，2026-07-10；MCP/Wukong 固定来源 `4574f7022c32cf4c033e9b7b4156e2fec815fed8`
>
> 相关文档：[Schema 契约](./schema-contract.md)、[Schema 执行问题修复记录](./schema-command-issue-fixes.md)

## 1. 背景与结论

这项工作不只是补齐 `--node required`，也不是要求 `--help` 与 Schema 复制同一段文案，而是建立一套稳定的命令事实、Agent 语义、版本化生成物和质量门禁体系。

统一原则如下：

- Go/Cobra 和实际运行时校验负责可执行事实。
- 强类型 Schema 注解负责 CLI 无法从 flag 类型直接表达的契约。
- 内嵌 MCP/IR 元数据负责接口事实，不负责运行时服务发现。
- Skill 负责意图路由、工作流、使用场景、禁用场景和操作建议。
- 版本化 JSON 负责固定发布版本内的 command surface、接口元数据和 Agent 元数据。
- 构建期将上述来源编译为统一 Command Catalog。
- `--help` 和补全继续反映实际可执行 Cobra 树；`schema` 读取构建期从同一树冻结的 Catalog，质量门禁负责两者对齐。
- 启动和查询 Schema 时不执行 MCP `tools/list`，也不依赖运行时服务发现。

```text
Go/Cobra 硬编码命令 + 强类型 Schema 注解 + 版本化 JSON + Skills
                         |
                         v  构建期生成/校验
                    统一 Command Catalog
          +--------------+---------------+
          |              |               |
        --help         schema         补全/测试
        面向人          面向 Agent      质量门禁
```

## 2. 当前基线

当前实现已经具备版本内稳定 Schema，Agent 语义覆盖仍需继续补齐。

| 指标 | 当前结果 | 说明 |
|---|---:|---|
| 公开产品 / canonical tools | 21 / 504 | compat alias 不重复计为工具；internal roots 不暴露 |
| hardcoded / runtime CLIOverlay | 285 / 219 | 说明命令构建来源，最终均冻结进入 Catalog |
| MCP `tools/list` 原始快照 | 53/54 服务，969 tools | `notify` 当前 initialize 无有效结果 |
| 已投影 MCP 接口事实 | 419/504 | 83.1%；178 条通过显式/跨产品映射绑定 |
| 有 Agent 元数据的工具 | 460/504 | 91.3%；MCP fallback 新增覆盖 29 个工具 |
| 有 `agent_summary` 的工具 | 419/504 | 83.1%；其中 272 条由固定 MCP description 补空生成，均标记未人工审核 |
| 有 `effect` 的工具 | 301/504 | 59.7% |
| 有显式风险的工具 | 22/504 | 高风险补齐仍是后续重点 |
| Wukong envelope 精确匹配 | 147/245 eligible | 98 条进入 unmatched 审计 |
| 最终 Catalog | 21 产品 / 504 leaf details | 路径集合与 command surface 完全一致 |

当前输出体积：

| 查询 | 大小约值 | 用途 |
|---|---:|---|
| `dws schema` | 4.8 KB pretty / 3.6 KB compact JSON | 21 产品路由概览 |
| `dws schema --all` | 531 KB | CI/审计，不应进入普通 Agent 上下文 |
| 单工具示例 `calendar event create` | 7.3 KB | 20 个参数及完整语义 |

Sheet 的递归 Skill 解析缺口已关闭：58 个工具已进入生成物，其中 6 个有显式 `agent_summary`、36 个有 `effect`。MCP summary fallback 将无 Agent 元数据工具从 73 个降到 44 个；剩余项均无直接 MCP 映射，包括 33 个本地会议动作、9 个 Dev helper、`aitable.create` 和 `chat.upload_conversation_file`，需要从 Go/Cobra、产品说明或人工 Hint 补齐。

### 2.1 当前实施检查点

已完成递归 Skill 解析、多行示例、canonical command surface、`public/compat/internal` visibility、Hint JSON 合并、Wukong 描述导入、MCP `tools/list` 静态投影、MCP Agent summary fallback 和最终 Catalog 嵌入。相同二进制在正常缓存与空缓存下生成的 overview/`--all` SHA-256 完全相同；真实 `dws schema` 入口还会跳过 runtime Registry 和插件命令装配。

### 2.2 本轮验证结果

- `go test ./... -count=1` 全部通过。
- Catalog 路径/哈希/敏感信息门禁和 Agent 生成物 drift 检查通过。
- 全量 Schema dry-run：504 个工具展开为 560 个约束分支，560/560 通过。
- Catalog 连续两次生成文件哈希相同；正常缓存/空缓存的 overview、`--all` 和单工具输出逐字节相同。
- 已使用发布二进制完成认证状态检查和 Calendar 只读真实调用，返回成功。

## 3. 目标与非目标

### 3.1 目标

1. 建立唯一、稳定、可追溯的 Command Catalog。
2. Schema 中的路径、参数、类型、默认值、必填和约束与实际 CLI 一致。
3. Skill 中的意图、工作流和安全规则可编译进入 Agent Schema。
4. 兼容别名继续可执行，但默认不污染 Agent 工具选择。
5. 内部 server/product 不作为独立产品暴露给 Agent。
6. 每个字段能说明来源，生成物在版本内固定。
7. Agent 可以渐进式查询，而不是加载完整 531 KB Catalog。
8. 通过静态门禁、全分支 dry-run 和 Agent golden cases 防止漂移。

### 3.2 非目标

- 不在 Schema 查询时调用运行时 `tools/list`。
- 不让 Skill 定义或覆盖真实 flag、类型和必填事实。
- 不要求 Help 与 Agent 提示逐字相同。
- 不把全部 Skill 工作流原文塞入每个工具 Schema。
- 不在一个 PR 中人工补齐全部 504 个工具。

## 4. 数据分工

| 数据源 | 负责内容 | 不负责内容 |
|---|---|---|
| Go/Cobra | 真实可执行路径、flag、CLI 类型、默认值、隐藏状态、硬编码 helper、运行时校验 | Agent 意图和业务工作流 |
| 强类型注解 | required、one-of、互斥、联动、格式、枚举、位置参数、Schema visibility | 长篇使用说明 |
| IR/MCP 接口元数据 | RPC property、接口类型、接口描述、接口枚举和默认值 | CLI 别名、Helper 兼容语义、Agent 风险决策 |
| 版本化 command surface JSON | 当前版本公开路径、canonical 映射、hash | 动态服务发现 |
| Skill/Markdown | `use_when`、`avoid_when`、前置条件、tips、工作流、示例、消歧和风险规则 | CLI 参数事实 |
| 显式 Hint JSON | 无法从 Markdown 稳定推断的强类型 Agent 语义和参数填写策略 | 重复保存整个命令定义 |
| 生成后的 Agent JSON | Skill + Hint 的确定性编译产物，随二进制内嵌 | 人工直接编辑 |

现有版本化文件继续保留：

- `internal/cli/schema_command_surface.json`：无 endpoint 的命令 surface。
- `internal/cli/schema_mcp_metadata.json`：清洗后的接口事实。
- `internal/cli/schema_agent_metadata/*.json`：构建期生成的 Agent 元数据。
- `internal/cli/schema_catalog.json`：构建期生成的最终查询 Catalog；运行时 `dws schema` 只读此文件的嵌入副本。

建议新增人工维护的强类型 Agent Hint 源：

```text
skills/mono/schema-hints/
  aitable.json
  calendar.json
  chat.json
  doc.json
  sheet.json
  ...
```

这些 Hint 文件与 Skill 一起参与 source hash；`internal/cli/schema_agent_metadata/*.json` 仍是生成物，不手工修改。

### 4.1 Wukong MCP 元数据来源

仓库和线上域名的职责必须分开：

| 来源 | 用途 |
|---|---|
| `mcp.dingtalk.com` | OAuth、MCP 市场和悟空发现入口 |
| `mcp-gw.dingtalk.com` | 各产品实际 MCP Server endpoint |
| `dws-wukong/envelope/channel/wukong/prod/*.json` | 版本化生产 CLI overlay，包括产品/工具描述、示例、flag 映射和敏感操作标记 |
| MCP `tools/list` 快照 | 完整 RPC 输入结构等接口事实 |

`dws-wukong` envelope 是高质量 CLI/Agent 元数据，但不是完整 `tools/list inputSchema`。因此当前导入器遵循以下规则：

- 只在构建期读取指定 revision 的 `prod` envelope。
- 只保留与 `schema_command_surface.json` 精确匹配的公开命令。
- 导入产品/工具 `agent_summary`、示例和 `isSensitive` 风险提示。
- 不导入 endpoint、token 或用户数据。
- 不用 envelope 的 `required/type` 覆盖 Cobra 和强类型注解。
- 不对未匹配路径做模糊自动绑定；全部写入审计文件人工处理。
- 发布二进制和 `dws schema` 运行时不访问发现接口。

当前固定来源为 `dws-wukong@4574f7022c32cf4c033e9b7b4156e2fec815fed8`：31 个 active 产品、286 条 tool override；按 504-tool surface 过滤后，245 条具备公开 CLI 名称，147 条精确匹配，98 条进入 unmatched 审计，其余为 hidden/缺少 CLI 名称。

更新命令：

```bash
make generate-schema-command-surface
make generate-schema-wukong-agent-hints \
  WUKONG_ENVELOPE_DIR=/path/to/dws-wukong/envelope/channel/wukong/prod \
  WUKONG_REVISION=<full-commit-sha>
make generate-schema-agent-metadata
make generate-schema-catalog
```

版本化输入为 `skills/mono/schema-hints/imported/wukong.json`，导入审计为 `internal/cli/schema_wukong_agent_hints_audit.json`。二者都记录 source revision 和原始 envelope hash。

### 4.2 MCP description 的 Agent fallback

`internal/cli/schema_mcp_metadata.json` 来自固定版本的真实 MCP `initialize` + `tools/list` 快照。Agent metadata 生成器会在 Skill、Wukong 和显式 Hint 都没有 `agent_summary` 时，使用该文件中的 description 补空：

- 只处理已投影到公开 command surface 的 MCP 工具，不做模糊路径绑定。
- 提取首个有效句子，最多 120 个字符；拒绝纯 RPC/identifier 描述。
- 写入 `agent_summary_source=<source>@<revision>`、source ref 和 `reviewed:false`。
- 不从自然语言自动推断 `effect`、`risk`、`confirmation` 或幂等性。
- 已有 summary 永远优先，MCP fallback 不覆盖 Skill/Wukong/显式 Hint。

本轮 419 条 MCP 匹配工具中，146 条已有高优先级 summary，272 条通过 fallback 补齐，1 条纯 RPC 名称被拒绝但已有 Skill summary。该过程使 `agent_summary` 覆盖达到 419/504，并使工具级 Agent 元数据覆盖达到 460/504。详细计数和拒绝列表保存在 `internal/cli/schema_agent_metadata_audit.json`。

## 5. 字段所有权和合并优先级

不同字段必须分别定义优先级，不能用一个 Hint 对象无条件覆盖所有事实。

### 5.1 命令和参数事实

优先级：

```text
运行时强类型注解 > 实际 Cobra command/flag > 内嵌 IR/MCP 接口事实 > 受控临时 override
```

规则：

- Skill 和 Agent Hint 不得修改 `cli_path`、flag 存在性和 hidden 状态。
- CLI `required` 以实际运行时校验和强类型注解为准，MCP required 只能作为无本地适配时的后备。
- CLI 类型与接口类型分开保存，避免将逗号分隔 flag 和 MCP array 混为一谈。
- 受控 override 必须包含原因、来源和对应测试，后续迁回命令注册处。

### 5.2 Agent 语义

优先级：

```text
显式 Hint JSON > 受约束 Skill 解析结果 > 命令动词推断 > 无值
```

规则：

- 高风险规则只允许显式来源覆盖低风险推断。
- `effect_source`、`agent_source_refs` 和字段级 provenance 必须保留。
- `reviewed: true` 表示人工确认过，即使 `avoid_when` 为空也不是漏填。
- 命令动词只能推断 `read/write`，不能自动推断高风险或用户确认。

## 6. 统一 Command Catalog

Command Catalog 在生成阶段是强类型中间表示，并以 `internal/cli/schema_catalog.json` 固化为发布产物。文件本身包含完整 leaf details，运行时仍按 root/product/group/tool 渐进返回，不会默认把完整文件送入 Agent 上下文。

概念结构如下：

```json
{
  "schema_version": 2,
  "build": {
    "surface_hash": "sha256:...",
    "interface_hash": "sha256:...",
    "agent_metadata_hash": "sha256:..."
  },
  "products": {},
  "commands": {
    "sheet.range_set_style": {
      "canonical_path": "sheet.range_set_style",
      "cli_path": "sheet range set-style",
      "aliases": [],
      "visibility": "public",
      "help": {},
      "interface": {},
      "contract": {},
      "agent": {},
      "provenance": {}
    }
  }
}
```

### 6.1 Visibility

每个命令明确标记：

| 值 | 含义 | 默认 Schema 行为 |
|---|---|---|
| `public` | 推荐 Agent 使用的 canonical 命令 | 展示 |
| `compat` | 旧路径或旧 flag，运行时继续兼容 | 合并到 `aliases`，不单独列工具 |
| `internal` | daemon、redirect、内部 server 或实现细节 | 隐藏 |

`doc-comment`、`hrmregister` 等 source product 应折叠到公开产品，不作为 Agent 顶层路由目标。兼容 flag 应继续注册并 hidden，Schema 只显示 canonical 参数。

### 6.2 Help、接口和 Agent 描述分离

建议避免继续复用一个 `description` 字段承载三类不同语义：

| 字段 | 来源 | 用途 |
|---|---|---|
| `description` | Cobra `Short/Long` 或 flag usage | 与 Help 对齐的人读事实 |
| `interface_description` | 内嵌 IR/MCP 元数据 | 解释后端 property/RPC |
| `agent_summary` | Skill/Hint | 帮 Agent 选择工具 |

现有 v1 平铺字段保留一个兼容周期；v2 增加明确字段并提供 `schema_version`。

## 7. Agent Hint 模型

### 7.1 工具级字段

| 字段 | 含义 |
|---|---|
| `agent_summary` | 一句话说明工具最终效果，不使用路径型占位文本 |
| `use_when` | 应使用该工具的用户意图和场景 |
| `avoid_when` | 容易误选但不应使用的场景，以及推荐替代工具 |
| `prerequisites` | 执行前必须具备或查询的对象 |
| `tips` | Agent 填参、分页、轮询、结果验证建议 |
| `effect` | `read`、`write`、`destructive` |
| `risk` | `low`、`medium`、`high` |
| `confirmation` | `not_required`、`user_required`、`runtime_managed` |
| `idempotency` | `idempotent`、`conditional`、`non_idempotent` |
| `workflow_refs` | 对应 Skill 渐进式参考文件或章节 |
| `examples` | 已通过 command surface 和 dry-run 校验的示例 |
| `reviewed` | 是否已完成人工语义审核 |

### 7.2 参数级字段

| 字段 | 含义 |
|---|---|
| `value_source` | `user`、`previous_output`、`lookup`、`literal` |
| `resolver` | 获取真实 ID/值的 canonical tool |
| `do_not_guess` | 禁止 Agent 猜测该值 |
| `value_hint` | 填写策略，不替代参数事实描述 |
| `example` | 单个合法代表值 |
| `sensitive` | 是否为 secret/token 等敏感值 |

### 7.3 示例 Hint

```json
{
  "version": 1,
  "product": "sheet",
  "tools": {
    "sheet.range_set_style": {
      "agent_summary": "设置指定单元格区域的样式，不修改单元格值",
      "use_when": [
        "只修改背景色、字体、对齐、换行或数字格式"
      ],
      "avoid_when": [
        "写值同时设置样式时使用 sheet.range_update",
        "按条件动态高亮时使用 sheet.create_cond_format"
      ],
      "effect": "write",
      "risk": "medium",
      "confirmation": "not_required",
      "idempotency": "idempotent",
      "workflow_refs": [
        "skills/mono/references/products/sheet/sheet-style-format.md"
      ],
      "parameters": {
        "sheet-id": {
          "value_source": "lookup",
          "resolver": "sheet.get_all_sheets",
          "do_not_guess": true
        },
        "bg-colors-json": {
          "value_hint": "二维数组行列数必须与 --range 完全一致"
        }
      },
      "reviewed": true
    }
  }
}
```

## 8. 参数约束模型

现有模型已支持：

- `required`
- `required_when` 文本
- `mutually_exclusive`
- `require_one_of`
- `require_together`
- `enum`
- `format`
- `example`
- positionals

下一步需要把条件必填从自由文本逐步升级为机器可执行规则：

```json
{
  "requires_if": [
    {
      "when": {
        "parameter": "recurrence-type",
        "operator": "in",
        "values": ["weekly", "relativeMonthly"]
      },
      "require": ["recurrence-days-of-week"]
    }
  ],
  "forbidden_if": [],
  "exactly_one": [
    ["group", "user", "open-dingtalk-id"]
  ]
}
```

兼容期继续输出 `required_when` 文本，但 smoke 和 Agent 可以优先读取结构化规则。

## 9. Skill 编译规则

### 9.1 递归加载

生成器必须递归读取：

```text
skills/mono/SKILL.md
skills/mono/references/intent-guide.md
skills/mono/references/products/**/*.md
skills/mono/schema-hints/*.json
```

渐进式子文档通过父目录确定 product，例如：

- `products/sheet/sheet-style-format.md` -> `sheet`
- `products/doc/doc-update.md` -> `doc`
- `products/aitable/aitable-record-query.md` -> `aitable`

### 9.2 受约束解析

Markdown 只解析稳定结构：

- `Reference 索引`
- `适用范围` / `使用场景`
- `意图判断`
- `前置条件`
- `关键说明` / `特性说明`
- `核心工作流`
- `Usage` / `Example` fenced blocks
- 危险操作表和跨产品消歧表

不使用任意自然语言推断参数 required/type。多行 shell 示例需要先规范化，再匹配 command surface。

### 9.3 引用解析

- 每条 Skill 命令引用必须解析到 `canonical_path`、公开 alias 或显式 allowlist。
- 父命令和帮助示例不能被误判为可执行 leaf。
- 旧路径应明确映射为 compat alias，而不是静默生成新工具。
- unmatched 不能只计数，必须输出具体源文件、行号、原始路径和候选路径。

## 10. Agent 统一调用协议

Agent 使用 DWS 时按以下顺序工作：

```text
用户意图
  -> Skill 做产品/工作流路由
  -> 获取 canonical_path
  -> dws schema <canonical_path> 读取精确参数和约束
  -> 按 resolver 查询真实 ID，禁止猜测
  -> --dry-run 验证
  -> 高风险操作获取用户确认
  -> 加 --yes 执行
  -> 根据 Skill workflow 回读/轮询/验收
```

查询策略：

1. 产品不确定时读取 `dws schema` 根概览。
2. 已由 Skill 确定产品和命令时，直接查询 leaf。
3. 只确定产品组时查询 `dws schema <product.group>`。
4. 普通 Agent 流程禁止读取 `schema --all`。
5. `--help` 用于人读展示、异常排查和 Schema 缺失时兜底，不作为 Agent 的主要结构化输入。

## 11. 构建期生成流程

```text
1. 构建实际 Cobra command tree
2. 导出并校验 schema_command_surface.json
3. 加载 schema_mcp_metadata.json
4. 按指定 revision 清洗 Wukong envelope，生成版本化 imported Hint
5. 递归解析 Skill Markdown
6. 加载 skills/mono/schema-hints/**/*.json
7. 按字段所有权和优先级合并
8. 校验 path、alias、visibility、参数和风险规则
9. 生成 internal/cli/schema_agent_metadata/*.json
10. 合并 Cobra 契约、接口事实和 Agent 语义，生成 schema_catalog.json
11. 校验 Catalog key 与 command surface 完全一致，计算 source/surface/interface hash
12. 将版本化元数据和 Catalog 嵌入发布二进制
```

发布二进制只读取内嵌 Catalog。Schema 查询不得发起网络请求，不读取用户本地 MCP cache，不受服务端当日变更影响。

## 12. 渐进式 Schema 输出

| 层级 | 输出内容 | 建议预算 |
|---|---|---:|
| Root | 公开产品、简短用途、tool count | <= 5 KB pretty JSON |
| Product | 分组和工具简短 summary，不带 examples/source refs | <= 16 KB |
| Group | 该组工具的 use/avoid/effect/risk | <= 12 KB |
| Tool | 完整参数、约束、Agent Hint、示例和 provenance | <= 8 KB |
| `--all` | 完整审计 Catalog | 仅 CI/离线工具 |

大型产品如 AITable、Chat 应优先依赖 Skill 直接路由或按 group 展开，避免一次加载全部工具。

## 13. 质量门禁

### 13.1 Command Surface Gate

- `canonical_path` 唯一。
- `cli_path` 必须真实可执行。
- compat alias 只能归属一个 canonical tool。
- internal/redirect 命令不得进入默认 Schema。
- Schema 参数必须存在于对应 command flag/positionals。

### 13.2 Contract Gate

- required、default、enum、format 与运行时一致。
- one-of、互斥、联动和条件必填引用的参数必须存在。
- hidden flag 不进入 Schema。
- 所有 one-of 分支分别构造命令并 dry-run。

### 13.3 Skill Gate

- 递归 Skill 命令引用全部可解析。
- unmatched 为 0；特殊情况使用带原因的 allowlist。
- 示例只能引用公开 canonical path 或 compat alias。
- `workflow_refs` 指向的文件和章节必须存在。

### 13.4 Agent Metadata Gate

- 所有公开工具有 `agent_summary`、`effect` 和 `reviewed`。
- 禁止 `calendar/create`、`get`、`list` 等路径型或单动词描述。
- 易混淆命令必须有 `avoid_when` 和替代工具。
- 复杂参数必须有合法 example 或 value strategy。
- 所有参数 resolver 必须指向存在的 read tool。

### 13.5 Safety Gate

- 所有 destructive 工具必须 `risk=high`、`confirmation=user_required`。
- 高风险 Skill 规则与命令 `--yes` 行为一致。
- Agent 不得自行生成确认结果。
- 每个工具必须显式 reviewed，不能把字段缺失解释为低风险。

### 13.6 Generated Drift Gate

- Skill/Hint/source hash 变化后必须重新生成 Agent metadata。
- command surface 变化后必须重新生成并审核 snapshot。
- CI 比较生成目录，禁止 stale generated JSON 合入。

### 13.7 Agent Golden Evaluation

从 Skill 意图和消歧表生成 JSONL：

```json
{
  "prompt": "通过 Webhook 发告警到群里",
  "expected": "chat.send_message_by_custom_robot",
  "forbidden": ["chat.send_robot_message"],
  "risk": "write"
}
```

评测指标：

- 产品路由 Top-1。
- 工具选择 Top-1/Top-3。
- forbidden tool 命中率。
- 参数填充成功率。
- dry-run 成功率。
- 高风险确认召回率。
- 单次选择所需上下文 p50/p95。

## 14. 分阶段实施计划

### P0：修复生成基础

范围：

- 递归加载 56 个产品子文档。
- 支持多行 shell 示例。
- 输出 unmatched 明细和候选路径。
- 增加 `public/compat/internal` visibility。
- 隐藏或折叠内部 source product。
- 生成覆盖率和低质量描述报告。

验收：生成行为确定、运行时命令行为不变、现有 smoke 继续全过。

### P1：引入 Catalog v2 和 Hint JSON

范围：

- 增加 `schema_version=2`。
- 引入 `skills/mono/schema-hints/*.json`。
- 增加 `agent_summary/prerequisites/tips/idempotency/reviewed`。
- 增加参数 `value_source/resolver/do_not_guess/value_hint`。
- 分离 Help、接口和 Agent 描述。
- 增加字段级 provenance。
- v1 平铺字段保持兼容。

验收：旧 Agent 仍可读取 v1 字段，新 Agent 可读取 v2 字段。

### P2：分产品补齐 Agent Hint

优先顺序：

1. 高风险写操作和 destructive 命令。
2. Calendar、Chat、Doc、Sheet、AITable。
3. 易混淆工具和跨产品 resolver。
4. 其余公开工具。
5. 迁移 `schema_hints_*.go` 中纯数据 Hint，Go 只保留命令注册和强类型注解。

每个产品独立提交，避免一次改动 504 个工具导致不可审查。

### P3：结构化条件和执行测试

范围：

- 增加 `requires_if/forbidden_if/exactly_one`。
- smoke 根据结构化条件生成分支组合。
- 参数 example 必须可构造合法 dry-run。
- 覆盖 alias、positionals、hidden flag 和 redirect。

### P4：Agent 评测和发布门禁

范围：

- 从 Skill 自动生成 golden cases。
- 增加静态路由评测和可选模型评测。
- 将覆盖率、安全规则和上下文预算加入 CI。
- 发布包验证内嵌 hash、Schema 无网络请求。

## 15. 最终验收标准

- 公开产品 Agent 路由元数据覆盖率 100%。
- 公开 canonical tool Agent 元数据覆盖率 100%。
- Skill unmatched 命令引用为 0，或全部在带原因 allowlist 中。
- 路径型低质量工具描述为 0。
- 公开工具 `effect` 和 `reviewed` 覆盖率 100%。
- destructive/high-risk 工具风险和用户确认覆盖率 100%。
- 易混淆命令组 `avoid_when` 覆盖率 100%。
- Schema 与 Help 参数集合差异为 0。
- one-of/条件约束分支 dry-run 通过率 100%。
- Agent golden case 工具选择 Top-1 >= 95%。
- 默认 Root <= 10 KB，Group <= 12 KB，Tool <= 8 KB。
- Schema 启动和查询网络请求数为 0。
- 生成物 drift 检查通过。

## 16. 推荐的第一张 PR

第一张 PR 只完成 P0，不引入新的外部 Schema 字段：

1. 将 `filepath.Glob(products/*.md)` 改为受控递归加载 Markdown。
2. 根据相对路径稳定识别 product。
3. 解析 Reference 索引和渐进式子文档。
4. 支持反斜杠续行命令示例。
5. 输出 unmatched 的源文件、行号、原路径和候选 canonical path。
6. 增加 visibility 审计和内部产品报告。
7. 重新生成 Agent metadata，并比较覆盖率变化。
8. 保持 `go test ./...`、`make test`、`make policy` 和全量 Schema smoke 通过。

该 PR 建立后，后续 Hint 补齐才有稳定、可量化、可审核的基础。
