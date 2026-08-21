# Attendance Shortcut 下游业务能力需求规格

> 日期：2026-08-18
> Rebased executable 基线：`69bda96e49c7a478729b5f9232677fd9055e5d7d`；最终 clean PR HEAD 的 live SHA 与发布复核结果记录在 PR 证据中
> 对比基线：Lark CLI 1.0.87
> 范围：Attendance Shortcut only；不改 DWS 产品 Skill 的路由、流程或业务逻辑。仓库 policy 强制的可见 Shortcut 自动生成块单独机械同步。

## 1. 执行摘要

- Attendance 共审核 35 个源码 Shortcut；8 个具备 Agent 公开条件，27 个保持 unavailable。为守住已发布 CLI 的 argv/Help 兼容，其中 11 个历史可见入口继续以 compatibility-visible 形式可发现，但仍从 Agent public Catalog 排除、保持 legacy 输出且不发布 Result/Pagination；其余 16 个保持 hidden。公开数量按「严格响应合同 + 稳定身份 + 安全真实 fixture」的发布门计算，不把空数组或仅退出码 0 计为通过。
- 这 11 个 compatibility-visible 入口在完整 Schema 中保留历史 `availability=available` 与既有 workflow property，仅表示旧调用仍可执行；它们的 Shortcut 语义状态仍为 `public=false/unavailable`，默认 Shortcut 列表与 Agent public Catalog 均不发布。底层 MCP 字段名由 Execute 的显式 adapter 负责，不能在未经过版本化迁移时重定向已发布 Schema property。
- `+check-result` 已覆盖 Lark CLI 当前唯一 Attendance 用户任务 `attendance user_tasks query`；DWS inventory 还包含打卡流水、审批、班次、规则、设置、假期和个人视图等更宽能力。排班查询入口虽然保留历史 CLI 兼容，但因 `DS-ATTENDANCE-008` 当前保持 Agent-unavailable。
- 已确认 8 组下游需求：补卡规则详情返回空结果、报表合同不足、打卡结果分页缺少服务端确定终止证据、缺少安全可回收的管理员/写操作 fixture、6 个读场景缺少请求绑定字段或 nonempty/zero 双态 fixture、班次详情不回显稳定 ID、个人设置缺少逐场景权限发现与安全 fixture，以及排班查询对合法非空/空请求均返回 `exit 0 + literal null`。
- 审批模板的同类型多模板问题已在上游修复：以 `processCode` 作为资源身份，`approveType` 只做请求绑定，并要求 `submitUrl` 非空。班次详情与个人设置仍有下游合同/权限前置，不能以请求 echo 或部分场景成功伪造整体可用。

| ID | 优先级 | 类型 | 用户任务 | 当前状态 | 建议 Owner | 解锁的 Shortcut |
|---|---|---|---|---|---|---|
| `DS-ATTENDANCE-001` | P1 | business-service defect / contract insufficient | 搜索后读取补卡规则详情 | unavailable | Attendance Wukong 规则服务 | `+get-adjustment-rule` |
| `DS-ATTENDANCE-002` | P1 | business-service defect / contract insufficient | 发现报表列并查询考勤/假期报表 | unavailable | Attendance 报表服务 / MCP adapter | `+list-report-columns`, `+query-report-data`, `+query-report-leave` |
| `DS-ATTENDANCE-003` | P2 | contract insufficient | 可靠翻完打卡结果 | partial | Attendance 打卡查询服务 | `+check-result` 完整分页 |
| `DS-ATTENDANCE-004` | P1 | tenant-or-fixture / permission | 验证考勤组、全局设置、余额和写操作 | blocked / unavailable | Attendance 产品测试基础设施 / 权限 Owner | 14 个读写 Shortcut |
| `DS-ATTENDANCE-005` | P1 | response contract / tenant-or-fixture | 可验证地读取摘要、假期、签到和个人考勤 | blocked / unavailable | Attendance 查询服务 / 产品测试基础设施 | 6 个读 Shortcut |
| `DS-ATTENDANCE-006` | P1 | response contract | 用搜索得到的班次 ID 精确读取同一班次详情 | unavailable | Attendance Wukong 班次服务 | `+get-class` |
| `DS-ATTENDANCE-007` | P1 | capability / permission fixture | 可发现地读取全部个人设置场景 | blocked / unavailable | Attendance 设置服务 / 权限 Owner / 测试基础设施 | `+get-self-setting` |
| `DS-ATTENDANCE-008` | P1 | response contract | 可验证地读取员工排班 | unavailable | Attendance Wukong 排班服务 / MCP adapter | `+get-schedule` |

