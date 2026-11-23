# 企业运动健康（钉钉运动）命令参考

命令前缀统一为 `dws sport`，属于 dingtalk-misc 长尾能力，不在 `dws --help` 展示；仍可直接执行 `dws sport --help` 和各叶子命令。所有调用追加 `--format json`。命令树已发布到 Agent Schema；参数、约束或确认语义不确定时，优先读取精确 compact leaf Schema（`dws schema "sport ..." --compact`），仅在 Schema 不可用或与 Cobra 实际行为发生漂移时读取同一叶子的 `--help`。不要直接调用 MCP、HTTP API 或自行猜测工具名。

## 能力范围与前置条件

覆盖企业钉钉运动步数榜查询与运动比赛（挑战赛）管理：开通状态、企业/部门/员工步数排行与多日明细、企业比赛列表、比赛创建与期次查询、我的比赛/参与记录/个人达标、比赛 TopN 与全员排行榜、编辑/删除/审批/邀请、开赛群播报、报名/参与/达标/趋势统计、比赛数据与历史步数报表导出。CLI 共接入 38 个工具，不接入 `get_user_org_list`。

前置条件：企业已在钉钉客户端开通运动步数榜；比赛功能需要企业已开通运动健康服务。调用身份为当前登录用户所属企业。

Use when：用户要查企业步数榜开通状态、企业步数概况与总步数、员工/部门步数排行、某人或某部门多日步数明细，或列出企业/我的比赛、查我的报名状态/达标进度/参与记录、查比赛排行榜，或创建步数/跑步/综合运动挑战赛、查比赛配置规则期次、开赛群播报、统计比赛报名/参与/达标/趋势、管理和邀请比赛、审批成员比赛、导出比赛或步数报表。

Avoid when：

- 个人健康数据（非企业步数榜）或钉钉运动客户端内设置，不在本能力范围。
- 群创建、群成员管理、普通群消息发送走 `dws chat`；比赛绑定群必须已存在。数字群 id（groupId）优先从 `competition list` / `config` / `my-list` 真实返回的 `groupId`+`groupName` 获取，禁止猜测；`dws chat` 只返回加密会话 id（openConversationId 体系），不能当数字群 id 用，只有加密会话 id 时用 `--open-conversation-id`。
- 找人获取 `--user-id` 对应的员工时走 `dws aisearch` / `dws contact`，用真实 staffId。
- 普通通知、OA 审批走各自产品；本能力的审批仅指运动比赛审批，不是 OA 审批。

## 意图路由

| 用户意图 | 命令族 |
|---|---|
| 查步数榜是否开通 | `dws sport org status` |
| 判断当前用户是否企业管理员（受限操作前预判） | `dws sport org admin` |
| 企业步数概况、区间总步数 | `dws sport step org-info` / `dws sport step org-total` |
| 员工/部门/部门内员工步数排行 | `dws sport step emp-ranking` / `dept-ranking` / `dept-emp-ranking` |
| 某员工/某部门多日步数明细 | `dws sport step emp-info` / `emp-infos` / `dept-infos` |
| 列出企业启用中的比赛、找 ruleCode | `dws sport competition list` |
| 我的比赛、可参与数量、参与过的期次与总数 | `dws sport competition my-list` / `my-can-join-count` / `my-participated` / `my-participated-count` |
| 活动详情与我的报名状态、我的达标进度 | `dws sport competition activity-detail` / `my-achievement` |
| 比赛 TopN 榜 / 全员分页排行榜 | `dws sport competition rank-top` / `ranking` |
| 创建比赛、查配置/规则/期次 | `dws sport competition create` / `config` / `rule-detail` / `instances` / `current-instance` / `next-instance` |
| 开赛群播报 | `dws sport competition announce` |
| 邀请员工或目标群参赛 | `dws sport competition invite` |
| 编辑、删除、审批比赛 | `dws sport competition update` / `delete` / `audit` |
| 比赛导出与任务查询 | `dws sport competition export` / `export-task` |
| 历史步数报表导出 | `dws sport step export` |
| 报名/参与/达标/趋势统计 | `dws sport stats signup` / `participants-daily` / `participants` / `reached` / `trend` |

## 执行规则

