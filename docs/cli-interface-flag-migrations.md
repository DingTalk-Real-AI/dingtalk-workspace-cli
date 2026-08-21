# CLI Help / Schema 兼容迁移治理

本文定义两种受控 flag 迁移：

1. `flag_rename`：保留旧 flag 的可执行兼容性，但把它从 Help 与 Agent Schema 中隐藏，并将新的规范 flag 设为唯一可见入口；rename 必须保持原 flag 的 requiredness，optional 只能迁到 optional，required 只能迁到 required。
2. `requiredness_change`：同一个公开 flag 从 optional 精确提升为 required；flag 的名称、类型、作用域、可见性、shorthand、`no_opt` 与 alias 关系必须保持不变。

两种原语都只放行清单精确登记的变化，不是通用 breaking-change 豁免，也不得在同一 command/flag 上叠加以绕过 rename 的 requiredness 保持规则。

同一套 base-owned lifecycle 也治理两类跨命令迁移：旧命令保留执行能力但从 Help / Schema 导航隐藏，并迁到新的公开命令路径；或把旧命令中的一个可选 flag 拆成新的专用命令。跨命令迁移只允许清单精确声明的 `command_became_hidden` / `flag_became_hidden` 及其 Schema 投影，不是通用 command-path breaking-change 豁免。

同名 flag 的精确类型迁移属于另一类评审机制，只能进入
`internal/interfacesnapshot/reviewed.go` 与 legacy smoke helper 的镜像表；flag rename
只能进入本文的 JSON lifecycle ledger。一项迁移不得跨两种机制组合授权。

## 唯一比较入口与信任边界

PR 与本地兼容性审批的唯一权威比较入口是 modern Interface Snapshot：

- `cmd/interface-snapshot` 生成和比较快照；
- `internal/interfacesnapshot` 实现兼容规则和迁移生命周期；
- `scripts/policy/check-command-compatibility.sh` 组装 candidate、PR merge-base 和最近可达且未撤回的 stable GA 三份快照；
- `scripts/policy/check-authoritative-interface-baselines.sh` 只保留为 Makefile 的兼容包装，不再维护第二套判断逻辑。

紧随其后的 Schema compatibility 不是第二份审批清单。它从同一 merge-base-owned
ledger 和同一组三方 Interface Snapshot 取得已经完成 lifecycle 校验的
authorization。merge-base-owned checker 会分别规范化 merge-base 与 stable 的完整
Schema，并让 candidate 对两份历史 contract 独立执行检查；授权的 flag rename 只会
精确投影到当前被检查的历史副本。candidate 不能为 CLI 与 Schema 分别提供两套例外。

`make interface-integrity` 也调用上述 authoritative wrapper；默认 base 为
`origin/main`，stable 可由包装脚本自动解析，candidate 默认为已提交的 `HEAD`。旧
`scripts/policy/check-interface-baseline.sh` 只供
`make update-interface-baseline` / `make reset-interface-baseline` 维护非权威 CLI
Smoke fixture，不参与迁移审批。

直接调用 `interface-snapshot compare` 时，只要提供 migration manifest 参数，就必须
同时提供 `--base` 与 `--stable`；核心 lifecycle 也拒绝缺失 stable 的非空清单，避免
调用方因漏传历史参考而提前清理 consumed receipt。

PR merge-base 同时拥有快照生成器、比较器和已审批清单。门禁用这套 base-owned helper 检查同一个已提交 candidate revision、merge-base 与 stable，candidate 不能通过修改自己的 Go 比较 helper 来放宽规则。candidate 中的清单只参与迁移状态流转，不能批准同一个 PR 引入的接口变化。首次引入 flag 机制时，merge-base 尚无迁移解析器；bootstrap 会用 merge-base 已有的 modern Interface Snapshot 做不带豁免的普通比较，并只接受 candidate 中逐字匹配的空 flag 清单。后续引入 command migration 扩展时，base 已拥有 flag comparator；bootstrap 仍只执行 base-owned 普通比较，不向旧 helper 传入新的 command ledger，因此允许随治理 PR 提交仍处于 before 的 pending 计划，也不会授予任何迁移豁免。bootstrap 无法让旧 helper 证明新治理实现本身正确，因此本治理 PR 的新 parser、lifecycle、launcher 与 hostile tests 仍是必须由真人评审的受保护策略变更；它们合入后才成为后续 PR 的 base-owned authority。

