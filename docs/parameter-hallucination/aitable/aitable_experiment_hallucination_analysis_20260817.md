# DWS AI 表格实验参数幻觉与 Shortcut 命令名幻觉复核

复核日期：2026-08-17
代码参考：`origin/main@a5b9e5a1`
实验来源：

- `/Users/hyz/works/data/aitable/raw-opencode`
- `/Users/hyz/works/data/aitable/eval_20260812_094400_aitable_v3_multi_aitable_run2_clean_c1/report_20260812_105618_rejudged.json`

## 1. 结论

两组实验暴露的是三类不同问题，不能全部写入 `param_concepts.json`：

1. `raw-opencode` 有 7 次不存在的 Shortcut 名称猜测，涉及 5 个错误名称；中央参数别名表只处理 flag 名称，不能修复命令路径。
2. `raw-opencode` 有一组可以安全由本轮 AITable 别名表处理的参数名不一致：例如 `+record-share-links --base-id/--table-id`。另有动态 `+resolve-context --base/--table`，但该命令不在当前仓库的可运行 Cobra 命令树中，正式生成器不允许为它生成中央 alias。
3. clean eval 没有发现新的 Shortcut 名称幻觉，但 5 个 case 反复使用 `+record-query --all`，其中 1 个 case 还使用 `--page-limit`。最新 CLI 没有这两个 flag，而最新 Skill 仍明确推荐它们，这是 Skill/CLI 契约漂移，不是中央别名表漏配。

本轮正式 AITable 参数表可以解决“同一业务对象、同一角色、同一基数、值无需转换”的名称差异；命令名猜测、缺少必填参数、结构化值格式错误和全量分页语义不应伪装成参数 alias。

## 2. 数据范围

### 2.1 raw-opencode

| 指标 | 数量 |
|---|---:|
| case | 61 |
| Bash 工具调用 | 331 |
| 以 `dws` 开头的调用 | 323 |
| 以 `dws aitable` 开头的调用 | 307 |

统计直接读取 61 个 `turn_001.jsonl` 中的 Bash tool use，不把 Skill 文本、Help 输出中的示例或命令输出里的字符串重复计为调用。

### 2.2 clean eval

| 指标 | 数量 |
|---|---:|
| case | 61 |
| 使用 Shortcut 的 case | 46 |
| 评测报告中的 `wrong_subcommand` | 10 |
| `wrong_skill` | 2 |
| `dependency_failed` | 12 |
| `param_mismatch` | 2 |

`param_mismatch` 不是参数幻觉的同义词。本报告回看了实际 `command_runs` 和输出；合法使用 `--filters` 代替评测器预期的 `--query`，或合法使用 `--query` 代替评测器预期的 `--record-ids/--field-ids`，不算未知参数。

## 3. raw-opencode：Shortcut 命令名幻觉

| 错误命令 | 次数 | case | 实际情况 | 处理方向 |
|---|---:|---|---|---|
| `aitable +field-create` | 2 | `0017`、`0053` | 不存在；正式入口是 `aitable field create` | 修 Skill/路由或增加经过审核的命令路径 fallback；不是参数 alias |
| `aitable +field` | 1 | `0046` | 不存在的 Shortcut Help 探测 | 不应兜底到某一个 field 子命令，保持拒绝 |
| `aitable +field-help` | 1 | `0013` | 不存在 | 不应创建“帮助类”业务 Shortcut |
| `aitable +table-create` | 1 | `0057` | 不存在；正式入口是 `aitable table create` | 可评估精确命令路径 fallback，不放入参数表 |
| `aitable +section-list` | 2 | `0005`、`0056` | 不存在；现有能力分为 `+section-list-empty` 和 `+section-list-nodes` | 语义不唯一，不能自动猜一个目标，应该修路由或要求消歧 |

合计 7 次、7 个 case。另有 `aitable dashboard list --base-id ...` 1 次：`dashboard` 下没有 `list` leaf，Cobra 落在父命令后把 `--base-id` 报为 unknown flag。这是原生命令路径幻觉，也不是参数名问题。

## 4. raw-opencode：参数与契约问题

### 4.1 当前正式候选可以安全兜底

| 原调用 | 正确参数 | case | 当前结论 |
|---|---|---|---|
| `+record-share-links --base-id ... --table-id ...` | `--base ... --table ...` | `0044` | 已由 `base_id`、`table_id` concept 覆盖；名称归一后仍需校验 `record-ids` 值格式 |

这条映射满足同一实体、同一角色、同一基数且值原样传递。它适合中央归一化；本轮已把 `base-id→base`、`table-id→table` 固化为两条 reviewed fixture，并通过 canonical/alias 最终 transport payload 等价测试。

### 4.2 已由 CLI 原生兼容解决，不再重复写 alias

| 原调用 | case | 当前 main 状态 |
|---|---|---|
| `table create --table-name ...` | `0057` | `table create` 已声明隐藏兼容 flag `--table-name`，执行时回退到 `--name` |

生成器会拒绝为真实存在的 native flag 再配置 semantic alias，这是正确约束。

### 4.3 参数名称看似可映射，但中央表当前无法落地

| 原调用 | 次数/case | 表面映射 | 不能落地的原因 |
|---|---|---|---|
| `+resolve-context --base ...` | 5：`0026`、`0028`、`0030`、`0040`、`0050` | `base→base-id` | `+resolve-context` 是实验环境提供的动态 Shortcut，不是当前仓库的 runnable Cobra leaf；生成器会拒绝悬空命令 |
| `+resolve-context --table ...` | 1：`0028` | `table→table-id` | 同上 |