1. 先定位真实对象。`--user-id` 用当前会话已验证的员工 staffId（纯数字）；`--dept-id` 必须取自 `dept-ranking` 真实返回；`--rule-code` / `--competition-code` 必须取自创建返回或 `competition list` / `my-list` 等查询命令的真实结果，禁止按名称猜测。
2. 日期一律 `yyyyMMdd`；周榜/月榜的 `--stat-date` 传周期内任意一天；多日明细 `--stat-dates` 逗号分隔、1~30 个。
3. `org status` 返回 false 表示企业未开通步数榜，是正常业务状态：排行接口会返回空列表，向用户说明需在钉钉客户端开通，不要反复重试。
4. 排行分页用 `--cursor`（首页 0）+ `--size`（1~100），按返回的 nextCursor 续翻到满足范围或服务端结束。
5. `competition create` 是无幂等写操作，`confirmation=user_required`、`non_idempotent`：执行前必须把全部参数逐项列给用户确认，确认后才加 `--yes`；禁止用试探性参数反复创建。
6. `competition announce` 是写操作且无幂等：同一 ruleCode 重复播报会重复打扰群成员，执行前必须获得用户确认。
7. 创建或查询后优先用返回的真实 `ruleCode` 回读 `competition config` 验证。
8. 新增 invite/update/delete/audit/export/export-task 和 step export 不接受操作人 `--user-id` 或 `--corp-id`，身份由平台注入；邀请中的 `--receiver-user-id` 与导出的 `--staff-ids` 是业务工号，须保持字符串及前导零。旧 create/rule-detail/announce 仍按各自 Schema 提供原有参数。
9. 所有写操作先确认。`invite` 的 `--receiver-user-id` 与 `--target-group-id` 至少一个，可同时传；无幂等，成功不保证送达，待审批比赛可能改发群主审批提醒。
10. 新增的 list/rank-top/ranking/my-*/activity-detail 均为只读，不需 `--yes`；企业与“我”的身份由平台注入，不接受 `--user-id`/`--corp-id`，也不能查询他人的个人视角数据。空列表、0、`rankNo=-1` 都是正常结果，不要当失败重试。
11. `org admin` 只读无入参，身份平台注入：result=true=管理员，result=false=确认非管理员；调用失败（success=false，组织服务查询不可用）时命令返回错误而不是 false，绝不能把失败当非管理员回复，应提示用户稍后重试。可在导出/删除/审批等受限操作前预判并提前话术分流；预判与执行之间有权限变化窗口，受限工具仍会服务端复查，结论以受限工具为准。

## 比赛创建约束（CLI 本地校验）

- 当前仅支持 `--competition-type FIXED`（固定不循环），RECURRING 创建在 CLI 侧直接阻断，不要用空值或猜测日期绕过。
- 绑定群用 `--group-id`（数字群 cid）与 `--open-conversation-id`（`cid...==` 加密会话 id）至少一个，也可同时传（都传时服务端以群 id 为准）；只传会话 id 时服务端换取数字 cid 补到 groupId。
- `--start-date` / `--end-date` 必须是真实日期（含闰年校验）；结束不得早于开始。
- `--target-day-value` 不得超过单期实际天数。
- `--daily-limit`（可选）必须 ≥ `--goal-value`。
- `--cover-image-url` 优先使用官方默认头图；绑定群必须属于当前企业，群标识二选一见上。

## 标准流程

### 查询企业步数数据

1. `sport org status` 确认开通状态。
2. `sport step org-info --stat-date <yyyyMMdd>` 查当日概况，或 `sport step org-total --begin-date <yyyyMMdd> --end-date <yyyyMMdd>` 查区间总量。
3. 排行用 `sport step emp-ranking --stat-date <yyyyMMdd> --stat-type D --cursor 0 --size 20`（D=日/W=周/M=月；部门榜仅 W/M）。
4. 多日明细用 `emp-infos --user-id <staffId> --stat-dates 20261101,20261102,...`。

### 创建挑战赛

1. 拿真实群标识：数字群 id 从 `competition list`/`config`/`my-list` 返回的 `groupId` 获取；只有加密会话 id（`cid...==`）时改用 `--open-conversation-id`。与用户确认比赛名称、类型（WALK 走路=步 / RUN 跑步=米 / COMPREHENSIVE 综合=千卡）、起止日期、每日目标与达标天数。
2. 向用户逐项确认全部参数，确认后执行 `sport competition create ... --yes`。
3. 保存返回的 `ruleCode`；`status=1` 表示已生效（群主/管理员创建），`0` 表示待群主审批（普通成员创建），据实告知。
4. 用 `sport competition config --rule-code <ruleCode>` 回读验证。

### 查询企业比赛、我的比赛与排行