这条边界保护比较规则和审批数据，不是任意代码沙箱。GitHub workflow / launcher 的变更仍由仓库保护规则和真人评审负责；candidate Cobra 构建也会执行 candidate 代码，因此对同一 runner 上的主动恶意代码，需要独立进程或文件系统隔离，不能把本门禁描述成已经解决。

已审批清单固定为：

```text
scripts/policy/interface-migrations/approved-flag-migrations-v1.json
scripts/policy/interface-migrations/approved-command-migrations-v1.json
```

清单使用严格 JSON 解析：版本、字段名大小写、JSON 值类型、命令路径和 flag 名都必须精确；拒绝重复键、未知键、scalar `null` 与尾随 JSON 值，`reason` 不能为空；禁止 `*`、`?`、前缀规则或其他 wildcard。历史未声明 `kind` 的记录按 `flag_rename` 解释；新增同名 requiredness 迁移必须显式写 `kind: requiredness_change` 和单一 `flag` before/after。清单中的 `pending` 记录只记录已评审计划，并授权其精确列出的后续产品迁移；候选与 merge-base 仍必须精确匹配 `before`，不能授权同一个提交中的接口变化，也不能作为其他命令或参数的通配豁免。

首次引入一个旧 merge-base 不认识的新 `kind` 时，机制 PR 不得同时写入该 kind 的 pending 记录，因为旧的 base-owned 严格解析器会拒绝未知字段。必须先合入 parser、lifecycle、CLI/Schema adapter 与 hostile tests；待这些实现成为新的 merge-base authority 后，再用独立治理审批 PR 新增 pending，最后才由产品 PR 消费。

## 跨命令迁移原语

`approved-command-migrations-v1.json` 只接受两种 `kind`：

| kind | CLI after 状态 | Schema 允许的精确投影 |
|---|---|---|
| `command_move` | legacy 命令仍 runnable、由 visible 变 hidden；replacement 由 absent 变 visible runnable | 同一 stable tool identity 的 `primary_cli_path` 改到 replacement；只允许清单列出的参数改名，参数类型、property、requiredness、default 等必须等价 |
| `flag_extraction` | legacy 命令保持 visible runnable；指定 legacy flag 仍可执行但由 visible 变 hidden；replacement 由 absent 变 visible runnable | source tool 只删除指定参数；replacement tool 必须位于精确的新路径，并保持 source 的 interface 与 safety identity；清单必须完整列出每个 source 参数到 replacement 参数或常量 property 的承接关系 |

`command_move` 只能隐藏没有子命令的 legacy leaf，且 legacy 与 replacement
不得互为祖先路径；整棵命令树的迁移需要单独设计逐叶治理，不能复用这一原语。
稳定 Schema tool 可以继续接受普通的 optional 参数新增，但不得借路径迁移引入清单未登记的
`required`、`cli_required` 或 `required_when` 参数；参数改名的目标也不得与历史
Schema 中已有的其他参数重名，避免把两个历史参数静默合并。`flag_extraction` 只接受
optional bool legacy flag，不能隐藏仍由 Cobra hard-required 的参数。它必须对 source tool
的全部历史参数逐项声明：普通参数使用精确 `from` → `to`（同名也必须显式写出），且恰好
一个与 legacy flag 同名的 `from` 使用 `replacement_constant`，不得同时声明 `to`；所有
`from` 与 replacement 参数/property 目标必须唯一。legacy bool flag 的 `no_opt` 必须等于
常量布尔值的字符串形式。v1 只治理 optional bool flag 的 `NoOpt=true` 激活分支，因此
`replacement_constant.value` 与 legacy `no_opt` 都必须是 `true`；negative flag、默认即
`true` 或固定 `false` 的语义不在本轮证明范围，必须另行设计，不能借本清单放行。