## 2. 用户任务与能力缺口总览

| 用户任务 / Golden Route | DWS Shortcut | Lark CLI 对应 | 当前能力 | 缺口分类 | 临时处置 |
|---|---|---|---|---|---|
| 批量查询员工打卡结果 | `attendance +check-result` | `attendance user_tasks query` | covered；框架分页 token 由当前页保守派生 | contract insufficient | 声明 `Pagination(kind=cursor,cursor_parameter=offset)`；续页只放 `meta.pagination`，业务 `data` 仅含 `count/records` |
| 搜索并读取班次 | `+search-class` → `+get-class` | 无同级入口 | partial | response contract | 只公开搜索；详情因不回显请求 classId 而 unavailable |
| 搜索并读取补卡规则 | `+search-adjustment-rule` → `+get-adjustment-rule` | 无同级入口 | partial | business-service defect | 只公开搜索；详情 unavailable |
| 发现字段并查询考勤报表 | `+list-report-columns` → `+query-report-data` | 无同级入口 | unavailable | contract insufficient | 两个入口均不进入 Agent Catalog；历史 `+query-report-data` 仅保留 CLI 兼容可见性 |
| 查询假期报表 | `+query-report-leave` | 无同级入口 | unavailable | business-service defect | hidden/unavailable |
| 搜索并读取考勤组 | `+search-group` → `+get-group` | 无同级入口 | blocked | tenant-or-fixture | 无已知非空安全 fixture；历史 `+search-group` 仅保留 CLI 兼容可见性，二者都不进入 Agent Catalog |
| 查询企业全局设置和假期余额 | `+get-global-setting`, `+get-leave-balance` | 无同级入口 | blocked | permission / fixture | hidden/unavailable |
| 查询个人设置 | `+get-self-setting` | 无同级入口 | partial | capability / permission fixture | 前五个场景已验证；全部场景发布前保持 Agent-unavailable，仅保留历史 CLI 兼容可见性 |
| 查询员工排班 | `+get-schedule` | 无同级入口 | unavailable | response contract | 合法非空与保证零命中请求均收到 `exit 0 + literal null`；旧 CLI 兼容可见，但不进入 Agent Catalog |
| 修改排班、班次、考勤组、假期和打卡结果 | 9 个写 Shortcut | 无同级入口 | unsafe to verify | tenant-or-fixture / contract insufficient | hidden/unavailable，不以 dry-run 记通过 |

## 3. 下游需求明细

### `DS-ATTENDANCE-001` — 让搜索得到的补卡规则可被稳定读取

#### A. 用户任务与现状

- 用户任务：先按名称浏览补卡规则，再用结果中的稳定主键读取完整规则。
- canonical Shortcut：`attendance +search-adjustment-rule`、`attendance +get-adjustment-rule`。
- atomic/raw route：`attendance adjustment search`、`attendance adjustment get`。
- Exact Shortcut 与 atomic/raw 均使用搜索返回的同一候选主键；搜索明确成功且非空，详情调用明确 `success=true`，但 `result=null`。
- 已排除上游空数组投影、整数解析和候选字段遗漏：多个可作为候选的数值字段均未得到非空详情；加班规则的相邻搜索→详情闭环正常。
- 置信度：高。仍需下游确认“搜索 ID 与详情 ID 不同”还是详情服务未返回对象。
- 安全证据句柄：`ATT-DETAIL-NULL-01`；仓库不保存 raw body、资源 ID 或 trace。

#### B. 需要下游提供的合同

- 明确 `get_adjustment_rule` 列表项中哪个字段是 `get_adjustment_rule_detail.adjustmentId` 的稳定主键；名称和类型必须在 Schema 中一致。
- 对存在且有权限的规则返回 `success=true` 和非空对象 `result`，对象必须回显同一稳定规则 ID。
- 对不存在、已删除、无权限、租户未开通分别返回稳定的 typed error；不得以 `success=true + result=null` 表示任一失败。
- 如详情接口不受支持，提供可发现的 capability/feature 状态，或在搜索结果中返回足以完成详情任务的完整对象并声明字段稳定性。
- 改动应 additive/versioned；旧字段保留兼容期，禁止静默改变现有 ID 的语义。