1. 企业比赛：`competition list --cursor 0 --size 20` 列出启用中的配置（不含审批中/已拒绝/已删除），行内 `code` 即 ruleCode。
2. 我的比赛：`competition my-list --list-scope JOINED|CREATED|CAN_JOIN`，一条规则一行；可参与数量用 `my-can-join-count`。`canEdit`/`canInvite`/`canExport` 只是提示，不替代写操作确认和服务端校验；导出须用行内 `competitionCode`，为 null 时先查期次。
3. 我参与过的期次：`competition my-participated --page-size 20`，总数用 `my-participated-count`（列表无 total）。`achievedDays` 仅已加入且进行中填充。
4. 详情与进度：`activity-detail --rule-code <ruleCode>` 查我的报名状态 signedUp、isCreator/canApprove（不代表已审批）；`my-achievement --competition-code <code>` 查今日进度、累计值、名次和本周7天达标。
5. 排行榜：`rank-top --rule-code <ruleCode> --rank-type DAY|WEEK|MONTH|TOTAL [--top-n 10]`，TOTAL 为本期总榜，加 `--sort-by-reached-times=true` 按达标次数排；全员名单用 `ranking --competition-code <code>`。榜单 userId 为 staffId，可能为 null。
6. 分页：这些列表每次只取一页，`data.result.list` 是本页数据，续页读取 `meta.pagination.next_token` 原样传给 `--cursor`，以 `endpoint_exhausted=true` 判断结束；空页不代表结束。个人列表与 ranking 首页不传 cursor（发送 null），企业 list 首页为 0；切换 `--list-scope` 必须从首页重新开始。

### 比赛运营统计

1. `sport stats signup --rule-code <ruleCode>` 查报名数。
2. `sport stats participants --rule-code <ruleCode>`（可加 `--competition-code` 指定期次）查参与人数。
3. `sport stats reached --rule-code <ruleCode> --competition-code <code> --query-date <yyyyMMdd> --rank-type DAY` 查达标人数（口径 DAY/WEEK/MONTH，不支持 TOTAL）。
4. `sport stats trend --rule-code <ruleCode>` 查开赛至今逐日参与/达标趋势（最多 90 天），返回 null/空数组表示参数非法或期次未开始。

### 比赛编辑、删除与审批

- 编辑：先 `competition rule-detail` 读取当前值，再按用户确认修改，执行 `competition update --request '<完整对象>'`（可用 `--request @文件`）。不自动查详情、不自动合并字段或重放写操作。
- `request` 是全量赋值：CLI 要求显式给出 `ruleCode`、`competitionName`、`coverImageUrl`、`competitionType`、`firstPeriodStartDate`、`firstPeriodEndDate`、`periodType`、`cycleCount`、`goalValue`、`targetDayValue`、`dailyLimit`、`extra` 共12键；可空字段写 JSON null，不是省略即“不修改”。数字传 JSON number，不能带引号；不传 sportType/报名开关/操作人身份。完整请求缺键时先查询或确认，不能自行补默认值。
- FIXED 循环字段为 null 且须结束日期；RECURRING 须周期 WEEK/MONTH/CUSTOM 与1~12期，CUSTOM 须结束日期，MONTH 开始日≤28。该编辑接口的循环能力不意味着旧 create 接口已支持 RECURRING。
- 未开始：应用可修改字段并重建实例；进行中：只应用名称、头图、期数，其他结构字段被忽略；已结束：可能静默成功但无变更。编辑后回查详情确认生效，不把 success=true 直接解释成修改完成。
- 删除：仅创建人可操作，不可恢复；未开始物理删、进行中截断日期并软删、已结束软删。删除后比赛查询不可见，历史打卡明细仍可查；重复删除报“活动已删除”，不能自动重试或用删除重建替代编辑。
- 审批：仅绑定群群主可审批待审批比赛。`--approved=true` 同意并补发开赛广播，`--approved=false` 拒绝；必须显式提供结论。审批有真实副作用，勿重复提交。
- update/audit 受灰度限制，非灰度企业可能报“系统繁忙”；delete/invite 不受该灰度限制，不因此忽略权限校验。

### 数据导出