如果 `command_move` 的参数 `from` 在更早 stable 中仍使用另一历史名称，Schema adapter
只能把同一 legacy command 上、已经由 base-owned lifecycle 返回且
`state=consumed` 的 flag rename 回执作为前驱边。例如
`group → conversation-id` 与 `conversation-id → open-topic-id` 可以组合，但不能把
candidate 自增的 pending 记录、其他命令的同名参数、参数概念词典或 CLI alias 当作证据。
首次消费 pending command 回执时，merge-base 的 normalized Schema 必须真实发布中间参数，
并逐跳验证参数签名和 constraints；command 回执合入为 consumed 后，中间 Schema 已从 main
消失，此时保留的两份 consumed 回执可继续对 stable 做受限重放，直到 stable 也达到 after
并让回执转为惰性记录或由独立 PR 清理。两种阶段都拒绝残留 predecessor/intermediate、字段漂移、环、分叉、
target 碰撞或 primary path/tool identity 不唯一；positionals 不在该组合授权面内。

`replacement_constant` 不是清单自报即可成立的例外。after 阶段的 Interface Snapshot
必须从 replacement 命令的同一份框架运行时声明中捕获完全一致的 property/value，缺失、
值不符或额外常量都会使 lifecycle 落入 partial。对于 #1054，`dws chat topic create`
必须通过 `NewLeafCommand` 的 `ConstParams` 声明并实际注入
`convThreadEnabled=true`；手写 `RunE` 固定值、Cobra annotation 或只改清单都不能提供这份
同源证据，Snapshot 只读取 `corecmd` 包内私有注册表公开的只读副本。第一次向旧快照增加
bool 常量证据属于 bootstrap；一旦任一历史快照已记录该
证据，普通 Interface Compare 会持续要求 property/value 集合完全一致，因此 ledger 清理后
删除、翻转或增加常量仍会阻塞。若 candidate 改动 command ledger，则
`internal/corecmd/corecmd.go`、`internal/corecmd/interface_const_params.go` 与
`internal/helpers/leaf.go` 三份执行/证据桥必须保持 base Git blob 不变；框架演进必须先用
独立 PR 合入，不能和产品消费混在一起。

replacement 必须保留 source 已发布的 dry-run 能力：历史 `dry_run` 非空时不得删除或改值；
历史未声明时允许 replacement 新增 dry-run。这与普通 Schema 兼容规则保持同一单调边界。

两种迁移都要求旧 argv 继续可执行。删除旧命令、删除旧 flag、把 legacy 改成 non-runnable、改变未登记的历史参数、改变 interface / safety，或只完成部分 before → after 转换都会 fail closed。命令别名会先规范到 reference 的 canonical path，但清单本身仍只能记录精确 canonical 命令，不能用 alias 或前缀扩大授权。

跨命令清单复用下文同一套 `pending → consumed → inert/cleanup` 生命周期。治理 PR 只能新增 `pending` 且产品 surface 必须仍是 before；后续产品 PR 才能一次性切到 after 并改为 `consumed`。candidate 新增的 pending 记录不能批准自己的改动。

当前首批 pending 记录覆盖 `chat topic` 收口：`chat group create --thread` 拆到 `chat topic create`，以及 `chat message list-topic-replies` / `forward-topic` 迁到对应的 `chat topic` 命令。前一条完整登记 `name` / `type` / `users` 的同名承接，以及 `thread` → `convThreadEnabled=true` 的常量承接。产品 PR 消费这些记录时只能把三条 `state` 改为 `consumed`，不得改写其 before、after、Schema mapping、constant 或 reason。

## 两阶段迁移与回执清理

rename 以 `(kind, command, legacy flag, canonical flag)` 为唯一精确键；requiredness change 以 `(kind, command, flag)` 为唯一精确键。二者经历同一生命周期：

| 阶段 | PR 可以做什么 | 必须满足的快照状态 |
|---|---|---|
| 1. 治理审批 | 新增 `state: pending` 的精确记录；不得在同一个 PR 修改产品 surface | candidate 和 merge-base 都与记录中的 `before` 完全一致；该记录不改变 stable 的判断 |
| 2. 产品迁移 | merge-base 已拥有 `pending` 后，按记录一次性切到精确 `after`，并把记录改为 `state: consumed` | rename 的 legacy 仍存在但由 visible 变 hidden，且声明 `alias_of`，canonical requiredness 保持不变；requiredness change 只把同名 flag 从 optional 提升为 required |
| 3. 保留回执 | 产品 PR 合入后，如果 stable 仍是 `before`，继续保留 `consumed` | merge-base 或 stable 仍有任一份尚未达到 `after` |
| 4. 惰性保留或清理 | 当 merge-base 和 stable 都已经是 `after`，该记录不再提供任何授权；后续 PR 可以原样保留或删除 | 两份参考快照均精确匹配 `after`；保留时仍必须是不可改写的 `consumed`，接口偏离 `after` 继续失败 |