#### C. 验收标准

1. 创建或选择隔离规则，atomic search 非空并取得稳定 ID。
2. atomic detail 和 exact `+get-adjustment-rule` 均返回同一 ID 的非空对象。
3. 不存在 ID、无权限和已删除 ID 分别返回非零 typed error。
4. 上游恢复公开后，搜索→详情 E2E 通过且仓库/远端无测试残留。

#### D. 临时处置

`+get-adjustment-rule` 保持 Agent-unavailable 并从公开 Catalog 排除；旧 CLI 入口仅为 argv/Help 兼容继续可见，`+search-adjustment-rule` 不再承诺详情入口可用。

### `DS-ATTENDANCE-002` — 提供可发现、可验证的考勤报表合同

#### A. 用户任务与现状

- Golden Route：列出企业可查询报表列 → 选择稳定列 ID → 查询一批员工的列值；另一路径按假期类型查询时长报表。
- canonical Shortcut：`+list-report-columns`、`+query-report-data`、`+query-report-leave`。
- atomic/raw operations：`get_report_columns`、`get_report_columns_value`、`get_leave_time_by_leave_names`。
- 观察：列发现与假期报表调用均退出码 0 且 payload 为 JSON `null`；使用未经验证的列 ID 查询列值仅得到显式空数组，不能证明列 ID 有效或查询正确。
- 已排除上游投影丢失：原子调用本身即返回 `null`；Shortcut 现已拒绝把 `null` 当作合法空集合。
- 置信度：高。权限/租户功能可能是触发条件，但接口没有返回可区分的状态。
- 安全证据句柄：`ATT-REPORT-NULL-01`。

#### B. 需要下游提供的合同

- `get_report_columns`：成功时必须返回显式列数组；每项含稳定 `columnId`、显示名、值类型、单位、支持的日期/人员范围和是否需要管理员权限。
- 合法无列必须是 `success=true + result=[]`；未开通、无权限和服务异常必须是不同 typed error，不得返回裸 `null`。
- `get_report_columns_value`：返回值必须绑定请求的用户集合、列 ID 和时间范围；未知列返回 `COLUMN_NOT_FOUND`，不能静默得到空数组。
- `get_leave_time_by_leave_names`：返回显式数组并包含稳定用户身份、假期类型标识、单位和数值；合法零记录为显式空数组。
- 列值和假期报表若分页，必须提供 page/cursor、hasMore 和终止证据；批量用户存在部分失败时返回逐项 ledger 与整体 partial status。
- 提供安全 capability discovery：租户是否开通、调用身份所需权限、最大用户数、最大列数、最大时间跨度。

#### C. 验收标准

1. 管理员测试租户中列发现有已知非空和明确空租户两组 E2E。
2. 使用发现的同一 `columnId` 执行 atomic 与 exact Shortcut，返回与请求用户/区间绑定的非空值。
3. 未知列、无权限、未开通和超范围分别产生稳定非零错误。
4. 假期报表至少覆盖已知非空、合法空和未知假期类型。
5. 分页/partial 分支和远端零残留通过。

#### D. 临时处置

三个报表 Shortcut 均保持 Agent-unavailable；其中历史 `+query-report-data` 只保留 CLI 兼容可见性。不得用 `null`、请求 echo 或未验证列产生的空数组标记 PASS。

### `DS-ATTENDANCE-003` — 为打卡结果提供确定的分页终止证据

#### A. 用户任务与现状

- `+check-result` 已真实返回非空打卡结果并覆盖 Lark 任务；当前接口只接受 `offset/limit`，响应缺少稳定总量、hasMore 或 nextOffset。
- DWS 只能在返回条数小于 limit 时证明结束；满页时保守输出 `meta.pagination.endpoint_exhausted=false` 和 `next_token=offset+count`，不能声明全量完成。`complete/nextOffset/limit` 仅保留在 legacy 兼容输出，unified 业务 `data` 不冒充分页协议。
- 安全证据句柄：`ATT-CHECK-PAGE-01`。

#### B. 需要下游提供的合同