- 比赛导出：确认真实 competitionCode 与导出范围后提交 `competition export`。`--export-scope` 缺省 ALL；STAFF 时必须有 `--staff-ids`（逗号分隔）。实际权限是比赛创建人，不因报错文案写群主/管理员而放宽；比赛未开始不可导出，缺省每人每天最多5次。
- 导出受理为 `outcome=pending`，`data.result` 是数字 taskId，保留精度，用 `competition export-task --task-id <真实ID>` 查询同一任务及相同企业/profile。PROCESSING 继续 pending，SUCCESS 才代表文件生成，FAILED 报错；不能重提导出来代替查询。
- `data.result.ossUrl` 和 `dailyOssUrls` 是需钉钉登录态的钉盘链接，原样交付；dailyOssUrls 可以为 null，超过30天可能按自然月拆分。不要自行下载、解析或猜测链接。
- 步数报表：`step export --stat-date <开始日> --end-date <结束日>`，含首尾最多31天且结束日须早于今天，CLI 以本机日期预检，最终以服务端为准。可用 `--staff-ids` 指定工号，不传/空串为全员；仅空白/逗号不是有效名单。
- 步数报表返回 success 仅表示已受理，无 taskId、无查询接口；告诉用户留意操作人钉钉文件，不在对话里等待文件。报表为一人一天一行的步数明细，不含排名。生产仅企业主/子管理员，预发通过不证明生产权限；缺省企业级10分钟冷却，不自动重复提交。

## 命令目录

| 命令 | 必填参数 | 用途 |
|---|---|---|
| `dws sport org status` | 无 | 查询当前企业是否开通步数榜 |
| `dws sport org admin` | 无 | 判断当前用户是否企业管理员（失败不等于非管理员） |
| `dws sport step org-info` | `--stat-date` | 企业当日步数概况（人数、总步数） |
| `dws sport step org-total` | `--begin-date`, `--end-date` | 企业区间总步数 |
| `dws sport step emp-ranking` | `--stat-date`, `--stat-type` | 员工步数排行（D/W/M），cursor 分页 |
| `dws sport step dept-ranking` | `--stat-date`, `--stat-type`(W/M) | 部门步数排行（无日榜） |
| `dws sport step dept-emp-ranking` | `--dept-id`, `--stat-date`, `--stat-type` | 指定部门内员工排行 |
| `dws sport step emp-info` | `--user-id`, `--stat-date` | 单员工单日步数扩展信息 |
| `dws sport step emp-infos` | `--user-id`, `--stat-dates` | 单员工多日（1~30 天）步数明细 |
| `dws sport step dept-infos` | `--dept-id`, `--stat-dates` | 部门多日（1~30 天）步数明细 |
| `dws sport competition create` | `--user-id`, `--name`, `--sport-type`, `--competition-type`, `--start-date`, `--end-date`, `--goal-value`, `--target-day-value`, `--group-id`/`--open-conversation-id` 至少一个, `--need-signup`, `--allow-free-join`, `--cover-image-url` | 创建 FIXED 比赛（写操作，需 `--yes`） |
| `dws sport competition list` | 无（`--cursor` 首页0，`--size` 1~100） | 企业启用中的比赛配置分页 |
| `dws sport competition my-list` | 无（`--list-scope` 缺省 JOINED，`--size` 1~50） | 我的比赛规则分页 |
| `dws sport competition my-can-join-count` | 无 | 我可参与的比赛数 |
| `dws sport competition my-participated` | 无（`--page-size` 1~50） | 我参与过的比赛期次分页 |
| `dws sport competition my-participated-count` | 无 | 我参与过的比赛总数 |
| `dws sport competition activity-detail` | `--rule-code`（`--competition-code` 可选） | 活动详情与我的报名状态 |
| `dws sport competition my-achievement` | `--competition-code` | 我的达标进度与本周记录 |
| `dws sport competition rank-top` | `--rule-code`, `--rank-type` | TopN 榜（`--top-n` 1~50） |
| `dws sport competition ranking` | `--competition-code` | 全员排行榜分页（`--size` 1~50） |
| `dws sport competition config` | `--rule-code` | 比赛配置 |
| `dws sport competition rule-detail` | `--rule-code`, `--user-id` | 比赛规则详情（创建人身份） |
| `dws sport competition instances` | `--rule-code` | 比赛期次列表 |
| `dws sport competition current-instance` | `--rule-code` | 当前进行中期次 |
| `dws sport competition next-instance` | `--rule-code` | 下一期次 |
| `dws sport competition announce` | `--rule-code`, `--user-id` | 向绑定群播报开赛（写操作，无幂等，需 `--yes`） |
| `dws sport competition invite` | `--rule-code`，`--receiver-user-id` / `--target-group-id` 至少一个 | 邀请私聊/目标群（需确认） |
| `dws sport competition update` | `--request` | 全量编辑12键对象（仅创建人，需确认） |
| `dws sport competition delete` | `--rule-code` | 删除比赛（不可恢复，需确认） |
| `dws sport competition audit` | `--rule-code`, `--approved=true/false` | 群主审批（同意有广播副作用，需确认） |
| `dws sport competition export` | `--competition-code`；STAFF 时 `--staff-ids` | 异步提交导出（需确认） |
| `dws sport competition export-task` | `--task-id` | 查询任务，不重复提交 |
| `dws sport step export` | `--stat-date`, `--end-date` | 提交步数报表，钉钉异步投递（需确认） |
| `dws sport stats signup` | `--rule-code` | 报名人数 |
| `dws sport stats participants-daily` | `--rule-code`（`--stat-date` 可选，缺省今天） | 指定日参与人数 |
| `dws sport stats participants` | `--rule-code`（`--competition-code` 可选） | 累计参与人数 |
| `dws sport stats reached` | `--rule-code`, `--competition-code`, `--query-date`, `--rank-type` | 达标人数 |
| `dws sport stats trend` | `--rule-code`（`--competition-code` 可选） | 开赛至今逐日参与/达标趋势 |

