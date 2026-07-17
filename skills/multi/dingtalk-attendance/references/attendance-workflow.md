# 核心工作流

## 核心工作流

```bash
# 导入排班记录
dws attendance schedule import --groupId 123456 \
  --scheduleVOS '[{"userId":"user001","classId":123,"workDate":"2026-04-22","checkBeginTime":"09:00","checkEndTime":"18:00"}]' \
  --user-say-yes --format json

#  获取排班记录 — 禁止直接调用，必须走 attendance-schedule.md 排班查询导出工作流
# python scripts/attendance_schedule_export.py --users user001,user002 --start 2026-04-01 --end 2026-04-30

# 查询可管理的班次列表
dws attendance class search --format json
dws attendance class search --query "早班" --filter-type MINE_OWN --format json

# 查询班次详情
dws attendance class get --class-id 1170996821 --format json

# 更新班次
dws attendance class update --class-id 1170996821 --name "新早班" --timeout 10 --format json
dws attendance class update --class-id 1170996821 --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"08:30","across":0},{"checkType":"OffDuty","checkTime":"17:30","across":0}]}]}' --timeout 10 --format json

# 查询补卡规则
dws attendance adjustment search --page 1 --limit 20 --format json
dws attendance adjustment search --query "标准" --page 1 --limit 20 --format json

# 查询补卡规则详情
dws attendance adjustment get --adjustment-id 12345 --format json

# 查询加班规则
dws attendance overtime search --page 1 --limit 20 --format json

# 查询加班规则详情
dws attendance overtime get --overtime-id 12345 --format json

# 查询考勤组列表
dws attendance group search --query "研发" --page 1 --limit 20 --format json
dws attendance group search --type FIXED --page 1 --limit 20 --format json

# 查询考勤组全量信息
dws attendance group get --group-id 123456 --format json

# 按需查询考勤组成员/地址/蓝牙/Wifi
dws attendance group filtered-get --group-id 123456 --member --format json
dws attendance group filtered-get --group-id 123456 --position --wifi --format json

# 更新考勤组成员
dws attendance group update-members --group-id 123456 --add-users userId1,userId2 --timeout 10 --format json
dws attendance group update-members --group-id 123456 --remove-users userId1 --timeout 10 --format json
dws attendance group update-members --group-id 123456 --add-depts deptId1 --remove-users userId2 --timeout 10 --format json

# 更新考勤组配置
dws attendance group update --group-id 123456 --name "研发考勤组" --timeout 10 --format json
dws attendance group update --group-id 123456 --classIds '[1374234767]' --timeout 10 --format json
dws attendance group update --group-id 123456 --group-vo '{"positions":[{"title":"总部","address":"北京市","latitude":39.9,"longitude":116.4,"offset":200}]}' --timeout 10 --format json

# 创建考勤组
dws attendance group create --name "研发考勤组" --type FIXED --group-vo '{"defaultClassId":1170996821,"workDayClassList":[0,1170996821,0,0,0,0,0]}' --timeout 10 --format json
dws attendance group create --name "自由工时分组" --type NONE --timeout 10 --format json

# 查看考勤统计摘要
dws attendance summary --user <USER_ID> --date 2026-03-12 --stats-type week --format json

# 查看考勤组和规则
dws attendance rules --date 2026-03-14 --format json

# 查看指定用户的打卡提醒设置
dws attendance selfsetting get --setting-scene checkRemind --user <USER_ID> --format json

# 查看指定用户的极速打卡设置
dws attendance selfsetting get --setting-scene fastCheck --user <USER_ID> --format json

# 开启指定用户的打卡结果通知
dws attendance selfsetting save --setting-scene checkResultNotify --user <USER_ID> --check-result-msg 1 --format json

# 更新指定用户的极速打卡设置
dws attendance selfsetting save --setting-scene fastCheck --user <USER_ID> \
  --onduty-check-type 3 --voice-remind-switch=true --format json

# 获取考勤字段列表（管理员）
dws attendance report columns --format json

# 根据字段查询考勤数据（管理员）
dws attendance report query-data --users userId1,userId2 \
  --columns 1001,1002 --start "2026-03-01 00:00:00" --end "2026-03-31 23:59:59" --format json

# 查询用户假期数据（管理员）
dws attendance report query-leave --users userId1,userId2 \
  --leave-names 年假,病假 --start "2026-03-01 00:00:00" --end "2026-03-31 23:59:59" --format json

# 查看假期规则列表
dws attendance vacation types --format json

# 查看指定员工假期余额
dws attendance vacation balance --users userId1,userId2 --format json

# 查看指定员工某类假期余额
dws attendance vacation balance --users userId1 --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --format json

# 查看指定员工假期余额变更记录
dws attendance vacation records --user USER_ID --start 2026-04-01 --end 2026-04-22 --format json

# 更新假期规则名称
dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --name "事假（修改版）" --format json

# 更新假期单位
dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --unit hour --per-hours 8 --format json

# 改为指定部门可见
dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --visibility-rules '[{"type":"dept","visible":["1","2","3"]}]' --format json

# 改为全公司可见（哨兵约定：必须显式传 "-1"，不能传空数组 []）
dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --visibility-rules '[{"type":"dept","visible":["-1"]}]' --format json

# 设置员工假期余额完整流程
# 1. 查询当前余额
dws attendance vacation balance --users user001 \
  --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --format json

# 2. 根据查询结果计算新值（如当前5天，要设置为8天）

# 3. 执行设置（SET操作，会替换当前余额）
dws attendance vacation save-balance --target user001 \
  --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --num 8 --reason "年度发放" --start 2024-01-01 --end 2024-12-31 --format json

# 增加员工假期余额完整流程（ADD场景）
# 1. 查询当前余额（假设返回5天）
dws attendance vacation balance --users user001 \
  --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --format json

# 2. 计算增加后的新值（5 + 3 = 8天）

# 3. 设置新总额度
dws attendance vacation save-balance --target user001 \
  --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --num 8 --reason "绩效奖励发放3天" --format json

# 查询签到记录
dws attendance checkin records --operator-staff-id op001 --staff-ids user001,user002 \
  --start "2026-04-01 00:00:00" --end "2026-04-07 00:00:00" --format json
```
