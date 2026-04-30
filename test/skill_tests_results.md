# DWS Skill Test Results

## Summary

- **Total Test Cases**: 202
- **Passed**: 202
- **Failed**: 0
- **Pass Rate**: 100.0%

## Pass Rate by Product

| Product | Total | Passed | Failed | Pass Rate |
|---------|-------|--------|--------|-----------|
| aitable | 41 | 41 | 0 | 100.0% |
| attendance | 16 | 16 | 0 | 100.0% |
| calendar | 30 | 30 | 0 | 100.0% |
| chat | 31 | 31 | 0 | 100.0% |
| contact | 14 | 14 | 0 | 100.0% |
| devdoc | 5 | 5 | 0 | 100.0% |
| ding | 5 | 5 | 0 | 100.0% |
| report | 26 | 26 | 0 | 100.0% |
| todo | 30 | 30 | 0 | 100.0% |
| workbench | 4 | 4 | 0 | 100.0% |

## All Test Case Results

### aitable

**aitable_aitable_base_create_001** ✅ PASS

- Prompt: 创建 AI 表格，name 为 项目跟踪
- Expected: `dws aitable base create --name 项目跟踪 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base create)
- Flags: PASS (1 flags validated)

**aitable_aitable_base_create_002** ✅ PASS

- Prompt: 用模板tpl123创建一个AI表格叫项目管理
- Expected: `dws aitable base create --name "项目管理" --template-id tpl123 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base create)
- Flags: PASS (2 flags validated)

**aitable_aitable_base_delete_001** ✅ PASS

- Prompt: 删除 AI 表格
- Expected: `dws aitable base delete --base-id <BASE_ID> --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base delete)
- Flags: PASS (1 flags validated)

**aitable_aitable_base_delete_002** ✅ PASS

- Prompt: 删除AI表格base123，原因是项目已结项
- Expected: `dws aitable base delete --base-id base123 --reason "项目已结项" --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base delete)
- Flags: PASS (2 flags validated)

**aitable_aitable_base_get_001** ✅ PASS

- Prompt: 获取 AI 表格 base123 的信息
- Expected: `dws aitable base get --base-id base123 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base get)
- Flags: PASS (1 flags validated)

**aitable_aitable_base_get_002** ✅ PASS

- Prompt: 查看一下 ID 为 base456 的多维表格的详情
- Expected: `dws aitable base get --base-id base456 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base get)
- Flags: PASS (1 flags validated)

**aitable_aitable_base_list_001** ✅ PASS

- Prompt: 获取我的 AI 表格列表
- Expected: `dws aitable base list --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base list)
- Flags: N/A (no flags specified)

**aitable_aitable_base_list_002** ✅ PASS

- Prompt: 获取 AI 表格列表，每页5个
- Expected: `dws aitable base list --limit 5 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base list)
- Flags: PASS (1 flags validated)

**aitable_aitable_base_list_003** ✅ PASS

- Prompt: 获取 AI 表格列表，从游标 NEXT_CURSOR 继续翻页
- Expected: `dws aitable base list --cursor NEXT_CURSOR --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base list)
- Flags: PASS (1 flags validated)

**aitable_aitable_base_list_004** ✅ PASS

- Prompt: 分页获取 AI 表格列表，从 NEXT_CURSOR 开始每页5个
- Expected: `dws aitable base list --limit 5 --cursor NEXT_CURSOR --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base list)
- Flags: PASS (2 flags validated)

**aitable_aitable_base_search_001** ✅ PASS

- Prompt: 搜索 AI 表格，query 为 项目管理
- Expected: `dws aitable base search --query 项目管理 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base search)
- Flags: PASS (1 flags validated)

**aitable_aitable_base_search_002** ✅ PASS

- Prompt: 搜索名为 销售 的 AI 表格，从游标 cursor123 开始翻页
- Expected: `dws aitable base search --query 销售 --cursor cursor123 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base search)
- Flags: PASS (2 flags validated)

**aitable_aitable_base_update_001** ✅ PASS

- Prompt: 更新 AI 表格，name 为 新名称
- Expected: `dws aitable base update --base-id <BASE_ID> --name 新名称 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base update)
- Flags: PASS (2 flags validated)

**aitable_aitable_base_update_002** ✅ PASS

- Prompt: 更新AI表格base123的名字为项目跟踪表，备注是用于Q2项目跟踪
- Expected: `dws aitable base update --base-id base123 --name "项目跟踪表" --desc "用于Q2项目跟踪" --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable base update)
- Flags: PASS (3 flags validated)

**aitable_aitable_field_create_001** ✅ PASS

- Prompt: 在 base123 的 table456 里创建一个单选字段 状态，选项有待办、进行中、已完成
- Expected: `dws aitable field create --base-id base123 --table-id table456 --fields '[{"fieldName":"状态","type":"singleSelect","config":{"options":[{"name":"待办"},{"name":"进行中"},{"name":"已完成"}]}}]' --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable field create)
- Flags: PASS (3 flags validated)

**aitable_aitable_field_create_002** ✅ PASS

- Prompt: 给 base789 的 tblABC 新加一个文本字段叫 备注
- Expected: `dws aitable field create --base-id base789 --table-id tblABC --fields '[{"fieldName":"备注","type":"text"}]' --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable field create)
- Flags: PASS (3 flags validated)

**aitable_aitable_field_delete_001** ✅ PASS

- Prompt: 删除 base123 的 table456 中的字段 fld789
- Expected: `dws aitable field delete --base-id base123 --field-id fld789 --table-id table456 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable field delete)
- Flags: PASS (3 flags validated)

**aitable_aitable_field_delete_002** ✅ PASS

- Prompt: 把 base456 的 tblXYZ 里那个字段 fldABC 删掉
- Expected: `dws aitable field delete --base-id base456 --field-id fldABC --table-id tblXYZ --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable field delete)
- Flags: PASS (3 flags validated)

**aitable_aitable_field_get_001** ✅ PASS

- Prompt: 获取数据表 base123 的 table456 的所有字段详情
- Expected: `dws aitable field get --base-id base123 --table-id table456 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable field get)
- Flags: PASS (2 flags validated)

**aitable_aitable_field_get_002** ✅ PASS

- Prompt: 只获取字段 fld1,fld2 的详情，表格 ID 是 table456，AI 表格 ID 是 base123
- Expected: `dws aitable field get --base-id base123 --table-id table456 --field-ids fld1,fld2 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable field get)
- Flags: PASS (3 flags validated)

**aitable_aitable_field_update_001** ✅ PASS

- Prompt: 更新字段，name 为 新字段名称
- Expected: `dws aitable field update --base-id <BASE_ID> --field-id <FIELD_ID> --table-id <TABLE_ID> --name "新字段名称" --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable field update)
- Flags: PASS (4 flags validated)

**aitable_aitable_field_update_002** ✅ PASS

- Prompt: 把 base123 的 table456 中字段 fld1 的名字改为 状态字段
- Expected: `dws aitable field update --base-id base123 --table-id table456 --field-id fld1 --name 状态字段 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable field update)
- Flags: PASS (4 flags validated)

**aitable_aitable_field_update_003** ✅ PASS

