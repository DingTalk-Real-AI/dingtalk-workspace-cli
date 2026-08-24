# Report 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上最新 `origin/main` 的冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一事实基线。该提交于
2026-08-20 10:53:27 +08:00 合并，分析使用同一提交重新构建的 `dws`、运行时组装
Schema、官方 Cobra 命令树、仓库内置 Shortcut、产品 Skill 和正式
`internal/cli/param_concepts.json`。未使用历史 badcase、实验工作簿、固定 Catalog、
用户自定义 Shortcut 或已安装插件。

Report 共有 9 个 Agent 可见 Schema 工具，另有 8 个仍可执行的兼容入口。参数风险
主要集中在日志模板名称与模板 ID、日志 ID、发送人与接收人标识、创建/修改时间、
分页参数，以及结构化 `contents` 与本地文件路径的混用。现有参数兜底可以处理一部分
“只改参数名、值原样传递”的问题，但不能完成模板查询、用户标识转换、单复数转换、
日期补时区或把普通文本包装为日志字段 JSON。

候选草稿已完成生成、PreParse、alias/canonical dry-run、block/ambiguous、非目标抽样、
generated drift 和 Schema Catalog 政策验证。候选尚不能直接替换正式表：仓库要求为
新增活跃别名补 complete-command payload 模板，当前门禁报告 207 个活跃命令中只有
200 个具备模板；Report 缺少 7 个活跃命令模板。正式落地状态为“规则已审核、运行链路
已验证，落地前需补 E2E 模板”。

## 参数问题

### 1. 日志模板名称、模板 ID 与模板类型容易串用

- `report template get` 的真实参数是 `--name`，值是模板名称。
- `report outbox list` 与 `report +outbox-list` 使用 `--template-name` 过滤模板名称。
- `report entry submit` 使用必填 `--template-id`，值必须来自 `template list`。
- 正式表已经在 `report outbox list` 拦截 `--template-type`，但未保护模板 ID、模板名和
  泛化 `--template` 的其他交叉误用。

模板名不能直接改名为模板 ID；提交前必须查询模板列表和字段定义。候选仅在精确命令
内映射 `--template-name`/`--report-template-name` 与真实名称参数，映射
`--report-template-id` 到 `--template-id`；跨值域输入进入 block/ambiguous。

### 2. `reportId` 的命名过于专用，Agent 容易生成通用 ID

`report entry get` 和 `report entry stats` 都只接受 `--report-id`。在这两个命令上，
`--id`、`--log-id`、`--entry-id` 的实体、角色、值域和单值形态一致，可以安全地按
命令映射到 `--report-id`。`--message-id`、`--task-id`、`--template-id` 属于不同
资源，候选明确拦截。

### 3. 发送人和接收人 ID 的角色、值域及单复数混杂

收件箱使用复数 `--sender-user-ids`，Help 明确描述为发送人 `staffId` 列表；提交命令
使用 `--to-user-ids`，运行时把逗号分隔的 `userId` 转成接收人数组。两者不能互换。
候选只允许 `--sender-staff-ids`/`--from-staff-ids` 映射到收件箱发送人列表，以及
`--recipient-user-ids`/`--receiver-user-ids` 映射到提交接收人列表；单数、角色缺失或
反向角色输入被拦截或标为歧义。

此外，Skill 的按发件人示例写成“`userId/staffId`”，而 Help 明确写 `staffId`。该值域
不一致应修改 Skill/参数声明来源，不能用别名表把两种 ID 假定为天然等价。

### 4. 创建时间、修改时间和日期输入边界不清

收件箱只有必填 `--start`/`--end`；发件箱同时有创建时间 `--start`/`--end` 和修改
时间 `--modified-start`/`--modified-end`。候选把 `from-date`、`end-date` 等现有
时间概念限制到审核过的 Report 列表命令，并为 `updated-start`、`modified-from` 等
明确修改角色的写法增加命令级别名。

别名只传递原值，不补齐时分秒和 `+08:00`。泛化 `--date` 被拦截；裸日期、UTC 与
ISO-8601 时区转换仍由调用方和命令校验负责。

### 5. 分页大小与游标模型容易混用

Report 列表命令公开 `--size` 和整数 `--cursor`。原子命令还保留隐藏原生兼容参数
`--limit`，Shortcut 没有该隐藏参数。候选把现有 `pagination_size` 概念扩展到精确的
Report 列表入口，使 `page-size`、`max-results` 等安全归一为真实 `size`，原生
`limit` 继续保持原生行为。

没有扩展 `page_cursor`：正式概念包含 `page-token`、`next-token` 等字符串令牌写法，
而 Report 的 `cursor` 是整数。名称相似不足以证明值域一致。

### 6. `contents`、`contents-file` 与普通文本不是同一种输入

提交命令要求 `--contents` 是结构化 JSON 数组，或由 `--contents-file`/stdin 提供该
数组。普通 `--content`、`--body`、`--text` 不能只改名后安全提交；系统还需要查询
模板字段并构造 `key/sort/content/contentType/type`。

