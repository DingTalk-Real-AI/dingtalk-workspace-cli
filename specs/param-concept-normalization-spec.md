# Spec：参数幻觉治理 —— 受审命令范围内的概念归一化

- 状态：Implemented，待最终回归
- 分支：`fix/param-hallucination`
- 评测来源：`param_alias_map_merged_20260720`（440 条 badcase）及参数幻觉分析报告

## 1. 目标与边界

本次工作的目标是提高已知参数幻觉的执行成功率，并重点避免“参数被错误归一后成功执行”。主要方案是：

1. 以 `internal/cli/param_concepts.json` 作为 reviewed 参数概念与逐命令决策源。
2. 只在概念明确列出的真实 Cobra 命令上做 build-time 归约，生成逐命令别名/保护表。
3. 在现有 PreParse 流水线中把安全别名改写为 canonical flag。
4. 对语义不完整、存在歧义或仍待确认的名字 fail closed，不进入自动执行。

本次不以“内部路径与历史实现完全一致”为目标。有明确收益且通过语义审查、最终 payload 验证的行为变化可以接受；但必须满足本文的安全边界。

本次不做：

- 不解决命令或接口本身缺少能力的红类问题。
- 不改 Schema/Catalog wire contract。
- 不把所有产品的 hidden flag、fallback、helper Go 文件清理作为交付范围。
- 不将 Calendar 试点扩展为全 Calendar 或全产品治理。
- 不做值转换、单位换算或需要补充第二个参数的复杂推理。

## 2. 最终链路

```text
用户/Agent argv
  │
  ├─ Cobra Traverse：定位真实叶子命令并读取真实 flags
  │
  └─ PreParse（同一个 Context）
       1. AliasHandler
          处理 camelCase/snake_case/kebab-case 等形态差异
       2. SemanticAliasHandler
          按“精确命令路径”读取 generatedParamAliases
          ├─ alias      → 改写为 canonical flag
          ├─ blocked    → 保持原样并写入 ProtectedFlags
          └─ ambiguous  → 保持原样并写入 ProtectedFlags
       3. StickyHandler
          拆分安全的粘连参数；不得处理 ProtectedFlags
       4. ParamNameHandler
          处理普通近似拼写；不得处理 ProtectedFlags
       5. 检测 alias/canonical 冲突
          同一 canonical 出现两个不同拼写时直接报错
  │
  ├─ Cobra ParseFlags：只解析归一化后的 canonical/native flags
  ├─ RunE：现有 helper 读取 canonical/native 值并校验
  └─ MCP/HTTP payload：只包含命令原本定义的最终接口字段
```

归一化参数名只影响 Cobra 解析前的 argv。它不会向 RunE 或 MCP payload 注入新的业务字段；RunE 仍按原有 canonical flag 构造请求。对已保留的真实 hidden/native flag，归一化体系不抢占其原生路径。

## 3. Reviewed 源与生成物

### 3.1 概念

每个概念包含：

```go
type Concept struct {
    ID            string
    Denotes       string
    CanonicalHint string
    Members       []string
    Excludes      []string
    Commands      []string // 精确、受审、可运行的 Cobra 叶子路径
    Risk          string
}
```

`commands` 是强制字段。概念只允许在这些精确路径上参与归约；新命令不会因为碰巧出现同名 flag 就自动获得所有概念别名。这一约束用于阻断全局词汇碰撞，例如：

- `from` 在时间查询中可能表示开始时间，在邮件中可能表示发件人。
- `name` 可能是名称，也可能被某些接口错误地当作 ID。
- `time` 不足以说明 RFC3339、毫秒时间戳或日期边界。

概念成员仍需全局唯一；同一概念内 `members` 与 `excludes` 不得重叠。

### 3.2 逐命令覆盖

`command_overrides` 只表达当前命令的特殊决策：

- `bind`：把真实 generic flag 绑定到一个概念。
- `scoped_aliases`：明确的命令局部 alias → real flag。
- `block`：语义不完整或错误映射，禁止自动处理。
- `ambiguous`：存在多个合理含义，禁止自动选择。
- `confirm` / `investigate`：待审状态；不得生成可执行 alias。