- 响应增加 `hasMore` 与 `nextOffset`，或 `totalCount`；这些字段必须与同一快照/排序一致。
- 固定稳定排序键和同 offset 重放语义；说明并发新增/修改是否可能造成重复或漏项。
- 空页且 `hasMore=true` 必须仍给出前进 token/offset；重复或倒退 offset 为协议错误。
- 声明最大 limit、最大时间跨度和超过上限的 typed validation error。

#### C. 验收标准与临时处置

- 验收覆盖多页、最后一页、零记录、满页但仍有下一页、重复 token/offset 和并发变更。
- 下游完成前，DWS 使用框架 `PaginationSpec` 和 `meta.pagination`表达保守续页；`cursor_parameter=offset` 表示调用者将 `next_token` 作为下一次 `--offset`，不表示下游已提供服务端 opaque cursor。满页始终不会被当作已完整。

### `DS-ATTENDANCE-004` — 建立可回收的 Attendance 管理员与写操作测试资源

#### A. 用户任务与现状

- 受影响读取：`+search-group`、`+get-group`、`+get-group-filtered`、`+get-global-setting`、`+get-leave-balance`。
- 受影响写入：`+import-schedule`、`+create-class`、`+update-class`、`+update-group-members`、`+create-group`、`+update-group`、`+update-leave-type`、`+save-leave-balance`、`+boss-check`。
- 当前安全身份没有已知非空考勤组 fixture；全局设置被权限拒绝；余额读取没有可验证结果。写操作会影响真实员工规则，且部分资源缺删除/恢复能力，因此未执行生产数据写入。
- 这不是对业务接口必然有 bug 的结论，而是可测试性和权限前置不足。
- 安全证据句柄：`ATT-FIXTURE-GAP-01`。

#### B. 需要的测试基础设施与合同

- 提供隔离租户或专用测试组织，包含：管理员测试身份、两个无业务含义测试成员、一个可删除考勤组、一个可删除班次、一个可恢复假期类型、可控排班与打卡结果。
- 只授予完成相应接口所需的最小 scopes；提供 capability discovery，区分权限不足、功能未开通和资源不存在。
- 写接口返回稳定资源 ID、逐项结果、幂等/commit-unknown 语义；所有更新支持精确读回。
- 为不可删除的企业设置提供 snapshot/restore 或专用 reset API；余额和 BOSS 改签必须能恢复原值。
- Fixture 有 TTL、Owner 和自动清理告警；日志只保留受控 evidence handle，不输出业务内容或身份值。

#### C. 验收标准与临时处置

1. 考勤组搜索有已知非空和保证零命中；详情绑定同一 ID。
2. create→get→update→restore/delete 覆盖班次、考勤组与排班。
3. 成员、余额和打卡结果写入均有 before/after 精确读回并恢复原值。
4. 未确认时远程写调用为 0；任一 partial/commit-unknown 非零退出。
5. 测试结束远端和本地均零残留。

在完整 fixture 到位前，相关 Shortcut 保持 hidden/unavailable。

### `DS-ATTENDANCE-005` — 为 6 个读场景提供请求绑定与双态 fixture

#### A. 用户任务与现状

- `+get-summary`：真实响应只含统计项，不回显请求 user、period 或 statsType，上游无法证明返回属于哪个请求。
- `+list-leave-types`：当前安全租户只有已知非空列表，而命令无筛选参数；不能用越界分页或错误请求伪造合法空结果。
- `+get-leave-records`、`+get-checkin-record`：当前只取得合法空结果，缺少已知非空流水 fixture，无法排除响应投影或请求绑定错误。
- `+my-attendance`、`+this-month`：上游已严格验证当前用户 profile 与每条打卡 ID，但当前期间仅有合法空数组，缺少同一身份下的已知非空 fixture。
- 安全证据句柄：`ATT-READ-FIXTURE-GAP-01`；不保存 raw body、用户 ID 或打卡时间。

#### B. 需要下游提供的合同与 fixture

- 摘要响应回显稳定 userId、统计周期起止和 statsType，或返回可校验的请求摘要；任一字段不一致必须 typed failure。
- 提供隔离的「无假期类型」测试租户，以显式 `success=true + result=[]` 证明 `+list-leave-types` 的合法空语义。
- 提供可创建、读取并清理的假期变更流水、签到流水和打卡流水；每项都必须包含稳定 ID、请求用户和时间范围回显。
- 为 nonempty 与 guaranteed-zero 提供独立 fixture；未知用户、无权限、未开通和合法空集合必须可区分，不得都返回裸 `null` 或无标识空数组。

