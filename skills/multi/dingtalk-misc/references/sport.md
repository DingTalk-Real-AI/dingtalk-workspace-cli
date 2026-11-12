# 企业运动健康（钉钉运动）命令参考

命令前缀统一为 `dws sport`，属于 dingtalk-misc 长尾能力，不在 `dws --help` 展示；仍可直接执行 `dws sport --help` 和各叶子命令。所有调用追加 `--format json`。命令树已发布到 Agent Schema；参数、约束或确认语义不确定时，优先读取精确 compact leaf Schema（`dws schema "sport ..." --compact`），仅在 Schema 不可用或与 Cobra 实际行为发生漂移时读取同一叶子的 `--help`。不要直接调用 MCP、HTTP API 或自行猜测工具名。

## 能力范围与前置条件

覆盖企业钉钉运动步数榜查询与运动比赛（挑战赛）管理：开通状态、企业/部门/员工步数排行与多日明细、比赛创建与期次查询、开赛群播报、报名/参与/达标/趋势统计。

前置条件：企业已在钉钉客户端开通运动步数榜；比赛功能需要企业已开通运动健康服务。调用身份为当前登录用户所属企业。

Use when：用户要查企业步数榜开通状态、企业步数概况与总步数、员工/部门步数排行、某人或某部门多日步数明细，或创建步数/跑步/综合运动挑战赛、查比赛配置规则期次、开赛群播报、统计比赛报名/参与/达标/趋势。

Avoid when：

- 个人健康数据（非企业步数榜）或钉钉运动客户端内设置，不在本能力范围。
- 群创建、群成员管理、普通群消息发送走 `dws chat`；比赛绑定群必须已存在，`--group-id` 从 `dws chat` 真实返回获取，禁止猜测。
- 找人获取 `--user-id` 对应的员工时走 `dws aisearch` / `dws contact`，用真实 staffId。
- 通知、审批等运营动作走各自产品；本能力只提供比赛播报命令。

## 意图路由

| 用户意图 | 命令族 |
|---|---|
| 查步数榜是否开通 | `dws sport org status` |
| 企业步数概况、区间总步数 | `dws sport step org-info` / `dws sport step org-total` |
| 员工/部门/部门内员工步数排行 | `dws sport step emp-ranking` / `dept-ranking` / `dept-emp-ranking` |
| 某员工/某部门多日步数明细 | `dws sport step emp-info` / `emp-infos` / `dept-infos` |
| 创建比赛、查配置/规则/期次 | `dws sport competition create` / `config` / `rule-detail` / `instances` / `current-instance` / `next-instance` |
| 开赛群播报 | `dws sport competition announce` |
| 报名/参与/达标/趋势统计 | `dws sport stats signup` / `participants-daily` / `participants` / `reached` / `trend` |

## 执行规则

1. 先定位真实对象。`--user-id` 用当前会话已验证的员工 staffId（纯数字）；`--dept-id` 必须取自 `dept-ranking` 真实返回；`--rule-code` / `--competition-code` 必须取自创建返回或查询命令真实结果，禁止按名称猜测。
2. 日期一律 `yyyyMMdd`；周榜/月榜的 `--stat-date` 传周期内任意一天；多日明细 `--stat-dates` 逗号分隔、1~30 个。
3. `org status` 返回 false 表示企业未开通步数榜，是正常业务状态：排行接口会返回空列表，向用户说明需在钉钉客户端开通，不要反复重试。
4. 排行分页用 `--cursor`（首页 0）+ `--size`（1~100），按返回的 nextCursor 续翻到满足范围或服务端结束。
5. `competition create` 是写操作且创建后无法删除，`confirmation=user_required`、`non_idempotent`：执行前必须把全部参数逐项列给用户确认，确认后才加 `--yes`；禁止用试探性参数反复创建。
6. `competition announce` 是写操作且无幂等：同一 ruleCode 重复播报会重复打扰群成员，执行前必须获得用户确认。
7. 创建或查询后优先用返回的真实 `ruleCode` 回读 `competition config` 验证。

## 比赛创建约束（CLI 本地校验）

- 当前仅支持 `--competition-type FIXED`（固定不循环）。RECURRING 因服务端 Schema 将 `groupId`、`firstPeriodEndDate` 列为无条件必填而工具描述仍称循环模式可省结束日期，契约冲突期间 CLI 侧直接阻断，不要用空值或猜测日期绕过。
- `--start-date` / `--end-date` 必须是真实日期（含闰年校验）；结束不得早于开始。
- `--target-day-value` 不得超过单期实际天数。
- `--daily-limit`（可选）必须 ≥ `--goal-value`。
- `--cover-image-url` 优先使用官方默认头图；`--group-id` 群必须属于当前企业。

## 标准流程

### 查询企业步数数据

1. `sport org status` 确认开通状态。
2. `sport step org-info --stat-date <yyyyMMdd>` 查当日概况，或 `sport step org-total --begin-date <yyyyMMdd> --end-date <yyyyMMdd>` 查区间总量。
3. 排行用 `sport step emp-ranking --stat-date <yyyyMMdd> --stat-type D --cursor 0 --size 20`（D=日/W=周/M=月；部门榜仅 W/M）。
4. 多日明细用 `emp-infos --user-id <staffId> --stat-dates 20261101,20261102,...`。

### 创建挑战赛