候选仅映射 `--contents-json` 到 `--contents`，以及 `--content-file`、
`--payload-file`、`--report-file` 到 `--contents-file`。普通文本被 block，泛化
`--file`、`--payload`、`--data` 被标为 ambiguous，均在写调用前终止。

### 7. Schema、Help、Skill 与兼容入口不是同一可见面

运行时 Schema 发布 9 个 Agent 工具；Cobra 还保留 `report template detail`、
`report inbox`、`report create/detail/list/stats/sent/created` 等兼容入口及原子列表命令的
隐藏 `--limit`。Skill 主要描述推荐主路径，不应把中央兜底或隐藏兼容参数当成新的
公开参数。候选覆盖兼容入口以防 PreParse 行为分叉，但推荐输出始终保持主路径与
canonical flags。

## 当前别名表可以实施的方案

1. 扩展现有 `time_start`、`time_end` 和 `pagination_size` 到审核过的 Report 列表入口。
2. 对模板名、模板 ID、日志 ID、发送人 staffId 列表、接收人 userId 列表、修改时间
   和 contents 传输形态增加精确命令级别名。
3. 对模板名/ID、不同资源 ID、发送人/接收人、单值/多值、普通文本/结构化 JSON、
   date-only/ISO 时间和整数 cursor/token 分页增加 block 或 ambiguous 保护。
4. 保留原子命令隐藏 `--limit` 的原生行为，不重复制造另一套公开参数。
5. 修正 Skill 中发送人 ID 的值域说明，并为 7 个活跃命令补 complete-command payload
   模板后再评审落地。

## 当前能力支持不了的事项

- 根据模板名称查询 `templateId`；
- 调用人员查询把姓名、userId 与 staffId 相互转换；
- 把单个 ID 自动扩成列表或拆分、合并列表；
- 给裸日期补时间、时区或改变时间单位；
- 把普通文本包装为模板字段对应的 `contents` JSON；
- 在 `contents` 与 `contents-file` 之间读取文件或改写传输方式；
- 把字符串 page token 转成 Report 的整数 cursor；
- 自动判断泛化 `--source` 是创建来源还是发送人。

这些情况应保持明确错误、引导用户使用真实参数，或先调用解析/查询命令；不得只通过
参数改名继续执行。

## 第一轮改造建议

第一轮建议落地低风险、值原样传递的模板、日志 ID、分页大小、时间角色和文件路径
别名，同时启用相应 block/ambiguous。落地 PR 必须同步补齐以下 7 个活跃命令的
complete-command payload 模板：`report template get`、`report entry get`、
`report entry stats`、`report entry submit`、`report inbox list`、
`report outbox list`、`report +outbox-list`。Skill 的发送人值域应在同一轮修正或明确
标记为待确认；在此之前不要开放 role-free `--user-ids` 映射。

## 候选 `param_concepts.json` 改动与审核

候选文件是冻结提交正式表的完整副本，不是增量片段。相对正式文件：

- 修改 3 个既有 concept 的精确命令范围；
- 新增或修改 16 个 Report command override；
- 新增 15 个审核 fixture；
- 生成差异只涉及 Report 命令；非目标 concept、override 和保护规则保持原值；
- 每条自动 alias 的目标都是该精确命令真实接受的 flag，值不需要查询、拆分、合并、
  包装、类型转换或单位转换；
- 值域、角色、单复数或传输形态不一致的规则经审核后进入 block/ambiguous 或被移除。

候选位置：`docs/parameter-hallucination/report/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与真实生成器 | 通过 | `go generate ./internal/cli`，583 个命令 |
| alias/canonical 最终 payload | 通过 | 日志 ID、时间/分页、提交模板/接收人 dry-run payload 完全一致 |
| block/ambiguous | 通过 | 普通 content、单数发送人 ID、无角色 user IDs 均在 dispatch 前停止 |
| 原生参数 | 通过 | canonical flags 和隐藏 `limit` 保持 Cobra 原生行为 |
| 非目标回归 | 通过 | 生成差异限定为 Report；抽样 Ding payload 未变化 |
| `internal/cli`、`internal/pipeline` | 通过 | 隔离 worktree 执行 |
| generated drift | 通过 | 生成两次结果确定，Schema 组装确定 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具 |
| complete-command payload 门禁 | 未通过 | 200/207 个活跃命令已有模板；Report 缺 7 个命令模板 |

正式替换前必须补齐 7 个模板，重新执行完整 `internal/app` 测试和仓库政策；未完成前，
本候选只能作为待审核草稿。

## 可复用分析流程

后续产品继续复用同一流程：冻结同一 commit → 重建二进制 → 合并 Schema、官方命令树、
兼容入口和 Shortcut → 逐命令对账 Help/完整 Schema/Skill → 按实体、角色、值域、
单复数、单位和传输形态聚合问题 → 生成基于正式表的独立完整候选 → 语义审核 → 在隔离
worktree 临时替换正式输入 → 验证生成、PreParse、payload、保护规则、非目标回归和政策
门禁 → 如实标注落地状态。