- Prompt: 更新字段 fld2 的配置，将选项值改为 JSON 配置
- Expected: `dws aitable field update --base-id base123 --table-id table456 --field-id fld2 --config '{"options":[{"name":"进行中"}]}' --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable field update)
- Flags: PASS (4 flags validated)

**aitable_aitable_field_update_004** ✅ PASS

- Prompt: 同时更新 base123 的 table456 中字段 fld3 的名称为 优先级 并修改其选项配置
- Expected: `dws aitable field update --base-id base123 --table-id table456 --field-id fld3 --name 优先级 --config '{"options":[{"name":"高"},{"name":"中"},{"name":"低"}]}' --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable field update)
- Flags: PASS (5 flags validated)

**aitable_aitable_record_create_001** ✅ PASS

- Prompt: 在 base123 的 table456 中新增一条记录，任务名称为 开发需求评审，优先级为高
- Expected: `dws aitable record create --base-id base123 --table-id table456 --records '[{"cells":{"任务名称":"开发需求评审","优先级":"高"}}]' --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable record create)
- Flags: PASS (3 flags validated)

**aitable_aitable_record_create_002** ✅ PASS

- Prompt: 往 base789 的 tblABC 里加一条记录，数据是姓名张三、部门研发
- Expected: `dws aitable record create --base-id base789 --table-id tblABC --records '[{"cells":{"姓名":"张三","部门":"研发"}}]' --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable record create)
- Flags: PASS (3 flags validated)

**aitable_aitable_record_query_001** ✅ PASS

- Prompt: 查询记录
- Expected: `dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable record query)
- Flags: PASS (2 flags validated)

**aitable_aitable_record_query_002** ✅ PASS

- Prompt: 在base123的table456中搜索包含待处理的记录，每页5条
- Expected: `dws aitable record query --base-id base123 --table-id table456 --keyword "待处理" --limit 5 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable record query)
- Flags: PASS (4 flags validated)

**aitable_aitable_record_query_003** ✅ PASS

- Prompt: 按 ID 查询 base123 的 table456 中指定记录 rec1,rec2
- Expected: `dws aitable record query --base-id base123 --table-id table456 --record-ids rec1,rec2 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable record query)
- Flags: PASS (3 flags validated)

**aitable_aitable_record_query_004** ✅ PASS

- Prompt: 查询 base123 的 table456 中的记录，只返回字段 fld1,fld2
- Expected: `dws aitable record query --base-id base123 --table-id table456 --field-ids fld1,fld2 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable record query)
- Flags: PASS (3 flags validated)

**aitable_aitable_record_query_005** ✅ PASS

- Prompt: 查询 base123 的 table456 中的记录，按字段 fld1 降序排列
- Expected: `dws aitable record query --base-id base123 --table-id table456 --sort '[{"fieldId":"fld1","order":"desc"}]' --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable record query)
- Flags: PASS (3 flags validated)

**aitable_aitable_record_query_006** ✅ PASS

- Prompt: 查询 base123 的 table456 中状态为进行中的记录
- Expected: `dws aitable record query --base-id base123 --table-id table456 --filters '{"operator":"and","operands":[{"operator":"eq","operands":["fldStatus","进行中"]}]}' --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable record query)
- Flags: PASS (3 flags validated)

**aitable_aitable_record_query_007** ✅ PASS

- Prompt: 在 base123 的 table456 中搜索关键词 项目评审，只返回字段 fld1,fld2，每页10条
- Expected: `dws aitable record query --base-id base123 --table-id table456 --keyword 项目评审 --limit 10 --field-ids fld1,fld2 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable record query)
- Flags: PASS (5 flags validated)

**aitable_aitable_table_create_001** ✅ PASS

- Prompt: 在 base123 中创建一张名为 任务表 的数据表，包含文本字段任务名称和单选字段优先级
- Expected: `dws aitable table create --base-id base123 --name 任务表 --fields '[{"fieldName":"任务名称","type":"text"},{"fieldName":"优先级","type":"singleSelect","config":{"options":[{"name":"高"},{"name":"中"},{"name":"低"}]}}]' --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable table create)
- Flags: PASS (3 flags validated)

**aitable_aitable_table_create_002** ✅ PASS

- Prompt: 帮我在多维表格 base456 里新建一张叫 人员信息 的表，字段有姓名和部门
- Expected: `dws aitable table create --base-id base456 --name 人员信息 --fields '[{"fieldName":"姓名","type":"text"},{"fieldName":"部门","type":"text"}]' --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable table create)
- Flags: PASS (3 flags validated)

**aitable_aitable_table_delete_001** ✅ PASS

- Prompt: 删除数据表
- Expected: `dws aitable table delete --base-id <BASE_ID> --table-id <TABLE_ID> --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable table delete)
- Flags: PASS (2 flags validated)

**aitable_aitable_table_delete_002** ✅ PASS

- Prompt: 删除 base123 中的数据表 table456，说明原因是该表已废弃
- Expected: `dws aitable table delete --base-id base123 --table-id table456 --reason 该表已废弃 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable table delete)
- Flags: PASS (3 flags validated)

**aitable_aitable_table_get_001** ✅ PASS

- Prompt: 获取 AI 表格 base123 里的所有数据表
- Expected: `dws aitable table get --base-id base123 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable table get)
- Flags: PASS (1 flags validated)

**aitable_aitable_table_get_002** ✅ PASS

- Prompt: 只获取 AI 表格 base123 中指定的表 tbl1,tbl2
- Expected: `dws aitable table get --base-id base123 --table-ids tbl1,tbl2 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable table get)
- Flags: PASS (2 flags validated)

**aitable_aitable_table_update_001** ✅ PASS

- Prompt: 把 base123 里的数据表 table456 的名字改成 Q2任务跟踪
- Expected: `dws aitable table update --base-id base123 --table-id table456 --name Q2任务跟踪 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable table update)
- Flags: PASS (3 flags validated)

**aitable_aitable_table_update_002** ✅ PASS

- Prompt: 更新 base789 的 tblXYZ 表名为 销售漏斗
- Expected: `dws aitable table update --base-id base789 --table-id tblXYZ --name 销售漏斗 --format json`
- Skill Reference: references/products/aitable.md
- Command path: PASS (aitable table update)
- Flags: PASS (3 flags validated)

### attendance

**attendance_attendance_record_get_001** ✅ PASS

- Prompt: 查询个人考勤详情，date 为 2026-03-08, user 为 <USER_ID>
- Expected: `dws attendance record get --date 2026-03-08 --user <USER_ID> --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance record get)
- Flags: PASS (2 flags validated)

**attendance_attendance_record_get_002** ✅ PASS

- Prompt: 查一下userId1在2026年3月16号的打卡情况
- Expected: `dws attendance record get --user userId1 --date 2026-03-16 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance record get)
- Flags: PASS (2 flags validated)

**attendance_attendance_record_get_003** ✅ PASS

- Prompt: 帮我查一下张三（user_zhangsan）今天2026-03-18的出勤情况
- Expected: `dws attendance record get --user user_zhangsan --date 2026-03-18 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance record get)
- Flags: PASS (2 flags validated)

**attendance_attendance_record_get_004** ✅ PASS

- Prompt: 我想看员工 emp001 在 2026-03-10 的考勤详情
- Expected: `dws attendance record get --user emp001 --date 2026-03-10 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance record get)
- Flags: PASS (2 flags validated)

**attendance_attendance_rules_001** ✅ PASS