因此，新增 `pending` 和修改产品 surface 不能发生在同一个 PR；candidate 自己新增的记录不能 self-approve。迁移也不能部分执行：legacy、canonical、`alias_of` 或状态只要有一项不匹配，门禁即失败。stable 发布只会让已经追平的 `consumed` 回执变成无授权效果的审计记录，不会在没有代码变更时让后续业务 PR 失去合规性；清理仍可作为独立的账本压缩动作，但不再是下一个 PR 的强制前置条件。

下面只是清单结构示例，不代表已审批命令；实际字段必须从 Interface Snapshot 核对：

```json
{
  "version": 1,
  "migrations": [
    {
      "command": "dws chat message recall",
      "legacy": {
        "name": "msg-id",
        "before": {
          "present": true,
          "type": "string",
          "required": true,
          "scope": "local"
        },
        "after": {
          "present": true,
          "type": "string",
          "hidden": true,
          "scope": "local",
          "alias_of": "message-id"
        }
      },
      "canonical": {
        "name": "message-id",
        "before": { "present": false },
        "after": {
          "present": true,
          "type": "string",
          "required": true,
          "scope": "local"
        }
      },
      "state": "pending",
      "reason": "保留旧 argv 兼容性，并将规范 flag 设为唯一可见入口"
    }
  ]
}
```

产品迁移 PR 必须保持同一条记录的命令、flag、before/after 和 reason 不变，只把 `pending` 改成 `consumed`。

同名 flag requiredness 迁移的清单结构如下；示例不代表已经审批：

```json
{
  "version": 1,
  "migrations": [
    {
      "kind": "requiredness_change",
      "command": "dws report entry submit",
      "flag": {
        "name": "to-user-ids",
        "before": {"present": true, "type": "string", "scope": "local"},
        "after": {"present": true, "type": "string", "required": true, "scope": "local"}
      },
      "state": "pending",
      "reason": "Reject report submissions that have no visible recipient."
    }
  ]
}
```

## `alias_of` 是框架来源的受评审关系证据

`alias_of` 不是 Schema 同义词、参数概念词典或任意文字声明。它只能由 `FlagSpec.Aliases` 写入，并与内部 origin `corecmd.flag_spec_aliases.v1` 成对出现；每次 Interface Integrity 都会在已提交的 detached candidate 上执行源码门禁，禁止其他生产文件写入或复刻这些 evidence token。Interface Snapshot 会验证：

- legacy 与 canonical 位于同一个可执行命令；
- canonical flag 确实存在；
- legacy 与 canonical 类型一致；
- legacy 不是指向自身，也不存在 alias chain；
- legacy 的 after 状态精确指向该记录中的 canonical flag。

通过命令框架声明 `FlagSpec.Aliases` 时，框架会自动注册隐藏的兼容 flag，并写入两项 relation annotation；仅手写 `alias_of`、伪造 origin、重复值或不精确值都会让快照生成失败。不要用 Schema overlay、迁移清单或手写 Cobra annotation 伪造关系。

这项关系证据只证明 legacy/canonical 经过受控框架路径建立关系，不证明最终 transport payload 等价，也不会替产品代码实现命令特有的值同步。当前框架还禁止把
`MarkRequired` 与 `FlagSpec.Aliases` 直接组合，因为 Cobra 的 hard-required
校验只识别 canonical spelling。若产品迁移同时需要 canonical 的 Cobra required
标记和 legacy spelling，产品 PR 必须提供明确的运行时方案，并通过 canonical / legacy
最终 payload 等价、同值输入一致、冲突输入在 transport 前失败、legacy 仍可调用但 Help 隐藏等测试；迁移清单和 relation evidence 都不能替代这些证明。

