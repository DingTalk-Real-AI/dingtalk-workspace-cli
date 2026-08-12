# CLI flag 兼容迁移治理

本文定义一种受控迁移：保留旧 flag 的可执行兼容性，但把它从 Help 与 Agent Schema 中隐藏，并将新的规范 flag 提升为必填。它只解决这一种精确变更，不是通用 breaking-change 豁免。

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

PR merge-base 同时拥有快照生成器、比较器和已审批清单。门禁用这套 base-owned helper 检查同一个已提交 candidate revision、merge-base 与 stable，candidate 不能通过修改自己的 Go 比较 helper 来放宽规则。candidate 中的清单只参与迁移状态流转，不能批准同一个 PR 引入的接口变化。首次引入本机制时，merge-base 尚无迁移解析器；bootstrap 会用 merge-base 已有的 modern Interface Snapshot 做不带豁免的普通比较，并只接受 candidate 中逐字匹配的空清单，不会让 candidate 新增的 comparator 决定本 PR 是否兼容。bootstrap 无法让旧 helper 证明新治理实现本身正确，因此本治理 PR 的新 parser、lifecycle、launcher 与 hostile tests 仍是必须由真人评审的受保护策略变更；它们合入后才成为后续 PR 的 base-owned authority。

这条边界保护比较规则和审批数据，不是任意代码沙箱。GitHub workflow / launcher 的变更仍由仓库保护规则和真人评审负责；candidate Cobra 构建也会执行 candidate 代码，因此对同一 runner 上的主动恶意代码，需要独立进程或文件系统隔离，不能把本门禁描述成已经解决。

已审批清单固定为：

```text
scripts/policy/interface-migrations/approved-flag-migrations-v1.json
```

清单使用严格 JSON 解析：版本、字段名大小写、JSON 值类型、命令路径和 flag 名都必须精确；拒绝重复键、未知键、scalar `null` 与尾随 JSON 值，`reason` 不能为空；禁止 `*`、`?`、前缀规则或其他 wildcard。当前清单为空，因此本治理 PR **不授权 PR #904 或任何产品接口变化**。

## 两阶段迁移与回执清理

每条迁移以 `(command, legacy flag, canonical flag)` 为唯一精确键，并经历以下生命周期：

| 阶段 | PR 可以做什么 | 必须满足的快照状态 |
|---|---|---|
| 1. 治理审批 | 新增 `state: pending` 的精确记录；不得在同一个 PR 修改产品 surface | candidate 和 merge-base 都与记录中的 `before` 完全一致；该记录不改变 stable 的判断 |
| 2. 产品迁移 | merge-base 已拥有 `pending` 后，按记录一次性切到精确 `after`，并把记录改为 `state: consumed` | legacy 仍存在但由 visible 变 hidden，且声明 `alias_of`；canonical 达到记录的必填状态 |
| 3. 保留回执 | 产品 PR 合入后，如果 stable 仍是 `before`，继续保留 `consumed` | merge-base 或 stable 仍有任一份尚未达到 `after` |
| 4. 单独清理 | 当 merge-base 和 stable 都已经是 `after`，在后续 PR 删除该记录 | 两份参考快照均精确匹配 `after`；继续保留过期回执会被门禁拒绝 |

因此，新增 `pending` 和修改产品 surface 不能发生在同一个 PR；candidate 自己新增的记录不能 self-approve。迁移也不能部分执行：legacy、canonical、`alias_of` 或状态只要有一项不匹配，门禁即失败。

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

一条 base-owned、状态正确且前后快照精确匹配的记录，只会从普通兼容报告中移除以下两类预期 finding：

1. legacy flag 的 `flag_became_hidden`（visible → hidden）；
2. canonical flag 的 `required_flag_added`（新增时即必填）或 `flag_became_required`（已有 flag 从可选变必填）。

以下变化仍按普通兼容规则阻塞，不能被迁移记录掩盖：

- 删除 legacy、canonical、命令或其他 flag；
- flag 类型或迁移记录中的 scope、shorthand、`no_opt` 漂移；
- `alias_of` 缺失、指向变化或 alias chain；
- 命令路径及任何无关的阻塞性接口变化；
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
- `required` / `cli_required` 只能保持不变或按审批从 `false` 提升为 `true`，禁止降低；
- constraint 只允许在同一 tool 内按已枚举的 legacy → canonical map 做 member 替换、
  排序与去重；group kind、非迁移 member 或 group 增删仍然阻塞；
- 多个 legacy 指向同一 canonical 时，所有历史 parameter signature 必须一致，否则
  fail closed。

adapter 先构造经过上述验证的历史 contract 副本，再调用原 Schema checker；它不会按
错误字符串删除 finding。这样既能处理纯 rename，也能阻止“旧 required 参数改名后意外
变为 optional”或 property 漂移等伪兼容。`consumed` 回执在 merge-base Schema 已经处于
canonical-only `after` 状态时不需要再次投影；adapter 保持 baseline 不变，由原 checker
验证 candidate 是否仍与该 canonical contract 兼容。

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