- Prompt: 查询 2026-03-14 的考勤组与考勤规则
- Expected: `dws attendance rules --date 2026-03-14 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance rules)
- Flags: PASS (1 flags validated)

**attendance_attendance_rules_002** ✅ PASS

- Prompt: 今天2026年3月18日的考勤规则是什么
- Expected: `dws attendance rules --date 2026-03-18 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance rules)
- Flags: PASS (1 flags validated)

**attendance_attendance_rules_003** ✅ PASS

- Prompt: 帮我看看2026-03-20这天的打卡规则
- Expected: `dws attendance rules --date 2026-03-20 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance rules)
- Flags: PASS (1 flags validated)

**attendance_attendance_rules_004** ✅ PASS

- Prompt: 查一下 2026-03-17 09:00:00 对应的考勤组信息
- Expected: `dws attendance rules --date "2026-03-17 09:00:00" --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance rules)
- Flags: PASS (1 flags validated)

**attendance_attendance_shift_list_001** ✅ PASS

- Prompt: 查询用户 userId1 和 userId2 在 2026-03-03 到 2026-03-07 的班次
- Expected: `dws attendance shift list --end 2026-03-07 --start 2026-03-03 --users userId1,userId2 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance shift list)
- Flags: PASS (3 flags validated)

**attendance_attendance_shift_list_002** ✅ PASS

- Prompt: 帮我看看张三（user001）上周一到上周五的排班情况
- Expected: `dws attendance shift list --end 2026-03-13 --start 2026-03-09 --users user001 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance shift list)
- Flags: PASS (3 flags validated)

**attendance_attendance_shift_list_003** ✅ PASS

- Prompt: 查询这周（2026-03-16到2026-03-22）团队成员 emp001,emp002,emp003 的排班
- Expected: `dws attendance shift list --start 2026-03-16 --end 2026-03-22 --users emp001,emp002,emp003 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance shift list)
- Flags: PASS (3 flags validated)

**attendance_attendance_shift_list_004** ✅ PASS

- Prompt: 我想知道 staffA 和 staffB 下周的当班安排，日期是2026-03-23到2026-03-27
- Expected: `dws attendance shift list --start 2026-03-23 --end 2026-03-27 --users staffA,staffB --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance shift list)
- Flags: PASS (3 flags validated)

**attendance_attendance_summary_001** ✅ PASS

- Prompt: 查询用户 user123 在 2026-03-12 的考勤统计摘要
- Expected: `dws attendance summary --date "2026-03-12 15:00:00" --user user123 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance summary)
- Flags: PASS (2 flags validated)

**attendance_attendance_summary_002** ✅ PASS

- Prompt: 帮我看看员工 emp456 今天 2026-03-18 的打卡情况
- Expected: `dws attendance summary --date "2026-03-18 00:00:00" --user emp456 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance summary)
- Flags: PASS (2 flags validated)

**attendance_attendance_summary_003** ✅ PASS

- Prompt: 给我看 emp789 在 2026-03-15 这天的考勤汇总
- Expected: `dws attendance summary --date "2026-03-15 00:00:00" --user emp789 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance summary)
- Flags: PASS (2 flags validated)

**attendance_attendance_summary_004** ✅ PASS

- Prompt: 统计一下 userId99 在 2026-03-18 09:00:00 的考勤数据
- Expected: `dws attendance summary --date "2026-03-18 09:00:00" --user userId99 --format json`
- Skill Reference: references/products/attendance.md
- Command path: PASS (attendance summary)
- Flags: PASS (2 flags validated)

### calendar

**calendar_calendar_busy_search_001** ✅ PASS

- Prompt: 查询用户 user001 在 2026-03-10 14:00 到 18:00 的闲忙状态
- Expected: `dws calendar busy search --end 2026-03-10T18:00:00+08:00 --start 2026-03-10T14:00:00+08:00 --users user001 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar busy search)
- Flags: PASS (3 flags validated)

**calendar_calendar_busy_search_002** ✅ PASS

- Prompt: 看看 emp123 和 emp456 明天上午9点到12点是否有空
- Expected: `dws calendar busy search --start 2026-03-19T09:00:00+08:00 --end 2026-03-19T12:00:00+08:00 --users emp123,emp456 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar busy search)
- Flags: PASS (3 flags validated)

**calendar_calendar_event_create_001** ✅ PASS

- Prompt: 创建日程，end 为 2026-03-10T15:00:00+08:00, start 为 2026-03-10T14:00:00+08:00, title 为 Q1 复盘会
- Expected: `dws calendar event create --end 2026-03-10T15:00:00+08:00 --start 2026-03-10T14:00:00+08:00 --title "Q1 复盘会" --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event create)
- Flags: PASS (3 flags validated)

**calendar_calendar_event_create_002** ✅ PASS

- Prompt: 创建一个日程，标题产品评审会，从2026-03-20T14:00:00+08:00到2026-03-20T16:00:00+08:00，描述是讨论Q2产品规划
- Expected: `dws calendar event create --title "产品评审会" --start "2026-03-20T14:00:00+08:00" --end "2026-03-20T16:00:00+08:00" --desc "讨论Q2产品规划" --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event create)
- Flags: PASS (4 flags validated)

**calendar_calendar_event_delete_001** ✅ PASS

- Prompt: 删除日程
- Expected: `dws calendar event delete --id <EVENT_ID> --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event delete)
- Flags: PASS (1 flags validated)

**calendar_calendar_event_delete_002** ✅ PASS

- Prompt: 把那个会议取消了
- Expected: `dws calendar event delete --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event delete)
- Flags: N/A (no flags specified)
- Note: [ASK_USER] case - only command path validated

**calendar_calendar_event_get_001** ✅ PASS

- Prompt: 获取日程 evt123 的详情
- Expected: `dws calendar event get --id evt123 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event get)
- Flags: PASS (1 flags validated)

**calendar_calendar_event_get_002** ✅ PASS

- Prompt: 查看一下这个日程 evtABC 的具体内容
- Expected: `dws calendar event get --id evtABC --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event get)
- Flags: PASS (1 flags validated)

**calendar_calendar_event_list_001** ✅ PASS

- Prompt: 查询日程列表，start 为 2026-03-10T14:00:00+08:00, end 为 2026-03-10T18:00:00+08:00
- Expected: `dws calendar event list --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00" --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event list)
- Flags: PASS (2 flags validated)

**calendar_calendar_event_list_002** ✅ PASS

- Prompt: 看看我最近的日程
- Expected: `dws calendar event list --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event list)
- Flags: N/A (no flags specified)

**calendar_calendar_event_update_001** ✅ PASS

- Prompt: 修改日程 ev123，只把标题改为 紧急会议
- Expected: `dws calendar event update --id ev123 --title 紧急会议 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event update)
- Flags: PASS (2 flags validated)

**calendar_calendar_event_update_002** ✅ PASS

- Prompt: 把日程 ev456 的开始时间改到 2026-03-26T09:00:00+08:00
- Expected: `dws calendar event update --id ev456 --start "2026-03-26T09:00:00+08:00" --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event update)
- Flags: PASS (2 flags validated)

**calendar_calendar_event_update_003** ✅ PASS

- Prompt: 把日程 ev789 的结束时间推迟到 2026-03-26T12:00:00+08:00
- Expected: `dws calendar event update --id ev789 --end "2026-03-26T12:00:00+08:00" --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event update)
- Flags: PASS (2 flags validated)

