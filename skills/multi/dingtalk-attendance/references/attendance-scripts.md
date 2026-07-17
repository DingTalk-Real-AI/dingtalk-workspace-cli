# 自动化脚本

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [attendance_my_record.py](../scripts/attendance_my_record.py) | 查看我今天/指定日期的考勤记录 | `python attendance_my_record.py today` |
| [attendance_team_shift.py](../scripts/attendance_team_shift.py) | 查询团队成员本周排班 | `python attendance_team_shift.py --users userId1,userId2` |
| [attendance_report_common.py](../scripts/attendance_report_common.py) | 考勤报表导出公共模块（不可单独执行） | — |
| [attendance_vacation_balance.py](../scripts/attendance_vacation_balance.py) | 假期余额列表 Excel 导出 | **禁止直接调用**，必须先读 [attendance-vacation.md](./attendance-vacation.md) 按工作流执行 |
| attendance_report_detail.py | 考勤报表 — **明细粒度** |  **禁止直接调用**，必须先读 [attendance-report.md](./attendance-report.md) 按工作流执行 |
| attendance_report_monthly.py | 考勤报表 — **月度汇总** |  **禁止直接调用**，必须先读 [attendance-report.md](./attendance-report.md) 按工作流执行 |
| attendance_report_daily.py | 考勤报表 — **每日统计** |  **禁止直接调用**，必须先读 [attendance-report.md](./attendance-report.md) 按工作流执行 |
| attendance_report_record.py | 考勤报表 — **考勤记录**（补卡/请假/出差/外出） |  **禁止直接调用**，必须先读 [attendance-report.md](./attendance-report.md) 按工作流执行 |
| attendance_schedule_import.py | 排班导入（含校验、回显、执行） | **禁止直接调用**，必须先读 [attendance-schedule.md](./attendance-schedule.md) 按工作流执行 |
| attendance_schedule_export.py | 排班查询导出（分批查询、排班表 Excel） | **禁止直接调用**，必须先读 [attendance-schedule.md](./attendance-schedule.md) 按工作流执行 |

> 说明：
> - `attendance_report_*.py` 四个脚本由 [attendance-report.md](./attendance-report.md) 工作流编排使用：detail/monthly/daily 自动处理 `--users` 超过 20 人分批、`--start/--end` 超过 32 天按月切片；record 自包含数据查询+解析+Excel 生成（补卡/请假/出差/外出审批记录）
> - `attendance_schedule_import.py` 由 [attendance-schedule.md](./attendance-schedule.md) 排班导入工作流编排使用，自动处理考勤组校验、班次校验、排班回显确认
> - `attendance_schedule_export.py` 由 [attendance-schedule.md](./attendance-schedule.md) 排班查询导出工作流编排使用，自动处理分批查询（超 20 人自动分批）、userId→姓名转换、classId→班次名称转换、输出排班表格式 Excel
