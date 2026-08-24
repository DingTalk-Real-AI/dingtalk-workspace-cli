# Audit 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交重新构建的
`dws`、运行时 Schema、Cobra Help、audit 实现与冻结正式
`internal/cli/param_concepts.json`。Audit 是本地 CLI 运维产品，现有 DingTalk Skills 没有
对应产品命令说明；该事实被记录为公开契约缺口，没有用其他业务 Skill 猜测参数。未使用固定
Catalog、历史 badcase、用户 Shortcut 或已安装插件，也没有修改当前工作区正式别名表。

Audit 有 3 个 Agent 可见叶：`audit tail`、`audit export`、`audit verify`。主要风险是把
tail 的最近记录数误写成分页参数，把 export 的日期边界误写成时间戳，把三个命令的输入文件、
输出文件与审计目录混成一个 `--file/--path`，以及忽略 `export --format` 与其他 audit 命令
全局 `--format` 的同名异义。冻结 Help 隐藏了真实全局 `--output`，但生成器能看到该 Cobra
flag；独立审核据此修正了初稿，最终候选精确扩围既有 `local_output_path`，没有错误拦截真实
输出能力。

最终候选已通过生成器、PreParse、4 组 alias/canonical 输出逐字节比较、3 组输出文件实际
落盘、12 组代表 block/ambiguous、原生参数、非目标结构恒等、`internal/cli`、
`internal/pipeline`、generated drift 与 Schema Catalog 政策。完整 `internal/app` 仅
complete-command 覆盖门禁未通过：缺 3 个 audit 模板，16 条 active fixture 尚未进入最终
payload 等价验证。正式状态为“规则与链路已验证，补 3 个模板后方可落地”。

## 参数问题

### 1. 最近记录数不是页大小、游标或文件大小

`audit tail --lines` 表示从最新审计文件尾部返回最近 N 条记录，要求正整数；它不是 page、
cursor、offset，也不是文件字节数。Agent 很容易沿用列表命令经验生成 `--limit`、`--count`、
`--page-size`。

候选只在 `audit tail` 上把 `limit/count/max-results/tail-lines` 原值映射到 `lines`；
`page/page-size/cursor/offset` block。它不把单值改成范围，不改单位，也不替命令修正零或负数。

### 2. export 的日期边界必须保留上下界角色和 YYYY-MM-DD 值域

`--since` 与 `--until` 是包含端点的文件日期边界，实现只移除连字符后比较
`audit-YYYYMMDD.jsonl` 文件名。通用 `start/end/from/to` 可能携带时间戳，裸 `--date` 又无法
判断是下界还是上界。

候选只允许角色与格式都明确的 `start-date/from-date → since`、
`end-date/to-date → until`，值完全不变；时间戳式名称、裸 date 和 time-min/time-max block。
现有别名表不能验证日期值，运行时也缺少严格 YYYY-MM-DD 校验，这应通过命令契约和实现修复。

### 3. 同名 `format` 在 audit 内有两种语义

`audit export --format` 是导出内容格式，只接受 `jsonl|csv`；`tail/verify` 的全局
`--format` 是 CLI 展示格式。把 output-format 全局映射到 export 的本地 format，可能把
`json/table` 误送给只接受 `jsonl/csv` 的参数。

候选仅在 export 上允许角色明确的 `export-format → format`，并 block `output-format`；
tail/verify 上 `export-format` block。真实 `--format` 始终保持原生，不能建立跨命令 concept。

### 4. 输入审计文件、输出文件和审计目录是三个不同角色

`verify --file` 是一个输入 JSONL 文件；tail 总是读取最新文件，export 根据日期范围选择文件；
三者都继承隐藏全局 `--output`，可把命令结果写入本地文件。审计源目录没有 CLI flag，只能由
`DWS_AUDIT_DIR` 环境配置。

候选在 verify 精确映射 `audit-file/audit-log-file/input-file/file-path → file`，在三个命令
精确扩围既有 `local_output_path`，使 `output-path/destination-path/save-path → output`。
`audit-dir/dir/directory` 与错误命令上的 input-file 角色 block；verify 的泛化
`path/log/audit-log/target` ambiguous。别名层不读取文件、不从目录选文件，也不创建审计链。

### 5. 全局真实 flag 不能被别名表伪装成产品参数

Help 展示 `--dry-run/--yes/--profile/--jq/--fields` 等全局 flag，另有隐藏真实
`--output`。Audit 三个叶都是本地只读，但这些 flags 仍属于真实 Cobra 面。生成器会拒绝对
真实 flag 配置 block，这次初稿对 `output` 的冲突正是在隔离生成阶段被捕获。

最终候选保持全部真实 flag 原生，只对非真实拼写做归一或保护。是否应在 audit Help/Schema
更清楚地区分本地参数、全局展示参数和隐藏输出能力，属于契约治理，不属于中央别名表。

### 6. Help、Schema 与 Skill 的公开契约仍有缺口

Help 与运行时 Schema 对 3 个叶和 5 个局部参数一致，但 Schema 参数 property 全由 flag 名
推断，export 日期没有严格格式约束；仓库安装的产品 Skills 中没有 audit 命令文档。同时
`--output` 是真实且有用的全局能力，却因隐藏而不在普通 Help 中展示。