#### C. 验收标准与临时处置

1. 每个集合叶子都用 exact Shortcut 和 owning atomic/raw 在同一参数下各证明一次已知非空和一次合法保证零命中。
2. 非空项的稳定 ID、用户和时间绑定在两层结果中一致；空结果仍有显式业务 success 和正确集合容器。
3. malformed/null/success=false/错身份/超范围均非零失败，且不会继续调用后续考勤接口。

在上述证据完整前，6 个 Shortcut 均保持 Agent-unavailable，并仅为历史 argv/Help 保留 CLI 兼容可见性；已实现的严格校验不等于已获得发布证据。

### `DS-ATTENDANCE-006` — 让班次详情回显可验证的稳定身份

#### A. 用户任务与现状

- 用户任务：先用 `+search-class` 浏览班次并取得稳定 `classId`，再用同一 ID 读取班次详情。
- canonical Shortcut：`+search-class`、`+get-class`；atomic/raw route：`attendance class search`、`attendance class get`。
- 在 clean discovery HEAD 上，搜索 exact/raw 均返回同一组非空正整数 `classId`；使用其中真实 ID 调用 raw detail，服务端返回 `success=true` 和非空 `shiftVO`，但对象没有 `id` 或 `classId`。
- 上游不能把请求 ID 注入响应来伪造 readback，也不能仅凭“非空详情”证明详情属于请求资源。因此 `+get-class` 保持 unavailable。
- 安全证据句柄：`ATT-CLASS-ID-ECHO-GAP-01`；不保存 raw body、资源 ID 或 trace。

#### B. 需要下游提供的合同

- `get_class_detail` 成功对象必须回显与请求精确一致的稳定 `id`/`classId`，类型与 `get_class_list` 列表身份字段一致。
- 存在、已删除、不存在、无权限和租户未开通必须返回可区分的 typed terminal 状态；不得以非空但无身份对象表示可验证成功。
- 明确班次 ID 的租户作用域、生命周期和搜索→详情一致性；如详情存在版本号，也应返回稳定版本字段以支持更新前读回。
- 改动需 additive/versioned；现有详情业务字段保持兼容。

#### C. 验收标准与临时处置

1. exact/raw 搜索得到同一非空 `classId`，同 ID detail 均返回身份精确匹配的非空对象。
2. 不存在、已删除和无权限分别非零 typed failure，不能成为 `success=true + result=null` 或无身份对象。
3. 上游 `+get-class` 的 missing/false/null/malformed/wrong-ID 回归与真实 E2E 全部通过。

下游补齐稳定 ID 回显前，`+get-class` 保持 hidden/unavailable；`+search-class` 仍可独立公开。

### `DS-ATTENDANCE-007` — 提供个人设置逐场景 capability 与权限安全 fixture

#### A. 用户任务与现状

- `+get-self-setting` 公开参数包含 6 个场景。clean discovery HEAD 上，前 5 个场景的 exact/raw 均能精确绑定请求 userId、场景字段和已观测类型；`bossAttendStatNotify` 在两层均返回稳定业务错误 `NO_PERMISSION`。
- 当前接口没有 capability discovery 告知调用身份可读哪些场景，也没有可安全授权的隔离 fixture。只验证 5/6 不能宣称整个公开枚举可用。
- 这不是把权限错误误判为业务空结果；exact/raw 均非零退出。上游保留严格 user/scene/type 校验，但发布面整体降级。
- 安全证据句柄：`ATT-SELF-SETTING-PERMISSION-GAP-01`。

#### B. 需要下游提供的合同与 fixture

- 提供 capability discovery，返回当前调用身份逐场景的 readable/forbidden/unsupported 状态、所需最小 scope/角色和租户功能开通状态。
- 为 6 个场景提供字段名、类型、可空性和版本化语义；成功必须回显请求 userId，并明确返回对应场景字段。
- 提供隔离测试身份或可撤销的临时最小权限授权 fixture，使 6 个场景均能完成 exact/raw 同场景验证；测试后权限必须回收。
- 无权限、场景不支持、用户不存在和设置未配置必须返回不同 typed error；不得统一为 `null`、空对象或无标识空成功。

