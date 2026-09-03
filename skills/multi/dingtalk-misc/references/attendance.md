# 考勤查询、规则与设置

本 Reference 处理打卡流水/结果、个人详情、签到、审批记录与入口、班次定义、考勤组、考勤规则、个人/全局设置和管理员修正。

以下场景不要先读本文件：

- 排班导入、调班、排休、排班表 Excel → [attendance-schedule.md](attendance-schedule.md)
- 考勤/月度/每日/明细/审批记录/签到 Excel → [attendance-report.md](attendance-report.md)
- 假期类型、余额、变更流水、假期规则或余额导出 → [attendance-vacation.md](attendance-vacation.md)

## 最小执行契约

- 已知路由和参数足够时直接执行，结构化结果统一加 `--format json`。不要先查产品级 Schema、Shortcut 列表或 Help。
- 只有参数/约束/确认语义不确定时查一次精确 leaf Schema；只有真实出现 `unknown command` / `unknown flag` 时查一次精确 leaf Help。禁止连续试探同义命令。
- 姓名先用 `dws aisearch person --query "<姓名>" --dimension name --format json` 解析；零命中、多候选或跨 profile 时停止，禁止选择第一项。后续只复用本轮真实返回的 userId/groupId/classId/ruleId/leaveCode。
- profile、组织和操作者身份从目标解析到最终读取/写入必须保持一致。宿主或 CLI 自动注入的 `corpId` / `opUserId` 不向用户索要；精确 leaf 明确要求的操作者字段除外。
- 写操作是否确认以 Runtime/精确 Schema 的 `confirmation` 为准。`user_required` 时先读当前值、展示目标和差异、获得确认，再执行并回读；存储示例不预填 `--yes`。
- 退出码 0 或传输成功不等于业务成功；检查真实业务结果、稳定 ID、分页完整性和失败项。可能已提交的写请求不得盲目重试。
- 性能优化只减少重复发现和重复请求，不删减用户要求的记录、字段、分页、失败项或验证。窄投影必须保留稳定 ID、请求所需业务字段、业务状态和完整性元数据；最终回答必须自包含，不用“以上/如上”代替结果。

## Golden Route

### 记录、结果与统计

| 用户意图 | 推荐入口 | 关键参数/边界 |
|---|---|---|
| 某人某天个人考勤详情 | `dws attendance record get` | `--user <userId> --date YYYY-MM-DD`；返回详情，不替代跨日统计 |
| 原始打卡流水：实际时间、地点、定位方式 | `dws attendance +check-record` | `--users <ids> --start YYYY-MM-DD --end YYYY-MM-DD`；区间不超过 1 个月 |
| 打卡判定：Normal/Late/Early/NotSigned/Absenteeism | `dws attendance +check-result` | 同上；最多 100 人；`--limit 1..1000 --offset >=0` |
| 周/月摘要 | `dws attendance summary` | `--user <id> --date <日期> --stats-type week\|month` |
| 当天适用考勤组、范围与规则 | `dws attendance rules` | `--date YYYY-MM-DD` 或 `YYYY-MM-DD HH:mm:ss` |
| 未来 7 天内计划班次 | `dws attendance shift list` | `--users <ids> --start YYYY-MM-DD --end YYYY-MM-DD`；最多 50 人、最多 7 天 |
| 已导入的排班记录 | `dws attendance schedule get` | `--users <ids> --start <日期> --end <日期>`；只查看/对照时直接用，不强制导出 Excel |
| 签到/外勤签到记录 | `dws attendance checkin records` | `--operator-corp-id`、`--operator-staff-id`、`--staff-ids`、`--start/--end "YYYY-MM-DD HH:mm:ss"`；最多 100 人、最多 7 天 |

`签到` 是外勤签到，走 `checkin records`；`打卡流水` 走 `+check-record`；`打卡结果/迟到/缺卡/旷工` 走 `+check-result`。三者不能互换。

分页规则：