如果该动态 Shortcut 要继续发布，应在它自己的声明中直接接受 native flag alias，或者先迁入 distribution-owned 命令树，再纳入中央 reviewed concept。不能在正式表里留一个当前发行版无法绑定的命令路径。

### 4.4 应保持拒绝，不应映射

| 错误调用 | case | 原因 |
|---|---|---|
| `+base-bootstrap --query ...` | `0009` | `query` 表示查找条件，而 `--name` 是新建 Base 名称；语义和副作用不同，不能做 `query→name` |
| `+field-update --type primaryDoc` | `0046` | 字段类型不可修改；把 `type` 映射到 `config` 或其他 flag 会隐藏不支持的业务动作 |
| `+resolve-context --include-dashboards` | `0005` | 命令没有该能力，不存在同义 canonical flag |

### 4.5 不是名称归一问题

| 类型 | 实际表现 | case | 为什么别名表不能解决 |
|---|---|---|---|
| 缺少必填目标 | `+resolve-context --include-fields` 未提供 table 目标 | `0030`、`0032`、`0040`、`0053`、`0056` | alias 不能凭空选择 table |
| 缺少必填目标 | `form list` 缺 `--table-id` | `0054` | alias 不生成业务 ID |
| 缺少必填参数 | `chart create` 缺 `--layout` | `0056` | alias 不能生成布局 JSON |
| 列表值格式错误 | `--record-ids '["R1","R2"]'`，命令要求 CSV/string-slice | `0044` | 当前能力只改参数名，不转换 JSON 数组为 CSV |
| 结构化值格式错误 | `field create --options "待联系,跟进中,已成交"`，实际要求 JSON 数组 | `0019` | 需要值解析/构造，不是名称同义词 |
| 结构化值格式错误 | `+record-query --filters '[...]'`，实际要求根对象 | `0032` | 需要修正 JSON 结构 |
| 枚举/能力错误 | `field create --type dateTime` 当前不支持 | `0020` | 不能靠改 flag 名解决值域错误 |

## 5. clean eval：真实参数幻觉

### 5.1 `+record-query --all`

在报告的 `command_runs` 中发现 12 个带 `--all` 的执行步骤，覆盖 5 个 case：

- `dws_aitable_0010`：8 次；
- `dws_aitable_0034`：1 次；
- `dws_aitable_0035`：1 次；
- `dws_aitable_0036`：1 次；
- `dws_aitable_0044`：1 次。

其中 11 次直接保留了 `unknown flag: --all` 输出；`0036` 的一次调用被 shell 管道/Python 后处理吞掉了 DWS 非零状态，最终表现成“0 条记录”，但根因仍是未知 flag。

### 5.2 `+record-query --page-limit`

`dws_aitable_0036` 有 4 个包含 `--page-limit` 的 command-run 步骤。Agent 在发现 `--all --page-limit 500` 失败后，又通过 shell/Python 循环继续用 `--page-limit 100` 重试，产生空结果和解析异常；读取 Help 后才改用真实的 `--limit/--cursor`。

### 5.3 根因是 Skill/CLI 漂移

最新 `origin/main@a5b9e5a1` 的 `+record-query` 公开参数只有：

```text
base-id, table-id, record-ids, field-ids,
filters, sort, query, limit, cursor
```

实现会在一次调用内按服务页聚合，最多返回 `--limit` 指定的 1–100 条；它没有 `--all` 或 `--page-limit`。但同一版本的 `skills/multi/dingtalk-aitable/SKILL.md` 仍写着：

```text
分页用 --cursor，全表用 --all --page-limit N
```

因此该问题会持续诱导模型生成无效参数。正确方案只能二选一：

1. 以当前 CLI 为准，立即把 Skill 改为 `--limit/--cursor`，并明确单次最多 100 条；需要更大范围时由调用方按 cursor 编排；
2. 如果产品确实承诺全量语义，在 `+record-query` leaf 上正式声明并实现 `--all/--page-limit`，补齐分页完整性、上限、Schema、Help 和测试。

不能在 `param_concepts.json` 中做 `all→某个 flag`，也不能把 `page-limit→limit`：前者没有 canonical 目标，后者会把“最大分页次数/全量预算”错误变成“最多返回记录数”，业务语义不同。

## 6. clean eval 中不应误判的两条

报告将 `0030`、`0031` 标为 `param_mismatch`，但实际命令使用的都是真实合法 flag：

- `0030` 用结构化 `--filters` 完成筛选，评测器期望 `--query`；
- `0031` 用 `--query` 搜索两个名称，评测器期望 `--record-ids/--field-ids`。

这两条是“调用形态没有精确匹配评测器模板”，不是 unknown flag，也不是参数名称幻觉。除非业务结果或完整性不满足任务要求，否则不应据此向别名表添加映射。

## 7. 对当前别名表草稿的影响

实验没有推翻本轮 AITable 草稿设计：

- `base_id`、`table_id` 可覆盖 `+record-share-links --base-id/--table-id`；
- `table create --table-name` 由 native compatibility flag 负责，不重复配置；
- 不把实验专属的动态 `+resolve-context` 路径写入正式表；
- 不为 `--all/--page-limit`、缺少必填参数、JSON 值转换或命令路径猜测制造虚假 alias。

需要单独进入后续治理的问题是：

1. 修复 AITable Skill 中 `+record-query --all --page-limit` 的错误指引；
2. 评估精确且无歧义的命令路径 fallback，例如 `+field-create→field create`、`+table-create→table create`；
3. `+section-list` 保持不自动修正，因为 `list-empty` 与 `list-nodes` 语义不同；
4. 若继续发布动态 `+resolve-context`，由该 Shortcut 自身声明 `base/table` 兼容，或先迁入正式命令树。
