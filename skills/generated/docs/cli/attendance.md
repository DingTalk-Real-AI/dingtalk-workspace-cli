# Canonical Product: attendance

Generated from shared Tool IR. Do not edit by hand.

- Display name: 钉钉考勤打卡
- Description: 考勤打卡MCP，支持查询考勤统计数据、排班信息等
- Server key: `33b1011e8b8382a6`
- Endpoint: `https://mcp-gw.dingtalk.com/server/72c8e63fa17cae0ea5bf507e2594d56c7b286122a747a9a28d4c30ac430cc774`
- Protocol: `2025-03-26`
- Degraded: `false`

## Tools

- `shift list`
  - Path: `attendance.batch_get_employee_shifts`
  - CLI route: `dws attendance shift list`
  - Description: 批量查询多个员工在指定日期的考勤班次信息，返回每条记录包含：用户 ID（userId）、工作日期（workDate，毫秒时间戳）、打卡类型（checkType，如 OnDuty 表示上班）、计划打卡时间（planCheckTime，毫秒时间戳）以及是否为休息日（isRest，"Y"/"N"）。结果基于组织考勤配置生成，仅返回调用者有权限查看的员工数据，适用于排班核对、考勤预览等场景。
  - Flags: `--start`, `--end`, `--users`
  - Schema: `skills/generated/docs/schema/attendance/batch_get_employee_shifts.json`
- `summary`
  - Path: `attendance.get_attendance_summary`
  - CLI route: `dws attendance summary`
  - Description: 获取考勤统计摘要
  - Flags: `--date`, `--user`
  - Schema: `skills/generated/docs/schema/attendance/get_attendance_summary.json`
- `record get`
  - Path: `attendance.get_user_attendance_record`
  - CLI route: `dws attendance record get`
  - Description: 查询指定用户在某一天的考勤详情，包括实际打卡记录（如上班/下班时间、是否正常打卡）、当日所排班次、所属考勤组信息、是否为休息日、出勤工时（如 "0Hours"）、加班时长等。返回数据受组织权限和隐私策略限制，仅当调用者有权限查看该用户考勤信息时才返回有效内容。适用于员工自助查询、HR 核对出勤或审批关联场景。
  - Flags: `--user`, `--date`
  - Schema: `skills/generated/docs/schema/attendance/get_user_attendance_record.json`
- `rules`
  - Path: `attendance.query_attendance_group_or_rules`
  - CLI route: `dws attendance rules`
  - Description: 查询考勤组/考勤规则："我属于哪个考勤组""我们的打卡范围是什么""弹性工时是怎么算的"
  - Flags: `--date`
  - Schema: `skills/generated/docs/schema/attendance/query_attendance_group_or_rules.json`