- 用户明确要“前 50 条再下一页”时，先 `+check-result --limit 50 --offset 0`，再使用返回 continuation/下一 offset；不要重复第一页。
- “全部/完整”需要继续到 `meta.pagination.endpoint_exhausted=true`；达到边界但未耗尽时返回 next token，不得宣称完整。
- 无流水只是“无打卡数据”，不等于正常，也不等于缺卡。缺卡/旷工以 `+check-result` 的判定为准；完全无记录单独列出。

### 考勤审批

| 用户意图 | 推荐入口 | 关键参数/边界 |
|---|---|---|
| 查已提交的请假/加班/出差外出/补卡记录 | `dws attendance +list-approve` | `--users <ids> --types overtime,trip,leave,patch --start YYYY-MM-DD --end YYYY-MM-DD` |
| 获取可提交表单入口 | `dws attendance +get-approve-template` | `--type leave\|overtime\|repair-check\|travel\|out`；只返回入口，不代填提交 |
| 查可用假期类型与余额（请假套件前置） | `dws attendance approve leave-types` | `--user <userId>` 可选；返回 `leaveCode`/`leaveViewUnit`/余额；`bizType` 不可靠，哺乳假等自定义类型按 `leaveName` 识别 |
| 计算请假时长（服务端口径） | `dws attendance approve leave-duration` | `--leave-code --start --end` 必填；时长禁止本地估算 |
| 请假提交前资格校验 | `dws attendance approve leave-check` | 时长必须取自 leave-duration 输出；`success=false` 非零退出并转告 `errorMsg` 后终止 |
| 匹配补卡目标异常班次 | `dws attendance approve supply-plans` | `--time "YYYY-MM-DD HH:mm"` 必填；`plans` 空数组属正常结果；多班次须用户选择 |
| 补卡提交前资格校验 | `dws attendance approve supply-check` | `--timestamp` 必须取自 supply-plans 输出的 `supplyDate`；`qualify=false` 非零退出并转告 `title`/`desc` 后终止 |

查询接口把出差和外出合并为 `trip`；表单入口必须区分外出 `travel` 与出差 `out`。多个模板全部返回，将名称最匹配者置前，并用 `[表单名称](submitUrl)` 展示，禁止裸露 URL 或只返回一个模板。

请假与补卡的**提交**意图路由到 oa 域套件工作流（见 [oa.md](oa.md)「发起请假审批」/「发起补卡审批」章节）；上表 `approve` 命令为其前置只读查询/校验。`+get-approve-template` 的 `submitUrl` 入口用于加班/外出/出差，以及不支持 CLI 发起的请假/补卡模板场景。

### 补卡规则、加班规则与班次定义

| 用户意图 | 推荐入口 | 关键参数/边界 |
|---|---|---|
| 补卡规则候选 ID/名称 | `dws attendance +search-adjustment-rule` | `--query <名称> --page <n> --limit <=200`；投影只保证 ID/名称 |
| 补卡规则完整列表字段 | `dws attendance adjustment search` | 同样使用 `--query/--page/--limit`；需要有效期、次数、适用范围时用此原子入口 |
| 已知补卡规则 ID 取详情 | `dws attendance adjustment get --adjustment-id <id>` | 当前下游可能返回空详情；空 result 必须报告 unavailable，不把搜索摘要冒充详情 |
| 加班规则候选 ID/名称 | `dws attendance +search-overtime-rule` | `--query <名称> --page <n> --limit <=200` |
| 加班规则完整列表字段 | `dws attendance overtime search` | 比较工作日/休息日/节假日计算方式时使用 |
| 已知加班规则 ID 取完整详情 | `dws attendance +get-overtime-rule --overtime-id <id>` | 响应 ID 必须匹配请求 ID |
| 班次候选 ID/名称 | `dws attendance +search-class` | `--query <名称> --filter-type ALL\|MINE_OWN --page <n> --limit <=200`；窄投影 |
| 班次完整配置列表 | `dws attendance class search` | 需要上下班、休息段或完整配置时使用；分页参数仍是 `--page/--limit` |
| 已知班次 ID 精确详情 | `dws attendance class get --class-id <id>` | 仅对当轮真实 ID 使用 |