#### C. 验收标准与临时处置

1. capability discovery 与 6 个场景实际调用一致，不遗漏权限前置。
2. 每个场景 exact/raw 的 userId、场景字段、类型和对象内容一致；`null`、错类型、错用户均非零。
3. bogus user、invalid scene、无权限和未开通均返回可区分非零错误。
4. 权限 fixture 全程最小化、可撤销，结束后无授权残留。

能力发现和安全 fixture 到位前，`+get-self-setting` 保持 Agent-unavailable；旧 CLI 入口仅保留兼容可见性。

### `DS-ATTENDANCE-008` — 让排班查询返回可判定的成功集合或业务错误

#### A. 用户任务与现状

- 用户任务：按员工和日期范围读取逐日排班，用稳定排班 ID 继续执行只读分析或受控的 BOSS 改签。
- canonical Shortcut：`attendance +get-schedule`；owning raw route：`attendance-wukong/getScheduleByRange`。
- 两次独立 clean HEAD 的真实验证中，已知历史非空区间与保证零命中的未来区间都得到同一结果：owning raw 进程退出 0，但响应为 literal `null`；Exact Shortcut 均以 `response_validation/empty_tool_response` 非零拒绝。
- 这既不能证明排班非空，也不能证明合法为空。上游严格校验已避免把 `null` 投影成 `[]`，但在下游提供可判定合同前无法公开该能力。
- 安全证据句柄：`ATT-SCHEDULE-NULL-01`；仓库不保存用户、日期、排班 ID、raw body 或 trace。

#### B. 需要下游提供的合同

- 成功查询必须返回显式排班数组；每项包含稳定非空排班 ID、请求用户身份、业务日期、班次身份和是否休息等字段。
- 合法零结果必须返回 `success=true + result=[]`（或等价的已审核显式集合），不得以裸 `null`、缺字段或空 body 表示。
- 无权限、用户不存在、租户未开通、日期范围非法和服务异常必须返回可区分的 typed nonzero error；不得继续用进程退出 0 掩盖业务失败。
- 如服务存在分页，必须提供页大小、前进 token/页号、hasMore/total 和明确终止证据；同一请求的 item identity 不得跨页重复。

#### C. 验收标准与临时处置

1. 已知非空 fixture 的 raw 与 exact 均返回同一显式数组，稳定 ID 集合、用户和日期绑定一致。
2. 保证零命中 fixture 的 raw 与 exact 均返回显式空数组，并有明确终止证据。
3. `null`、缺集合、错型 item、重复/空 ID、错用户和越界日期全部非零；错误 reason 可稳定区分。
4. 新 clean HEAD 完成 nonempty/zero 双层 E2E，仓库和远端均无测试残留。

下游修复前，`+get-schedule` 保持 `public=false/unavailable`、legacy 输出且不发布 Result/Pagination；旧 CLI/Help/full Schema 仅为历史兼容继续可发现，不代表 Agent 可用。

## 4. Lark 对齐与平台差异

| Lark 用户任务 | 所需下游能力 | 可精确对齐 | 平台差异 | DWS 推荐结论 |
|---|---|---|---|---|
| `attendance user_tasks query` 查询打卡结果 | 现有 `query_check_result`；最好补分页终止证据 | yes，分页完整性 partial | Lark 当前没有同级的排班、规则、报表和企业设置任务 | 保留 `+check-result` 为主对齐入口，报告分页边界 |

无法对齐的不是 DWS 缺入口，而是部分钉钉管理面缺少可验证下游合同或安全 fixture；不能为追求同名率伪造成功。

## 5. 超越 Lark 的产品机会

| 产品原生能力 | 所需下游支持 | 可形成的 DWS Shortcut | 安全/验证要求 | 优先级 |
|---|---|---|---|---|
| 异常考勤处置队列 | 稳定异常记录 ID、原因、关联审批、处理状态、分页和可恢复更正 | `attendance +exceptions` / `+resolve-exception` | 读写分离；更正确认；写后同 ID 终态读回；可恢复 | P2 |
| 跨员工考勤汇总 | 可按组织/成员批量聚合迟到、缺卡、加班、请假并给出统计口径版本 | `attendance +team-summary` | 最小权限、聚合脱敏、口径版本、分页完整性 | P2 |
| 规则影响预览 | 更新班次/考勤组/假期前返回受影响成员与日期范围，不提交写入 | `attendance +rule-impact-preview` | 只读、稳定影响计数、无副作用、与最终写请求同参数语义 | P1 |

