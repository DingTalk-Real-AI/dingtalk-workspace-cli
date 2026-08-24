# DWS AI 表格 CLI 参数幻觉与参数契约分析

初次分析：2026-08-10
本次复核：2026-08-17
产品面基线：`origin/main@104eb715c4d5b110cb3442078356a2ec091f57bd`
最新兼容复核：`origin/main@a5b9e5a13f71`
分析对象：DWS `aitable` 的正式原生命令、仓库内置快捷命令、运行时 Schema、Cobra Help、产品 Skill、最新正式中央参数别名表。

## 1. 结论摘要

原报告需要更新。最新 `main` 通过 `819355b3 feat: add aitable workflow run and history commands` 新增了两个原生命令：

- `dws aitable workflow run`：立即执行工作流，是需要用户确认的写命令；
- `dws aitable workflow history`：按状态、Unix 毫秒时间范围和零基页码查询工作流执行历史。

除这两个新增命令外，机器对账确认旧基线的 209 个 AITable 工具在 `canonical_path`、`cli_path` 和完整参数集合上均未变化。因此，原分析的 11 类根因仍成立，但数量和候选别名表必须以最新正式表重建，不能继续使用 2026-08-10 的旧草稿直接覆盖。

更新后的产品面为：

| 指标 | 旧基线 | 最新 main | 变化 |
|---|---:|---:|---:|
| Agent 可见工具 | 209 | 211 | +2 |
| 原生 primary tools | 117 | 119 | +2 |
| 内置 `+` shortcut | 92 | 92 | 0 |
| 参数出现次数 | 735 | 746 | +11 |
| 不同公开参数名 | 114 | 118 | +4 |

新增的四个公开参数名是 `after-time`、`before-time`、`page` 和 `status`。两个新命令同时复用了 `base-id`、`workflow-id`、`table-id`、`record-ids`、`size` 等既有参数，因此带来以下新增风险：

1. `workflow run --record-id` 在没有保护时会被通用编辑距离纠错静默改成 `--record-ids`。这不是经过审核的同义别名，而且会掩盖“单值/列表”和“记录触发模式”的理解错误。
2. `workflow history` 使用零基 `--page` 加 `--size`，不是 cursor/offset 分页，也不是通常理解的一基 `page-number`。
3. `--after-time/--before-time` 要求 Unix 毫秒。名称可以安全兼容 `start-time/end-time`，但别名层不能把 ISO 时间或秒级时间转换为毫秒。
4. 两个命令的 CLI 名称是 `--workflow-id`，服务 payload 分别使用 `workflowId`/`flowId`。`--flow-id` 可以作为同值 alias，但通用 `--id`、`--task-id` 不能自动归一。

本轮仍归为 11 类问题，在工作簿中展开为 428 条命令级表现，覆盖 202 个不同命令；相对旧报告新增 7 条，全部来自 `workflow run/history`。

## 2. 分析口径与数据来源

本报告不使用历史 badcase、`dws-eval` 结果或旧工作簿作为产品结论来源。事实优先级是：

```text
同提交 Runtime/Cobra Help
  > 同提交 runtime-assembled Schema
  > 当前 AITable Skill
  > 最新正式 internal/cli/param_concepts.json
```

执行方式：

1. 固定最新 `origin/main@104eb715`；
2. 在独立 worktree 编译当前提交，避免使用终端中的旧 `./dws`；
3. 从同一命令树读取 Cobra Help 和运行时组装 Schema；
4. 将旧基线与最新基线按工具路径、参数名、类型和约束做机器 diff；
5. 读取当前 AITable Skill 和最新正式别名表；
6. 基于最新正式表生成完整候选，再生成代码并执行 CLI dry-run、block 和确认门禁测试。

当前 Schema 是声明驱动、运行时组装的结果，不依赖提交态 `schema_catalog.json`。本轮不会把 MCP 或 Skill 文本当成能够创建真实 CLI flag 的来源。

最终复核继续跟到 `origin/main@a5b9e5a1`。`104eb715..a5b9e5a1` 调整了 AITable record query 的有界分页、空页和精确 ID 校验实现，但没有新增或删除 `+record-query` 的公开 flag，也没有改变本报告涉及的 AITable 参数名称集合。最新 Skill 仍错误提到不存在的 `--all/--page-limit`，该实验结论单独记录在 `aitable_experiment_hallucination_analysis_20260817.md`。

## 3. 11 类参数问题

### 3.1 Base ID 名称跨命令不统一

最新版本中 190 个工具涉及单个 Base ID：186 个公开为 `--base-id`，4 个快捷命令公开为 `--base`。新增 `workflow run/history` 都使用 `--base-id`。