按名称搜索时，严格使用 `--query`，不要猜 `--name` 或 `--page-size`。用户指定“前两页”时只读 page 1 和 page 2；若需唯一目标，零命中停止，多候选先消歧。

### 考勤组与设置

| 用户意图 | 推荐入口 | 关键参数/边界 |
|---|---|---|
| 搜索考勤组 | `dws attendance group search` | `--query`、`--type FIXED\|TURN\|NONE`、`--page/--limit`；按需 `--query-position/--query-ble` |
| 考勤组完整配置 | `dws attendance group get --group-id <id>` | 完整配置、负责人、班次与范围 |
| 只取成员/地址/Wifi/蓝牙 | `dws attendance group filtered-get` | `--group-id` 加 `--member/--position/--wifi/--bles`；避免全量读取 |
| 当前登录用户本人的个人提醒/极速打卡/通知设置 | `dws attendance selfsetting get` | `--user <本人 userId> --setting-scene <scene>`；不支持查询其他员工，管理员也不能代查 |
| 企业全局设置（管理员） | `dws attendance globalsetting get` | `--scope 企业\|全公司\|所有人 --setting-scene <scene>` |
| 简单报表字段查询（非 Excel） | `dws attendance report columns` → `report query-data` | 先取有权字段 ID；`query-data` 最多 20 人、32 天 |
| 简单假期报表查询（非 Excel） | `dws attendance report query-leave` | `--users --leave-names --start/--end "YYYY-MM-DD HH:mm:ss"` |

`setting-scene` 只取：`checkRemind`、`fastCheck`、`checkResultNotify`、`lackRemind`、`personalAttendStatNotify`、`bossAttendStatNotify`。只查询用户指定的 scene，不默认把六类全部读取。

`selfsetting get/save` 仅支持当前登录用户本人，`--user` 必须是本人 userId。目标为其他员工时立即报告能力不支持；禁止调用 selfsetting，禁止改查当前用户，禁止用 globalsetting 冒充替代，也不要 Help 或重复尝试。能力缺陷允许正确阻塞，不为追求成功率绕过服务端边界。

## 写操作最短路径

下列命令均为 `confirmation=user_required`：

| 操作 | 命令 | 写前/写后要求 |
|---|---|---|
| 创建班次 | `attendance class create --name --class-vo` | `class-vo.sections[].times` 明确上下班；先按同名 search 防重复，写后按返回 ID/唯一名称回读 |
| 更新班次 | `attendance class update --class-id [--name/--owner/--class-vo]` | 先 `class get` 保存完整原值；只改目标字段；写后回读 |
| 创建考勤组 | `attendance group create --name --type [--owner/--group-vo]` | 同名 search 防重复；FIXED 的 `group-vo` 必须含 `defaultClassId` 和 7 天 `workDayClassList` |
| 更新考勤组 | `attendance group update --group-id [--name/--type/--owner/--enable-outside-check/--classIds/--group-vo]` | 先 `group get` 保存原值；复杂对象以当前配置为基线，禁止凭空全量覆盖 |
| 更新成员 | `attendance group update-members --group-id` 加至少一个增删 flag | 每类最多 20 个 ID；写前 `filtered-get --member`，写后回查目标差异 |
| 更新本人个人设置 | `attendance selfsetting save --user <本人 userId> --setting-scene <scene> <scene字段>` | 仅当前登录用户本人；先同 scene `get`；禁止把不同 scene 字段混用 |
| 更新全局设置 | `attendance globalsetting save --scope --setting-scene <scene> <scene字段>` | 仅管理员；先同 scope/scene `get`，写后回读 |
| 管理员修正打卡 | `attendance boss-check --plan-id <id>` 或 `--result-id <id>` | 两种 ID 至少一个；先定位唯一记录并展示 `当前值 → 新值`，写后重新查询结果 |

临时修改并恢复时：写前保存完整原配置和稳定 ID；一次性展示“修改 + 验证 + 恢复”计划并获得明确确认；修改后回读，再用保存值恢复并再次回读。中途验证失败也要尝试恢复；恢复失败必须报告 partial/commit-unknown，不能宣称任务完成。