## 豁免边界

一条 base-owned、状态正确且前后快照精确匹配的记录，只会从普通兼容报告中移除以下三类预期 finding：

1. legacy flag 的 `flag_became_hidden`（visible → hidden）；
2. required legacy 被新增的 required canonical 替代时产生的 `required_flag_added`；如果 canonical 在 before 阶段只是 hidden 占位符，则允许它在转为公开拼写时继承 legacy 的 requiredness。已有的 visible canonical 不允许借 rename 改变 requiredness。
3. `requiredness_change` 中同名 flag 从 optional 提升为 required 时产生的 `flag_became_required`。

以下变化仍按普通兼容规则阻塞，不能被迁移记录掩盖：

- 删除 legacy、canonical、命令或其他 flag；
- flag 类型或迁移记录中的 scope、shorthand、`no_opt` 漂移；
- `alias_of` 缺失、指向变化或 alias chain；
- 命令路径及任何无关的阻塞性接口变化；
- requiredness change 同时发生的 rename、隐藏、类型、scope、shorthand、`no_opt` 或 alias 漂移；
- 不精确、部分完成、超出记录范围的 surface 变化。

## Schema 投影边界

Agent-visible command 会把 visible Cobra flag 投影为 Schema parameter，因此合法的
legacy hidden 迁移会同时表现为历史 parameter 消失，constraint member 也可能从
legacy 名改为 canonical 名。Schema adapter 只接受已经由三方 Interface Snapshot
判定为 authorized 的迁移，并按 tool 的精确 `primary_cli_path` 绑定：

- reference 仍处于 `before` 且该 flag 有 Schema surface 时，baseline legacy parameter
  必须存在，candidate legacy parameter 必须消失，candidate canonical parameter 必须存在；
  如果 baseline 只有 canonical、没有 legacy，则 adapter 不得借 CLI ledger 提升
  `required` / `cli_required` 或重写 constraint；
- rename 前后的 `type`、`property`、`interface_type`、default、format、enum 与
  `required_when` 必须完全一致；
- `required` / `cli_required` 必须在 rename 前后完全一致，升高或降低都失败；
- constraint 只允许在同一 tool 内按已枚举的 legacy → canonical map 做 member 替换、
  排序与去重；group kind、非迁移 member 或 group 增删仍然阻塞；
- 多个 legacy 指向同一 canonical 时，所有历史 parameter signature 必须一致，否则
  fail closed。

adapter 先构造经过上述验证的历史 contract 副本，再调用原 Schema checker；它不会按
错误字符串删除 finding。这样既能处理纯 rename，也能阻止“旧 required 参数改名后意外
变为 optional”或 property 漂移等伪兼容。`consumed` 回执在 merge-base Schema 已经处于
canonical-only `after` 状态时不需要再次投影；adapter 保持 baseline 不变，由原 checker
验证 candidate 是否仍与该 canonical contract 兼容。

`requiredness_change` 的 Schema adapter 只把历史同名 parameter 的 `required` 与
`cli_required` 提升到 candidate 的 `true` 值，并要求 candidate 两者都为 `true`。parameter
不存在、tool/path 不匹配时不制造 Schema surface；type、property、interface type、default、
format、enum、`required_when`、constraints、positionals 与 safety 等全部字段仍交给原 checker，
任何不相干漂移继续阻塞。

## 本地验证

先确保 merge-base 和 stable tag 已在本地，然后运行与 CI 相同的权威门禁：

```sh
make interface-integrity \
  BASE_REF=<merge-base> \
  STABLE_REF=<stable-tag> \
  CANDIDATE_REF=<candidate-sha>

make schema-compatibility \
  BASE_REF=<merge-base> \
  STABLE_REF=<stable-tag> \
  CANDIDATE_REF=<candidate-sha>
```

`STABLE_REF` 必须解析到从该 merge-base 可达的最高未撤回 stable GA tag；primary checker 会按 release contract 独立核对，不能用任意 after commit 或已撤回版本提前清理回执。包装脚本可以在省略时自动解析。`CANDIDATE_REF` 省略时固定为命令启动时的已提交 `HEAD`；评审和复现 CI 时应显式传入 candidate SHA，避免 surface 与清单来自不同 revision。