当前 reviewed 源不允许残留 `confirm=true` 或 `investigate=true`。未来若再次引入，归约器也必须将相关名字置为保护态而不是激活。

### 3.3 生成物

`internal/generator/cmd_param_aliases` 使用真实 `app.NewRootCommand()`：

1. 校验每个 `commands` 路径存在且是可运行叶子。
2. 校验概念在该命令上至少命中一个真实 member 或 reviewed bind。
3. 用共享的 `cmdutil.Morph` 归一名称。
4. 一个概念只命中一个可见真实 flag 时，生成安全 alias。
5. 命中多个可见真实 flag 时，必须有精确 `ambiguous` 决策，否则生成失败。
6. `scoped_aliases`/`bind` 目标必须是真实 flag。
7. alias、blocked、ambiguous 不得互相重叠，也不得覆盖真实 canonical/native flag。

输出 `internal/cli/param_aliases_generated.go` 是确定性生成物，不得手改。

## 4. 映射准入规则

名称相似不是准入依据。自动映射必须同时满足：

1. 同一实体：源与目标表达同一个业务对象。
2. 同一卡数：单值不能静默转成列表，列表不能静默缩成单值。
3. 同一值域/格式：日期、RFC3339、Unix 毫秒等不能只换名字就互换。
4. 同一单位：秒、毫秒、数量、页号等不能做 name-only 映射。
5. 单参数可完成：若正确语义需要补充另一个参数，则必须 block。
6. 目标是该命令真实 flag，且最终 payload 字段已核对。
7. 写命令需要逐命令说明，并以 mock caller 断言最终 payload。

### 4.1 本次重点审计结论

| 风险 | 决策 | 例子 |
|---|---|---|
| 单数/复数 | 拆成不同概念或 block | `user_id` 与 `user_ids`、`dept_id` 与 `dept_ids` |
| ID/名称 | 只有源代码/接口证明同实体才开放 | `contact +resolve-dept --query → --name` 可开放；`list-sub-depts --name → --dept` 被 block |
| 时间格式 | 只保留值格式不变的命令局部映射 | Calendar `--date → --start` 由相同 ISO 解析与 payload 验证；`list-by-sender --time` 被 block |
| 分页基数 | 页号、页索引、条数、游标分开 | `page-index` 不再等同 `page-number`；`count` 不并入 `limit` 概念 |
| 值单位 | 不做隐式换算 | name-only 归一化不得改变秒/毫秒或日期/时间戳 |
| 多参数转换 | block | `before-block-id` 实际需要 `--ref-block` 和 `--where before` |
| 机器人身份 | 不把 code 与 ID 混用 | `robot-id` 不再归入 `robot_code` |

已存在的真实 hidden/native compatibility flag 不由本体系重新解释。它仍由现有 Cobra/RunE 路径负责，避免中央规则改变原命令已有语义。

## 5. 保护与冲突规则

### 5.1 blocked / ambiguous

SemanticAlias 命中保护项时：

- 不改写 argv。
- 把 morphed flag 写入 `Context.ProtectedFlags`。
- Sticky 与 ParamName 必须跳过该名字。
- Cobra 最终返回 unknown flag。
- 最终错误使用 generated table 给出结构化 `blocked_flag` / `ambiguous_flag` 原因并引导该命令的 `--help`；不得再给最近邻猜测。

裸 `--` 是流水线终止符。它之后的内容视为 positional data，任何 handler 都不得继续当作 flag 处理。

### 5.2 alias 与 canonical 同时出现

若 argv 同时包含指向同一 canonical 的两个不同拼写，例如：

```text
--date A --start B
--start B --date A
```

无论顺序、无论值是否相同，一律在 PreParse 返回 `FlagConflictError`，RunE 不执行。这样不会由“最后一个参数获胜”静默决定结果，也避免将来不同 pflag 类型产生不一致行为。

同一个拼写重复出现仍保持 Cobra 原有语义，本次不额外改变。

