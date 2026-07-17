---
name: dingtalk-attendance
description: 钉钉考勤、打卡、排班与考勤报表。Use when 用户说考勤/打卡记录/查打卡/排班/班次/考勤报表/导出考勤/出勤汇总/考勤明细/迟到早退统计/全员考勤数据/某月考勤统计/考勤表格/考勤Excel。不做日程会议（走 dingtalk-calendar）、日报周报（走 dingtalk-report）、审批请假流程（走 dingtalk-oa）。命令前缀：dws attendance。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉考勤 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 渐进式参考：[attendance-index.md](references/attendance-index.md)。日期范围或“签到”场景必须先读 [attendance-routing.md](references/attendance-routing.md)；报表导出再读 [attendance-report.md](references/attendance-report.md)。

> 旧路径兼容入口：[attendance.md](references/attendance.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service attendance --format json` 读取参数、约束、风险和示例，并以 `dws attendance <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws attendance +check-record` | read | 查询用户打卡流水（打卡时间/地点/定位方式） |
| `dws attendance +check-result` | read | 查询用户打卡结果（迟到/早退/缺卡等） |
| `dws attendance +get-adjustment-rule` | read | 根据补卡规则主键 ID 查询补卡规则详情 |
| `dws attendance +get-approve-template` | read | 查询补卡/请假/加班/外出/出差审批提交链接 |
| `dws attendance +get-checkin-record` | read | 查询指定员工一段时间内的签到记录 |
| `dws attendance +get-leave-records` | read | 查询指定员工的假期余额变更记录 |
| `dws attendance +get-overtime-rule` | read | 根据加班规则主键 ID 查询加班规则详情 |
| `dws attendance +get-schedule` | read | 获取指定用户一段时间内的排班记录 |
| `dws attendance +get-self-setting` | read | 查询个人规则设置（打卡提醒/极速打卡/缺卡提醒等） |
| `dws attendance +get-summary` | read | 查询某个人的考勤统计摘要（周/月） |
| `dws attendance +list-approve` | read | 查询用户考勤审批单（补卡/加班/请假/出差外出） |
| `dws attendance +list-leave-types` | read | 查询当前用户可用的假期规则列表 |
| `dws attendance +my-attendance` | read | 查我今天的考勤打卡记录（打卡流水，自动解析当前用户） |
| `dws attendance +query-report-data` | read | 根据字段查询考勤报表数据（仅管理员） |
| `dws attendance +search-adjustment-rule` | read | 查询当前用户可管理的补卡规则列表 |
| `dws attendance +search-class` | read | 查询当前用户可管理的班次详情列表 |
| `dws attendance +search-group` | read | 查询当前用户可管理的考勤组列表 |
| `dws attendance +search-overtime-rule` | read | 查询当前用户可管理的加班规则列表 |
| `dws attendance +this-month` | read | 查我本月的考勤打卡记录（打卡流水，自动解析当前用户） |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查我自己的打卡 / 某天考勤" | `python scripts/attendance_my_record.py 2026-03-08` 或 `dws attendance record get --user <userId> --date <YYYY-MM-DD>` |
| "查团队排班" | `python scripts/attendance_team_shift.py --users <userId1,userId2> --from <YYYY-MM-DD> --to <YYYY-MM-DD>` |
| "导出考勤报表 / 月度汇总 / 考勤明细 / 每日统计" | **必须先读 [attendance-report.md](references/attendance-report.md)** 强制门禁后选择 `attendance_report_{detail,monthly,daily}.py` |
| "创建班次 / 设置班次" | 先读 [attendance.md](references/attendance-commands.md) 的 `class create`，确认后执行 |
| "导入排班 / 安排排班" | 先读 [attendance.md](references/attendance-commands.md) 的 `schedule import`，确认后执行 |
| "加入/移出考勤组 / 更新考勤组成员" | `dws attendance group update-members ...`（需确认） |

## 高频硬约束

- 不要在读完 [attendance.md](references/attendance-commands.md) 前判断"CLI 不支持"。`class create`、`schedule import`、`group update-members`、`group update` 都是已支持写操作，但必须先展示摘要并等用户确认。
- 查询迟到/缺勤名单时，空打卡结果不等于"没人迟到"。必须结合排班、`NotSigned`、`Absenteeism`、无记录人员分别说明；数据缺失要标为"无记录/无法判断"，不要归为正常。
- 做部门 Top N 排名时，用户要求前 N 名就必须输出 N 个部门；无打卡记录或无可计算数据的部门按 0 或"无数据"保留在排名中，不能只输出有数据的少数部门。
- 处理请假/补卡/加班审批时，先用考勤审批模板或 OA 查询确认可操作范围；没有直接提交接口时返回可点击提交链接并说明无法代填提交，不要假装已提交。
- 更新考勤组成员固定按 `aisearch/contact` 解析 userId → `group search` 解析 groupId → 展示变更摘要并确认 → `group update-members` → `group filtered-get --member` 回查。
- 所有 dws 命令带 `--format json`，时间/日期按命令要求分别使用 `YYYY-MM-DD` 或 reference 指定格式。

## 跨产品协作

- 拿到 userId 前先用 `dingtalk-aisearch` 解析人名
- 报表导出涉及多人 / 多月 → 脚本内部自动分批 + 切片，输出 xlsx
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)。