**calendar_calendar_event_update_004** ✅ PASS

- Prompt: 把日程eventId123的标题改成产品评审v2，时间改为2026-03-25T10:00:00+08:00到2026-03-25T12:00:00+08:00
- Expected: `dws calendar event update --id eventId123 --title "产品评审v2" --start "2026-03-25T10:00:00+08:00" --end "2026-03-25T12:00:00+08:00" --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event update)
- Flags: PASS (4 flags validated)

**calendar_calendar_event_update_005** ✅ PASS

- Prompt: 帮我把会议 mtgXYZ 改名为周会，时间挪到下周一上午10点到11点
- Expected: `dws calendar event update --id mtgXYZ --title "周会" --start "2026-03-23T10:00:00+08:00" --end "2026-03-23T11:00:00+08:00" --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar event update)
- Flags: PASS (4 flags validated)

**calendar_calendar_participant_add_001** ✅ PASS

- Prompt: 给日程 evt123 添加参与者 user001
- Expected: `dws calendar participant add --event evt123 --users user001 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar participant add)
- Flags: PASS (2 flags validated)

**calendar_calendar_participant_add_002** ✅ PASS

- Prompt: 把 user002 和 user003 加到会议 evtXYZ 里
- Expected: `dws calendar participant add --event evtXYZ --users user002,user003 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar participant add)
- Flags: PASS (2 flags validated)

**calendar_calendar_participant_delete_001** ✅ PASS

- Prompt: 从日程 evt123 中移除参与者 user001
- Expected: `dws calendar participant delete --event evt123 --users user001 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar participant delete)
- Flags: PASS (2 flags validated)

**calendar_calendar_participant_delete_002** ✅ PASS

- Prompt: 把 user002 从会议 evtXYZ 的参与者里删掉
- Expected: `dws calendar participant delete --event evtXYZ --users user002 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar participant delete)
- Flags: PASS (2 flags validated)

**calendar_calendar_participant_list_001** ✅ PASS

- Prompt: 查看日程 evt123 的参与者列表
- Expected: `dws calendar participant list --event evt123 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar participant list)
- Flags: PASS (1 flags validated)

**calendar_calendar_participant_list_002** ✅ PASS

- Prompt: 这个会议 evtXYZ 有哪些人参加
- Expected: `dws calendar participant list --event evtXYZ --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar participant list)
- Flags: PASS (1 flags validated)

**calendar_calendar_room_add_001** ✅ PASS

- Prompt: 给日程 evt123 预定会议室 room001
- Expected: `dws calendar room add --event evt123 --rooms room001 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar room add)
- Flags: PASS (2 flags validated)

**calendar_calendar_room_add_002** ✅ PASS

- Prompt: 把 roomABC 这个会议室加到会议 evtXYZ
- Expected: `dws calendar room add --event evtXYZ --rooms roomABC --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar room add)
- Flags: PASS (2 flags validated)

**calendar_calendar_room_delete_001** ✅ PASS

- Prompt: 从日程 evt123 中移除会议室 room001
- Expected: `dws calendar room delete --event evt123 --rooms room001 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar room delete)
- Flags: PASS (2 flags validated)

**calendar_calendar_room_delete_002** ✅ PASS

- Prompt: 取消 evtXYZ 会议的会议室预定 roomABC
- Expected: `dws calendar room delete --event evtXYZ --rooms roomABC --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar room delete)
- Flags: PASS (2 flags validated)

**calendar_calendar_room_list_groups_001** ✅ PASS

- Prompt: 获取所有会议室分组列表
- Expected: `dws calendar room list-groups --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar room list-groups)
- Flags: N/A (no flags specified)

**calendar_calendar_room_list_groups_002** ✅ PASS

- Prompt: 查一下公司有哪些会议室分组
- Expected: `dws calendar room list-groups --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar room list-groups)
- Flags: N/A (no flags specified)

**calendar_calendar_room_search_001** ✅ PASS

- Prompt: 搜索会议室，end 为 2026-03-10T15:00:00+08:00, start 为 2026-03-10T14:00:00+08:00
- Expected: `dws calendar room search --end 2026-03-10T15:00:00+08:00 --start 2026-03-10T14:00:00+08:00 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar room search)
- Flags: PASS (2 flags validated)

**calendar_calendar_room_search_002** ✅ PASS

- Prompt: 搜一下2026年3月20日下午2点到4点有空的会议室
- Expected: `dws calendar room search --start "2026-03-20T14:00:00+08:00" --end "2026-03-20T16:00:00+08:00" --available --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar room search)
- Flags: PASS (3 flags validated)

**calendar_calendar_room_search_003** ✅ PASS

- Prompt: 在分组 group123 中搜索 2026-03-25 上午9点到10点的空闲会议室
- Expected: `dws calendar room search --start 2026-03-25T09:00:00+08:00 --end 2026-03-25T10:00:00+08:00 --group-id group123 --format json`
- Skill Reference: references/products/calendar.md
- Command path: PASS (calendar room search)
- Flags: PASS (3 flags validated)

### chat

**chat_chat_bot_search_001** ✅ PASS

- Prompt: 搜索我创建的机器人
- Expected: `dws chat bot search --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat bot search)
- Flags: N/A (no flags specified)

**chat_chat_bot_search_002** ✅ PASS

- Prompt: 搜索我创建的名字包含日报的机器人，第1页每页10个
- Expected: `dws chat bot search --name "日报" --page 1 --size 10 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat bot search)
- Flags: PASS (3 flags validated)

**chat_chat_group_create_001** ✅ PASS

- Prompt: 创建群 — 当前登录用户自动成为群主，users 为 userId1,userId2,userId3, name 为 Q1 项目冲刺群
- Expected: `dws chat group create --users userId1,userId2,userId3 --name "Q1 项目冲刺群" --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group create)
- Flags: PASS (2 flags validated)

**chat_chat_group_create_002** ✅ PASS

- Prompt: 拉一个群讨论Q2计划，把userId1和userId2拉进来，群名叫Q2规划讨论
- Expected: `dws chat group create --users userId1,userId2 --name "Q2规划讨论" --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group create)
- Flags: PASS (2 flags validated)

**chat_chat_group_members_001** ✅ PASS

- Prompt: 查看群 openConvABC 的成员列表
- Expected: `dws chat group members --id openConvABC --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group members)
- Flags: PASS (1 flags validated)

**chat_chat_group_members_002** ✅ PASS

- Prompt: 查看群 openConvABC 成员列表的下一页，游标是 cursor999
- Expected: `dws chat group members --id openConvABC --cursor cursor999 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group members)
- Flags: PASS (2 flags validated)

**chat_chat_group_members_add_001** ✅ PASS

- Prompt: 向群 conv123 添加成员 user001 和 user002
- Expected: `dws chat group members add --id conv123 --users user001,user002 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group members add)
- Flags: PASS (2 flags validated)

**chat_chat_group_members_add_002** ✅ PASS

- Prompt: 把张三（empABC）拉进 convXYZ 这个群
- Expected: `dws chat group members add --id convXYZ --users empABC --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group members add)
- Flags: PASS (2 flags validated)

**chat_chat_group_members_add_bot_001** ✅ PASS