候选将 `base/base-id/base-token` 绑定到经过审核的 190 个精确命令。它们表示同一实体、同一角色、同一基数，值可原样传递；`source-base-id` 和 `target-base-id` 仍保持独立。

### 3.2 Table ID 缺少完整中央概念

最新版本有 108 个工具涉及单个 Table ID：106 个使用 `--table-id`，2 个快捷命令使用 `--table`。新增 `workflow run` 使用 `--table-id`，并要求它与 `--record-ids` 同时出现或同时不出现。

候选 `table_id` concept 只包含 `table/table-id`，继续排除：

- `table-ids`：多个数据表 ID；
- `source-table-id`、`target-table-id`：带业务角色；
- `name`：表名称，不是 ID。

### 3.3 搜索、时间边界与分页模型容易混用

旧问题包括 `query/keyword`、`limit/page-size/max-results` 和 cursor 同义拼写。新增 `workflow history` 又引入了独立分页模型：

```text
--after-time / --before-time  Unix 毫秒
--page                        从 0 开始
--size                        每页条数，1..100，默认 20
```

安全处理分为两类：

- 可归一：`start-time→after-time`、`end-time→before-time`、`page-index→page`、`page-size→size`，值保持不变；
- 必须拦截：`cursor`、`offset`、`page-token`、`page-number`、`page-no`、`current-page`。它们的分页模型或值基不同，不能只改参数名。

### 3.4 业务参数与全局参数同名

9 个命令存在局部业务 flag 与全局 `format/fields/timeout` 同名的情况。中央 PreParse 无法仅凭参数名判断用户想表达业务字段还是输出控制。

候选仍只在 `aitable +export-data` 上增加单向 `export-format→format`；其他情况需要 CLI 命名、Help 或 Skill 治理。

### 3.5 单值、多值和来源/目标角色不能自动合并

受影响命令由 20 个增至 21 个。新增 `workflow run` 接受 1–5 个 `record-ids`，并与 `table-id` 成对出现。

实测最新正式逻辑中：

```text
--record-id R
  ↓ 未命中 reviewed semantic alias/block
  ↓ 通用参数名模糊纠错
  ↓ 静默改成 --record-ids R
```

候选为该命令明确 block `record-id` 和通用 `id`，不把单值隐式包装成列表，也不跳过原命令的成对和数量校验。

### 3.6 高置信度同义参数

受影响命令由 19 个增至 21 个：

- 7 个纯文本 `desc/description`；
- 4 个上传命令的字节数 `size/file-size`；
- 10 个工作流命令的 `workflow-id/flow-id`，包含新增的 run/history。

`size` 同时是分页概念，文件大小不能并入全局 `pagination_size`，仍使用命令级 scoped alias。

### 3.7 结构化载荷不是参数改名

5 个 field/form/record 命令包含文件读取、JSON 包装或多个 flag 合并。中央别名只改名称且保留原值，不能承接这些转换。

### 3.8 同义布尔参数类型不一致

8 个原生/快捷命令对 `enabled/hidden/required` 使用 string 或 bool 两种类型。问题在值解析和是否允许裸 flag，不在参数名称。

### 3.9 Schema 必填约束和示例漂移

旧有 16 个命令问题没有被新增 workflow 提交修复：7 个合法替代参数组被错误固定为 required，9 个已发布示例缺少真实必填参数。必须修改 leaf Contract/ParamDecl，别名表不能降低 required 或制造缺失值。

### 3.10 Skill 直接示例缺少必填参数

旧有 27 个命令仍存在直接示例缺少 `base-id/table-id` 等必填值的问题。本次 main 只新增和完善 workflow 文档，没有修复这些旧示例。

### 3.11 真实 `--view-id` 被接受但不生效

`record query/list` 仍接受隐藏 `--view-id`，但执行实现忽略它。因为它已经是真实 flag，unknown alias guard 不会接管；需要命令校验显式报错或真正实现 view 过滤。

## 4. 最新候选别名表

候选文件从当时最新正式 `internal/cli/param_concepts.json` 重新构建，不是在旧草稿上继续追加。本轮补齐最终 payload 模板后，AITable 增量已经同步到当前工作分支的正式表；文档目录中的候选继续保留，供和最新 main 合并时审计。

对 `origin/main@a5b9e5a1` 的结构化差分确认：候选的 metadata、schema version、其他 38 个 concept、全部非 AITable override 和全部非 AITable fixture 与最新 main 完全相同；差异严格收敛为下述 4 个扩展 concept、3 个新增 concept、8 个 AITable override 和 28 个 AITable fixture。因此后续合并应以文档候选向最新 main 应用 AITable 增量，不能用当前旧分支的整份正式表覆盖 main。