复杂 payload、枚举和罕见字段只在精确 leaf Schema 中读取，禁止为已知简单 flag 加载产品级 Schema。班次/考勤组复杂 JSON 必须基于写前真实对象做最小变更。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "attendance +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws attendance <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service attendance --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws attendance +check-record` | read | 查询用户打卡流水（打卡时间/地点/定位方式） |
| `dws attendance +check-result` | read | 查询用户打卡结果（迟到/早退/缺卡等） |
| `dws attendance +get-approve-template` | read | 查询补卡/请假/加班/外出/出差审批提交链接 |
| `dws attendance +get-overtime-rule` | read | 根据加班规则主键 ID 查询加班规则详情 |
| `dws attendance +list-approve` | read | 查询用户考勤审批单（补卡/加班/请假/出差外出） |
| `dws attendance +search-adjustment-rule` | read | 查询当前用户可管理的补卡规则列表 |
| `dws attendance +search-class` | read | 查询当前用户可管理的班次详情列表 |
| `dws attendance +search-overtime-rule` | read | 查询当前用户可管理的加班规则列表 |
<!-- VISIBLE_SHORTCUTS_END -->

## 复用脚本

- [attendance_my_record.py](../scripts/attendance_my_record.py)：自动解析当前用户，查询今天或指定日期的本人考勤记录；适合“查我今天考勤”这类单人快捷查询。
- [attendance_team_shift.py](../scripts/attendance_team_shift.py)：按明确的 userId 列表查询团队日期范围内的排班；默认本周一至周五，最多 50 人。

脚本已覆盖身份解析、日期默认值或批量边界时直接运行；不要先手写同一批查询再让脚本重复调用。

## 日期、结果与聚合

- 本周=周一到周日；本月=1 日到真实月末；用户给定范围时原样使用。按当前会话时区计算，禁止硬编码。
- `check/approve/shift/schedule/vacation records` 使用 `YYYY-MM-DD`；`checkin/report` 使用 `YYYY-MM-DD HH:mm:ss`。不要混用。
- 超过 3 条记录的求和、分组、计数、排序或跨字段核对使用本地脚本处理，保留原始单位；不要靠目测。仅当用户要求 Excel 时切换专项报表脚本。
- 返回中没有某人/某日时标记“无记录”；`NotSigned`、`Absenteeism` 分别按真实结果展示；不把缺失数据补成 0 或 Normal。
- 对“最近一个有记录的日期”，从实际返回的日期字段取最大值，再把这个日期传给后续 `rules`/`record get`/`shift list`；禁止用今天或区间结束日代替。

## 错误最短路径

1. 零命中、多候选、目标类型不明：停止后续写入，展示候选或要求用户明确；禁止默认第一项。
2. `unknown flag`：只查该 leaf Help 并修正一次；`unknown command`：只查一次精确父级/Shortcut 清单，禁止猜后缀。
3. 参数或确认不清楚：只查一次精确 leaf compact Schema，字段投影限制为 `parameters,constraints,confirmation`；禁止产品级或 `--all`。
4. 权限错误：说明需要的管理员/管理范围，不切 profile、不改目标、不用底层接口绕过。
5. 只读后端依赖错误可保持原参数重试一次；写操作在无法证明未开始时先回读对账，禁止自动重放。
6. 返回 `null`、缺稳定 ID、响应 ID 不匹配或分页停滞：按真实失败/不完整处理，不包装成空成功。

## 跨产品边界

- 考勤类请假/加班/补卡/出差外出记录和提交入口属于 attendance；通用 OA 审批实例状态、同意/拒绝/转交属于 `dingtalk-misc` 的 OA Reference。
- 人名解析优先 `dingtalk-aisearch`；已知 userId 的批量姓名/部门补齐用 `dingtalk-contact`。
- 普通活动签到、日历会议、持续事件监听不属于 attendance；分别路由日历/事件产品。