- Prompt: 将机器人 robot001 添加到群 conv123
- Expected: `dws chat group members add-bot --id conv123 --robot-code robot001 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group members add-bot)
- Flags: PASS (2 flags validated)

**chat_chat_group_members_add_bot_002** ✅ PASS

- Prompt: 把 botXYZ 机器人拉进 convABC 这个群
- Expected: `dws chat group members add-bot --id convABC --robot-code botXYZ --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group members add-bot)
- Flags: PASS (2 flags validated)

**chat_chat_group_members_remove_001** ✅ PASS

- Prompt: 移除群成员 — 从指定群聊中移除成员，需传入群 ID 与待移除的用户 ID 列表，users 为 userId1,userId2
- Expected: `dws chat group members remove --id <openconversation_id> --users userId1,userId2 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group members remove)
- Flags: PASS (2 flags validated)

**chat_chat_group_members_remove_002** ✅ PASS

- Prompt: 把userId1和userId2从群abc123里踢出去
- Expected: `dws chat group members remove --id abc123 --users userId1,userId2 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group members remove)
- Flags: PASS (2 flags validated)

**chat_chat_group_rename_001** ✅ PASS

- Prompt: 更新群名称，name 为 新群名
- Expected: `dws chat group rename --id <openconversation_id> --name 新群名 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group rename)
- Flags: PASS (2 flags validated)

**chat_chat_group_rename_002** ✅ PASS

- Prompt: 把群abc123改名叫新项目讨论组
- Expected: `dws chat group rename --id abc123 --name "新项目讨论组" --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat group rename)
- Flags: PASS (2 flags validated)

**chat_chat_message_list_001** ✅ PASS

- Prompt: 拉取群聊会话消息内容，time 为 2025-03-01 00:00:00
- Expected: `dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00" --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message list)
- Flags: PASS (2 flags validated)

**chat_chat_message_list_002** ✅ PASS

- Prompt: 拉取和userId123的单聊消息记录，从2025年3月1号开始
- Expected: `dws chat message list --user userId123 --time "2025-03-01 00:00:00" --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message list)
- Flags: PASS (2 flags validated)

**chat_chat_message_list_003** ✅ PASS

- Prompt: 拉取群abc123从2025年3月10号开始往前的消息，最多50条
- Expected: `dws chat message list --group abc123 --time "2025-03-10 00:00:00" --forward false --limit 50 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message list)
- Flags: PASS (4 flags validated)

**chat_chat_message_recall_by_bot_001** ✅ PASS

- Prompt: 用机器人 robot1 撤回群 openConvABC 中的消息，消息key是 msgKey001
- Expected: `dws chat message recall-by-bot --group <openconversation_id> --keys <process-query-key> --robot-code <robot-code> --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message recall-by-bot)
- Flags: PASS (3 flags validated)

**chat_chat_message_recall_by_bot_002** ✅ PASS

- Prompt: 用机器人myBot撤回单聊消息，消息key是key123和key456
- Expected: `dws chat message recall-by-bot --robot-code myBot --keys key123,key456 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message recall-by-bot)
- Flags: PASS (2 flags validated)

**chat_chat_message_send_001** ✅ PASS

- Prompt: 在群 openConvABC 发一条消息：大家好
- Expected: `dws chat message send --group openConvABC "大家好" --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message send)
- Flags: PASS (1 flags validated)

**chat_chat_message_send_002** ✅ PASS

- Prompt: 给userId123发一条私聊消息，标题是提醒，内容是请查收报告
- Expected: `dws chat message send --user userId123 --title "提醒" "请查收报告" --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message send)
- Flags: PASS (2 flags validated)

**chat_chat_message_send_003** ✅ PASS

- Prompt: 在群 groupId456 发一条带标题的通知：周报提醒，请大家本周五前提交周报
- Expected: `dws chat message send --group groupId456 --title "周报提醒" "请大家本周五前提交周报" --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message send)
- Flags: PASS (2 flags validated)

**chat_chat_message_send_by_bot_001** ✅ PASS

- Prompt: 机器人发送群聊消息，text 为 ## 今日完成..., title 为 日报
- Expected: `dws chat message send-by-bot --group <openconversation_id> --robot-code <robot-code> --text "## 今日完成..." --title 日报 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message send-by-bot)
- Flags: PASS (4 flags validated)

**chat_chat_message_send_by_bot_002** ✅ PASS

- Prompt: 用机器人myBot给userId1和userId2发私聊消息，标题是通知，内容是请查收周报
- Expected: `dws chat message send-by-bot --robot-code myBot --users userId1,userId2 --title "通知" --text "请查收周报" --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message send-by-bot)
- Flags: PASS (4 flags validated)

**chat_chat_message_send_by_webhook_001** ✅ PASS

- Prompt: 通过 Webhook token1 发一条告警消息：CPU 超 90%
- Expected: `dws chat message send-by-webhook --text "CPU 超 90%" --title 告警 --token token1 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message send-by-webhook)
- Flags: PASS (3 flags validated)

**chat_chat_message_send_by_webhook_002** ✅ PASS

- Prompt: 通过webhook发群消息，token是tokenABC，标题告警，内容CPU使用率超过90%，@所有人
- Expected: `dws chat message send-by-webhook --token tokenABC --title "告警" --text "CPU使用率超过90%" --at-all --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message send-by-webhook)
- Flags: PASS (4 flags validated)

**chat_chat_message_send_by_webhook_003** ✅ PASS

- Prompt: 用 Webhook tokenXYZ 发消息，标题是「审批提醒」，内容是「请及时审批」，并@用户 user001 和 user002
- Expected: `dws chat message send-by-webhook --token tokenXYZ --title "审批提醒" --text "@user001 @user002 请及时审批" --at-users user001,user002 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message send-by-webhook)
- Flags: PASS (4 flags validated)

**chat_chat_message_send_by_webhook_004** ✅ PASS

- Prompt: 用 Webhook tokenDEF 发通知，标题是「会议通知」，内容是「请参加今日下午的会议」，并@手机号 13800138000 和 13900139000
- Expected: `dws chat message send-by-webhook --token tokenDEF --title "会议通知" --text "@13800138000 @13900139000 请参加今日下午的会议" --at-mobiles 13800138000,13900139000 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat message send-by-webhook)
- Flags: PASS (4 flags validated)

**chat_chat_search_001** ✅ PASS

- Prompt: 根据名称搜索会话列表，query 为 项目冲刺
- Expected: `dws chat search --query 项目冲刺 --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat search)
- Flags: PASS (1 flags validated)

**chat_chat_search_002** ✅ PASS

- Prompt: 搜一下有没有叫项目组的群
- Expected: `dws chat search --query "项目组" --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat search)
- Flags: PASS (1 flags validated)

**chat_chat_search_003** ✅ PASS

- Prompt: 继续搜索关键词 项目冲刺 的下一页结果，游标 cursorXYZ
- Expected: `dws chat search --query 项目冲刺 --cursor cursorXYZ --format json`
- Skill Reference: references/products/chat.md
- Command path: PASS (chat search)
- Flags: PASS (2 flags validated)

### contact

**contact_contact_dept_list_children_001** ✅ PASS