## 6. 无需下游变更的上游修复

| Shortcut | 上游根因 | 已完成修复 | 回归证据 |
|---|---|---|---|
| 最终保留公开的 Attendance 集合查询 | 容错 projector 可能把缺字段、错型或坏元素投成 `[]` | 共享严格 success/result/collection 校验；显式空数组才合法；稳定 ID 和请求用户/时间/类型必须绑定 | 单元负向矩阵与最终 clean runtime tree 的 8 个公开入口真实 nonempty/zero、详情或模板 exact/raw 双层复核均完成 |
| `+check-record` | 初版误用业务归属日 `workDate` 校验按 `checkDateFrom/checkDateTo` 发起的实际打卡查询，导致跨午夜下班卡被静默丢弃 | 改用 `userCheckTime` 严格绑定请求日期范围；`workDate` 只作为班次归属日原样保留。完整 raw 集合仍必须先通过显式 collection、全量正整数唯一 ID、请求用户和实际打卡时间校验；任何实际时间越界都整次 fail-closed，不再静默过滤 | 最终 live 复核 exact/raw 均为 157 条且完整对象一致；旧轮 `workDate=start-24h`、`userCheckTime` 在范围内的跨午夜 OffDuty 记录明确保留；fresh zero 双层显式空，不由过滤制造 |
| `+check-result`, `+list-approve` | 初版把裸日期 `--end` 解析为当天 00:00，可能拒绝结束日白天的结果；旧 end-of-day 语义还会漏最后 999ms | 裸日期结束边界改为本地下一日 00:00 前 1ms；显式 datetime 保持精确值；结束日中午与最后 1ms 可接受，下一日 00:00 非零拒绝 | Execute 回归覆盖结束日中午/最后毫秒/下一日并锁定 reason；最终 live 的 `+check-result` 有真实 end-date item，`+list-approve` end-date 单日 probe exact/raw 一致 |
| `+get-approve-template` | 把请求维度 `approveType` 误作集合唯一身份，会拒绝同一类型下多个合法模板 | 改用非空唯一 `processCode` 作为资源身份；`approveType` 仅做请求精确绑定；每项 `submitUrl` 必须非空；允许 TRAVEL/OUT 同类型多项 | missing/wrong/duplicate processCode、wrong approveType、missing/blank submitUrl 负向矩阵；clean HEAD 上 5 个类型 exact/raw 全通过，TRAVEL/OUT 双项集合一致 |
| `+search-class`, `+search-adjustment-rule`, `+search-overtime-rule` | 嵌套 `shiftVO/entityVO` 导致身份投影风险 | 固定审核路径、展开 wrapper、要求正整数且不重复的稳定 ID，严格校验分页矛盾与无前进页 | 坏 item/空 ID/重复 ID/分页矛盾单元回归通过；clean HEAD 上 nonempty/guaranteed-zero 与 raw 对照通过，班次/加班规则另完成实际多页前进与终止 |
| `+get-overtime-rule` | 能力存在但缺少请求 ID 与响应对象的强绑定 | 详情对象要求非空且 `id` 与请求精确一致 | missing/false/null/malformed/wrong-ID/valid Execute 级矩阵；clean HEAD 上 exact/raw 同真实搜索 ID 对象一致，raw 对不存在 ID 返回错对象时 exact 非零拒绝 |
| `+get-class` | 上游已严格要求 `shiftVO.id`，但真实下游详情不回显任何 ID | 没有注入请求 ID 或放宽校验；按真实合同降级 unavailable | discovery HEAD 上真实搜索→raw detail 非空但 ID 缺失；等待 `DS-ATTENDANCE-006`，修复后再重跑 |
| `+get-self-setting` | 仅检查场景 key 存在会让 `null` 伪成功；用户外围空白可造成下传/比较漂移 | 用户输入只归一化一次并以同值下传/比较；场景字段必须非空且符合已观测 object/boolean/integer 类型；因 1/6 场景权限不可验证而整体 unavailable | 5 个 scene exact/raw 对照通过；boss scene exact/raw 均 `NO_PERMISSION`，等待 `DS-ATTENDANCE-007`，不把部分场景成功当整体 PASS |
| `+my-attendance`, `+this-month` | 旧的当前用户解析可跳过 malformed row，也可把 success=false 中的 stale result 当身份 | 改为严格 business success/result/唯一用户身份，坏 profile 后考勤 raw 调用为 0；每条打卡要求唯一正整数 ID | 静态/Execute 回归已通过；因当前只有合法空集合而保持 unavailable，不记 live PASS |

