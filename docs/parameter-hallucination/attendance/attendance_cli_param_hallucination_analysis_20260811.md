# DWS Attendance 产品 CLI 参数幻觉分析

> 状态说明（2026-08-21）：本文件保留 2026-08-11 的原始事实盘点和问题证据；其中 concept 数量、
> concept 名称及落位方案已被同目录 `attendance_cli_param_hallucination_review_20260821.md` 与最新候选
> `param_concepts.json` 取代，不应作为当前合入清单。最新候选基线为线上 main
> `11934eed057267d97e7442ddd420c711ee1802dc`。

## 1. 结论摘要

本报告以线上 `main` 提交 `fd24619437afcb92638d6a71e0bfd9254815fe06`（2026-08-11 14:08:11 +0800）为冻结基线，严格按照 `specs/product-cli-param-hallucination-analysis-spec.md` 对 Attendance（日程考勤）产品进行全量参数分析。事实来源只包括同一提交重新构建的 DWS、运行时组装 Schema、逐命令 `--help`、Attendance Skill、仓库内置 Shortcut、Cobra 隐藏兼容 flag 和必要实现代码；没有使用历史 badcase、`dws-eval`、`merged_scan.json`、历史工作簿或固定 Catalog。

冻结基线共有 57 个可执行 Attendance 工具，其中 19 个是仓库内置 Shortcut；52 个命令有业务参数，5 个命令没有业务参数。52 个有参命令累计出现 216 次业务参数，形成 97 个不同的公开 flag 名。逐 57 个 leaf 对账表明，公开 Help 与运行时 Schema 的业务 flag 集合差异为 0，说明本轮问题不是 Schema 生成滞后，而是产品内部的人员角色、单复数、时间语义、领域 ID、枚举结构，以及公开 camelCase 与隐藏兼容层的可发现性不统一。

本轮归纳出 7 类问题，参数问题明细共 120 行，覆盖全部 52 个有参命令；同一命令可以同时存在人员、时间、ID 和结构化值等多类风险，所以 120 行不能理解成 120 个命令。5 个无业务参数命令分别是 `attendance report columns`、`attendance +list-leave-types`、`attendance +my-attendance`、`attendance +this-month` 和 `attendance vacation types`，已完成盘点但无需进入参数别名治理。

优先级最高的问题是：

- `user/users/staff-ids/owner/target/operator-staff-id` 指向的人员角色和 cardinality 不同；
- `start/end/date/time` 的区间、单日和检查时刻语义不能只凭名称互换；
- 考勤组 ID、聊天会话 ID、班次 ID、规则 ID、审批计划 ID 和结果 ID 值域不同；
- `type/types`、`name/leave-names`、`class-id/classIds`、报表列 ID 与 JSON 列表存在单复数和结构差异；
- `schedule import` 公开使用 `--groupId/--scheduleVOS`，但隐藏接受 `--group-id/--schedules`，而 `--schedule-vos` 当前仍不能被中央链路兜底；
- JSON、枚举、单位、布尔开关和 `user-say-yes` 确认参数不能通过改 flag 名完成值转换或安全语义转换。

已在正式表的冻结副本上生成完整候选 `param_concepts.json`，没有替换当前工作区正式文件。候选新增 11 个 Attendance concept、仅扩展 7 个既有 concept 的 Attendance 命令范围，并新增 44 个精确 Attendance command override。生成后共有 52 个 Attendance 命令进入治理，形成 659 条 alias、370 条 block 和 6 条 ambiguous 名称项。

候选已通过结构化差异审核、生成器、24 组 alias/canonical 最终 dry-run payload 等价、5 组 block/ambiguous、原生隐藏兼容回归、`internal/cli`、`internal/pipeline`、`internal/app` 全包测试、生成漂移和 Schema 契约门禁。它仍是“待审核草稿”，因为本轮没有修改正式 `validation_fixture`，也没有把 24 组代表性测试转为仓库长期 complete-command payload 模板；正式替换前仍需补齐这些审核资产。

## 2. 产品参数现状