- Prompt: 查看部门 12345 的子部门列表
- Expected: `dws contact dept list-children --id 12345 --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact dept list-children)
- Flags: PASS (1 flags validated)

**contact_contact_dept_list_children_002** ✅ PASS

- Prompt: 技术部（ID 67890）下面有哪些二级部门
- Expected: `dws contact dept list-children --id 67890 --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact dept list-children)
- Flags: PASS (1 flags validated)

**contact_contact_dept_list_members_001** ✅ PASS

- Prompt: 查看部门 12345 和 67890 的成员列表
- Expected: `dws contact dept list-members --ids 12345,67890 --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact dept list-members)
- Flags: PASS (1 flags validated)

**contact_contact_dept_list_members_002** ✅ PASS

- Prompt: 研发部（ID 11111）有哪些员工
- Expected: `dws contact dept list-members --ids 11111 --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact dept list-members)
- Flags: PASS (1 flags validated)

**contact_contact_dept_search_001** ✅ PASS

- Prompt: 搜索部门，keyword 为 技术部
- Expected: `dws contact dept search --query 技术部 --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact dept search)
- Flags: PASS (1 flags validated)

**contact_contact_dept_search_002** ✅ PASS

- Prompt: 搜一下技术部的部门信息
- Expected: `dws contact dept search --query "技术部" --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact dept search)
- Flags: PASS (1 flags validated)

**contact_contact_user_get_001** ✅ PASS

- Prompt: 批量获取用户 user001 和 user002 的详细信息
- Expected: `dws contact user get --ids user001,user002 --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact user get)
- Flags: PASS (1 flags validated)

**contact_contact_user_get_002** ✅ PASS

- Prompt: 查一下员工 empABC 的信息
- Expected: `dws contact user get --ids empABC --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact user get)
- Flags: PASS (1 flags validated)

**contact_contact_user_get_self_001** ✅ PASS

- Prompt: 获取当前用户信息
- Expected: `dws contact user get-self --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact user get-self)
- Flags: N/A (no flags specified)

**contact_contact_user_get_self_002** ✅ PASS

- Prompt: 我是谁，查一下当前登录账号的资料
- Expected: `dws contact user get-self --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact user get-self)
- Flags: N/A (no flags specified)

**contact_contact_user_search_001** ✅ PASS

- Prompt: 按关键词搜索用户，keyword 为 张三
- Expected: `dws contact user search --query 张三 --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact user search)
- Flags: PASS (1 flags validated)

**contact_contact_user_search_002** ✅ PASS

- Prompt: 帮我找一下张三的联系方式
- Expected: `dws contact user search --query "张三" --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact user search)
- Flags: PASS (1 flags validated)

**contact_contact_user_search_mobile_001** ✅ PASS

- Prompt: 按手机号 13800138000 搜索用户
- Expected: `dws contact user search-mobile --mobile 13800138000 --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact user search-mobile)
- Flags: PASS (1 flags validated)

**contact_contact_user_search_mobile_002** ✅ PASS

- Prompt: 查一下 15912345678 这个手机号是哪个员工
- Expected: `dws contact user search-mobile --mobile 15912345678 --format json`
- Skill Reference: references/products/contact.md
- Command path: PASS (contact user search-mobile)
- Flags: PASS (1 flags validated)

### devdoc

**devdoc_devdoc_article_search_001** ✅ PASS

- Prompt: 搜索开放平台文档，keyword 为 OAuth2 接入
- Expected: `dws devdoc article search --keyword "OAuth2 接入" --format json`
- Skill Reference: references/products/devdoc.md
- Command path: PASS (devdoc article search)
- Flags: PASS (1 flags validated)

**devdoc_devdoc_article_search_002** ✅ PASS

- Prompt: 搜索开放平台文档中关于 Webhook 的内容，第 2 页每页 5 条
- Expected: `dws devdoc article search --keyword Webhook --page 2 --size 5 --format json`
- Skill Reference: references/products/devdoc.md
- Command path: PASS (devdoc article search)
- Flags: PASS (3 flags validated)

**devdoc_devdoc_article_search_003** ✅ PASS

- Prompt: 搜索开放平台文档中关于消息卡片的内容，从第 2 页开始
- Expected: `dws devdoc article search --keyword 消息卡片 --page 2 --format json`
- Skill Reference: references/products/devdoc.md
- Command path: PASS (devdoc article search)
- Flags: PASS (2 flags validated)

**devdoc_devdoc_article_search_004** ✅ PASS

- Prompt: 搜索关键词为机器人的开放平台文档，每页返回 10 条
- Expected: `dws devdoc article search --keyword 机器人 --size 10 --format json`
- Skill Reference: references/products/devdoc.md
- Command path: PASS (devdoc article search)
- Flags: PASS (2 flags validated)

**devdoc_devdoc_article_search_005** ✅ PASS

- Prompt: 帮我在开发者文档里找一下关于免登鉴权的资料
- Expected: `dws devdoc article search --keyword 免登鉴权 --format json`
- Skill Reference: references/products/devdoc.md
- Command path: PASS (devdoc article search)
- Flags: PASS (1 flags validated)

### ding

**ding_ding_message_recall_001** ✅ PASS

- Prompt: 撤回 DING 消息 ding123，机器人代码 robot001
- Expected: `dws ding message recall --id ding123 --robot-code robot001 --format json`
- Skill Reference: references/products/ding.md
- Command path: PASS (ding message recall)
- Flags: PASS (2 flags validated)

**ding_ding_message_recall_002** ✅ PASS

- Prompt: 把刚发的 DING 消息 dingXYZ 用机器人 botABC 撤回
- Expected: `dws ding message recall --id dingXYZ --robot-code botABC --format json`
- Skill Reference: references/products/ding.md
- Command path: PASS (ding message recall)
- Flags: PASS (2 flags validated)

**ding_ding_message_send_001** ✅ PASS

- Prompt: 发送 DING 消息，content 为 请查看, robot-code 为 <ROBOT_CODE>, users 为 <USER_ID_1>
- Expected: `dws ding message send --content 请查看 --robot-code <ROBOT_CODE> --users <USER_ID_1> --format json`
- Skill Reference: references/products/ding.md
- Command path: PASS (ding message send)
- Flags: PASS (3 flags validated)

**ding_ding_message_send_002** ✅ PASS

- Prompt: 用机器人botCode123给userId1发一条电话DING，内容是紧急请回电
- Expected: `dws ding message send --robot-code botCode123 --users userId1 --content "紧急请回电" --type call --format json`
- Skill Reference: references/products/ding.md
- Command path: PASS (ding message send)
- Flags: PASS (4 flags validated)

**ding_ding_message_send_003** ✅ PASS

- Prompt: 用机器人 botXYZ DING 提醒 alice 和 bob 参加今天下午的会议
- Expected: `dws ding message send --content "请参加今天下午的会议" --robot-code botXYZ --users alice,bob --format json`
- Skill Reference: references/products/ding.md
- Command path: PASS (ding message send)
- Flags: PASS (3 flags validated)

### report

**report_report_detail_001** ✅ PASS

- Prompt: 获取日报 rpt123 的详情
- Expected: `dws report detail --report-id rpt123 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report detail)
- Flags: PASS (1 flags validated)

**report_report_detail_002** ✅ PASS

