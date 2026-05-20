---
name: dingtalk-attendance
description: 钉钉考勤。Use when 用户说 考勤/打卡记录/查打卡/排班/班次/考勤报表/导出考勤/出勤汇总/考勤明细/迟到早退统计/全员考勤数据/某月考勤统计/考勤表格/考勤Excel。命令前缀：dws attendance。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉考勤 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[attendance.md](references/attendance.md)；考勤报表导出工作流：[attendance-report.md](references/attendance-report.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查我自己的打卡 / 某天考勤" | `python scripts/attendance_my_record.py --date 2026-03-08` 或 `dws attendance record get --user <userId> --date <YYYY-MM-DD>` |
| "查团队排班" | `python scripts/attendance_team_shift.py --dept-id <id> --date <YYYY-MM-DD>` |
| "导出考勤报表 / 月度汇总 / 考勤明细 / 每日统计" | **必须先读 [attendance-report.md](references/attendance-report.md)** 强制门禁后选择 `attendance_report_{detail,monthly,daily}.py` |
| "创建班次 / 设置班次" | 先读 [attendance.md](references/attendance.md) 的 `class create`，确认后执行 |
| "导入排班 / 安排排班" | 先读 [attendance.md](references/attendance.md) 的 `schedule import`，确认后执行 |
| "加入/移出考勤组 / 更新考勤组成员" | `dws attendance group update-members ...`（需确认） |

## 评测高频硬约束

- 不要在读完 [attendance.md](references/attendance.md) 前判断"CLI 不支持"。`class create`、`schedule import`、`group update-members`、`group update` 都是已支持写操作，但必须先展示摘要并等用户确认。
- 查询迟到/缺勤名单时，空打卡结果不等于"没人迟到"。必须结合排班、`NotSigned`、`Absenteeism`、无记录人员分别说明；数据缺失要标为"无记录/无法判断"，不要归为正常。
- 做部门 Top N 排名时，用户要求前 N 名就必须输出 N 个部门；无打卡记录或无可计算数据的部门按 0 或"无数据"保留在排名中，不能只输出有数据的少数部门。
- 处理请假/补卡/加班审批时，先用考勤审批模板或 OA 查询确认可操作范围；没有直接提交接口时返回可点击提交链接并说明无法代填提交，不要假装已提交。
- 更新考勤组成员时必须实际调用 `group update-members`：先 `aisearch/contact` 解析 userId、`group search` 解析 groupId，确认后执行，再 `group filtered-get --member` 回查。
- 所有 dws 命令带 `--format json`，时间/日期按命令要求分别使用 `YYYY-MM-DD` 或 reference 指定格式。

## 跨产品协作

- 拿到 userId 前先用 `dingtalk-aisearch` 解析人名
- 报表导出涉及多人 / 多月 → 脚本内部自动分批 + 切片，输出 xlsx