1. 从 `dws chat` 拿真实群会话 `--group-id`，与用户确认比赛名称、类型（WALK 走路=步 / RUN 跑步=米 / COMPREHENSIVE 综合=千卡）、起止日期、每日目标与达标天数。
2. 向用户逐项确认全部参数，确认后执行 `sport competition create ... --yes`。
3. 保存返回的 `ruleCode`；`status=1` 表示已生效（群主/管理员创建），`0` 表示待群主审批（普通成员创建），据实告知。
4. 用 `sport competition config --rule-code <ruleCode>` 回读验证。

### 比赛运营统计

1. `sport stats signup --rule-code <ruleCode>` 查报名数。
2. `sport stats participants --rule-code <ruleCode>`（可加 `--competition-code` 指定期次）查参与人数。
3. `sport stats reached --rule-code <ruleCode> --competition-code <code> --query-date <yyyyMMdd> --rank-type DAY` 查达标人数（口径 DAY/WEEK/MONTH，不支持 TOTAL）。
4. `sport stats trend --rule-code <ruleCode>` 查开赛至今逐日参与/达标趋势（最多 90 天），返回 null/空数组表示参数非法或期次未开始。

## 命令目录

| 命令 | 必填参数 | 用途 |
|---|---|---|
| `dws sport org status` | 无 | 查询当前企业是否开通步数榜 |
| `dws sport step org-info` | `--stat-date` | 企业当日步数概况（人数、总步数） |
| `dws sport step org-total` | `--begin-date`, `--end-date` | 企业区间总步数 |
| `dws sport step emp-ranking` | `--stat-date`, `--stat-type` | 员工步数排行（D/W/M），cursor 分页 |
| `dws sport step dept-ranking` | `--stat-date`, `--stat-type`(W/M) | 部门步数排行（无日榜） |
| `dws sport step dept-emp-ranking` | `--dept-id`, `--stat-date`, `--stat-type` | 指定部门内员工排行 |
| `dws sport step emp-info` | `--user-id`, `--stat-date` | 单员工单日步数扩展信息 |
| `dws sport step emp-infos` | `--user-id`, `--stat-dates` | 单员工多日（1~30 天）步数明细 |
| `dws sport step dept-infos` | `--dept-id`, `--stat-dates` | 部门多日（1~30 天）步数明细 |
| `dws sport competition create` | `--user-id`, `--name`, `--sport-type`, `--competition-type`, `--start-date`, `--end-date`, `--goal-value`, `--target-day-value`, `--group-id`, `--need-signup`, `--allow-free-join`, `--cover-image-url` | 创建 FIXED 比赛（写操作，需 `--yes`） |
| `dws sport competition config` | `--rule-code` | 比赛配置 |
| `dws sport competition rule-detail` | `--rule-code`, `--user-id` | 比赛规则详情（创建人身份） |
| `dws sport competition instances` | `--rule-code` | 比赛期次列表 |
| `dws sport competition current-instance` | `--rule-code` | 当前进行中期次 |
| `dws sport competition next-instance` | `--rule-code` | 下一期次 |
| `dws sport competition announce` | `--rule-code`, `--user-id` | 向绑定群播报开赛（写操作，无幂等，需 `--yes`） |
| `dws sport stats signup` | `--rule-code` | 报名人数 |
| `dws sport stats participants-daily` | `--rule-code`（`--stat-date` 可选，缺省今天） | 指定日参与人数 |
| `dws sport stats participants` | `--rule-code`（`--competition-code` 可选） | 累计参与人数 |
| `dws sport stats reached` | `--rule-code`, `--competition-code`, `--query-date`, `--rank-type` | 达标人数 |
| `dws sport stats trend` | `--rule-code`（`--competition-code` 可选） | 开赛至今逐日参与/达标趋势 |

## 权限、确认与错误处理

- 读命令 effect=read、risk=low；`competition create` 与 `competition announce` effect=write、confirmation=user_required、idempotency=non_idempotent，Runtime gate 要求时必须加 `--yes`。
- 普通成员创建的比赛返回 `status=0` 待群主审批，不是失败；如实转告用户等待审批。
- `QW_COMPETITION_NOT_FOUND`（比赛配置不存在）：核对 `--rule-code` 是否来自真实创建/查询返回。
- 服务端偶发 `SERVICE_NOT_FOUND`（HSF 服务未注册）为 MCP 后端环境问题：保留 trace_id 重试一次，仍失败时向用户说明需服务端排查，不要修改参数反复重试。
- 401/认证失败：先 `dws auth status` 检查登录态，不要调整端点或绕过白名单。

## 命令与 MCP 能力映射

仅用于诊断和维护，不应绕过 CLI 直接调用（server ID=sport）：

| CLI 命令 | MCP 工具 |
|---|---|
| `org status` | `get_org_step_participate_status` |
| `step org-info` / `step org-total` | `get_org_health_step_info` / `get_org_total_step_count` |
| `step emp-ranking` / `dept-ranking` / `dept-emp-ranking` | `list_emp_step_ranking` / `list_dept_step_ranking` / `list_emp_ranking_in_dept` |
| `step emp-info` / `emp-infos` / `dept-infos` | `get_emp_step_ext_info` / `get_emp_step_infos` / `get_dept_step_infos_by_dept_id` |
| `competition create` / `config` / `rule-detail` | `create_competition` / `get_competition_config` / `get_competition_rule_detail` |
| `competition instances` / `current-instance` / `next-instance` | `list_competition_instances` / `get_current_competition_instance` / `get_next_competition_instance` |
| `competition announce` | `send_competition_group_msg` |
| `stats signup` / `participants-daily` / `participants` / `reached` / `trend` | `get_signup_count` / `count_daily_participants` / `count_total_participants` / `count_reached_users` / `list_daily_participation_trend` |