- Prompt: 查看一下这份日志 rptXYZ 的完整内容
- Expected: `dws report detail --report-id rptXYZ --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report detail)
- Flags: PASS (1 flags validated)

**report_report_list_001** ✅ PASS

- Prompt: 获取 2026-03-10 收到的前20条日报，从游标 0 开始
- Expected: `dws report list --cursor 0 --end 2026-03-10T23:59:59+08:00 --size 20 --start 2026-03-10T00:00:00+08:00 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report list)
- Flags: PASS (4 flags validated)

**report_report_list_002** ✅ PASS

- Prompt: 查今天 2026-03-18 我收到的所有日报，每页10条
- Expected: `dws report list --cursor 0 --start 2026-03-18T00:00:00+08:00 --end 2026-03-18T23:59:59+08:00 --size 10 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report list)
- Flags: PASS (4 flags validated)

**report_report_sent_001** ✅ PASS

- Prompt: 查询当前人创建的日志列表
- Expected: `dws report sent --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: N/A (no flags specified)
- Note: [ASK_USER] case - only command path validated

**report_report_sent_002** ✅ PASS

- Prompt: 查看我从2026年3月1号到3月15号写的周报
- Expected: `dws report sent --start "2026-03-01T00:00:00+08:00" --end "2026-03-15T23:59:59+08:00" --template-name "周报" --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (3 flags validated)

**report_report_sent_003** ✅ PASS

- Prompt: 从游标位置 50 继续查询我提交的日志
- Expected: `dws report sent --cursor 50 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (1 flags validated)

**report_report_sent_004** ✅ PASS

- Prompt: 查询我提交的日志，每页返回 20 条
- Expected: `dws report sent --size 20 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (1 flags validated)

**report_report_sent_005** ✅ PASS

- Prompt: 查询我在 2026-03-01 之后提交的日志
- Expected: `dws report sent --start 2026-03-01T00:00:00+08:00 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (1 flags validated)

**report_report_sent_006** ✅ PASS

- Prompt: 查询我在 2026-03-31 之前提交的日志
- Expected: `dws report sent --end 2026-03-31T23:59:59+08:00 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (1 flags validated)

**report_report_sent_007** ✅ PASS

- Prompt: 查询 2026-03-10 之后修改过的日志
- Expected: `dws report sent --modified-start 2026-03-10T00:00:00+08:00 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (1 flags validated)

**report_report_sent_008** ✅ PASS

- Prompt: 查询 2026-03-15 之前最后修改的日志
- Expected: `dws report sent --modified-end 2026-03-15T23:59:59+08:00 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (1 flags validated)

**report_report_sent_009** ✅ PASS

- Prompt: 查询使用「日报」模板提交的日志
- Expected: `dws report sent --template-name 日报 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (1 flags validated)

**report_report_sent_010** ✅ PASS

- Prompt: 查看我2026年3月份提交的日志，时间范围从3月1号到3月31号
- Expected: `dws report sent --end 2026-03-31T23:59:59+08:00 --start 2026-03-01T00:00:00+08:00 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (2 flags validated)

**report_report_sent_011** ✅ PASS

- Prompt: 分页查询我提交的日志，从游标 100 开始，每次取 10 条
- Expected: `dws report sent --cursor 100 --size 10 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (2 flags validated)

**report_report_sent_012** ✅ PASS

- Prompt: 查询我在3月1到3月31日之间修改过的日志
- Expected: `dws report sent --modified-end 2026-03-31T23:59:59+08:00 --modified-start 2026-03-01T00:00:00+08:00 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (2 flags validated)

**report_report_sent_013** ✅ PASS

- Prompt: 查询3月份用「月报」模板提交的日志，时间范围3月1号到3月31号
- Expected: `dws report sent --end 2026-03-31T23:59:59+08:00 --start 2026-03-01T00:00:00+08:00 --template-name 月报 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (3 flags validated)

**report_report_sent_014** ✅ PASS

- Prompt: 查询3月1到3月31日之间提交的日报，从游标 0 开始，每页返回 50 条
- Expected: `dws report sent --cursor 0 --end 2026-03-31T23:59:59+08:00 --size 50 --start 2026-03-01T00:00:00+08:00 --template-name 日报 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (5 flags validated)

**report_report_sent_015** ✅ PASS

- Prompt: 查看本月1号到15号用「周报」模板写的，且在3月10号到3月15号之间修改过的日志
- Expected: `dws report sent --end 2026-03-15T23:59:59+08:00 --modified-end 2026-03-15T23:59:59+08:00 --modified-start 2026-03-10T00:00:00+08:00 --start 2026-03-01T00:00:00+08:00 --template-name 周报 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: PASS (5 flags validated)

**report_report_sent_016** ✅ PASS

- Prompt: 帮我列出我写过的日志
- Expected: `dws report sent --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report sent)
- Flags: N/A (no flags specified)

**report_report_stats_001** ✅ PASS

- Prompt: 获取日报 rpt123 的统计数据
- Expected: `dws report stats --report-id rpt123 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report stats)
- Flags: PASS (1 flags validated)

**report_report_stats_002** ✅ PASS

- Prompt: 统计一下日志 rptXYZ 的提交、阅读人数
- Expected: `dws report stats --report-id rptXYZ --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report stats)
- Flags: PASS (1 flags validated)

**report_report_template_detail_001** ✅ PASS

- Prompt: 获取名称为 日报模板 的日志模版详情
- Expected: `dws report template detail --name 日报模板 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report template detail)
- Flags: PASS (1 flags validated)

**report_report_template_detail_002** ✅ PASS

- Prompt: 查看一下叫 周报模板 的这个日志模版是什么格式
- Expected: `dws report template detail --name 周报模板 --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report template detail)
- Flags: PASS (1 flags validated)

**report_report_template_list_001** ✅ PASS

- Prompt: 获取所有日志模版列表
- Expected: `dws report template list --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report template list)
- Flags: N/A (no flags specified)

**report_report_template_list_002** ✅ PASS

- Prompt: 公司现在有哪些日报、周报模板
- Expected: `dws report template list --format json`
- Skill Reference: references/products/report.md
- Command path: PASS (report template list)
- Flags: N/A (no flags specified)

### todo

**todo_todo_task_create_001** ✅ PASS

- Prompt: 创建待办，executors 为 <USER_ID_1>, title 为 修复线上Bug
- Expected: `dws todo task create --executors <USER_ID_1> --title 修复线上Bug --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task create)
- Flags: PASS (2 flags validated)

**todo_todo_task_create_002** ✅ PASS

- Prompt: 创建一个高优先级待办，标题修复登录Bug，执行人userId1，截止时间2026-03-20T18:00:00+08:00
- Expected: `dws todo task create --title "修复登录Bug" --executors userId1 --priority 30 --due "2026-03-20T18:00:00+08:00" --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task create)
- Flags: PASS (4 flags validated)

**todo_todo_task_create_003** ✅ PASS

- Prompt: 给 alice 安排一个整理文档的任务
- Expected: `dws todo task create --executors alice --title 整理文档 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task create)
- Flags: PASS (2 flags validated)

**todo_todo_task_create_004** ✅ PASS

- Prompt: 创建一个待办，执行人 userA，标题 准备季度汇报，截止日期 2026-04-01
- Expected: `dws todo task create --due 2026-04-01 --executors userA --title 准备季度汇报 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task create)
- Flags: PASS (3 flags validated)

**todo_todo_task_create_005** ✅ PASS

