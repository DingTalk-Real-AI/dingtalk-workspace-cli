---
name: dingtalk-attendance
description: 钉钉考勤。Use when 用户说 考勤/打卡记录/查打卡/查班次/考勤汇总/考勤规则/出勤情况/请假加班补卡/排班/考勤组/假期余额。命令前缀：dws attendance。支持查询与写操作（班次、排班、考勤组、个人规则设置、假期规则/余额等），写操作需二次确认后再加 --yes。
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

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[attendance.md](references/attendance.md)。

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
| "查我 / 某人 某天的打卡" | `dws attendance record get --user <userId> --date <YYYY-MM-DD>` |
| "查考勤组 / 考勤规则 / 打卡范围" | `dws attendance rules --date <YYYY-MM-DD>` |
| "查班次 / 排班 / 谁今天上什么班" | `dws attendance shift list --users <userId1,userId2> --start <YYYY-MM-DD> --end <YYYY-MM-DD>` |
| "考勤统计 / 周月汇总 / 出勤天数" | `dws attendance summary --user <userId> --date <YYYY-MM-DD> --stats-type week\|month` |
| "请假 / 加班 / 补卡 / 出差 记录或提交链接" | `dws attendance approve list` / `dws attendance approve templates --type <类型>` |
| "班次 / 补卡规则 / 加班规则 / 考勤组 查询与修改" | `class` / `adjustment` / `overtime` / `group` 子命令 |
| "排班 / 假期规则 / 假期余额 / 签到 / 报表" | `schedule` / `vacation` / `checkin` / `report` 子命令 |

> 完整命令集（含写操作与参数）见 [references/attendance.md](references/attendance.md)。

## 评测高频硬约束

- 当前 dws 已注册全部考勤子命令组（`record` / `check` / `approve` / `shift` / `schedule` / `class` / `adjustment` / `overtime` / `group` / `summary` / `rules` / `selfsetting` / `globalsetting` / `vacation` / `checkin` / `report` / `boss-check`），查询与写操作大多可直接调用后端。**不要再以"开源版只读/不支持写操作"为由拒答。** 创建班次、导入排班、加人入考勤组、保存个人规则设置、设置假期余额等写操作可执行，但必须先展示参数摘要并二次确认，再追加 `--yes`（或 `--user-say-yes`）执行；不要在未确认时直接写、也不要伪装成功。个别命令受权限/数据影响返回空或权限错误（如 `report` 系列仅管理员），如实说明即可。
- 查询迟到/缺勤名单时，空打卡结果不等于"没人迟到"。必须结合排班、`NotSigned`、`Absenteeism`、无记录人员分别说明；数据缺失要标为"无记录/无法判断"，不要归为正常。
- 做部门 Top N 排名时，用户要求前 N 名就必须输出 N 个部门；无打卡记录或无可计算数据的部门按 0 或"无数据"保留在排名中，不能只输出有数据的少数部门。
- `summary` 必须同时传 `--user`、`--date`、`--stats-type`（week/month），缺一返回 C0002。
- `shift list` 时间跨度不超过 7 天，最多 50 人；`record get` 一次只查一个用户一天。
- 所有 dws 命令带 `--format json`，时间/日期按命令要求分别使用 `YYYY-MM-DD` 或 `yyyy-MM-dd HH:mm:ss`。

## 跨产品协作

- 拿到 userId 前先用 `dingtalk-aisearch` 解析人名