| 量化项 | 结果 | 说明 |
|---|---:|---|
| 可执行 Attendance 工具 | 57 | 同提交 runtime-assembled Schema 与官方命令树 |
| 仓库内置 Shortcut | 19 | 属于正式产品面，已纳入；不含用户自定义 Shortcut 和插件 |
| 有业务参数的命令 | 52 | 全部进入问题覆盖和候选影响审计 |
| 无业务参数的命令 | 5 | 已盘点，无需别名治理 |
| 业务参数出现次数 | 216 | 按 52 个有参工具累计 |
| 不同公开 flag 名 | 97 | 不含 root 全局输出类参数 |
| 使用 `--start` / `--end` | 17 / 17 个命令 | 时间范围最常见 |
| 使用 `--users` | 13 个命令 | 多人列表，不应与单人 `--user` 自动互转 |
| 使用 `--limit` | 10 个命令 | 与 page 或 offset 组成不同分页模型 |
| Help/Schema 公开参数差异 | 0 | 逐 57 个 leaf 使用同一冻结二进制核对 |
| 正式表已有 Attendance 覆盖 | 2 个命令 | 仅 `attendance check result` 与 Shortcut `+check-result` 的 `user_ids` |

## 3. 七类参数问题

### 3.1 人员标识符、单复数与操作角色混杂（30 个命令）

Attendance 同时使用 `user`、`users`、`staff-ids`、`operator-staff-id`、`owner`、`target`、`member`，群成员更新还将 add/remove、普通成员/额外成员、人员/部门拆成不同参数。它们都与人员或组织成员有关，但不是同一个业务角色，也不总是同一种 cardinality。

候选复用 `user_id` 与 `user_ids`，只在精确命令中绑定同角色同值域名称。例如 `record get --user-id → --user`、`vacation save-balance --target-user-id → --target`。`checkin records` 同时存在操作人单值和目标员工列表，因此只接受角色明确的 `--operator-user-id → --operator-staff-id`，通用 `--user/--user-id/--staff-id` 返回 ambiguous。`approve list --type` 会因单数/复数冲突被 block，`group update-members --users` 会因无法判断 add/remove 方向而 ambiguous。

### 3.2 时间范围、单日与检查时刻使用相似名称（22 个命令）

17 个命令使用 `start/end`，汇总和记录命令使用 `date`，`boss-check` 使用 `time`；`schedule get` 的 Skill 示例还使用原生隐藏兼容名 `workDateBegin/workDateEnd`。这些名称看起来都表示时间，但分别承担区间起止、单个工作日和检查时刻，值格式也可能不同。

候选仅在相同角色内允许 `begin/from/start-date/start-time → start` 和 `until/to/end-date/end-time → end`，并为统计日期补充 `work-date/query-date`。它不会自动把日期补成时间戳、推导缺失的区间端点或改变时区。

### 3.3 检索、筛选与分页参数命名分散（10 个命令）

检索主要使用 `query`，分页一部分使用 `page/limit`，打卡结果使用 `offset/limit`，群组检索还存在 `query-ble` 和 `query-position` 等专用过滤器。`cursor`、`offset` 和页码不是同一分页模型，通用文本 query 也不能替代蓝牙、地点等专用条件。

候选扩展 `search_query`、`page_number` 和 `pagination_size`；`offset` 仅在精确命令 override 中保护和归一，不新增中央 concept。只对 `keyword/current-page/page-size/page-offset` 等值原样名称做归一；不做 page↔offset/cursor 换算，也不把通用 query 合并到专用筛选参数。

### 3.4 考勤领域 ID 与作用域对象容易被泛化为通用 id（25 个命令）

补卡规则、加班规则、班次、考勤组、审批计划/结果、假期类型、配置场景和企业分别使用 `adjustment-id`、`overtime-id`、`class-id`、`group-id/groupId`、`plan-id/result-id`、`leave-code`、`setting-scene` 和 corp/scope 字段。

候选仅为可跨命令复用的班次、假期编码和查询日期保留中央 concept；补卡规则、加班规则、配置场景等局部角色下沉为精确命令 override。`rule-id` 只在已经确定规则类型的命令中绑定；`boss-check --id` 因可能指向 plan 或 result 而 ambiguous；Attendance 的 `group-id` 明确 block `conversation-id/open-conversation-id`，防止把聊天会话 ID 当成考勤组主键。

### 3.5 类型、名称与列表参数的业务含义和基数不同（20 个命令）

`type/types` 分别可能是单枚举和枚举列表；`name/leave-names` 分别是显示名称与假期名称列表；`columns` 是报表列 ID 列表；`classIds` 是结构化班次 ID 列表；`result`、`stats-type`、`unit` 等又是不同枚举。