- Prompt: 新建一个高优先级的待办，执行人 userB，标题 发布版本v2.0
- Expected: `dws todo task create --executors userB --priority 30 --title 发布版本v2.0 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task create)
- Flags: PASS (3 flags validated)

**todo_todo_task_create_006** ✅ PASS

- Prompt: 帮我给 userId2 创建一个提交周报的提醒事项
- Expected: `dws todo task create --executors userId2 --title 提交周报 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task create)
- Flags: PASS (2 flags validated)

**todo_todo_task_delete_001** ✅ PASS

- Prompt: 删除待办任务 task123
- Expected: `dws todo task delete --task-id task123 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task delete)
- Flags: PASS (1 flags validated)

**todo_todo_task_delete_002** ✅ PASS

- Prompt: 把 taskXYZ 这个待办删掉，已经不需要了
- Expected: `dws todo task delete --task-id taskXYZ --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task delete)
- Flags: PASS (1 flags validated)

**todo_todo_task_done_001** ✅ PASS

- Prompt: 修改执行者的待办完成状态，status 为 true
- Expected: `dws todo task done --status true --task-id <taskId> --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task done)
- Flags: PASS (2 flags validated)

**todo_todo_task_done_002** ✅ PASS

- Prompt: 把待办taskId123标记为已完成
- Expected: `dws todo task done --task-id taskId123 --status true --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task done)
- Flags: PASS (2 flags validated)

**todo_todo_task_done_003** ✅ PASS

- Prompt: 把待办 taskId456 的完成状态设置为未完成
- Expected: `dws todo task done --status false --task-id taskId456 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task done)
- Flags: PASS (2 flags validated)

**todo_todo_task_done_004** ✅ PASS

- Prompt: 重新打开已关闭的待办任务 taskId789
- Expected: `dws todo task done --status false --task-id taskId789 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task done)
- Flags: PASS (2 flags validated)

**todo_todo_task_get_001** ✅ PASS

- Prompt: 获取待办任务 task123 的详情
- Expected: `dws todo task get --task-id task123 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task get)
- Flags: PASS (1 flags validated)

**todo_todo_task_get_002** ✅ PASS

- Prompt: 查一下待办 taskXYZ 的状态和内容
- Expected: `dws todo task get --task-id taskXYZ --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task get)
- Flags: PASS (1 flags validated)

**todo_todo_task_list_001** ✅ PASS

- Prompt: 查询待办列表
- Expected: `dws todo task list --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task list)
- Flags: N/A (no flags specified)

**todo_todo_task_list_002** ✅ PASS

- Prompt: 看看我还有哪些未完成的待办
- Expected: `dws todo task list --status false --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task list)
- Flags: PASS (1 flags validated)

**todo_todo_task_list_003** ✅ PASS

- Prompt: 查询第 2 页的待办，每页 10 条，且只要已完成的
- Expected: `dws todo task list --page 2 --size 10 --status true --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task list)
- Flags: PASS (3 flags validated)

**todo_todo_task_list_004** ✅ PASS

- Prompt: 查询第 3 页的待办列表
- Expected: `dws todo task list --page 3 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task list)
- Flags: PASS (1 flags validated)

**todo_todo_task_list_005** ✅ PASS

- Prompt: 每页显示 20 条待办
- Expected: `dws todo task list --size 20 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task list)
- Flags: PASS (1 flags validated)

**todo_todo_task_list_006** ✅ PASS

- Prompt: 分页查询待办，第 1 页，每页 5 条
- Expected: `dws todo task list --page 1 --size 5 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task list)
- Flags: PASS (2 flags validated)

**todo_todo_task_list_007** ✅ PASS

- Prompt: 查询已完成的待办，每页显示 15 条
- Expected: `dws todo task list --size 15 --status true --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task list)
- Flags: PASS (2 flags validated)

**todo_todo_task_list_008** ✅ PASS

- Prompt: 把我所有的待办任务都列出来看看
- Expected: `dws todo task list --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task list)
- Flags: N/A (no flags specified)

**todo_todo_task_update_001** ✅ PASS

- Prompt: 把待办 task123 的标题改成 修订报告V2
- Expected: `dws todo task update --task-id task123 --title 修订报告V2 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task update)
- Flags: PASS (2 flags validated)

**todo_todo_task_update_002** ✅ PASS

- Prompt: 把待办 task456 的截止时间改到 2025-04-30
- Expected: `dws todo task update --task-id task456 --due 2025-04-30 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task update)
- Flags: PASS (2 flags validated)

**todo_todo_task_update_003** ✅ PASS

- Prompt: 将待办 task789 的 done 属性更新为 true
- Expected: `dws todo task update --task-id task789 --done true --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task update)
- Flags: PASS (2 flags validated)

**todo_todo_task_update_004** ✅ PASS

- Prompt: 将任务 task012 的标题改为 紧急上线，优先级设为较高，截止日期 2025-03-20
- Expected: `dws todo task update --due 2025-03-20 --priority 30 --task-id task012 --title 紧急上线 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task update)
- Flags: PASS (4 flags validated)

**todo_todo_task_update_005** ✅ PASS

- Prompt: 把待办 taskAAA 的优先级调整为低优先级
- Expected: `dws todo task update --priority 10 --task-id taskAAA --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task update)
- Flags: PASS (2 flags validated)

**todo_todo_task_update_006** ✅ PASS

- Prompt: 把待办 taskBBB 的截止时间改到 2026-05-01，优先级设为普通
- Expected: `dws todo task update --due 2026-05-01 --priority 20 --task-id taskBBB --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task update)
- Flags: PASS (3 flags validated)

**todo_todo_task_update_007** ✅ PASS

- Prompt: 把任务 taskCCC 重新打开（标记为未完成），并将标题改为 重新审核方案
- Expected: `dws todo task update --done false --task-id taskCCC --title 重新审核方案 --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task update)
- Flags: PASS (3 flags validated)

**todo_todo_task_update_008** ✅ PASS

- Prompt: 任务 taskDDD 完成了，帮我更新一下状态
- Expected: `dws todo task update --done true --task-id taskDDD --format json`
- Skill Reference: references/products/todo.md
- Command path: PASS (todo task update)
- Flags: PASS (2 flags validated)

### workbench

**workbench_workbench_app_get_001** ✅ PASS

- Prompt: 批量获取应用 app001 和 app002 的详情
- Expected: `dws workbench app get --ids app001,app002 --format json`
- Skill Reference: references/products/workbench.md
- Command path: PASS (workbench app get)
- Flags: PASS (1 flags validated)

**workbench_workbench_app_get_002** ✅ PASS

- Prompt: 查一下应用 appXYZ 的详细信息
- Expected: `dws workbench app get --ids appXYZ --format json`
- Skill Reference: references/products/workbench.md
- Command path: PASS (workbench app get)
- Flags: PASS (1 flags validated)

**workbench_workbench_app_list_001** ✅ PASS

- Prompt: 查看所有工作台应用列表
- Expected: `dws workbench app list --format json`
- Skill Reference: references/products/workbench.md
- Command path: PASS (workbench app list)
- Flags: N/A (no flags specified)

**workbench_workbench_app_list_002** ✅ PASS

- Prompt: 钉钉工作台上有哪些应用
- Expected: `dws workbench app list --format json`
- Skill Reference: references/products/workbench.md
- Command path: PASS (workbench app list)
- Flags: N/A (no flags specified)