## 6. Calendar 试点结论

保留 `calendar event list` 试点，不回退，也不扩面。

该试点已经移除一组手写 hidden spelling flag，改为在 PreParse 统一归一到：

- `start`
- `end`
- `calendar-id`
- `cursor`
- `limit`

保留 `count` 原生兼容 flag，因为它不被概念层认定为通用 `limit` 同义词。端到端测试验证 Calendar 幻觉参数最终只形成正确的 `startTime`、`endTime`、`calendarId`、`cursor`、`limit` payload。

除这一已提交试点外，本次不继续修改 `internal/helpers/calendar.go`，也不要求其他 skill/helper 迁移 hidden flag 与 fallback。

## 7. 测试与门禁

| 层级 | 验证 |
|---|---|
| reviewed 源 | closed schema、命令范围非空、成员/排除约束、无待审项 |
| 归约器 | 精确命令准入、单/多命中、真实目标、真实 flag 不被覆盖、分类互斥 |
| handler 链 | protected 状态贯穿 Semantic/Sticky/ParamName、裸 `--`、冲突双顺序 |
| fixture | 每条 badcase 走真实 `RunPreParseArgs` 与真实 Cobra `ParseFlags` |
| 最终执行 | 代表性读命令 Calendar 与写命令 chat send 走真实 RunE + mock caller，并断言最终 payload |
| 最终错误 | blocked/ambiguous 使用 reviewed 错误路由，不回落到 generic fuzzy suggestion |
| policy | dictionary contract、全量 Cobra co-occurrence、fixture、generated drift |
| 回归 | CLI 契约、Skill static、Skill `--mock` E2E |

fixture 只有“被测参数名”而没有每个业务命令的完整必填业务数据，因此全量 fixture 的执行边界是 Cobra 解析；真实 RunE/payload 由有完整 argv 的代表性读写测试承担。二者组合覆盖从输入到最终请求的完整链路，同时避免 fixture 触发真实外部写操作。

## 8. 对原有链路的可能影响

允许且有收益的影响：

- 受审命令开始接受明确安全的语义别名。
- Calendar 试点由 hidden flag/fallback 迁移到 PreParse canonical 读取。
- alias/canonical 混传从顺序覆盖变为确定性报错。
- 某些曾被普通 fuzzy handler 猜中的危险名字现在明确失败。

需要持续约束的影响：

- 不能因为新增命令或真实 flag 而自动扩大概念覆盖；必须显式加入 `commands`。
- 不能让中央 alias 覆盖现存真实 flag。
- 不能降低 Cobra required 约束或绕过 RunE 校验/确认逻辑。
- 写命令的新映射必须审查最终接口字段，不能只断言 argv 被改写。
- 生成表、reviewed 源与运行时必须共用 `cmdutil.Morph`，避免 build/runtime 漂移。

## 9. 回退

本次修改只提交在 `fix/param-hallucination` 本地分支，不推送、不合并、不修改 main。

- 本地提交后发现问题：对本次 commit 执行 `git revert <sha>`，保留完整可审计历史。
- 如果日后已经合入 main：仍可在 main 上 `git revert <merge-or-commit-sha>`，再走正常审核与发布流程。
- reviewed 源与 `param_aliases_generated.go` 必须一起回退，或回退源后重新生成并通过 drift gate。
- Calendar 试点是更早的独立提交；若只回退本次安全加固，不会自动撤回 Calendar 试点。若需撤回试点，应单独 revert 对应提交。

## 10. 验收标准

1. 已知 fixture 全部通过。
2. 未确认、语义不完整或需要值/多参数转换的映射不会自动执行。
3. blocked/ambiguous 不会被后续 fuzzy handler 改写。
4. alias/canonical 冲突确定性失败且不进入 RunE。
5. 代表性读、写命令最终 payload 正确。
6. 参数概念、co-occurrence、生成漂移、CLI 契约及 Skill 回归门禁通过。
7. 改动集中在中央归一化体系，不开展全产品 helper 清理。