审批类型、报表列和假期名称均按精确命令 scoped alias/block 处理，不新增中央 concept；只映射明确同角色且值可以原样传递的名称。单数/复数、编码/名称和 ID/JSON 列表之间不会自动转换。

### 3.6 公开 camelCase、隐藏兼容参数与 Skill 可发现性不一致（3 个命令）

`attendance schedule import` 的公开 Help/Schema 使用 `--groupId` 与 `--scheduleVOS`，Cobra 同时隐藏接受 `--group-id` 与 `--schedules`；`attendance group update` 公开使用 `--classIds`，格式归一可接受 `--class-ids`；`attendance schedule get` 的 Skill 使用隐藏 `--userIdList/--workDateBegin/--workDateEnd`。

这些隐藏参数已经由命令原生接受，候选不重复声明成中央 alias，并以最终 payload 回归测试证明兼容仍在。候选新增 `--groupid → --groupId` 与 `--schedule-records → --scheduleVOS`。但 `--schedule-vos` 当前仍报 `unknown_flag`：生成/PreParse 使用 Morph 后的名称作为键，而真实 Cobra flag 保留 acronym camelCase，现有模型无法同时表达“morphed 名与 canonical 形态不同”的这一个别名。该问题需要修改真实 flag 或完善生成/匹配模型，不应伪装成已解决。

### 3.7 结构化值、布尔开关、数值单位与确认参数不能靠改名转换（10 个命令）

`class-vo`、`group-vo`、`scheduleVOS`、`visibility-rules` 等接受 JSON；假期余额使用 `num`、`unit`、有效期和原因；全局/个人设置包含大量布尔或枚举字段；`user-say-yes` 是写操作确认参数。

当前链路只修改 argv flag 名并原样保留值，不能生成 JSON、修改 JSON 内部字段、翻译枚举、换算小时/天、拆分列表或改变确认语义。候选只增加 `class-json/group-json/schedule-records` 等不改变值结构的名称，且不把 `user-say-yes` 纳入 concept。

## 4. 候选别名表的实施方案

候选采取四层治理：

1. 扩展既有 concept：仅为 Attendance 命令扩展 `search_query`、`pagination_size`、`page_number`、`time_start`、`time_end`、`user_id` 和 `user_ids`。
2. 新增产品概念：分页 offset、补卡规则 ID、加班规则 ID、班次 ID、查询日期、假期编码、配置场景、报表列 ID、审批类型列表、排班记录 JSON 和假期名称列表。
3. 使用 44 个精确 command override：处理 owner/target/operator、add/remove、plan/result、group/class、设置字段以及稳定命令与 Shortcut 的差异，必要时使用 `scope_strict`、block 或 ambiguous。
4. 保留原生兼容：`group-id/schedules/userIdList/workDateBegin/workDateEnd/class-ids` 已由 Cobra 或格式归一接受，不重复建立中央来源。

生成后从正式表的 2 个 Attendance 命令、2 条 alias、12 条 block，扩展到 52 个命令、659 条 alias、370 条 block、6 条 ambiguous。数量看起来较大，是因为 concept 的成员、excludes 与精确命令范围会组合展开；不是 1035 个独立业务映射。人工审核重点放在人员 cardinality、操作角色、时间角色、领域 ID 值域、结构化值和安全 flag 六个边界上。

## 5. 当前能力无法解决或不应该解决的事项

- `attendance schedule import --schedule-vos`：当前 Morph/真实 camelCase flag 表达存在缺口，仍为 `unknown_flag`；安全做法是使用公开 `--scheduleVOS`、原生隐藏 `--schedules` 或候选 `--schedule-records`。
- 单人和多人列表互转：不能把 `user` 自动包装成 `users`，也不能从 users 中选择一个 owner/target/operator。
- 成员更新方向推断：通用 `--users` 无法判断 add/remove，也无法判断普通成员或 extra member。
- ID 查询与值域转换：不能把聊天会话 ID 转为考勤组 ID，也不能由名称查询 class/group/rule ID。
- 日期、时刻、时区和范围推导：不能补全时间、换算时区或根据单日自动生成 start/end。
- JSON、枚举和单位变换：不能构造 scheduleVOS、class-vo、group-vo、visibility-rules，不能翻译枚举或换算小时/天。
- 安全确认语义：`user-say-yes` 必须保持命令原生安全行为，不能通过参数 concept 猜测或绕过。

这些事项并不阻塞第一轮名称治理。可用 block/ambiguous 阻止明显错误；无法完成的是自动查询、角色选择和参数值变换。