| AITable 增量 | 数量 |
|---|---:|
| 新增 concept | 3 |
| 扩展既有 concept | 4 |
| 新增 AITable command override | 8 |
| 新增 AITable validation fixture | 28 |

候选相对最新正式表只调整了以下 7 个 concept：

- 扩展：`base_id`、`search_query`、`pagination_size`、`page_cursor`；
- 新增：`table_id`、`plain_description`、`workflow_id`。

只增加了 8 个 AITable override，其中新增命令的核心规则是：

```json
"aitable workflow run": {
  "block": ["record-id"],
  "note": "单条 record-id 不自动变成 1–5 项 record-ids 列表"
}

"aitable workflow history": {
  "scoped_aliases": {
    "start-time": "after-time",
    "end-time": "before-time",
    "page-index": "page"
  },
  "block": [
    "cursor", "offset", "page-token", "next-cursor", "next-token",
    "page-no", "page-number", "current-page"
  ]
}
```

生成后的 AITable 规则面为：202 个命令、608 对 alias、554 条 blocked 拼写和 3 条 ambiguous。独立审计确认：非 AITable concept、override、fixture 与最新正式表没有漂移；旧草稿中的过期结构没有被带回。

## 5. 新增命令的最终链路验证

### 5.1 `workflow run`

canonical 与 alias dry-run 最终得到相同 payload：

```text
--base-id B --workflow-id W --table-id T --record-ids R
--base-token B --flow-id W --table T --record-ids R
  ↓
baseId=B, workflowId=W, tableId=T, recordIds=[R]
```

另外验证：

- `--record-id` 在进入 Runner 前返回 `blocked_flag`；
- 非交互环境不带 `--dry-run/--yes` 时返回 `confirmation_required`，没有越过写命令确认门禁。

### 5.2 `workflow history`

canonical 与 alias dry-run 最终得到相同 payload：

```text
--after-time 1000 --before-time 2000 --page 2 --size 25
--start-time 1000 --end-time 2000 --page-index 2 --page-size 25
  ↓
afterTime=1000, beforeTime=2000, page=2, size=25
```

`--cursor` 和 `--page-number` 均在进入 Runner 前返回 `blocked_flag`。

## 6. 验证结果与落地边界

补齐模板并在当前 `fix/param-hallucination` 工作分支替换正式表后：

| 验证 | 结果 |
|---|---|
| JSON/候选结构审计 | 通过 |
| `go generate ./internal/cli` | 通过 |
| `internal/cli`、`internal/pipeline`、alias generator | 通过 |
| `check-generated-drift.sh` | 通过 |
| `check-schema-catalog.sh` | 通过 |
| 新增命令代表行为 | 通过 |
| 原缺失的 10 个 complete-command 模板 | 已补齐 |
| 实验 badcase 新增 complete-command 模板 | 1 个（`+record-share-links`） |
| 新增代表 alias 最终 payload 等价 | 20 个 |
| `internal/app` 全量 | 通过（218.083s） |

补齐的完整命令模板是：

```text
aitable +find-record
aitable +record-share-links
aitable field search-options
aitable +workflow-list
aitable base list
aitable base update
aitable attachment upload
aitable workflow get
aitable +export-data
aitable workflow run
aitable workflow history
```

测试对每条 active fixture 先构造业务必填参数完整的 canonical 命令，再只替换一个 flag 拼写；代表集合继续执行到最终 transport capture，并比较 canonical 与 alias 的工具名和 payload。`list_workflows` 额外补了最小合法列表响应桩，避免结果投影在 payload 比较前因假响应结构不完整而失败。

## 7. 当前能力明确不能自动完成的事项

- cursor/offset/一基页码自动转换为零基 page；
- ISO 时间或秒级时间自动转换为 Unix 毫秒；
- 单个 `record-id` 自动包装为 `record-ids` 列表；
- source/target 角色猜测；
- 结构化 JSON、文件输入和多参数合并；
- required/示例/Skill 缺失值修复；
- 已存在但运行时被忽略的真实 flag 治理。

这些不是“候选遗漏”，而是参数名归一能力的边界。强行写入 alias 会隐藏真实的基数、角色、值基或执行语义差异。

## 8. 交付物

- `aitable_cli_param_hallucination_analysis_20260810.xlsx`：更新后的五页汇报工作簿；
- `param_concepts.json`：基于正式表重建的完整 AITable 候选；
- `aitable_experiment_hallucination_analysis_20260817.md`：两组实验的参数和 Shortcut 命令名幻觉复核；
- 本报告：最新产品变化、问题根因、候选设计、验证结果和正式落地前置条件。