## 权限、确认与错误处理

- 读命令 effect=read、risk=low。create/announce/invite/update/audit/competition export/step export 为需确认的无幂等写操作；delete 为 destructive/high，删除状态幂等但重复调用会报错。均在获得用户确认后才加 `--yes`；预览仅在 Schema 明确支持时使用 `--dry-run`，不得把预览当真实执行。
- 普通成员创建的比赛返回 `status=0` 待群主审批，不是失败；如实转告用户等待审批。
- `QW_COMPETITION_NOT_FOUND`（比赛配置不存在）：核对 `--rule-code` 是否来自真实创建/查询返回。`QW_COMPETITION_INSTANCE_NOT_FOUND`（没有可统计的期次）：ruleCode 存在但本期已结束或尚未开始，应如实说明，不要改 ruleCode 重试。`QW_COMPETITION_QUERY_FAILED` 的“活动不存在”：核对 ruleCode/competitionCode。
- `QW_COMPETITION_EXPORT_TASK_NOT_FOUND`：核对真实 taskId 与企业/profile；不要重新提交导出。`QW_IDENTITY_LOOKUP_FAILED`：身份查询服务异常；注入身份异常应核对平台配置，不通过业务 flag 伪造身份。
- 服务端 `SERVICE_NOT_FOUND` 或新工具未发布/映射未就绪：保留错误供服务端排查，不修改参数试探；写操作不自动重放。
- 401/认证失败：先 `dws auth status` 检查登录态，不要调整端点或绕过白名单。

## 命令与 MCP 能力映射

仅用于诊断和维护，不应绕过 CLI 直接调用（server ID=sport）：

| CLI 命令 | MCP 工具 |
|---|---|
| `org status` | `get_org_step_participate_status` |
| `org admin` | `is_org_admin` |
| `step org-info` / `step org-total` | `get_org_health_step_info` / `get_org_total_step_count` |
| `step emp-ranking` / `dept-ranking` / `dept-emp-ranking` | `list_emp_step_ranking` / `list_dept_step_ranking` / `list_emp_ranking_in_dept` |
| `step emp-info` / `emp-infos` / `dept-infos` | `get_emp_step_ext_info` / `get_emp_step_infos` / `get_dept_step_infos_by_dept_id` |
| `competition list` / `rank-top` / `ranking` | `list_competition_configs` / `get_competition_rank_top_n` / `list_competition_rank` |
| `competition my-list` / `my-can-join-count` / `activity-detail` | `list_my_competition_rules` / `count_my_can_join_competitions` / `get_competition_activity_detail` |
| `competition my-achievement` / `my-participated` / `my-participated-count` | `get_personal_achievement` / `list_my_participated_comp` / `count_my_participated_comp` |
| `competition create` / `config` / `rule-detail` | `create_competition` / `get_competition_config` / `get_competition_rule_detail` |
| `competition instances` / `current-instance` / `next-instance` | `list_competition_instances` / `get_current_competition_instance` / `get_next_competition_instance` |
| `competition announce` | `send_competition_group_msg` |
| `competition invite` / `update` / `delete` / `audit` | `send_competition_invite_msg` / `update_competition` / `delete_competition` / `audit_competition` |
| `competition export` / `export-task` | `submit_competition_export` / `get_competition_export_task` |
| `step export` | `submit_step_report_export` |
| `stats signup` / `participants-daily` / `participants` / `reached` / `trend` | `get_signup_count` / `count_daily_participants` / `count_total_participants` / `count_reached_users` / `list_daily_participation_trend` |