## 6. 候选草稿审核结果

相对冻结基线正式 `internal/cli/param_concepts.json`：

- 新增 11 个 concept，全部只服务 Attendance 命令；
- 仅扩展 7 个既有 concept 的 `commands`，没有改 canonical、members、excludes 或非 Attendance 范围；
- 新增 44 个 Attendance command override；
- 没有删除或修改非 Attendance override；
- `validation_fixture` 完全未变；
- 当前真实工作区的正式 `internal/cli/param_concepts.json` 和 `param_aliases_generated.go` 均未修改。

候选初版在生成审核中发现并移除了原生隐藏 flag 的重复声明，包括 `approve/check/schedule` 中已有的 `to`、`user-id-list`、`work-date-begin` 与 `work-date-end`。最终候选明确区分了新增中央治理、原生兼容和当前不支持三类状态。

## 7. 验证结果

所有行为验证都在冻结 main 的 `/private/tmp` 隔离副本执行；写命令使用 `--dry-run`，未调用真实业务 API。

| 验证项 | 结果 |
|---|---|
| JSON 结构校验 | 通过 |
| `go generate ./internal/cli` | 通过，生成 331 个命令条目 |
| 候选作用域审计 | 通过：11 个新 concept、7 个既有 concept 扩展、44 个 override、非 Attendance 语义变化 0 |
| alias/canonical 最终 payload 等价 | 24 组通过，均为 `dry_run=true`、`executed=false` |
| block/ambiguous | 5 组通过，均在 dispatch 前终止 |
| 原生兼容回归 | `schedule get` 旧参数、`schedule import` 隐藏参数、`class-ids` 均通过 |
| 已知不支持边界 | `attendance schedule import --schedule-vos` 仍为 `unknown_flag`，与报告一致 |
| `go test ./internal/cli ./internal/pipeline -count=1` | 通过，77.436 秒 / 0.512 秒 |
| `go test ./internal/app -count=1` | 通过，244.942 秒 |
| `check-generated-drift.sh` | 通过，Schema 组装确定性通过 |
| `check-schema-catalog.sh` | 通过，27 个产品、1018 个工具 |

第一次在受限沙箱内运行 Schema 门禁时，`httptest` 因不能绑定 `[::1]` 中止；随后在允许本机回环端口的隔离环境中原样重跑整条门禁并退出 0。该环境性失败没有被计为代码通过，最终结论只依据完整成功的重跑结果。

候选仍不能直接替换正式表：正式落地前需要把关键 alias、block、ambiguous 和原生兼容用例补入仓库长期 `validation_fixture` / complete-command payload 测试，并由 Attendance 产品或接口负责人确认隐藏兼容参数是否继续保留、是否要统一 camelCase 公开 flag。

## 8. 第一轮改造建议

1. 先落地人员、时间、分页和专用 ID 的同角色同值域 alias，以及已经验证的 block/ambiguous。
2. 把 24 组最终 payload 等价和 5 组 guard 用例转成长期测试，再替换正式表。
3. 单独决策 `groupId/scheduleVOS/classIds`：如果修改真实 flag，应同步 Help、Schema、Skill 和兼容期；如果不修改，应在 Skill 明示 public canonical 与 hidden compatibility。
4. 不在第一轮处理 JSON、枚举、单位、ID 查询或确认参数值转换。

## 9. 可复用到其他产品的流程

冻结线上 main → 从同提交构建官方二进制 → 对账官方 Cobra、runtime Schema、Help、Skill 和内置 Shortcut → 按实体、角色、值域、cardinality、单位和结构聚合问题 → 基于冻结正式表生成完整候选 → 审核非目标 diff 与原生兼容重复 → 在隔离副本验证生成、最终 payload、保护和政策门禁 → 输出同口径 Markdown、五页中文 XLSX 与候选草稿。

## 10. 交付物

- 本报告：`docs/parameter-hallucination/attendance/attendance_cli_param_hallucination_analysis_20260811.md`
- 汇报工作簿：`docs/parameter-hallucination/attendance/attendance_cli_param_hallucination_analysis_20260811.xlsx`
- 完整候选别名表：`docs/parameter-hallucination/attendance/param_concepts.json`

工作簿固定包含“汇报总览、参数问题明细、兜底解决方案、当前无法解决、分析依据”五个中文页面，用于管理汇报、逐命令审核和后续落地跟踪。