候选不能补参数 description、日期校验、Skill 路由或 Schema 声明。第一轮别名落地可独立
进行，但应另行补 audit 本地产品说明、显式 ParamDecl/约束以及日期错误提示。

## 当前别名表可以实施的方案

1. 将 `audit tail/export/verify` 精确追加到既有 `local_output_path` 命令范围。
2. 为三个叶各建一个 `scope_strict` command override，不新增跨产品 concept。
3. 对行数、显式日期边界、导出格式和 verify 输入文件做原值映射。
4. 对分页、日期/时间戳混用、目录、错误输入角色和格式角色做 block/ambiguous。
5. 保持所有真实全局 flags 原生；补齐 3 个 complete-command payload 模板后再正式替换。

## 当前能力支持不了的事项

- 校验或修正 YYYY-MM-DD，或把时间戳截断成日期；
- 自动交换 since/until、判断边界先后或补默认日期；
- 把 output-format 的值从 json/table 转成 jsonl/csv；
- 从目录中选择最新/指定审计文件，或把多个文件合并成 verify 输入；
- 创建、修复或重新签名审计哈希链；
- 把 `--file` 同时解释成 verify 输入与输出目标；
- 通过参数表新增 `--audit-dir` 或把环境变量变成 CLI flag；
- 修改 Help/Schema description、日期约束或补建 Audit Skill；
- 自动补 `--yes`、`--dry-run` 或替用户选择输出格式；
- 在没有 complete-command 模板时直接替换正式表。

## 第一轮改造建议

第一轮建议落地 1 个既有 concept 的 3 命令精确扩围和 3 个作用域 override。落地 PR 必须为
`audit tail/export/verify` 补 complete-command E2E 模板，覆盖 16 条 active fixture，并用
隔离审计目录验证输入文件与全局输出文件两条角色链。日期格式校验、ParamDecl 与 Audit Skill
作为并行契约修复，不应塞进 alias 表。

## 候选 `param_concepts.json` 改动与审核

- 未新增 concept；既有 `local_output_path` 只追加 3 个精确 audit 命令；
- 新增 3 个 command override；新增 28 个 fixture，其中 16 个 active、12 个 guard；
- `go generate ./internal/cli` 从 569 个命令作用域变为 572 个；
- 生成结果新增 22 个 alias、41 个 blocked、4 个 ambiguous token；fallback 无变化；
- 非目标 concept、override、fixture 结构恒等；guard 与真实 flag 冲突为 0；
- 初稿把真实 `--output` 当成不可用目标，生成器拒绝后已审核修正为输出路径 concept；
- 4 组 alias/canonical 退出码、stdout、stderr 逐字节相同；3 组输出别名实际落盘成功；
- 12 组保护检查稳定返回 `blocked_flag` 或 `ambiguous_flag`；原生 `--lines` 正常。

候选位置：`docs/parameter-hallucination/audit/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与生成器 | 通过 | 572 个命令作用域 |
| PreParse 与 alias/canonical | 通过 | tail、export since、export until、verify 四组逐字节一致 |
| 输出文件语义 | 通过 | tail/export/verify 三个输出路径别名均真实落盘 |
| block/ambiguous | 通过 | 12 组代表错误均在派发前停止 |
| 原生参数 | 通过 | `--lines` 与隐藏真实 `--output` 保持原生 |
| 非目标回归 | 通过 | JSON 结构恒等；生成 diff 仅 3 个 audit entry；fallback 无变化 |
| `internal/cli`、`internal/pipeline` | 通过 | CLI 79.383 秒 |
| generated drift | 通过 | alias 与 Schema 双次装配确定 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具；Runtime confirmation truth 通过 |
| 完整 `internal/app` | 未通过 | 298.359 秒；仅 complete-command 覆盖测试失败 |
| complete-command payload 门禁 | 未通过 | 200/203 个活跃命令有模板；缺 3 个命令、16 条 active fixture；395 active cases |

正式替换前必须补齐 3 个模板并重跑完整 `internal/app` 和政策门禁；未完成前，本候选只作为
完整待审核草稿。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00。
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`。
- 候选 SHA-256：
  `bf038d49c355732f40f6e033d804f3f84581ae6dc431104d191d69fe970a28f3`。
- 命令实现：`internal/app/audit_command.go`；文件选择与哈希链在 `internal/audit`。
- Schema 来源：同一冻结二进制运行时声明组装；3 个本地可用工具，5 个局部参数。
- Skill：当前已安装 DingTalk Skills 无 Audit 产品命令定义；未用相邻 Skill 推断。
- 明确未使用：历史或固定 Catalog、实验 badcase/工作簿、用户 Shortcut、已安装插件。

## 可复用分析流程

冻结提交并重建二进制；以官方 Cobra 树盘点真实/隐藏 flags；逐叶对账 Help、完整 Schema、
Skill 与实现；按实体、角色、值域、单复数和单位审核；只允许值可原样传递的精确映射；把
输入/输出/目录和同名异义显式拆开；在隔离副本生成并检查真实 flag 冲突；执行 PreParse、
alias/canonical、保护、非目标回归、包测试与仓库政策；最后用 complete-command 最终 payload
门禁决定“可落地”还是“待补测试”。