### 6.1 clean-HEAD live 发布门状态

| 叶子 | clean executable HEAD 双层证据 | 发布状态 |
|---|---|---|
| `+check-result` | exact/raw known-nonempty 以 20/20/8 三页前进并终止；48 个 ID、用户绑定与逐页对象一致；合法未来日显式空双层一致 | `PASS`；最终 SHA 见 PR 证据 |
| `+check-record` | exact/raw 均 157 条且完整对象、稳定 ID 集合一致；跨午夜 `workDate=start-24h`、`userCheckTime` 在范围内的记录已保留；fresh zero 两层均为显式空 | `PASS`；最终 SHA 见 PR 证据 |
| `+list-approve` | exact/raw known-nonempty 为 7 条，稳定 ID、用户、类型、日期范围及完整数组一致；合法未来日显式空双层一致 | `PASS`；最终 SHA 见 PR 证据 |
| `+get-schedule` | 两次独立 clean HEAD 的 known-nonempty 与 guaranteed-zero 均为 raw `exit 0 + literal null`，Exact Shortcut 均非零 `empty_tool_response`；没有把未知结果投影成空数组 | unavailable；等待 `DS-ATTENDANCE-008`，旧 CLI 仅兼容可见 |
| `+search-class`, `+search-adjustment-rule`, `+search-overtime-rule` | exact/raw known-nonempty 与随机唯一词 guaranteed-zero 通过；稳定 ID 集合与分页终止一致，班次为 5/5/3 三页，加班规则为 1/1/1 三页 | `PASS`；最终 SHA 见 PR 证据 |
| `+get-overtime-rule` | 使用本轮真实搜索取得的 ID，exact 与 raw 单项对象一致；不存在 ID 的 raw 返回错 ID 对象时 exact 非零拒绝 | `PASS`；最终 SHA 见 PR 证据 |
| `+get-approve-template` | 5 个 approveType 全部 exact/raw 通过，数量 1/1/1/2/2；TRAVEL/OUT 多项 `processCode` 非空唯一且集合一致，类型绑定和提交入口有效 | `PASS`；最终 SHA 见 PR 证据 |
| `+get-class` | raw 非空但不回显请求 ID | unavailable；等待下游合同，不以旧调用记 PASS |
| `+get-self-setting` | 5 个场景通过，1 个场景 `NO_PERMISSION` | unavailable；等待 capability/权限 fixture，不以部分结果记 PASS |

pre-rebase discovery 轮次的多页加班规则 raw 验证曾一次返回字面量 `null` 且进程退出 0；该次结果没有计为 PASS，重试后才完成同场景双层分页核对。这是 owning atomic/raw 的下游/renderer 终态合同风险：atomic 不应把 transport/null 失败表示为零退出。Shortcut 自身对 `null` 仍严格非零，不会把它投影为空集合；后续最终轮次未再出现该 transient。

上述 8 个公开入口均在最终 clean runtime tree 从零重跑，未继承 discovery PASS；最终可执行 SHA 写入 PR 证据，本文只保留脱敏业务断言。`+get-schedule` 的四次 raw `null` 与 Exact 非零结果作为降级证据保留，不计入公开通过数。

## 7. 安全与脱敏声明

- 本文不含真实用户、组织、租户、profile、规则、排班、考勤组或打卡记录 ID。
- 本文不含 trace/request ID、token、签名 URL、邮箱、电话、业务标题正文或真实日程内容。
- Raw 响应仅在仓库外临时目录中处理并已删除；本文只保留不可反查的证据句柄和聚合事实。
- 进入 Git 前必须扫描最终树、未跟踪文件和 `origin/main..HEAD` 全部历史。
